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

// environmentDeviceRepositoryPostgresImpl is the EnvironmentDeviceRepository
// of one Project, over the pairs of an Environment and a Device of the Draft
// (engineering.environment_devices, ADR 0004, amendment 2026-11-01): every
// query is scoped through the pair's Environment, which names the Project. A
// pair has no identifier of its own: (environment_id, device_id) is its key,
// as a Library Pin is identified by its Library.
type environmentDeviceRepositoryPostgresImpl struct {
	db      *sql.DB
	project id.ID[project.Project]
	store   aggregateStore[device.EnvironmentDevice]
}

var _ device.EnvironmentDeviceRepository = (*environmentDeviceRepositoryPostgresImpl)(nil)

// NewEnvironmentDeviceRepositoryPostgres returns the repository of the
// EnvironmentDevices of the Project aProjectID (ProjectScopes.ForProject is
// how the server opens one).
func NewEnvironmentDeviceRepositoryPostgres(aDb *sql.DB, aProjectID id.ID[project.Project], opts ...Option) device.EnvironmentDeviceRepository {
	o := newOptions(opts)
	return &environmentDeviceRepositoryPostgresImpl{
		db:      aDb,
		project: aProjectID,
		store: aggregateStore[device.EnvironmentDevice]{
			db: aDb, table: tableNameEnvironmentDevices, project: &aProjectID,
			notFound: device.ErrEnvironmentDeviceNotFound, conflict: device.ErrEnvironmentDeviceConflict,
			onCommit: o.onCommit,
		},
	}
}

