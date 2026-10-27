package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

type projectRepositoryPostgresImpl struct {
	db    *sql.DB
	store aggregateStore[project.Project]
}

var _ project.ProjectRepository = (*projectRepositoryPostgresImpl)(nil)

func NewProjectRepositoryPostgres(aDb *sql.DB, opts ...Option) project.ProjectRepository {
	o := newOptions(opts)
	return &projectRepositoryPostgresImpl{
		db: aDb,
		store: aggregateStore[project.Project]{
			db: aDb, table: tableNameProjects,
			notFound: project.ErrProjectNotFound, conflict: project.ErrProjectConflict,
			onCommit: o.onCommit,
		},
	}
}

// FindSoleOrganization returns the one Organization the server runs with
// until Users exist (ADR 0006, task 49), created by the migrations. Any other
// number of Organizations is an error.
func FindSoleOrganization(ctx context.Context, db *sql.DB) (id.ID[organization.Organization], error) {
	rows, err := db.QueryContext(ctx, `SELECT id FROM organizations ORDER BY pk_id LIMIT 2`)
	if err != nil {
		return id.ID[organization.Organization]{}, fmt.Errorf("cannot read organizations: %w", err)
	}
	defer rows.Close()
	var found []uuid.UUID
	for rows.Next() {
		var raw uuid.UUID
		if err := rows.Scan(&raw); err != nil {
			return id.ID[organization.Organization]{}, fmt.Errorf("cannot scan organization: %w", err)
		}
		found = append(found, raw)
	}
	if err := rows.Err(); err != nil {
		return id.ID[organization.Organization]{}, fmt.Errorf("cannot read organizations: %w", err)
	}
	if len(found) != 1 {
		return id.ID[organization.Organization]{}, fmt.Errorf("expected exactly one organization, found %d", len(found))
	}
	return id.NewID(id.IDWithUUID[organization.Organization](found[0])), nil
}

func (r projectRepositoryPostgresImpl) NextID() id.ID[project.Project] {
	return id.NewID[project.Project]()
}

