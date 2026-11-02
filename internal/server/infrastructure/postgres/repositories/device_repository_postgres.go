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
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// deviceRepositoryPostgresImpl is the DeviceRepository of one Project, over
// the Devices of the Draft (engineering.devices, ADR 0004, amendment
// 2026-11-01): every query names project_id.
type deviceRepositoryPostgresImpl struct {
	db      *sql.DB
	project id.ID[project.Project]
	store   aggregateStore[device.Device]
}

var _ device.DeviceRepository = (*deviceRepositoryPostgresImpl)(nil)

// NewDeviceRepositoryPostgres returns the repository of the Devices of the
// Project aProjectID (ProjectScopes.ForProject is how the server opens one).
func NewDeviceRepositoryPostgres(aDb *sql.DB, aProjectID id.ID[project.Project], opts ...Option) device.DeviceRepository {
	o := newOptions(opts)
	return &deviceRepositoryPostgresImpl{
		db:      aDb,
		project: aProjectID,
		store: aggregateStore[device.Device]{
			db: aDb, table: tableNameDevices, project: &aProjectID,
			notFound: device.ErrDeviceNotFound, conflict: device.ErrDeviceConflict,
			onCommit: o.onCommit,
		},
	}
}

func (r deviceRepositoryPostgresImpl) NextID() id.ID[device.Device] {
	return id.NewID[device.Device]()
}

