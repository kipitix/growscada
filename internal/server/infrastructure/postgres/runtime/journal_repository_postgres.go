package runtime

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	recordv0 "github.com/kipitix/growscada/contract/record/v0"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
)

// journalRepositoryPostgres is the read side of runtime.journal: pages of a
// Project's entries in the order of their positions and the Project's current
// position (task 42, ADR 0019). The Journal is append-only and is written with
// the changes it records, never here.
type journalRepositoryPostgres struct {
	db *sql.DB
}

var _ runtimeapp.JournalRepository = (*journalRepositoryPostgres)(nil)

// NewJournalRepositoryPostgres returns the read side of the Journal.
func NewJournalRepositoryPostgres(aDb *sql.DB) runtimeapp.JournalRepository {
	return &journalRepositoryPostgres{db: aDb}
}

// FindAfter reads the Project's entries after the position after, in the
// order of their positions. A Project that is not live is
// project.ErrProjectNotFound, not an empty page: a deleted Project keeps its
// rows until its cleanup removes them, and they are not its Journal to serve
// any more. Entries of another Project are never returned, whatever their
// position.
func (r journalRepositoryPostgres) FindAfter(ctx context.Context, projectID uuid.UUID, after int64, limit int) ([]recordv0.Entry, error) {
	if err := liveProject(ctx, r.db, projectID); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT seq, project_id, ts, type, payload, schema_version
		 FROM runtime.journal
		 WHERE project_id = $1 AND seq > $2
		 ORDER BY seq
		 LIMIT $3`,
		projectID, after, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("cannot read the journal of project %s after %d: %w", projectID, after, err)
	}
	defer rows.Close()
	entries := make([]recordv0.Entry, 0, limit)
	for rows.Next() {
		var row journalRow
		if err := rows.Scan(&row.Seq, &row.ProjectID, &row.Timestamp, &row.Type, &row.Payload, &row.SchemaVersion); err != nil {
			return nil, fmt.Errorf("cannot scan a journal entry of project %s: %w", projectID, err)
		}
		entry, err := decodeJournalEntry(row)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot iterate over the journal of project %s: %w", projectID, err)
	}
	return entries, nil
}

// Position returns the seq of the Project's latest entry, 0 if it has none.
func (r journalRepositoryPostgres) Position(ctx context.Context, projectID uuid.UUID) (int64, error) {
	if err := liveProject(ctx, r.db, projectID); err != nil {
		return 0, err
	}
	var position int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) FROM runtime.journal WHERE project_id = $1`,
		projectID,
	).Scan(&position); err != nil {
		return 0, fmt.Errorf("cannot read the journal position of project %s: %w", projectID, err)
	}
	return position, nil
}
