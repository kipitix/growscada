package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// environmentRepositoryPostgresImpl is the EnvironmentRepository of one
// Project, over the Environments of the Draft (engineering.environments, ADR
// 0004, amendment 2026-11-01): every query names project_id.
type environmentRepositoryPostgresImpl struct {
	db      *sql.DB
	project id.ID[project.Project]
	store   aggregateStore[device.Environment]
}

var _ device.EnvironmentRepository = (*environmentRepositoryPostgresImpl)(nil)

// NewEnvironmentRepositoryPostgres returns the repository of the Environments
// of the Project aProjectID (ProjectScopes.ForProject is how the server opens
// one).
func NewEnvironmentRepositoryPostgres(aDb *sql.DB, aProjectID id.ID[project.Project], opts ...Option) device.EnvironmentRepository {
	o := newOptions(opts)
	return &environmentRepositoryPostgresImpl{
		db:      aDb,
		project: aProjectID,
		store: aggregateStore[device.Environment]{
			db: aDb, table: tableNameEnvironments, project: &aProjectID,
			notFound: device.ErrEnvironmentNotFound, conflict: device.ErrEnvironmentConflict,
			onCommit: o.onCommit,
		},
	}
}

func (r environmentRepositoryPostgresImpl) NextID() id.ID[device.Environment] {
	return id.NewID[device.Environment]()
}