func (r deviceRepositoryPostgresImpl) Save(ctx context.Context, aDevice device.Device) (device.Device, error) {
	if err := r.store.checkProject(aDevice.ProjectID()); err != nil {
		return nil, err
	}
	saved, err := r.store.save(ctx, aDevice.ID(), aDevice.Version(), aDevice.PendingEvents(), rowWrite[device.Device]{
		insert: func(tx *sql.Tx, next version.Version[device.Device]) error {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO engineering.devices (id, project_id, name, comment, protocol, config, version)
				 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				aDevice.ID().UUID(), r.project.UUID(), aDevice.Name().String(), aDevice.Comment(),
				aDevice.Protocol().String(), aDevice.Config().Bytes(), next.Number(),
			)
			return r.deviceWriteError("insert new device", err)
		},
		update: func(tx *sql.Tx, current, next version.Version[device.Device]) (bool, error) {
			res, err := tx.ExecContext(ctx,
				`UPDATE engineering.devices
				 SET name = $1, comment = $2, protocol = $3, config = $4, version = $5
				 WHERE id = $6 AND version = $7 AND project_id = $8`,
				aDevice.Name().String(), aDevice.Comment(), aDevice.Protocol().String(),
				aDevice.Config().Bytes(), next.Number(),
				aDevice.ID().UUID(), current.Number(), r.project.UUID(),
			)
			if err != nil {
				return false, r.deviceWriteError("update device", err)
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
	return device.ReconstituteDevice(aDevice.ID(), r.project, aDevice.Name(), aDevice.Comment(), aDevice.Protocol(), aDevice.Config(), saved)
}

// deviceWriteError maps a failed write of a devices row: a name taken in the
// Project is ErrDeviceNameTaken.
func (r deviceRepositoryPostgresImpl) deviceWriteError(what string, err error) error {
	if err == nil {
		return nil
	}
	if isUniqueViolation(err, constraintDevicesName) {
		return device.ErrDeviceNameTaken
	}
	return fmt.Errorf("cannot %s: %w", what, err)
}

// Delete removes the Device once its Version is checked and nothing refers to
// it: a Tag it supplies or an EnvironmentDevice it stands in refuses the
// removal (ErrDeviceInUse), never a cascade (ADR 0008).
func (r deviceRepositoryPostgresImpl) Delete(ctx context.Context, aDevice device.Device) error {
	if err := r.store.checkProject(aDevice.ProjectID()); err != nil {
		return err
	}
	return r.store.inTx(ctx, aDevice.PendingEvents(), func(tx *sql.Tx) error {
		inUse, err := r.inUse(ctx, tx, aDevice.ID())
		if err != nil {
			return err
		}
		if inUse {
			return fmt.Errorf("%w: device %s", device.ErrDeviceInUse, aDevice.ID())
		}
		res, err := tx.ExecContext(ctx,
			`DELETE FROM engineering.devices WHERE id = $1 AND project_id = $2 AND version = $3`,
			aDevice.ID().UUID(), r.project.UUID(), aDevice.Version().Number(),
		)
		if err != nil {
			return r.deviceDeleteError(aDevice, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("cannot get rows affected on delete: %w", err)
		}
		if n != 1 {
			return r.store.classifyUpdateConflict(ctx, tx, aDevice.ID())
		}
		return nil
	})
}

// inUse reports whether a Tag of the Project is supplied by the Device or the
// Device stands in an Environment. A reference written after this check is
// refused by the foreign keys, which deviceDeleteError maps the same way.
func (r deviceRepositoryPostgresImpl) inUse(ctx context.Context, tx *sql.Tx, deviceID id.ID[device.Device]) (bool, error) {
	var inUse bool
	err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM engineering.tags WHERE device_id = $1)
		     OR EXISTS (SELECT 1 FROM engineering.environment_devices WHERE device_id = $1)`,
		deviceID.UUID(),
	).Scan(&inUse)
	if err != nil {
		return false, fmt.Errorf("cannot check the use of device %s: %w", deviceID, err)
	}
	return inUse, nil
}

// deviceDeleteError maps a failed removal: a reference that appeared since the
// check is ErrDeviceInUse.
func (r deviceRepositoryPostgresImpl) deviceDeleteError(aDevice device.Device, err error) error {
	switch {
	case isForeignKeyViolation(err, constraintTagsDeviceID),
		isForeignKeyViolation(err, constraintEnvironmentDevicesDeviceID):
		return fmt.Errorf("%w: device %s", device.ErrDeviceInUse, aDevice.ID())
	}
	return fmt.Errorf("cannot delete device: %w", err)
}

func (r deviceRepositoryPostgresImpl) FindAll(ctx context.Context) ([]device.Device, error) {
	return r.readDevices(ctx,
		`SELECT id, name, comment, protocol, config, version FROM engineering.devices
		 WHERE project_id = $1 ORDER BY pk_id`,
		r.project.UUID(),
	)
}

func (r deviceRepositoryPostgresImpl) FindAllInLiveProject(ctx context.Context, orgID id.ID[organization.Organization]) ([]device.Device, error) {
	return r.readDevices(ctx,
		`SELECT d.id, d.name, d.comment, d.protocol, d.config, d.version
		 FROM engineering.devices d JOIN projects p ON p.id = d.project_id
		 WHERE d.project_id = $1 AND p.deleted_at IS NULL AND p.organization_id = $2
		 ORDER BY d.pk_id`,
		r.project.UUID(), orgID.UUID(),
	)
}

// readDevices reads the devices query selects, its columns those of FindAll.
func (r deviceRepositoryPostgresImpl) readDevices(ctx context.Context, query string, args ...any) ([]device.Device, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error querying devices: %w", err)
	}
	defer rows.Close()
	devices := []device.Device{}
	for rows.Next() {
		found, err := r.scanDevice(rows.Scan)
		if err != nil {
			return nil, err
		}
		devices = append(devices, found)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over devices: %w", err)
	}
	return devices, nil
}

func (r deviceRepositoryPostgresImpl) FindByID(ctx context.Context, deviceID id.ID[device.Device]) (device.Device, error) {
	found, err := r.scanDevice(r.db.QueryRowContext(ctx,
		`SELECT id, name, comment, protocol, config, version FROM engineering.devices
		 WHERE id = $1 AND project_id = $2`,
		deviceID.UUID(), r.project.UUID(),
	).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, id.NotFound(device.ErrDeviceNotFound, deviceID)
	}
	return found, err
}

func (r deviceRepositoryPostgresImpl) FindByName(ctx context.Context, aName device.DeviceName) (device.Device, error) {
	found, err := r.scanDevice(r.db.QueryRowContext(ctx,
		`SELECT id, name, comment, protocol, config, version FROM engineering.devices
		 WHERE name = $1 AND project_id = $2`,
		aName.String(), r.project.UUID(),
	).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, device.ErrDeviceNotFound
	}
	return found, err
}

// scanDevice reads a single id, name, comment, protocol, config, version row.
func (r deviceRepositoryPostgresImpl) scanDevice(scan func(...any) error) (device.Device, error) {
	var (
		rawID       uuid.UUID
		rawName     string
		rawComment  string
		rawProtocol string
		rawConfig   []byte
		rawVersion  int
	)
	if err := scan(&rawID, &rawName, &rawComment, &rawProtocol, &rawConfig, &rawVersion); err != nil {
		return nil, err
	}
	name, err := device.NewDeviceName(rawName)
	if err != nil {
		return nil, fmt.Errorf("cannot create device name: %w", err)
	}
	protocol, err := device.NewDeviceProtocol(rawProtocol)
	if err != nil {
		return nil, fmt.Errorf("cannot create device protocol: %w", err)
	}
	config, err := device.NewDeviceConfig(rawConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot create device config: %w", err)
	}
	ver, err := version.New(version.WithNumber[device.Device](rawVersion))
	if err != nil {
		return nil, fmt.Errorf("cannot create device version: %w", err)
	}
	return device.ReconstituteDevice(id.NewID(id.IDWithUUID[device.Device](rawID)), r.project, name, rawComment, protocol, config, ver)
}
