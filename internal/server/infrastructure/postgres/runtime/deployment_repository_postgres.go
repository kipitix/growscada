package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	recordv0 "github.com/kipitix/growscada/contract/record/v0"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/tagstate"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// deploymentRepositoryPostgres is the Runtime node's store of Deployments
// (task 35): runtime.deployments, whose config column is the truth about what
// the Project is running. The copies of the Tag definitions, the Devices, the
// Environments and their pairs are derived data, rebuilt from the Deployment in
// the transaction that creates it (ADR 0011; the amendment recording the
// Deployment lands with task 35, part 5).
type deploymentRepositoryPostgres struct {
	db *sql.DB
}

var _ runtimeapp.DeploymentRepository = (*deploymentRepositoryPostgres)(nil)

// NewDeploymentRepositoryPostgres returns the repository of the Deployments.
func NewDeploymentRepositoryPostgres(aDb *sql.DB) runtimeapp.DeploymentRepository {
	return &deploymentRepositoryPostgres{db: aDb}
}

// CreateDeployment stores a new Deployment of the Project in one transaction: an
// advisory lock on the Project serializes concurrent Deploys, the number is
// the greatest stored plus one (the unique key is the safety net), and the
// derived copies are brought into line with the document by diff — never by
// dropping them all first, which would take the Tag states along the cascade
// and lose the values (task 35, Q5).
func (r deploymentRepositoryPostgres) CreateDeployment(ctx context.Context, projectID uuid.UUID, file projectv0.ProjectFile, cause runtimeapp.DeploymentCause, now time.Time) (runtimeapp.Deployment, error) {
	if file.ProjectID != projectID {
		return runtimeapp.Deployment{}, fmt.Errorf("cannot create deployment of project %s from a document of project %s", projectID, file.ProjectID)
	}
	config, err := json.Marshal(file)
	if err != nil {
		return runtimeapp.Deployment{}, fmt.Errorf("cannot encode project file: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return runtimeapp.Deployment{}, fmt.Errorf("cannot begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := prepareDeploymentTx(ctx, tx, projectID); err != nil {
		return runtimeapp.Deployment{}, err
	}
	number, err := insertDeployment(ctx, tx, projectID, config, cause, nil, now)
	if err != nil {
		return runtimeapp.Deployment{}, err
	}
	if err := refreshCopies(ctx, tx, projectID, file); err != nil {
		return runtimeapp.Deployment{}, err
	}
	if err := tx.Commit(); err != nil {
		return runtimeapp.Deployment{}, fmt.Errorf("cannot commit deployment %d of project %s: %w", number, projectID, err)
	}
	return runtimeapp.Deployment{
		Number:     number,
		DeployedAt: now.UTC(),
		Cause:      cause,
		Content:    file,
	}, nil
}

// Rollback copies the Deployment n of the Project into a new one: the same
// transaction as a Deploy — the advisory lock, the lock of the Project's row
// and the numbering — plus the read and the comparison of the documents
// (ADR 0014). The copied document is stored as it is, and the derived copies
// are rebuilt by the same diff a Deploy runs.
func (r deploymentRepositoryPostgres) Rollback(ctx context.Context, projectID uuid.UUID, n int, now time.Time) (runtimeapp.Deployment, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return runtimeapp.Deployment{}, fmt.Errorf("cannot begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := prepareDeploymentTx(ctx, tx, projectID); err != nil {
		return runtimeapp.Deployment{}, err
	}
	var currentConfig []byte
	err = tx.QueryRowContext(ctx,
		`SELECT config FROM runtime.deployments WHERE project_id = $1 ORDER BY number DESC LIMIT 1`,
		projectID,
	).Scan(&currentConfig)
	if errors.Is(err, sql.ErrNoRows) {
		// A Project that runs no Deployment has none to name.
		return runtimeapp.Deployment{}, fmt.Errorf("%w: deployment %d of project %s", runtimeapp.ErrDeploymentNotFound, n, projectID)
	}
	if err != nil {
		return runtimeapp.Deployment{}, fmt.Errorf("cannot read the running deployment of project %s: %w", projectID, err)
	}
	var targetConfig []byte
	err = tx.QueryRowContext(ctx,
		`SELECT config FROM runtime.deployments WHERE project_id = $1 AND number = $2`,
		projectID, n,
	).Scan(&targetConfig)
	if errors.Is(err, sql.ErrNoRows) {
		return runtimeapp.Deployment{}, fmt.Errorf("%w: deployment %d of project %s", runtimeapp.ErrDeploymentNotFound, n, projectID)
	}
	if err != nil {
		return runtimeapp.Deployment{}, fmt.Errorf("cannot read deployment %d of project %s: %w", n, projectID, err)
	}
	// jsonb equality, not the bytes: the same document written by another
	// path may reach the column with its keys in another order.
	var same bool
	if err := tx.QueryRowContext(ctx,
		`SELECT $1::jsonb = $2::jsonb`, string(currentConfig), string(targetConfig),
	).Scan(&same); err != nil {
		return runtimeapp.Deployment{}, fmt.Errorf("cannot compare deployment %d of project %s with the running one: %w", n, projectID, err)
	}
	if same {
		return runtimeapp.Deployment{}, fmt.Errorf("%w: deployment %d of project %s holds the document it runs", runtimeapp.ErrNothingToRollback, n, projectID)
	}
	var file projectv0.ProjectFile
	if err := json.Unmarshal(targetConfig, &file); err != nil {
		return runtimeapp.Deployment{}, fmt.Errorf("cannot decode deployment %d of project %s: %w", n, projectID, err)
	}
	rolledBackFrom := n
	number, err := insertDeployment(ctx, tx, projectID, targetConfig, runtimeapp.DeploymentCauseRollback, &rolledBackFrom, now)
	if err != nil {
		return runtimeapp.Deployment{}, err
	}
	if err := refreshCopies(ctx, tx, projectID, file); err != nil {
		return runtimeapp.Deployment{}, err
	}
	if err := tx.Commit(); err != nil {
		return runtimeapp.Deployment{}, fmt.Errorf("cannot commit deployment %d of project %s: %w", number, projectID, err)
	}
	return runtimeapp.Deployment{
		Number:         number,
		DeployedAt:     now.UTC(),
		Cause:          runtimeapp.DeploymentCauseRollback,
		RolledBackFrom: &rolledBackFrom,
		Content:        file,
	}, nil
}

// prepareDeploymentTx readies a transaction that writes a Deployment: two
// Environments may swap their names in one document, so the uniqueness of the
// copies' names is checked at the commit rather than row by row
// (runtime.environments, migration 20261103000002), the advisory lock on the
// Project serializes concurrent writers, and the Project's row is locked for
// share. The Draft is read-only while a Discard of its Project runs: a
// Deployment of a half-restored Draft would be a configuration nobody asked
// for, and the lock makes a Discard that starts after this point wait for the
// Deployment and its policy, which reads the current Deployment when it runs, see
// this one (task 35, Q10; ADR 0014).
func prepareDeploymentTx(ctx context.Context, tx *sql.Tx, projectID uuid.UUID) error {
	if _, err := tx.ExecContext(ctx, `SET CONSTRAINTS runtime.uq_runtime_environments_project_name DEFERRED`); err != nil {
		return fmt.Errorf("cannot defer the name uniqueness of project %s: %w", projectID, err)
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, projectID.String()); err != nil {
		return fmt.Errorf("cannot lock project %s: %w", projectID, err)
	}
	var live bool
	err := tx.QueryRowContext(ctx,
		`SELECT deleted_at IS NULL FROM projects WHERE id = $1 FOR SHARE`, projectID,
	).Scan(&live)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !live) {
		return fmt.Errorf("%w: %s", project.ErrProjectNotFound, projectID)
	}
	if err != nil {
		return fmt.Errorf("cannot lock project %s: %w", projectID, err)
	}
	return nil
}

// insertDeployment numbers and stores a Deployment in the caller's transaction:
// the number is the greatest stored plus one, the unique key being the safety
// net of a writer that bypassed the advisory lock. The Deployment's Journal
// entry is appended in the same transaction, with the Deployment's time: a
// Deployment that never commits leaves no entry, and the entry never carries
// the document (ADR 0014, ADR 0019).
func insertDeployment(ctx context.Context, tx *sql.Tx, projectID uuid.UUID, config []byte,
	cause runtimeapp.DeploymentCause, rolledBackFrom *int, now time.Time) (int, error) {
	var number int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(number), 0) + 1 FROM runtime.deployments WHERE project_id = $1`,
		projectID,
	).Scan(&number); err != nil {
		return 0, fmt.Errorf("cannot number the deployment of project %s: %w", projectID, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO runtime.deployments (project_id, number, deployed_at, cause, rolled_back_from, config)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		projectID, number, now, string(cause), rolledBackFrom, config,
	); err != nil {
		return 0, fmt.Errorf("cannot insert deployment %d of project %s: %w", number, projectID, err)
	}
	if _, err := appendJournalEntry(ctx, tx, newDeploymentEntry(projectID, now, number, journalCause(cause), rolledBackFrom)); err != nil {
		return 0, fmt.Errorf("cannot record deployment %d of project %s: %w", number, projectID, err)
	}
	return number, nil
}

// journalCause is the Deployment cause as the record names it: the record has
// its own enum and never imports the API's (ADR 0004, ADR 0019).
func journalCause(cause runtimeapp.DeploymentCause) recordv0.DeploymentCause {
	switch cause {
	case runtimeapp.DeploymentCauseRollback:
		return recordv0.DeploymentCauseRollback
	case runtimeapp.DeploymentCauseFile:
		return recordv0.DeploymentCauseFile
	default:
		return recordv0.DeploymentCauseDeploy
	}
}

func (r deploymentRepositoryPostgres) FindCurrent(ctx context.Context, projectID uuid.UUID) (runtimeapp.Deployment, bool, error) {
	var (
		number         int
		deployedAt     time.Time
		cause          string
		rolledBackFrom sql.NullInt64
		config         []byte
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT number, deployed_at, cause, rolled_back_from, config FROM runtime.deployments
		 WHERE project_id = $1
		 ORDER BY number DESC
		 LIMIT 1`,
		projectID,
	).Scan(&number, &deployedAt, &cause, &rolledBackFrom, &config)
	if errors.Is(err, sql.ErrNoRows) {
		return runtimeapp.Deployment{}, false, nil
	}
	if err != nil {
		return runtimeapp.Deployment{}, false, fmt.Errorf("cannot read the deployment of project %s: %w", projectID, err)
	}
	deployment, err := newDeployment(number, deployedAt, cause, rolledBackFrom, config)
	if err != nil {
		return runtimeapp.Deployment{}, false, fmt.Errorf("deployment %d of project %s: %w", number, projectID, err)
	}
	return deployment, true, nil
}

