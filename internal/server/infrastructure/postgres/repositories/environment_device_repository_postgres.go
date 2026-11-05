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
)

// environmentDeviceRepositoryPostgresImpl reads the pairs (Environment,
// Device) of one Project: every query joins the pair's Environment, which
// names the project. They are entities of the Project's Draft (ADR 0016):
// their writes go through draftRepositoryPostgres.
type environmentDeviceRepositoryPostgresImpl struct {
	db      *sql.DB
	project id.ID[project.Project]
}

var _ device.EnvironmentDeviceRepository = (*environmentDeviceRepositoryPostgresImpl)(nil)

// NewEnvironmentDeviceRepositoryPostgres returns the repository of the pairs
// of the Project aProjectID.
func NewEnvironmentDeviceRepositoryPostgres(aDb *sql.DB, aProjectID id.ID[project.Project], _ ...Option) device.EnvironmentDeviceRepository {
	return &environmentDeviceRepositoryPostgresImpl{db: aDb, project: aProjectID}
}

func (r environmentDeviceRepositoryPostgresImpl) FindAll(ctx context.Context) ([]device.EnvironmentDevice, error) {
	return r.readPairs(ctx, ``, r.project.UUID())
}

func (r environmentDeviceRepositoryPostgresImpl) FindByEnvironment(ctx context.Context, environmentID id.ID[device.Environment]) ([]device.EnvironmentDevice, error) {
	pairs, err := r.readPairs(ctx, ` AND ed.environment_id = $2`, r.project.UUID(), environmentID.UUID())
	if err != nil {
		return nil, err
	}
	if len(pairs) == 0 {
		if _, err := r.environmentOfProject(ctx, environmentID); err != nil {
			return nil, err
		}
	}
	return pairs, nil
}

// environmentOfProject fails with ErrEnvironmentNotFound unless the Project
// holds the Environment.
func (r environmentDeviceRepositoryPostgresImpl) environmentOfProject(ctx context.Context, environmentID id.ID[device.Environment]) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM engineering.environments WHERE id = $1 AND project_id = $2)`,
		environmentID.UUID(), r.project.UUID(),
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("cannot check environment %s: %w", environmentID, err)
	}
	if !exists {
		return false, id.NotFound(device.ErrEnvironmentNotFound, environmentID)
	}
	return true, nil
}

func (r environmentDeviceRepositoryPostgresImpl) FindByPair(ctx context.Context, environmentID id.ID[device.Environment], deviceID id.ID[device.Device]) (device.EnvironmentDevice, error) {
	found, err := r.scanPair(r.db.QueryRowContext(ctx,
		`SELECT ed.environment_id, ed.device_id, ed.params
		 FROM engineering.environment_devices ed
		 JOIN engineering.environments e ON e.id = ed.environment_id
		 WHERE ed.environment_id = $1 AND ed.device_id = $2 AND e.project_id = $3`,
		environmentID.UUID(), deviceID.UUID(), r.project.UUID(),
	).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, device.ErrEnvironmentDeviceNotFound
	}
	return found, err
}

// readPairs reads the pairs matching the condition and, its arguments
// numbered from $2, in creation order.
func (r environmentDeviceRepositoryPostgresImpl) readPairs(ctx context.Context, and string, args ...any) ([]device.EnvironmentDevice, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT ed.environment_id, ed.device_id, ed.params
		 FROM engineering.environment_devices ed
		 JOIN engineering.environments e ON e.id = ed.environment_id
		 WHERE e.project_id = $1`+and+` ORDER BY ed.pk_id`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("error querying environment devices: %w", err)
	}
	defer rows.Close()
	pairs := []device.EnvironmentDevice{}
	for rows.Next() {
		found, err := r.scanPair(rows.Scan)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, found)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over environment devices: %w", err)
	}
	return pairs, nil
}

// scanPair reads a single environment_id, device_id, params row.
func (r environmentDeviceRepositoryPostgresImpl) scanPair(scan func(...any) error) (device.EnvironmentDevice, error) {
	var (
		rawEnvironmentID uuid.UUID
		rawDeviceID      uuid.UUID
		rawParams        []byte
	)
	if err := scan(&rawEnvironmentID, &rawDeviceID, &rawParams); err != nil {
		return nil, err
	}
	params, err := device.NewEnvironmentDeviceParams(rawParams)
	if err != nil {
		return nil, fmt.Errorf("cannot create environment device params: %w", err)
	}
	return device.ReconstituteEnvironmentDevice(
		r.project,
		id.NewID(id.IDWithUUID[device.Environment](rawEnvironmentID)),
		id.NewID(id.IDWithUUID[device.Device](rawDeviceID)),
		params,
	), nil
}
