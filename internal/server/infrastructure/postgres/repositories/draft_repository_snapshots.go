package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// deleteOrder and upsertOrder are the orders a change's snapshots are written
// in: a delete before what refers to it, an insert after what it refers to
// (a Widget's WidgetType is in a Library Pin, a Tag's supplier is a Device).
var (
	deleteOrder = []draft.Kind{
		draft.KindWidget, draft.KindScene, draft.KindLibraryPin, draft.KindTag,
		draft.KindEnvironmentDevice, draft.KindEnvironment, draft.KindDevice,
	}
	upsertOrder = []draft.Kind{
		draft.KindDevice, draft.KindEnvironment, draft.KindEnvironmentDevice,
		draft.KindTag, draft.KindLibraryPin, draft.KindScene, draft.KindWidget,
	}
)

// applySnapshots writes the entities a change touched: those only in before
// go, those only in after come or change. The order is fixed by kind, not by
// the order the operation recorded them in.
func applySnapshots(ctx context.Context, tx *sql.Tx, scope editlock.Scope, before, after []draft.Snapshot) error {
	for _, kind := range deleteOrder {
		for _, snapshot := range before {
			if snapshot.Kind != kind || containsSnapshot(after, snapshot) {
				continue
			}
			if err := deleteSnapshot(ctx, tx, scope, snapshot); err != nil {
				return err
			}
		}
	}
	for _, kind := range upsertOrder {
		for _, snapshot := range after {
			if snapshot.Kind != kind {
				continue
			}
			if err := upsertSnapshot(ctx, tx, scope, snapshot); err != nil {
				return err
			}
		}
	}
	return nil
}

// containsSnapshot reports whether the same entity — the same kind and ID — is
// on the other side of the change: it was updated, not created or removed.
func containsSnapshot(list []draft.Snapshot, wanted draft.Snapshot) bool {
	for _, snapshot := range list {
		if snapshot.Kind == wanted.Kind && snapshot.ID == wanted.ID {
			return true
		}
	}
	return false
}

func deleteSnapshot(ctx context.Context, tx *sql.Tx, scope editlock.Scope, snapshot draft.Snapshot) error {
	projectID := scope.ProjectID().UUID()
	switch state := snapshot.State.(type) {
	case draft.TagState:
		_, err := tx.ExecContext(ctx,
			`DELETE FROM engineering.tags WHERE id = $1 AND project_id = $2`, snapshot.ID, projectID)
		return wrapWrite("delete tag", err)
	case draft.SceneState:
		_, err := tx.ExecContext(ctx,
			`DELETE FROM engineering.scenes WHERE id = $1 AND project_id = $2`, snapshot.ID, projectID)
		return wrapWrite("delete scene", err)
	case draft.WidgetState:
		_, err := tx.ExecContext(ctx,
			`DELETE FROM engineering.widgets WHERE id = $1 AND scene_id = $2`, snapshot.ID, state.SceneID)
		return wrapWrite("delete widget", err)
	case draft.DeviceState:
		_, err := tx.ExecContext(ctx,
			`DELETE FROM engineering.devices WHERE id = $1 AND project_id = $2`, snapshot.ID, projectID)
		return wrapWrite("delete device", err)
	case draft.EnvironmentState:
		_, err := tx.ExecContext(ctx,
			`DELETE FROM engineering.environments WHERE id = $1 AND project_id = $2`, snapshot.ID, projectID)
		return wrapWrite("delete environment", err)
	case draft.EnvironmentDeviceState:
		_, err := tx.ExecContext(ctx,
			`DELETE FROM engineering.environment_devices ed
			 USING engineering.environments e
			 WHERE e.id = ed.environment_id AND e.project_id = $1
			   AND ed.environment_id = $2 AND ed.device_id = $3`,
			projectID, state.EnvironmentID, state.DeviceID)
		return wrapWrite("delete environment device", err)
	case draft.LibraryPinState:
		_, err := tx.ExecContext(ctx,
			`DELETE FROM engineering.library_pins WHERE project_id = $1 AND library_id = $2`, projectID, state.LibraryID)
		return wrapWrite("delete library pin", err)
	default:
		return fmt.Errorf("cannot delete a draft entity of unknown state %T", snapshot.State)
	}
}

