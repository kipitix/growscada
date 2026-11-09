package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	recordv0 "github.com/kipitix/growscada/contract/record/v0"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
)

// checkpointRepositoryPostgres is the Runtime node's store of Checkpoints
// (task 44, ADR 0023): runtime.checkpoints, immutable snapshots of a Project's
// operational state keyed by the Journal position they describe. The table is
// written by a Deployment's transaction and by the background snapshot
// service, never updated.
type checkpointRepositoryPostgres struct {
	db *sql.DB
}

var _ runtimeapp.CheckpointRepository = (*checkpointRepositoryPostgres)(nil)

// NewCheckpointRepositoryPostgres returns the repository of the Checkpoints.
func NewCheckpointRepositoryPostgres(aDb *sql.DB) runtimeapp.CheckpointRepository {
	return &checkpointRepositoryPostgres{db: aDb}
}

// Nearest returns the latest Checkpoint at or before the position, and whether
// there is one. A Project that is not live is project.ErrProjectNotFound, not
// "no checkpoint": a deleted Project keeps its rows until its cleanup removes
// them, and they are not its snapshots to serve any more.
func (r checkpointRepositoryPostgres) Nearest(ctx context.Context, projectID uuid.UUID, position int64) (recordv0.Checkpoint, bool, error) {
	if err := liveProject(ctx, r.db, projectID); err != nil {
		return recordv0.Checkpoint{}, false, err
	}
	var row checkpointRow
	err := r.db.QueryRowContext(ctx,
		`SELECT seq, project_id, taken_at, payload, schema_version
		 FROM runtime.checkpoints
		 WHERE project_id = $1 AND seq <= $2
		 ORDER BY seq DESC
		 LIMIT 1`,
		projectID, position,
	).Scan(&row.Seq, &row.ProjectID, &row.TakenAt, &row.Payload, &row.SchemaVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return recordv0.Checkpoint{}, false, nil
	}
	if err != nil {
		return recordv0.Checkpoint{}, false, fmt.Errorf("cannot read the checkpoint of project %s at %d: %w", projectID, position, err)
	}
	checkpoint, err := decodeCheckpoint(row)
	if err != nil {
		return recordv0.Checkpoint{}, false, err
	}
	return checkpoint, true, nil
}

