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

// deviceRepositoryPostgresImpl reads the Devices of one Project: every query
// names project_id. The Devices are entities of the Project's Draft
// (ADR 0016): their writes go through draftRepositoryPostgres.
type deviceRepositoryPostgresImpl struct {
	db      *sql.DB
	project id.ID[project.Project]
}

var _ device.DeviceRepository = (*deviceRepositoryPostgresImpl)(nil)

// NewDeviceRepositoryPostgres returns the repository of the Devices of the
// Project aProjectID (ProjectScopes.ForProject is how the server opens one).
func NewDeviceRepositoryPostgres(aDb *sql.DB, aProjectID id.ID[project.Project], _ ...Option) device.DeviceRepository {
	return &deviceRepositoryPostgresImpl{db: aDb, project: aProjectID}
}

func (r deviceRepositoryPostgresImpl) NextID() id.ID[device.Device] {
	return id.NewID[device.Device]()
}

func (r deviceRepositoryPostgresImpl) FindAll(ctx context.Context) ([]device.Device, error) {
	return r.readDevices(ctx,
		`SELECT id, name, comment, protocol, config FROM engineering.devices
		 WHERE project_id = $1 ORDER BY pk_id`,
		r.project.UUID(),
	)
}

func (r deviceRepositoryPostgresImpl) FindAllInLiveProject(ctx context.Context, orgID id.ID[organization.Organization]) ([]device.Device, error) {
	return r.readDevices(ctx,
		`SELECT d.id, d.name, d.comment, d.protocol, d.config
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
		`SELECT id, name, comment, protocol, config FROM engineering.devices
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
		`SELECT id, name, comment, protocol, config FROM engineering.devices
		 WHERE name = $1 AND project_id = $2`,
		aName.String(), r.project.UUID(),
	).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, device.ErrDeviceNotFound
	}
	return found, err
}

// scanDevice reads a single id, name, comment, protocol, config row.
func (r deviceRepositoryPostgresImpl) scanDevice(scan func(...any) error) (device.Device, error) {
	var (
		rawID       uuid.UUID
		rawName     string
		rawComment  string
		rawProtocol string
		rawConfig   []byte
	)
	if err := scan(&rawID, &rawName, &rawComment, &rawProtocol, &rawConfig); err != nil {
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
	return device.ReconstituteDevice(id.NewID(id.IDWithUUID[device.Device](rawID)), r.project, name, rawComment, protocol, config)
}