func upsertSnapshot(ctx context.Context, tx *sql.Tx, scope editlock.Scope, snapshot draft.Snapshot) error {
	projectID := scope.ProjectID().UUID()
	switch state := snapshot.State.(type) {
	case draft.TagState:
		_, err := tx.ExecContext(ctx,
			`INSERT INTO engineering.tags (id, project_id, name, type, device_id)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (id) DO UPDATE
			 SET name = EXCLUDED.name, type = EXCLUDED.type, device_id = EXCLUDED.device_id
			 WHERE engineering.tags.project_id = EXCLUDED.project_id`,
			snapshot.ID, projectID, state.Name, state.Type, state.Supplier)
		if isUniqueViolation(err, constraintTagsName) {
			return tag.ErrTagNameTaken
		}
		if isForeignKeyViolation(err, constraintTagsDeviceID) {
			return fmt.Errorf("%w: the supplier is not a device of the project", tag.ErrSupplierOfAnotherProject)
		}
		return wrapWrite("write tag", err)
	case draft.SceneState:
		_, err := tx.ExecContext(ctx,
			`INSERT INTO engineering.scenes (id, project_id, name, width, height, background_html)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (id) DO UPDATE
			 SET name = EXCLUDED.name, width = EXCLUDED.width, height = EXCLUDED.height,
			     background_html = EXCLUDED.background_html
			 WHERE engineering.scenes.project_id = EXCLUDED.project_id`,
			snapshot.ID, projectID, state.Name, state.Width, state.Height, state.BackgroundHTML)
		return wrapWrite("write scene", err)
	case draft.WidgetState:
		if err := checkPinned(ctx, tx, scope.ProjectID(), id.NewID(id.IDWithUUID[library.WidgetType](state.TypeID))); err != nil {
			return err
		}
		bindings, err := marshalSnapshotBindings(state.PortBindings)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO engineering.widgets
			    (id, name, x, y, z, width, height, origin_x, origin_y, rotation_degrees,
			     type_id, scene_id, labels, port_bindings, project_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14::jsonb, $15)
			 ON CONFLICT (id) DO UPDATE
			 SET name = EXCLUDED.name, x = EXCLUDED.x, y = EXCLUDED.y, z = EXCLUDED.z,
			     width = EXCLUDED.width, height = EXCLUDED.height,
			     origin_x = EXCLUDED.origin_x, origin_y = EXCLUDED.origin_y,
			     rotation_degrees = EXCLUDED.rotation_degrees, type_id = EXCLUDED.type_id,
			     scene_id = EXCLUDED.scene_id, labels = EXCLUDED.labels,
			     port_bindings = EXCLUDED.port_bindings
			 WHERE engineering.widgets.project_id = EXCLUDED.project_id`,
			snapshot.ID, state.Name, state.X, state.Y, state.Z, state.Width, state.Height,
			state.OriginX, state.OriginY, state.RotationDegrees, state.TypeID, state.SceneID,
			pq.Array(state.Labels), bindings, projectID)
		return wrapWrite("write widget", err)
	case draft.DeviceState:
		_, err := tx.ExecContext(ctx,
			`INSERT INTO engineering.devices (id, project_id, name, comment, protocol, config)
			 VALUES ($1, $2, $3, $4, $5, $6::jsonb)
			 ON CONFLICT (id) DO UPDATE
			 SET name = EXCLUDED.name, comment = EXCLUDED.comment,
			     protocol = EXCLUDED.protocol, config = EXCLUDED.config
			 WHERE engineering.devices.project_id = EXCLUDED.project_id`,
			snapshot.ID, projectID, state.Name, state.Comment, state.Protocol, string(state.Config))
		if isUniqueViolation(err, constraintDevicesName) {
			return device.ErrDeviceNameTaken
		}
		return wrapWrite("write device", err)
	case draft.EnvironmentState:
		_, err := tx.ExecContext(ctx,
			`INSERT INTO engineering.environments (id, project_id, name, comment)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (id) DO UPDATE
			 SET name = EXCLUDED.name, comment = EXCLUDED.comment
			 WHERE engineering.environments.project_id = EXCLUDED.project_id`,
			snapshot.ID, projectID, state.Name, state.Comment)
		if isUniqueViolation(err, constraintEnvironmentsName) {
			return device.ErrEnvironmentNameTaken
		}
		return wrapWrite("write environment", err)
	case draft.EnvironmentDeviceState:
		_, err := tx.ExecContext(ctx,
			`INSERT INTO engineering.environment_devices (environment_id, device_id, params)
			 VALUES ($1, $2, $3::jsonb)
			 ON CONFLICT (environment_id, device_id) DO UPDATE SET params = EXCLUDED.params`,
			state.EnvironmentID, state.DeviceID, string(state.Params))
		if isUniqueViolation(err, constraintEnvironmentDevicesPair) {
			return device.ErrEnvironmentDeviceExists
		}
		return wrapWrite("write environment device", err)
	case draft.LibraryPinState:
		_, err := tx.ExecContext(ctx,
			`INSERT INTO engineering.library_pins (project_id, library_id, release_number)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (project_id, library_id) DO UPDATE SET release_number = EXCLUDED.release_number`,
			projectID, state.LibraryID, state.Release)
		if isUniqueViolation(err, constraintLibraryPinsProjectLibrary) {
			return fmt.Errorf("%w: library %s", librarypin.ErrLibraryPinExists, state.LibraryID)
		}
		if isForeignKeyViolation(err, constraintLibraryPinsRelease) {
			return fmt.Errorf("%w: release %d of library %s", library.ErrReleaseNotFound, state.Release, state.LibraryID)
		}
		return wrapWrite("write library pin", err)
	default:
		return fmt.Errorf("cannot write a draft entity of unknown state %T", snapshot.State)
	}
}

func wrapWrite(what string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("cannot %s: %w", what, err)
}

func marshalSnapshotBindings(bindings []draft.PortBindingState) ([]byte, error) {
	type binding struct {
		PortName string `json:"port_name"`
		TagID    string `json:"tag_id"`
	}
	raw := make([]binding, len(bindings))
	for i, b := range bindings {
		raw[i] = binding{PortName: b.PortName, TagID: b.TagID.String()}
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("cannot serialize port bindings: %w", err)
	}
	return encoded, nil
}

// --- the loader over the Draft's transaction ---

// draftLoader reads, inside the write's transaction, only the entities an
// operation touches, always within the Draft's own Project (ADR 0016).
type draftLoader struct {
	tx       *sql.Tx
	project  id.ID[project.Project]
	releases library.LibraryReleaseRepository
}

var _ draft.Loader = (*draftLoader)(nil)

func (l *draftLoader) Tag(ctx context.Context, tagID id.ID[tag.Tag]) (tag.Tag, error) {
	var (
		rawID    uuid.UUID
		name     string
		tagType  string
		deviceID *uuid.UUID
	)
	err := l.tx.QueryRowContext(ctx,
		`SELECT id, name, type, device_id FROM engineering.tags WHERE id = $1 AND project_id = $2`,
		tagID.UUID(), l.project.UUID()).Scan(&rawID, &name, &tagType, &deviceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, id.NotFound(tag.ErrTagNotFound, tagID)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read tag %s: %w", tagID, err)
	}
	typeName, err := tag.NewTagType(tagType)
	if err != nil {
		return nil, err
	}
	tagName, err := tag.NewTagName(name)
	if err != nil {
		return nil, err
	}
	return tag.ReconstituteTag(id.NewID(id.IDWithUUID[tag.Tag](rawID)), l.project, tagName, typeName, supplierOf(deviceID))
}

func (l *draftLoader) TagTypes(ctx context.Context, tagIDs []id.ID[tag.Tag]) (scene.TagTypes, error) {
	types := make(map[id.ID[tag.Tag]]tag.TagType, len(tagIDs))
	if len(tagIDs) == 0 {
		return scene.NewTagTypes(l.project, types), nil
	}
	raw := make([]string, len(tagIDs))
	for i, tagID := range tagIDs {
		raw[i] = tagID.UUID().String()
	}
	rows, err := l.tx.QueryContext(ctx,
		`SELECT id, type FROM engineering.tags WHERE id = ANY($1::uuid[]) AND project_id = $2`,
		pq.Array(raw), l.project.UUID())
	if err != nil {
		return scene.TagTypes{}, fmt.Errorf("cannot read tag types: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			rawID   uuid.UUID
			rawType string
		)
		if err := rows.Scan(&rawID, &rawType); err != nil {
			return scene.TagTypes{}, err
		}
		tagType, err := tag.NewTagType(rawType)
		if err != nil {
			return scene.TagTypes{}, err
		}
		types[id.NewID(id.IDWithUUID[tag.Tag](rawID))] = tagType
	}
	return scene.NewTagTypes(l.project, types), rows.Err()
}

func (l *draftLoader) AllTagTypes(ctx context.Context) (scene.TagTypes, error) {
	rows, err := l.tx.QueryContext(ctx,
		`SELECT id, type FROM engineering.tags WHERE project_id = $1`, l.project.UUID())
	if err != nil {
		return scene.TagTypes{}, fmt.Errorf("cannot read tag types: %w", err)
	}
	defer rows.Close()
	types := map[id.ID[tag.Tag]]tag.TagType{}
	for rows.Next() {
		var (
			rawID   uuid.UUID
			rawType string
		)
		if err := rows.Scan(&rawID, &rawType); err != nil {
			return scene.TagTypes{}, err
		}
		tagType, err := tag.NewTagType(rawType)
		if err != nil {
			return scene.TagTypes{}, err
		}
		types[id.NewID(id.IDWithUUID[tag.Tag](rawID))] = tagType
	}
	return scene.NewTagTypes(l.project, types), rows.Err()
}

func (l *draftLoader) TagsOfDevice(ctx context.Context, deviceID id.ID[device.Device]) (bool, error) {
	var supplied bool
	err := l.tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM engineering.tags WHERE device_id = $1 AND project_id = $2)`,
		deviceID.UUID(), l.project.UUID()).Scan(&supplied)
	return supplied, err
}