// DueProjects returns the live Projects whose current Journal position has
// moved at least everyEntries past their latest Checkpoint: the Projects a
// periodic snapshot is due for. A Project without a Checkpoint counts from
// position 0.
func (r checkpointRepositoryPostgres) DueProjects(ctx context.Context, everyEntries int64) ([]uuid.UUID, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT j.project_id
		 FROM (SELECT project_id, MAX(seq) AS position FROM runtime.journal GROUP BY project_id) j
		 JOIN projects p ON p.id = j.project_id AND p.deleted_at IS NULL
		 LEFT JOIN (
		     SELECT project_id, MAX(seq) AS seq FROM runtime.checkpoints GROUP BY project_id
		 ) c ON c.project_id = j.project_id
		 WHERE j.position >= COALESCE(c.seq, 0) + $1
		 ORDER BY j.project_id`,
		everyEntries,
	)
	if err != nil {
		return nil, fmt.Errorf("cannot read the projects due for a checkpoint: %w", err)
	}
	defer rows.Close()
	projects := make([]uuid.UUID, 0)
	for rows.Next() {
		var projectID uuid.UUID
		if err := rows.Scan(&projectID); err != nil {
			return nil, fmt.Errorf("cannot scan a project due for a checkpoint: %w", err)
		}
		projects = append(projects, projectID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot iterate over the projects due for a checkpoint: %w", err)
	}
	return projects, nil
}

// Snapshot takes one periodic Checkpoint of the Project in a single REPEATABLE
// READ transaction: the current position, the Deployment in force and every
// Tag state come from one snapshot, so the record never mixes two moments
// (ADR 0023). It reports whether it inserted one; a position that has not
// moved everyEntries past the latest Checkpoint inserts nothing, and a
// position already checkpointed is left alone (ON CONFLICT DO NOTHING).
func (r checkpointRepositoryPostgres) Snapshot(ctx context.Context, projectID uuid.UUID, everyEntries int64) (bool, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return false, fmt.Errorf("cannot begin the checkpoint transaction of project %s: %w", projectID, err)
	}
	defer func() { _ = tx.Rollback() }()

	var position, last int64
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) FROM runtime.journal WHERE project_id = $1`, projectID,
	).Scan(&position); err != nil {
		return false, fmt.Errorf("cannot read the journal position of project %s: %w", projectID, err)
	}
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) FROM runtime.checkpoints WHERE project_id = $1`, projectID,
	).Scan(&last); err != nil {
		return false, fmt.Errorf("cannot read the latest checkpoint of project %s: %w", projectID, err)
	}
	if position < last+everyEntries {
		return false, nil
	}
	deployment, err := currentDeploymentRef(ctx, tx, projectID)
	if err != nil {
		return false, err
	}
	inserted, err := insertCheckpoint(ctx, tx, projectID, position, deployment, time.Now().UTC())
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("cannot commit the checkpoint of project %s at %d: %w", projectID, position, err)
	}
	return inserted, nil
}

// insertCheckpoint writes a Checkpoint of the Project at seq in the caller's
// transaction: the Deployment in force and every runtime.tag_states row read
// there, in the order of their Tag IDs. ON CONFLICT DO NOTHING: the same
// position is never written twice, and a Deployment's anchor and a periodic
// snapshot that reach one position describe the same state — the snapshot
// reads the Journal and the states from one moment — so whichever inserts
// first is kept. It reports whether the row was inserted.
func insertCheckpoint(ctx context.Context, tx *sql.Tx, projectID uuid.UUID, seq int64,
	deployment *recordv0.Deployment, takenAt time.Time) (bool, error) {
	states, err := checkpointTagStates(ctx, tx, projectID)
	if err != nil {
		return false, err
	}
	checkpoint := recordv0.Checkpoint{
		Header:     recordv0.Header{SchemaVersion: recordv0.SchemaVersion()},
		Seq:        seq,
		ProjectID:  projectID,
		Timestamp:  takenAt.UTC().Format(time.RFC3339),
		Deployment: deployment,
		TagStates:  states,
	}
	row, err := encodeCheckpoint(checkpoint)
	if err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx,
		`INSERT INTO runtime.checkpoints (project_id, seq, taken_at, schema_version, payload)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (project_id, seq) DO NOTHING`,
		row.ProjectID, row.Seq, row.TakenAt, row.SchemaVersion, row.Payload,
	)
	if err != nil {
		return false, fmt.Errorf("cannot insert checkpoint %d of project %s: %w", seq, projectID, err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot read the result of checkpoint %d of project %s: %w", seq, projectID, err)
	}
	return inserted > 0, nil
}

// checkpointTagStates reads every Tag state of the Project, in the order of
// their Tag IDs.
func checkpointTagStates(ctx context.Context, tx *sql.Tx, projectID uuid.UUID) ([]recordv0.CheckpointTagState, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT s.tag_id, s.value, s.quality, s.seq
		 FROM runtime.tag_states s
		 JOIN runtime.tags t ON t.id = s.tag_id
		 WHERE t.project_id = $1
		 ORDER BY s.tag_id`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("cannot read the tag states of project %s: %w", projectID, err)
	}
	defer rows.Close()
	states := make([]recordv0.CheckpointTagState, 0)
	for rows.Next() {
		var state recordv0.CheckpointTagState
		if err := rows.Scan(&state.TagID, &state.Value, &state.Quality, &state.Seq); err != nil {
			return nil, fmt.Errorf("cannot scan a tag state of project %s: %w", projectID, err)
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot iterate over the tag states of project %s: %w", projectID, err)
	}
	return states, nil
}

// currentDeploymentRef reads the Deployment the Project runs as the record
// names it: number, cause and the numbers it carries, never the document. A
// Project without a Deployment has none.
func currentDeploymentRef(ctx context.Context, tx *sql.Tx, projectID uuid.UUID) (*recordv0.Deployment, error) {
	var (
		number         int
		cause          string
		rolledBackFrom sql.NullInt64
		releaseNumber  sql.NullInt64
	)
	err := tx.QueryRowContext(ctx,
		`SELECT number, cause, rolled_back_from, release_number
		 FROM runtime.deployments
		 WHERE project_id = $1
		 ORDER BY number DESC
		 LIMIT 1`,
		projectID,
	).Scan(&number, &cause, &rolledBackFrom, &releaseNumber)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read the running deployment of project %s: %w", projectID, err)
	}
	return &recordv0.Deployment{
		Number:         number,
		Cause:          recordv0.DeploymentCause(cause),
		RolledBackFrom: nullableInt(rolledBackFrom),
		ReleaseNumber:  nullableInt(releaseNumber),
	}, nil
}
