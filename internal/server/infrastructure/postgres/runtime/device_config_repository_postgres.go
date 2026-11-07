package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// deviceConfigRepositoryPostgres reads the configuration a Device Gateway
// serves from the Runtime node's copies — runtime.devices, runtime.environments,
// runtime.environment_devices and runtime.tags — which the running Deployment
// fills (task 35, Q5). It never reads the Draft, and never the engineering
// schema.
type deviceConfigRepositoryPostgres struct {
	db *sql.DB
}

var _ runtimeapp.DeviceConfigRepository = (*deviceConfigRepositoryPostgres)(nil)

// NewDeviceConfigRepositoryPostgres returns the repository of the Gateways'
// configuration.
func NewDeviceConfigRepositoryPostgres(aDb *sql.DB) runtimeapp.DeviceConfigRepository {
	return &deviceConfigRepositoryPostgres{db: aDb}
}

// FindConfig reads the Device, its Environment, their pair and the Tags the
// Device supplies in one transaction, so a Deploy that commits meanwhile
// cannot mix two Deployments in one answer. The transaction is REPEATABLE READ:
// READ COMMITTED would give every statement its own snapshot and let a Deploy
// land between them. A Project marked deleted is not found, as every read of
// its content.
func (r deviceConfigRepositoryPostgres) FindConfig(ctx context.Context, projectID, deviceID, environmentID uuid.UUID) (runtimeapp.DeviceConfig, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return runtimeapp.DeviceConfig{}, fmt.Errorf("cannot begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	live, err := projectIsLive(ctx, tx, projectID)
	if err != nil {
		return runtimeapp.DeviceConfig{}, err
	}
	if !live {
		return runtimeapp.DeviceConfig{}, id.NotFound(project.ErrProjectNotFound, id.NewID(id.IDWithUUID[project.Project](projectID)))
	}
	foundDevice, err := findConfigDevice(ctx, tx, projectID, deviceID)
	if err != nil {
		return runtimeapp.DeviceConfig{}, err
	}
	foundEnvironment, err := findConfigEnvironment(ctx, tx, projectID, environmentID)
	if err != nil {
		return runtimeapp.DeviceConfig{}, err
	}
	connection, err := findConfigConnection(ctx, tx, environmentID, deviceID)
	if err != nil {
		return runtimeapp.DeviceConfig{}, err
	}
	tags, err := findConfigTags(ctx, tx, projectID, deviceID)
	if err != nil {
		return runtimeapp.DeviceConfig{}, err
	}
	return runtimeapp.DeviceConfig{
		Device:      foundDevice,
		Environment: foundEnvironment,
		Connection:  connection,
		Tags:        tags,
	}, nil
}

// projectIsLive reports whether the Project is live: its content stops being
// served as soon as it is marked deleted. Callers that serve the Project's
// rows wrap a false into the error their port answers.
func projectIsLive(ctx context.Context, q rowQuerier, projectID uuid.UUID) (bool, error) {
	var live bool
	if err := q.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM projects WHERE id = $1 AND deleted_at IS NULL)`, projectID,
	).Scan(&live); err != nil {
		return false, fmt.Errorf("cannot read project %s: %w", projectID, err)
	}
	return live, nil
}

func findConfigDevice(ctx context.Context, tx *sql.Tx, projectID, rawDeviceID uuid.UUID) (runtimeapp.DeviceConfigDevice, error) {
	var (
		storedID uuid.UUID
		name     string
		protocol string
		config   []byte
	)
	err := tx.QueryRowContext(ctx,
		`SELECT id, name, protocol, config FROM runtime.devices WHERE project_id = $1 AND id = $2`,
		projectID, rawDeviceID,
	).Scan(&storedID, &name, &protocol, &config)
	if errors.Is(err, sql.ErrNoRows) {
		return runtimeapp.DeviceConfigDevice{}, id.NotFound(device.ErrDeviceNotFound, id.NewID(id.IDWithUUID[device.Device](rawDeviceID)))
	}
	if err != nil {
		return runtimeapp.DeviceConfigDevice{}, fmt.Errorf("cannot read device %s: %w", rawDeviceID, err)
	}
	return runtimeapp.DeviceConfigDevice{ID: storedID, Name: name, Protocol: protocol, Config: config}, nil
}

func findConfigEnvironment(ctx context.Context, tx *sql.Tx, projectID, rawEnvironmentID uuid.UUID) (runtimeapp.DeviceConfigEnvironment, error) {
	var (
		storedID uuid.UUID
		name     string
	)
	err := tx.QueryRowContext(ctx,
		`SELECT id, name FROM runtime.environments WHERE project_id = $1 AND id = $2`,
		projectID, rawEnvironmentID,
	).Scan(&storedID, &name)
	if errors.Is(err, sql.ErrNoRows) {
		return runtimeapp.DeviceConfigEnvironment{}, id.NotFound(device.ErrEnvironmentNotFound, id.NewID(id.IDWithUUID[device.Environment](rawEnvironmentID)))
	}
	if err != nil {
		return runtimeapp.DeviceConfigEnvironment{}, fmt.Errorf("cannot read environment %s: %w", rawEnvironmentID, err)
	}
	return runtimeapp.DeviceConfigEnvironment{ID: storedID, Name: name}, nil
}

func findConfigConnection(ctx context.Context, tx *sql.Tx, environmentID, deviceID uuid.UUID) ([]byte, error) {
	var params []byte
	err := tx.QueryRowContext(ctx,
		`SELECT params FROM runtime.environment_devices WHERE environment_id = $1 AND device_id = $2`,
		environmentID, deviceID,
	).Scan(&params)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: environment %s, device %s", device.ErrEnvironmentDeviceNotFound, environmentID, deviceID)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read the pair of environment %s and device %s: %w", environmentID, deviceID, err)
	}
	return params, nil
}

// findConfigTags returns the Tags the Device supplies, by ID: the copies of
// the running Deployment, not the Draft's definitions.
func findConfigTags(ctx context.Context, tx *sql.Tx, projectID, deviceID uuid.UUID) ([]runtimeapp.DeviceConfigTag, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, name, type FROM runtime.tags WHERE project_id = $1 AND device_id = $2 ORDER BY id`,
		projectID, deviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("cannot read the tags of device %s: %w", deviceID, err)
	}
	defer rows.Close()
	var tags []runtimeapp.DeviceConfigTag
	for rows.Next() {
		var (
			tagID   uuid.UUID
			name    string
			tagType string
		)
		if err := rows.Scan(&tagID, &name, &tagType); err != nil {
			return nil, fmt.Errorf("cannot scan a tag of device %s: %w", deviceID, err)
		}
		tags = append(tags, runtimeapp.DeviceConfigTag{ID: tagID, Name: name, Type: tagType})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot iterate over the tags of device %s: %w", deviceID, err)
	}
	return tags, nil
}