// Save writes the projects row. The CAS on update matches only a live row,
// and a row it does not match is classified by whether a live one is there.
func (r projectRepositoryPostgresImpl) Save(ctx context.Context, p project.Project) (project.Project, error) {
	saved, err := r.store.save(ctx, p.ID(), p.Version(), p.PendingEvents(), rowWrite[project.Project]{
		insert: func(tx *sql.Tx, next version.Version[project.Project]) error {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO projects (id, organization_id, name, version, deleted_at)
				 VALUES ($1, $2, $3, $4, CASE WHEN $5 THEN now() END)`,
				p.ID().UUID(), p.OrganizationID().UUID(), p.Name().String(), next.Number(), p.IsDeleted(),
			)
			return projectWriteError("insert new project", err)
		},
		update: func(tx *sql.Tx, current, next version.Version[project.Project]) (bool, error) {
			res, err := tx.ExecContext(ctx,
				`UPDATE projects
				 SET name = $1, version = $2, deleted_at = CASE WHEN $3 THEN now() END
				 WHERE id = $4 AND version = $5 AND deleted_at IS NULL`,
				p.Name().String(), next.Number(), p.IsDeleted(), p.ID().UUID(), current.Number(),
			)
			if err != nil {
				return false, projectWriteError("update project", err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return false, fmt.Errorf("cannot get rows affected on update: %w", err)
			}
			if n == 1 {
				return true, nil
			}
			// A project marked deleted is gone for the CAS: not found, not a
			// conflict, whatever its version.
			var live bool
			err = tx.QueryRowContext(ctx, `SELECT deleted_at IS NULL FROM projects WHERE id = $1`, p.ID().UUID()).Scan(&live)
			if errors.Is(err, sql.ErrNoRows) || (err == nil && !live) {
				return false, id.NotFound(project.ErrProjectNotFound, p.ID())
			}
			if err != nil {
				return false, fmt.Errorf("cannot check project existence: %w", err)
			}
			return false, nil
		},
	})
	if err != nil {
		return nil, err
	}
	return project.ReconstituteProject(p.ID(), p.OrganizationID(), p.Name(), p.IsDeleted(), saved), nil
}

// projectWriteError maps a failed write of a projects row: a name taken by a
// live project of the organization is ErrProjectNameTaken.
func projectWriteError(what string, err error) error {
	if err == nil {
		return nil
	}
	if isUniqueViolation(err, constraintProjectsLiveName) {
		return project.ErrProjectNameTaken
	}
	return fmt.Errorf("cannot %s: %w", what, err)
}

// Delete removes the row of a project marked deleted, whatever its version:
// nothing changes a deleted project. The foreign keys of its content refuse
// it while Tags, Scenes, Library Pins or Tag states remain.
func (r projectRepositoryPostgresImpl) Delete(ctx context.Context, p project.Project) error {
	return r.store.inTx(ctx, p.PendingEvents(), func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id = $1 AND deleted_at IS NOT NULL`, p.ID().UUID())
		if isForeignKeyViolation(err, constraintTagsProjectID) || isForeignKeyViolation(err, constraintScenesProjectID) ||
			isForeignKeyViolation(err, constraintLibraryPinsProjectID) || isForeignKeyViolation(err, constraintRuntimeTagsProjectID) {
			return fmt.Errorf("%w: %s", project.ErrProjectNotEmpty, p.ID())
		}
		if err != nil {
			return fmt.Errorf("cannot delete from projects: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("cannot get rows affected on delete: %w", err)
		}
		if n != 1 {
			return id.NotFound(project.ErrProjectNotFound, p.ID())
		}
		return nil
	})
}

const selectProjectColumns = `id, organization_id, name, deleted_at IS NOT NULL, version`

func (r projectRepositoryPostgresImpl) FindByID(ctx context.Context, projectID id.ID[project.Project]) (project.Project, error) {
	return r.findOne(ctx, projectID, `WHERE id = $1 AND deleted_at IS NULL`, projectID.UUID())
}

func (r projectRepositoryPostgresImpl) FindDeleted(ctx context.Context, projectID id.ID[project.Project]) (project.Project, error) {
	return r.findOne(ctx, projectID, `WHERE id = $1 AND deleted_at IS NOT NULL`, projectID.UUID())
}

func (r projectRepositoryPostgresImpl) findOne(ctx context.Context, projectID id.ID[project.Project], where string, args ...any) (project.Project, error) {
	found, err := r.scanProject(r.db.QueryRowContext(ctx, `SELECT `+selectProjectColumns+` FROM projects `+where, args...).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, id.NotFound(project.ErrProjectNotFound, projectID)
	}
	if err != nil {
		return nil, fmt.Errorf("error scanning project: %w", err)
	}
	return found, nil
}

func (r projectRepositoryPostgresImpl) FindByName(ctx context.Context, orgID id.ID[organization.Organization], aName project.ProjectName) (project.Project, error) {
	found, err := r.scanProject(r.db.QueryRowContext(ctx,
		`SELECT `+selectProjectColumns+` FROM projects
		 WHERE organization_id = $1 AND name = $2 AND deleted_at IS NULL`,
		orgID.UUID(), aName.String(),
	).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, project.ErrProjectNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("error scanning project: %w", err)
	}
	return found, nil
}

func (r projectRepositoryPostgresImpl) FindAll(ctx context.Context, orgID id.ID[organization.Organization]) ([]project.Project, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+selectProjectColumns+` FROM projects
		 WHERE organization_id = $1 AND deleted_at IS NULL ORDER BY pk_id`,
		orgID.UUID(),
	)
	if err != nil {
		return nil, fmt.Errorf("error querying projects: %w", err)
	}
	defer rows.Close()

	projects := []project.Project{}
	for rows.Next() {
		p, err := r.scanProject(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("error scanning project: %w", err)
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over projects: %w", err)
	}
	return projects, nil
}

// scanProject reads a row of selectProjectColumns.
func (r projectRepositoryPostgresImpl) scanProject(scan func(...any) error) (project.Project, error) {
	var (
		rawID, rawOrgID uuid.UUID
		rawName         string
		deleted         bool
		rawVersion      int
	)
	if err := scan(&rawID, &rawOrgID, &rawName, &deleted, &rawVersion); err != nil {
		return nil, err
	}
	name, err := project.NewProjectName(rawName)
	if err != nil {
		return nil, fmt.Errorf("cannot create project name: %w", err)
	}
	ver, err := version.New(version.WithNumber[project.Project](rawVersion))
	if err != nil {
		return nil, fmt.Errorf("cannot create project version: %w", err)
	}
	return project.ReconstituteProject(
		id.NewID(id.IDWithUUID[project.Project](rawID)),
		id.NewID(id.IDWithUUID[organization.Organization](rawOrgID)),
		name, deleted, ver,
	), nil
}