func (r deploymentRepositoryPostgres) FindAll(ctx context.Context, projectID uuid.UUID) ([]runtimeapp.DeploymentSummary, error) {
	if err := liveProject(ctx, r.db, projectID); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT number, deployed_at, cause, rolled_back_from FROM runtime.deployments
		 WHERE project_id = $1
		 ORDER BY number DESC`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("cannot read the deployments of project %s: %w", projectID, err)
	}
	defer rows.Close()
	summaries := make([]runtimeapp.DeploymentSummary, 0)
	for rows.Next() {
		var (
			number         int
			deployedAt     time.Time
			cause          string
			rolledBackFrom sql.NullInt64
		)
		if err := rows.Scan(&number, &deployedAt, &cause, &rolledBackFrom); err != nil {
			return nil, fmt.Errorf("cannot scan a deployment of project %s: %w", projectID, err)
		}
		summaries = append(summaries, runtimeapp.DeploymentSummary{
			Number:         number,
			DeployedAt:     deployedAt.UTC(),
			Cause:          runtimeapp.DeploymentCause(cause),
			RolledBackFrom: nullableInt(rolledBackFrom),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot iterate over the deployments of project %s: %w", projectID, err)
	}
	return summaries, nil
}

// DeploymentNamesLibrary reports whether the running Deployment of any live
// Project names a Release of the Library (task 71): the latest Deployment of
// each live Project is read, and its document's library_pins are searched by
// the Library's ID. The pins are provenance of the document, so the answer
// needs no copy of them in a table of their own.
func (r deploymentRepositoryPostgres) DeploymentNamesLibrary(ctx context.Context, libraryID uuid.UUID) (bool, error) {
	var named bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS (
		     SELECT 1
		     FROM (
		         SELECT DISTINCT ON (project_id) project_id, config
		         FROM runtime.deployments
		         ORDER BY project_id, number DESC
		     ) running
		     JOIN projects p ON p.id = running.project_id AND p.deleted_at IS NULL
		     WHERE EXISTS (
		         SELECT 1
		         FROM jsonb_array_elements(
		                  CASE WHEN jsonb_typeof(running.config -> 'library_pins') = 'array'
		                       THEN running.config -> 'library_pins'
		                       ELSE '[]'::jsonb END) AS pin
		         WHERE pin ->> 'library_id' = $1
		     )
		 )`,
		libraryID.String(),
	).Scan(&named)
	if err != nil {
		return false, fmt.Errorf("cannot read the running deployments naming library %s: %w", libraryID, err)
	}
	return named, nil
}