func (r environmentDeviceRepositoryPostgresImpl) Save(ctx context.Context, aPair device.EnvironmentDevice) (device.EnvironmentDevice, error) {
	if err := r.store.checkProject(aPair.ProjectID()); err != nil {
		return nil, err
	}
	current := aPair.Version()
	var next version.Version[device.EnvironmentDevice]
	switch {
	case current == version.Initial[device.EnvironmentDevice]():
		next = version.Committed[device.EnvironmentDevice]()
	case current.IsCommitted():
		next = current.Next()
	default:
		return nil, fmt.Errorf("undefined behavior with version %d", current.Number())
	}

	err := r.store.inTx(ctx, aPair.PendingEvents(), func(tx *sql.Tx) error {
		if current == version.Initial[device.EnvironmentDevice]() {
			if err := r.store.lockLiveProject(ctx, tx); err != nil {
				return err
			}
			if err := r.checkSidesOfProject(ctx, tx, aPair); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx,
				`INSERT INTO engineering.environment_devices (environment_id, device_id, params, version)
				 VALUES ($1, $2, $3, $4)`,
				aPair.EnvironmentID().UUID(), aPair.DeviceID().UUID(), aPair.Params().Bytes(), next.Number(),
			)
			return r.pairWriteError(aPair, err)
		}
		res, err := tx.ExecContext(ctx,
			`UPDATE engineering.environment_devices ed
			 SET params = $1, version = $2
			 WHERE ed.environment_id = $3 AND ed.device_id = $4 AND ed.version = $5
			   AND EXISTS (SELECT 1 FROM engineering.environments e
			               WHERE e.id = ed.environment_id AND e.project_id = $6)`,
			aPair.Params().Bytes(), next.Number(),
			aPair.EnvironmentID().UUID(), aPair.DeviceID().UUID(), current.Number(), r.project.UUID(),
		)
		if err != nil {
			return fmt.Errorf("cannot update environment device: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("cannot get rows affected on update: %w", err)
		}
		if n != 1 {
			return r.classifyPairWrite(ctx, tx, aPair)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return device.ReconstituteEnvironmentDevice(r.project, aPair.EnvironmentID(), aPair.DeviceID(), aPair.Params(), next), nil
}

// checkSidesOfProject fails unless both the Environment and the Device of the
// pair are of the Project: the foreign keys check existence alone.
func (r environmentDeviceRepositoryPostgresImpl) checkSidesOfProject(ctx context.Context, tx *sql.Tx, aPair device.EnvironmentDevice) error {
	var (
		environmentOfProject bool
		deviceOfProject      bool
	)
	err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM engineering.environments WHERE id = $1 AND project_id = $2),
		        EXISTS (SELECT 1 FROM engineering.devices WHERE id = $3 AND project_id = $2)`,
		aPair.EnvironmentID().UUID(), r.project.UUID(), aPair.DeviceID().UUID(),
	).Scan(&environmentOfProject, &deviceOfProject)
	if err != nil {
		return fmt.Errorf("cannot check the sides of environment device: %w", err)
	}
	if !environmentOfProject {
		return id.NotFound(device.ErrEnvironmentNotFound, aPair.EnvironmentID())
	}
	if !deviceOfProject {
		return id.NotFound(device.ErrDeviceNotFound, aPair.DeviceID())
	}
	return nil
}

// pairWriteError maps a failed insert of an environment_devices row.
func (r environmentDeviceRepositoryPostgresImpl) pairWriteError(aPair device.EnvironmentDevice, err error) error {
	switch {
	case err == nil:
		return nil
	case isUniqueViolation(err, constraintEnvironmentDevicesPair):
		return fmt.Errorf("%w: environment %s, device %s", device.ErrEnvironmentDeviceExists, aPair.EnvironmentID(), aPair.DeviceID())
	case isForeignKeyViolation(err, constraintEnvironmentDevicesEnvironmentID):
		return id.NotFound(device.ErrEnvironmentNotFound, aPair.EnvironmentID())
	case isForeignKeyViolation(err, constraintEnvironmentDevicesDeviceID):
		return id.NotFound(device.ErrDeviceNotFound, aPair.DeviceID())
	}
	return fmt.Errorf("cannot insert environment device: %w", err)
}

// classifyPairWrite tells why a compare-and-swap on the pair matched nothing:
// there is no such pair of the Project (not found) or its version moved on
// (conflict).
func (r environmentDeviceRepositoryPostgresImpl) classifyPairWrite(ctx context.Context, tx *sql.Tx, aPair device.EnvironmentDevice) error {
	var exists bool
	err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM engineering.environment_devices ed
		                JOIN engineering.environments e ON e.id = ed.environment_id
		                WHERE ed.environment_id = $1 AND ed.device_id = $2 AND e.project_id = $3)`,
		aPair.EnvironmentID().UUID(), aPair.DeviceID().UUID(), r.project.UUID(),
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("cannot check environment device existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("%w: environment %s, device %s", device.ErrEnvironmentDeviceNotFound, aPair.EnvironmentID(), aPair.DeviceID())
	}
	return device.ErrEnvironmentDeviceConflict
}

func (r environmentDeviceRepositoryPostgresImpl) Delete(ctx context.Context, aPair device.EnvironmentDevice) error {
	if err := r.store.checkProject(aPair.ProjectID()); err != nil {
		return err
	}
	return r.store.inTx(ctx, aPair.PendingEvents(), func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`DELETE FROM engineering.environment_devices ed
			 WHERE ed.environment_id = $1 AND ed.device_id = $2 AND ed.version = $3
			   AND EXISTS (SELECT 1 FROM engineering.environments e
			               WHERE e.id = ed.environment_id AND e.project_id = $4)`,
			aPair.EnvironmentID().UUID(), aPair.DeviceID().UUID(), aPair.Version().Number(), r.project.UUID(),
		)
		if err != nil {
			return fmt.Errorf("cannot delete environment device: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("cannot get rows affected on delete: %w", err)
		}
		if n != 1 {
			return r.classifyPairWrite(ctx, tx, aPair)
		}
		return nil
	})
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
		`SELECT ed.environment_id, ed.device_id, ed.params, ed.version
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
		`SELECT ed.environment_id, ed.device_id, ed.params, ed.version
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

// scanPair reads a single environment_id, device_id, params, version row.
func (r environmentDeviceRepositoryPostgresImpl) scanPair(scan func(...any) error) (device.EnvironmentDevice, error) {
	var (
		rawEnvironmentID uuid.UUID
		rawDeviceID      uuid.UUID
		rawParams        []byte
		rawVersion       int
	)
	if err := scan(&rawEnvironmentID, &rawDeviceID, &rawParams, &rawVersion); err != nil {
		return nil, err
	}
	params, err := device.NewEnvironmentDeviceParams(rawParams)
	if err != nil {
		return nil, fmt.Errorf("cannot create environment device params: %w", err)
	}
	ver, err := version.New(version.WithNumber[device.EnvironmentDevice](rawVersion))
	if err != nil {
		return nil, fmt.Errorf("cannot create environment device version: %w", err)
	}
	return device.ReconstituteEnvironmentDevice(
		r.project,
		id.NewID(id.IDWithUUID[device.Environment](rawEnvironmentID)),
		id.NewID(id.IDWithUUID[device.Device](rawDeviceID)),
		params, ver,
	), nil
}