func (l *draftLoader) AllTags(ctx context.Context) ([]tag.Tag, error) {
	rows, err := l.tx.QueryContext(ctx,
		`SELECT id, name, type, device_id FROM engineering.tags WHERE project_id = $1 ORDER BY name`, l.project.UUID())
	if err != nil {
		return nil, fmt.Errorf("cannot read tags: %w", err)
	}
	defer rows.Close()
	found := []tag.Tag{}
	for rows.Next() {
		var (
			rawID    uuid.UUID
			name     string
			tagType  string
			deviceID *uuid.UUID
		)
		if err := rows.Scan(&rawID, &name, &tagType, &deviceID); err != nil {
			return nil, err
		}
		typeName, err := tag.NewTagType(tagType)
		if err != nil {
			return nil, err
		}
		tagName, err := tag.NewTagName(name)
		if err != nil {
			return nil, err
		}
		reconstituted, err := tag.ReconstituteTag(id.NewID(id.IDWithUUID[tag.Tag](rawID)), l.project, tagName, typeName, supplierOf(deviceID))
		if err != nil {
			return nil, err
		}
		found = append(found, reconstituted)
	}
	return found, rows.Err()
}

func (l *draftLoader) Scenes(ctx context.Context) ([]scene.Scene, error) {
	return l.readScenes(ctx, ``)
}