func (r deploymentRepositoryPostgres) FindByNumber(ctx context.Context, projectID uuid.UUID, n int) (runtimeapp.Deployment, error) {
	if err := liveProject(ctx, r.db, projectID); err != nil {
		return runtimeapp.Deployment{}, err
	}
	var (
		deployedAt     time.Time
		cause          string
		rolledBackFrom sql.NullInt64
		config         []byte
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT deployed_at, cause, rolled_back_from, config FROM runtime.deployments
		 WHERE project_id = $1 AND number = $2`,
		projectID, n,
	).Scan(&deployedAt, &cause, &rolledBackFrom, &config)
	if errors.Is(err, sql.ErrNoRows) {
		return runtimeapp.Deployment{}, fmt.Errorf("%w: deployment %d of project %s", runtimeapp.ErrDeploymentNotFound, n, projectID)
	}
	if err != nil {
		return runtimeapp.Deployment{}, fmt.Errorf("cannot read deployment %d of project %s: %w", n, projectID, err)
	}
	deployment, err := newDeployment(n, deployedAt, cause, rolledBackFrom, config)
	if err != nil {
		return runtimeapp.Deployment{}, fmt.Errorf("deployment %d of project %s: %w", n, projectID, err)
	}
	return deployment, nil
}

// newDeployment assembles a Deployment read from the store.
func newDeployment(number int, deployedAt time.Time, cause string, rolledBackFrom sql.NullInt64, config []byte) (runtimeapp.Deployment, error) {
	var file projectv0.ProjectFile
	if err := json.Unmarshal(config, &file); err != nil {
		return runtimeapp.Deployment{}, fmt.Errorf("cannot decode the document: %w", err)
	}
	return runtimeapp.Deployment{
		Number:         number,
		DeployedAt:     deployedAt.UTC(),
		Cause:          runtimeapp.DeploymentCause(cause),
		RolledBackFrom: nullableInt(rolledBackFrom),
		Content:        file,
	}, nil
}

func nullableInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	n := int(value.Int64)
	return &n
}

// rowQuerier is what reads the Project's row in either a transaction or a
// plain connection.
type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// liveProject refuses a read of the Deployments of a Project that is not live:
// a deleted Project keeps its rows until its cleanup removes them, and they
// are not its Deployments to serve any more.
func liveProject(ctx context.Context, q rowQuerier, projectID uuid.UUID) error {
	live, err := projectIsLive(ctx, q, projectID)
	if err != nil {
		return err
	}
	if !live {
		return fmt.Errorf("%w: %s", project.ErrProjectNotFound, projectID)
	}
	return nil
}

// refreshCopies rebuilds the Runtime node's derived copies from the document
// of a Deployment: what the document no longer has is removed with its pairs,
// what it has is inserted or updated, and a Tag's state is left alone unless
// the Tag appeared or changed its TagType. Device Tokens are never touched: a
// token outlives a Deploy that drops its Device (ADR 0004, amendment
// 2026-11-01).
func refreshCopies(ctx context.Context, tx *sql.Tx, projectID uuid.UUID, file projectv0.ProjectFile) error {
	if err := removeAbsentCopies(ctx, tx, projectID, file); err != nil {
		return err
	}
	if err := upsertDevices(ctx, tx, projectID, file.Devices); err != nil {
		return err
	}
	if err := upsertEnvironments(ctx, tx, projectID, file.Environments); err != nil {
		return err
	}
	if err := refreshEnvironmentDevices(ctx, tx, projectID, file.EnvironmentDevices); err != nil {
		return err
	}
	return refreshTags(ctx, tx, projectID, file.Tags)
}

// removeAbsentCopies removes the copies the document no longer names. The
// pairs of an Environment or a Device go with it (ON DELETE CASCADE); the
// states of a removed Tag go with its copy.
func removeAbsentCopies(ctx context.Context, tx *sql.Tx, projectID uuid.UUID, file projectv0.ProjectFile) error {
	tagIDs := make([]string, len(file.Tags))
	for i, t := range file.Tags {
		tagIDs[i] = t.ID.String()
	}
	deviceIDs := make([]string, len(file.Devices))
	for i, d := range file.Devices {
		deviceIDs[i] = d.ID.String()
	}
	environmentIDs := make([]string, len(file.Environments))
	for i, e := range file.Environments {
		environmentIDs[i] = e.ID.String()
	}
	for _, remove := range []struct {
		what  string
		table string
		ids   []string
	}{
		{"tag definitions", `runtime.tags`, tagIDs},
		{"devices", `runtime.devices`, deviceIDs},
		{"environments", `runtime.environments`, environmentIDs},
	} {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM `+remove.table+` WHERE project_id = $1 AND NOT (id = ANY($2::uuid[]))`,
			projectID, pq.Array(remove.ids),
		); err != nil {
			return fmt.Errorf("cannot remove the %s absent from the document of project %s: %w", remove.what, projectID, err)
		}
	}
	return nil
}

