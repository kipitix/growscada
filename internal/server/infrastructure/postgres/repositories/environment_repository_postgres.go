package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// environmentRepositoryPostgresImpl reads the Environments of one Project:
// every query names project_id. They are entities of the Project's Draft
// (ADR 0016): their writes go through draftRepositoryPostgres.
type environmentRepositoryPostgresImpl struct {
	db      *sql.DB
	project id.ID[project.Project]
}

var _ device.EnvironmentRepository = (*environmentRepositoryPostgresImpl)(nil)

// NewEnvironmentRepositoryPostgres returns the repository of the Environments
// of the Project aProjectID.
func NewEnvironmentRepositoryPostgres(aDb *sql.DB, aProjectID id.ID[project.Project], _ ...Option) device.EnvironmentRepository {
	return &environmentRepositoryPostgresImpl{db: aDb, project: aProjectID}
}

func (r environmentRepositoryPostgresImpl) NextID() id.ID[device.Environment] {
	return id.NewID[device.Environment]()
}

func (r environmentRepositoryPostgresImpl) FindAll(ctx context.Context) ([]device.Environment, error) {
	return r.readEnvironments(ctx,
		`SELECT id, name, comment FROM engineering.environments
		 WHERE project_id = $1 ORDER BY pk_id`,
		r.project.UUID(),
	)
}

func (r environmentRepositoryPostgresImpl) FindAllInLiveProject(ctx context.Context, orgID id.ID[organization.Organization]) ([]device.Environment, error) {
	return r.readEnvironments(ctx,
		`SELECT e.id, e.name, e.comment
		 FROM engineering.environments e JOIN projects p ON p.id = e.project_id
		 WHERE e.project_id = $1 AND p.deleted_at IS NULL AND p.organization_id = $2
		 ORDER BY e.pk_id`,
		r.project.UUID(), orgID.UUID(),
	)
}

// readEnvironments reads the environments query selects, its columns those of
// FindAll.
func (r environmentRepositoryPostgresImpl) readEnvironments(ctx context.Context, query string, args ...any) ([]device.Environment, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
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
		`SELECT id, name, comment FROM engineering.environments
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
		`SELECT id, name, comment FROM engineering.environments
		 WHERE name = $1 AND project_id = $2`,
		aName.String(), r.project.UUID(),
	).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, device.ErrEnvironmentNotFound
	}
	return found, err
}

// scanEnvironment reads a single id, name, comment row.
func (r environmentRepositoryPostgresImpl) scanEnvironment(scan func(...any) error) (device.Environment, error) {
	var (
		rawID      uuid.UUID
		rawName    string
		rawComment string
	)
	if err := scan(&rawID, &rawName, &rawComment); err != nil {
		return nil, err
	}
	name, err := device.NewEnvironmentName(rawName)
	if err != nil {
		return nil, fmt.Errorf("cannot create environment name: %w", err)
	}
	return device.ReconstituteEnvironment(id.NewID(id.IDWithUUID[device.Environment](rawID)), r.project, name, rawComment)
}