func (l *draftLoader) Scene(ctx context.Context, sceneID id.ID[scene.Scene]) (scene.Scene, error) {
	found, err := l.readScenes(ctx, `AND id = $2`, sceneID.UUID())
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, id.NotFound(scene.ErrSceneNotFound, sceneID)
	}
	return found[0], nil
}

func (l *draftLoader) readScenes(ctx context.Context, and string, args ...any) ([]scene.Scene, error) {
	rows, err := l.tx.QueryContext(ctx,
		`SELECT `+selectSceneColumns+` FROM engineering.scenes WHERE project_id = $1 `+and+` ORDER BY pk_id`,
		append([]any{l.project.UUID()}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("cannot read scenes: %w", err)
	}
	defer rows.Close()
	var rawScenes []rawSceneRow
	for rows.Next() {
		var row rawSceneRow
		if err := rows.Scan(row.dest()...); err != nil {
			return nil, err
		}
		rawScenes = append(rawScenes, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(rawScenes) == 0 {
		return []scene.Scene{}, nil
	}
	sceneIDs := make([]uuid.UUID, len(rawScenes))
	for i, row := range rawScenes {
		sceneIDs[i] = row.id
	}
	widgets, err := findWidgetsBySceneIDs(ctx, l.tx, sceneIDs)
	if err != nil {
		return nil, err
	}
	result := make([]scene.Scene, 0, len(rawScenes))
	for _, row := range rawScenes {
		reconstructed, err := reconstructScene(l.project, row, widgets[row.id])
		if err != nil {
			return nil, err
		}
		result = append(result, reconstructed)
	}
	return result, nil
}

func (l *draftLoader) PinnedWidgetType(ctx context.Context, typeID id.ID[library.WidgetType]) (library.ReleasedWidgetType, error) {
	wt, err := scanReleasedWidgetType(l.tx.QueryRowContext(ctx, pinnedWidgetTypesQuery+` AND r.id = $2`, l.project.UUID(), typeID.UUID()).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return library.ReleasedWidgetType{}, id.NotFound(library.ErrWidgetTypeNotFound, typeID)
	}
	if err != nil {
		return library.ReleasedWidgetType{}, fmt.Errorf("cannot read the pinned widget type %s: %w", typeID, err)
	}
	return wt, nil
}

func (l *draftLoader) Release(ctx context.Context, libraryID id.ID[library.Library], number library.ReleaseNumber) (library.LibraryRelease, error) {
	return l.releases.FindByNumber(ctx, libraryID, number)
}

func (l *draftLoader) AllDevices(ctx context.Context) ([]device.Device, error) {
	rows, err := l.tx.QueryContext(ctx,
		`SELECT id, name, comment, protocol, config FROM engineering.devices WHERE project_id = $1 ORDER BY pk_id`,
		l.project.UUID())
	if err != nil {
		return nil, fmt.Errorf("cannot read devices: %w", err)
	}
	defer rows.Close()
	found := []device.Device{}
	for rows.Next() {
		d, err := scanDeviceRow(l.project, rows.Scan)
		if err != nil {
			return nil, err
		}
		found = append(found, d)
	}
	return found, rows.Err()
}

func (l *draftLoader) Device(ctx context.Context, deviceID id.ID[device.Device]) (device.Device, error) {
	d, err := scanDeviceRow(l.project, l.tx.QueryRowContext(ctx,
		`SELECT id, name, comment, protocol, config FROM engineering.devices WHERE id = $1 AND project_id = $2`,
		deviceID.UUID(), l.project.UUID()).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, id.NotFound(device.ErrDeviceNotFound, deviceID)
	}
	return d, err
}

func (l *draftLoader) AllEnvironments(ctx context.Context) ([]device.Environment, error) {
	rows, err := l.tx.QueryContext(ctx,
		`SELECT id, name, comment FROM engineering.environments WHERE project_id = $1 ORDER BY pk_id`,
		l.project.UUID())
	if err != nil {
		return nil, fmt.Errorf("cannot read environments: %w", err)
	}
	defer rows.Close()
	found := []device.Environment{}
	for rows.Next() {
		e, err := scanEnvironmentRow(l.project, rows.Scan)
		if err != nil {
			return nil, err
		}
		found = append(found, e)
	}
	return found, rows.Err()
}

func (l *draftLoader) Environment(ctx context.Context, environmentID id.ID[device.Environment]) (device.Environment, error) {
	e, err := scanEnvironmentRow(l.project, l.tx.QueryRowContext(ctx,
		`SELECT id, name, comment FROM engineering.environments WHERE id = $1 AND project_id = $2`,
		environmentID.UUID(), l.project.UUID()).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, id.NotFound(device.ErrEnvironmentNotFound, environmentID)
	}
	return e, err
}

func (l *draftLoader) EnvironmentDevices(ctx context.Context) ([]device.EnvironmentDevice, error) {
	return l.readPairs(ctx, ``, l.project.UUID())
}

func (l *draftLoader) Pair(ctx context.Context, environmentID id.ID[device.Environment], deviceID id.ID[device.Device]) (device.EnvironmentDevice, error) {
	found, err := l.readPairs(ctx, ` AND ed.environment_id = $2 AND ed.device_id = $3`, l.project.UUID(), environmentID.UUID(), deviceID.UUID())
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, device.ErrEnvironmentDeviceNotFound
	}
	return found[0], nil
}

func (l *draftLoader) PairsOfEnvironment(ctx context.Context, environmentID id.ID[device.Environment]) ([]device.EnvironmentDevice, error) {
	return l.readPairs(ctx, ` AND ed.environment_id = $2`, l.project.UUID(), environmentID.UUID())
}

func (l *draftLoader) PairsOfDevice(ctx context.Context, deviceID id.ID[device.Device]) ([]device.EnvironmentDevice, error) {
	return l.readPairs(ctx, ` AND ed.device_id = $2`, l.project.UUID(), deviceID.UUID())
}

func (l *draftLoader) readPairs(ctx context.Context, and string, args ...any) ([]device.EnvironmentDevice, error) {
	rows, err := l.tx.QueryContext(ctx,
		`SELECT ed.environment_id, ed.device_id, ed.params
		 FROM engineering.environment_devices ed
		 JOIN engineering.environments e ON e.id = ed.environment_id
		 WHERE e.project_id = $1`+and+` ORDER BY ed.pk_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read environment devices: %w", err)
	}
	defer rows.Close()
	found := []device.EnvironmentDevice{}
	for rows.Next() {
		p, err := scanPairRow(l.project, rows.Scan)
		if err != nil {
			return nil, err
		}
		found = append(found, p)
	}
	return found, rows.Err()
}

func (l *draftLoader) AllLibraryPins(ctx context.Context) ([]librarypin.LibraryPin, error) {
	rows, err := l.tx.QueryContext(ctx,
		`SELECT library_id, release_number FROM engineering.library_pins WHERE project_id = $1 ORDER BY pk_id`,
		l.project.UUID())
	if err != nil {
		return nil, fmt.Errorf("cannot read library pins: %w", err)
	}
	defer rows.Close()
	found := []librarypin.LibraryPin{}
	for rows.Next() {
		var (
			rawLibraryID uuid.UUID
			rawRelease   int
		)
		if err := rows.Scan(&rawLibraryID, &rawRelease); err != nil {
			return nil, err
		}
		release, err := library.NewReleaseNumber(rawRelease)
		if err != nil {
			return nil, err
		}
		found = append(found, librarypin.ReconstituteLibraryPin(l.project, id.NewID(id.IDWithUUID[library.Library](rawLibraryID)), release))
	}
	return found, rows.Err()
}

func (l *draftLoader) LibraryPin(ctx context.Context, libraryID id.ID[library.Library]) (librarypin.LibraryPin, error) {
	var (
		rawLibraryID uuid.UUID
		rawRelease   int
	)
	err := l.tx.QueryRowContext(ctx,
		`SELECT library_id, release_number FROM engineering.library_pins WHERE project_id = $1 AND library_id = $2`,
		l.project.UUID(), libraryID.UUID()).Scan(&rawLibraryID, &rawRelease)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: library %s", librarypin.ErrLibraryPinNotFound, libraryID)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read library pin %s: %w", libraryID, err)
	}
	release, err := library.NewReleaseNumber(rawRelease)
	if err != nil {
		return nil, err
	}
	return librarypin.ReconstituteLibraryPin(l.project, id.NewID(id.IDWithUUID[library.Library](rawLibraryID)), release), nil
}

func scanDeviceRow(projectID id.ID[project.Project], scan func(...any) error) (device.Device, error) {
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
		return nil, err
	}
	protocol, err := device.NewDeviceProtocol(rawProtocol)
	if err != nil {
		return nil, err
	}
	config, err := device.NewDeviceConfig(rawConfig)
	if err != nil {
		return nil, err
	}
	return device.ReconstituteDevice(id.NewID(id.IDWithUUID[device.Device](rawID)), projectID, name, rawComment, protocol, config)
}

func scanEnvironmentRow(projectID id.ID[project.Project], scan func(...any) error) (device.Environment, error) {
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
		return nil, err
	}
	return device.ReconstituteEnvironment(id.NewID(id.IDWithUUID[device.Environment](rawID)), projectID, name, rawComment)
}

func scanPairRow(projectID id.ID[project.Project], scan func(...any) error) (device.EnvironmentDevice, error) {
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
		return nil, err
	}
	return device.ReconstituteEnvironmentDevice(projectID,
		id.NewID(id.IDWithUUID[device.Environment](rawEnvironmentID)),
		id.NewID(id.IDWithUUID[device.Device](rawDeviceID)), params), nil
}