// upsertDevices stores the Devices of the document, matched by ID. A copy is
// derived data: its Version stays as it is, Deploy does not edit an aggregate.
func upsertDevices(ctx context.Context, tx *sql.Tx, projectID uuid.UUID, devices []projectv0.Device) error {
	for _, d := range devices {
		// json.Marshal, not []byte: the zero value of JSONObject is the empty
		// object, while a nil slice would reach the NOT NULL column as NULL.
		config, err := json.Marshal(d.Config)
		if err != nil {
			return fmt.Errorf("cannot encode the config of device %s: %w", d.ID, err)
		}
		var touched int
		err = tx.QueryRowContext(ctx,
			`INSERT INTO runtime.devices (id, project_id, name, comment, protocol, config, version)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (id) DO UPDATE
			 SET name = EXCLUDED.name, comment = EXCLUDED.comment, protocol = EXCLUDED.protocol, config = EXCLUDED.config
			 WHERE runtime.devices.project_id = EXCLUDED.project_id
			 RETURNING 1`,
			d.ID, projectID, d.Name, d.Comment, d.Protocol, config, 0,
		).Scan(&touched)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("cannot upsert device %s: its copy is of another project than %s", d.ID, projectID)
		}
		if err != nil {
			return fmt.Errorf("cannot upsert device %s: %w", d.ID, err)
		}
	}
	return nil
}