func (r environmentRepositoryPostgresImpl) Save(ctx context.Context, anEnvironment device.Environment) (device.Environment, error) {
	if err := r.store.checkProject(anEnvironment.ProjectID()); err != nil {
		return nil, err
	}
	saved, err := r.store.save(ctx, anEnvironment.ID(), anEnvironment.Version(), anEnvironment.PendingEvents(), rowWrite[device.Environment]{
		insert: func(tx *sql.Tx, next version.Version[device.Environment]) error {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO engineering.environments (id, project_id, name, comment, version)
				 VALUES ($1, $2, $3, $4, $5)`,
				anEnvironment.ID().UUID(), r.project.UUID(), anEnvironment.Name().String(),
				anEnvironment.Comment(), next.Number(),
			)
			return r.environmentWriteError("insert new environment", err)
		},
		update: func(tx *sql.Tx, current, next version.Version[device.Environment]) (bool, error) {
			res, err := tx.ExecContext(ctx,
				`UPDATE engineering.environments
				 SET name = $1, comment = $2, version = $3
				 WHERE id = $4 AND version = $5 AND project_id = $6`,
				anEnvironment.Name().String(), anEnvironment.Comment(), next.Number(),
				anEnvironment.ID().UUID(), current.Number(), r.project.UUID(),
			)
			if err != nil {
				return false, r.environmentWriteError("update environment", err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return false, fmt.Errorf("cannot get rows affected on update: %w", err)
			}
			return n == 1, nil
		},
	})
	if err != nil {
		return nil, err
	}
	return device.ReconstituteEnvironment(anEnvironment.ID(), r.project, anEnvironment.Name(), anEnvironment.Comment(), saved)
}

// environmentWriteError maps a failed write of an environments row: a name
// taken in the Project is ErrEnvironmentNameTaken.
func (r environmentRepositoryPostgresImpl) environmentWriteError(what string, err error) error {
	if err == nil {
		return nil
	}
	if isUniqueViolation(err, constraintEnvironmentsName) {
		return device.ErrEnvironmentNameTaken
	}
	return fmt.Errorf("cannot %s: %w", what, err)
}

// Delete removes the Environment once its Version is checked and it holds no
// EnvironmentDevice: a pair refuses the removal (ErrEnvironmentInUse), never
// a cascade (ADR 0008).
func (r environmentRepositoryPostgresImpl) Delete(ctx context.Context, anEnvironment device.Environment) error {
	if err := r.store.checkProject(anEnvironment.ProjectID()); err != nil {
		return err
	}
	return r.store.inTx(ctx, anEnvironment.PendingEvents(), func(tx *sql.Tx) error {
		inUse, err := r.holdsPairs(ctx, tx, anEnvironment.ID())
		if err != nil {
			return err
		}
		if inUse {
			return fmt.Errorf("%w: environment %s", device.ErrEnvironmentInUse, anEnvironment.ID())
		}
		res, err := tx.ExecContext(ctx,
			`DELETE FROM engineering.environments WHERE id = $1 AND project_id = $2 AND version = $3`,
			anEnvironment.ID().UUID(), r.project.UUID(), anEnvironment.Version().Number(),
		)
		if err != nil {
			return r.environmentDeleteError(anEnvironment, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("cannot get rows affected on delete: %w", err)
		}
		if n != 1 {
			return r.store.classifyUpdateConflict(ctx, tx, anEnvironment.ID())
		}
		return nil
	})
}

// holdsPairs reports whether the Environment holds an EnvironmentDevice. A
// pair written after this check is refused by the foreign key, which
// environmentDeleteError maps the same way.
func (r environmentRepositoryPostgresImpl) holdsPairs(ctx context.Context, tx *sql.Tx, environmentID id.ID[device.Environment]) (bool, error) {
	var holds bool
	err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM engineering.environment_devices WHERE environment_id = $1)`,
		environmentID.UUID(),
	).Scan(&holds)
	if err != nil {
		return false, fmt.Errorf("cannot check the pairs of environment %s: %w", environmentID, err)
	}
	return holds, nil
}

// environmentDeleteError maps a failed removal: a pair that appeared since
// the check is ErrEnvironmentInUse.
func (r environmentRepositoryPostgresImpl) environmentDeleteError(anEnvironment device.Environment, err error) error {
	if isForeignKeyViolation(err, constraintEnvironmentDevicesEnvironmentID) {
		return fmt.Errorf("%w: environment %s", device.ErrEnvironmentInUse, anEnvironment.ID())
	}
	return fmt.Errorf("cannot delete environment: %w", err)
}

func (r environmentRepositoryPostgresImpl) FindAll(ctx context.Context) ([]device.Environment, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, comment, version FROM engineering.environments
		 WHERE project_id = $1 ORDER BY pk_id`,
		r.project.UUID(),
	)
	if err != nil {
		return nil, fmt.Errorf("error querying environments: %w", err)
	}
	defer rows.Close()
	environments := []device.Environment{}
	for rows.Next() {
		found, err := r.scanEnvironment(rows.Scan)
		if err != nil {
			return nil, err
		}
		environments = append(environments, found)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over environments: %w", err)
	}
	return environments, nil
}

func (r environmentRepositoryPostgresImpl) FindByID(ctx context.Context, environmentID id.ID[device.Environment]) (device.Environment, error) {
	found, err := r.scanEnvironment(r.db.QueryRowContext(ctx,
		`SELECT id, name, comment, version FROM engineering.environments
		 WHERE id = $1 AND project_id = $2`,
		environmentID.UUID(), r.project.UUID(),
	).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, id.NotFound(device.ErrEnvironmentNotFound, environmentID)
	}
	return found, err
}

func (r environmentRepositoryPostgresImpl) FindByName(ctx context.Context, aName device.EnvironmentName) (device.Environment, error) {
	found, err := r.scanEnvironment(r.db.QueryRowContext(ctx,
		`SELECT id, name, comment, version FROM engineering.environments
		 WHERE name = $1 AND project_id = $2`,
		aName.String(), r.project.UUID(),
	).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, device.ErrEnvironmentNotFound
	}
	return found, err
}

// scanEnvironment reads a single id, name, comment, version row.
func (r environmentRepositoryPostgresImpl) scanEnvironment(scan func(...any) error) (device.Environment, error) {
	var (
		rawID      uuid.UUID
		rawName    string
		rawComment string
		rawVersion int
	)
	if err := scan(&rawID, &rawName, &rawComment, &rawVersion); err != nil {
		return nil, err
	}
	name, err := device.NewEnvironmentName(rawName)
	if err != nil {
		return nil, fmt.Errorf("cannot create environment name: %w", err)
	}
	ver, err := version.New(version.WithNumber[device.Environment](rawVersion))
	if err != nil {
		return nil, fmt.Errorf("cannot create environment version: %w", err)
	}
	return device.ReconstituteEnvironment(id.NewID(id.IDWithUUID[device.Environment](rawID)), r.project, name, rawComment, ver)
}