// upsertEnvironments stores the Environments of the document, matched by ID.
// A Gateway resolves its Environment by name, which stays unique in the
// Project.
func upsertEnvironments(ctx context.Context, tx *sql.Tx, projectID uuid.UUID, environments []projectv0.Environment) error {
	for _, e := range environments {
		var touched int
		err := tx.QueryRowContext(ctx,
			`INSERT INTO runtime.environments (id, project_id, name, comment, version)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (id) DO UPDATE
			 SET name = EXCLUDED.name, comment = EXCLUDED.comment
			 WHERE runtime.environments.project_id = EXCLUDED.project_id
			 RETURNING 1`,
			e.ID, projectID, e.Name, e.Comment, 0,
		).Scan(&touched)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("cannot upsert environment %s: its copy is of another project than %s", e.ID, projectID)
		}
		if err != nil {
			return fmt.Errorf("cannot upsert environment %s: %w", e.ID, err)
		}
	}
	return nil
}

// refreshEnvironmentDevices stores the pairs (Environment, Device) with their
// parameters and removes those the document no longer names. A pair of a copy
// an earlier step removed is already gone by the cascade.
func refreshEnvironmentDevices(ctx context.Context, tx *sql.Tx, projectID uuid.UUID, pairs []projectv0.EnvironmentDevice) error {
	environmentIDs := make([]string, len(pairs))
	deviceIDs := make([]string, len(pairs))
	for i, p := range pairs {
		environmentIDs[i] = p.EnvironmentID.String()
		deviceIDs[i] = p.DeviceID.String()
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM runtime.environment_devices ed
		 USING runtime.environments e
		 WHERE ed.environment_id = e.id AND e.project_id = $1
		   AND NOT EXISTS (
		       SELECT 1 FROM unnest($2::uuid[], $3::uuid[]) AS d(environment_id, device_id)
		       WHERE d.environment_id = ed.environment_id AND d.device_id = ed.device_id
		   )`,
		projectID, pq.Array(environmentIDs), pq.Array(deviceIDs),
	); err != nil {
		return fmt.Errorf("cannot remove the environment pairs absent from the document of project %s: %w", projectID, err)
	}
	for _, p := range pairs {
		// json.Marshal, not []byte: a nil JSONObject is the empty object.
		params, err := json.Marshal(p.Params)
		if err != nil {
			return fmt.Errorf("cannot encode the params of environment %s and device %s: %w", p.EnvironmentID, p.DeviceID, err)
		}
		// The pair is written only when both of its copies are of this Project:
		// an ID is unique across Projects, so a foreign one would pass the
		// foreign keys and rewrite the params of another Project's pair.
		touched, err := tx.ExecContext(ctx,
			`INSERT INTO runtime.environment_devices (environment_id, device_id, params)
			 SELECT e.id, d.id, $3
			 FROM runtime.environments e
			 JOIN runtime.devices d ON d.id = $2
			 WHERE e.id = $1 AND e.project_id = $4 AND d.project_id = $4
			 ON CONFLICT (environment_id, device_id) DO UPDATE SET params = EXCLUDED.params`,
			p.EnvironmentID, p.DeviceID, params, projectID,
		)
		if err != nil {
			return fmt.Errorf("cannot upsert the pair of environment %s and device %s: %w", p.EnvironmentID, p.DeviceID, err)
		}
		if affected, err := touched.RowsAffected(); err != nil {
			return fmt.Errorf("cannot read the result of the pair of environment %s and device %s: %w", p.EnvironmentID, p.DeviceID, err)
		} else if affected == 0 {
			return fmt.Errorf("cannot upsert the pair of environment %s and device %s: their copies are not of project %s", p.EnvironmentID, p.DeviceID, projectID)
		}
	}
	return nil
}

// refreshTags stores the Tag definitions of the document, matched by ID; the
// copies the document no longer names are already gone (removeAbsentCopies).
// A state is touched only when its Tag appeared or changed its TagType: then
// it is the initial state of the new TagType with Quality bad. A state written
// to an unchanged Tag survives the Deploy. The copies' Version stays as it is:
// a Deployment carries no aggregate versions.
func refreshTags(ctx context.Context, tx *sql.Tx, projectID uuid.UUID, defs []projectv0.TagDefinition) error {
	existing, err := storedTagTypes(ctx, tx, projectID)
	if err != nil {
		return err
	}
	for _, d := range defs {
		name, err := tag.NewTagName(d.Name)
		if err != nil {
			return fmt.Errorf("cannot create tag name of %s: %w", d.ID, err)
		}
		tagType, err := tag.NewTagType(d.Type)
		if err != nil {
			return fmt.Errorf("cannot create tag type of %s: %w", d.ID, err)
		}
		supplier := supplierOf(d.SupplierDeviceID)
		var touched int
		err = tx.QueryRowContext(ctx,
			`INSERT INTO runtime.tags (id, project_id, name, type, device_id, version)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (id) DO UPDATE
			 SET name = EXCLUDED.name, type = EXCLUDED.type, device_id = EXCLUDED.device_id
			 WHERE runtime.tags.project_id = EXCLUDED.project_id
			 RETURNING 1`,
			d.ID, projectID, name.String(), tagType.String(), supplierValue(supplier), 0,
		).Scan(&touched)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("cannot upsert tag definition %s: its copy is of another project than %s", d.ID, projectID)
		}
		if err != nil {
			return fmt.Errorf("cannot upsert tag definition %s: %w", d.ID, err)
		}
		storedType, known := existing[d.ID]
		initial, err := tagstate.Initial(tagstate.Definition{
			TagID:     id.NewID(id.IDWithUUID[tag.Tag](d.ID)),
			ProjectID: id.NewID(id.IDWithUUID[project.Project](projectID)),
			Name:      name,
			Type:      tagType,
			Supplier:  supplier,
			Version:   version.Initial[tag.Tag](),
		})
		if err != nil {
			return fmt.Errorf("cannot create the initial state of tag %s: %w", d.ID, err)
		}
		// A state is replaced when the Tag appeared or changed its TagType: it is
		// then the initial state of the new TagType. A state written to an
		// unchanged Tag survives the Deploy, but a copy that lost its state gets
		// one back: every read joins the two tables, so a copy without a state is
		// invisible and refuses writes.
		writeState := `INSERT INTO runtime.tag_states (tag_id, value, quality)
			VALUES ($1, $2, $3)
			ON CONFLICT (tag_id) DO UPDATE SET value = EXCLUDED.value, quality = EXCLUDED.quality`
		if known && storedType == tagType.String() {
			writeState = `INSERT INTO runtime.tag_states (tag_id, value, quality)
				VALUES ($1, $2, $3)
				ON CONFLICT (tag_id) DO NOTHING`
		}
		if _, err := tx.ExecContext(ctx, writeState,
			d.ID, initial.Value().String(), initial.Quality().String(),
		); err != nil {
			return fmt.Errorf("cannot set the initial state of tag %s: %w", d.ID, err)
		}
	}
	return nil
}

// storedTagTypes returns the TagType of every copied Tag definition of the
// Project, by Tag ID: what tells a Tag that changed its TagType from one that
// did not.
func storedTagTypes(ctx context.Context, tx *sql.Tx, projectID uuid.UUID) (map[uuid.UUID]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, type FROM runtime.tags WHERE project_id = $1`, projectID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the tag definitions of project %s: %w", projectID, err)
	}
	defer rows.Close()
	types := map[uuid.UUID]string{}
	for rows.Next() {
		var (
			tagID   uuid.UUID
			rawType string
		)
		if err := rows.Scan(&tagID, &rawType); err != nil {
			return nil, fmt.Errorf("cannot scan a tag definition of project %s: %w", projectID, err)
		}
		types[tagID] = rawType
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot iterate over the tag definitions of project %s: %w", projectID, err)
	}
	return types, nil
}
