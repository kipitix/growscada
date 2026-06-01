package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/scene"
	"github.com/kipitix/growscada/internal/domain/tag"
	"github.com/kipitix/growscada/internal/domain/version"
	"github.com/kipitix/growscada/internal/domain/widget"
)

type widgetRepositoryPostgresImpl struct {
	db *sql.DB
}

var _ widget.WidgetRepository = (*widgetRepositoryPostgresImpl)(nil)

func NewWidgetRepositoryPostgres(aDb *sql.DB) widget.WidgetRepository {
	return &widgetRepositoryPostgresImpl{db: aDb}
}

func (r widgetRepositoryPostgresImpl) NextID() id.ID[widget.Widget] {
	return id.NewID[widget.Widget]()
}

func (r widgetRepositoryPostgresImpl) Save(ctx context.Context, w widget.Widget) (widget.Widget, error) {
	portBindingsJSON, err := marshalPortBindings(w.PortBindings())
	if err != nil {
		return nil, fmt.Errorf("cannot serialize port bindings: %w", err)
	}

	if w.Version() == version.Initial[widget.Widget]() {
		sqlResult, err := r.db.ExecContext(ctx,
			`INSERT INTO widgets
			    (id, name, x, y, z, width, height, origin_x, origin_y, rotation_degrees,
			     type_id, scene_id, labels, port_bindings, version)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
			w.ID().UUID(), w.Name().String(),
			w.Position().X(), w.Position().Y(), w.Position().Z(),
			w.Size().Width(), w.Size().Height(),
			w.Origin().X(), w.Origin().Y(),
			w.Rotation().Degrees(),
			w.TypeID().UUID(), w.SceneID().UUID(),
			pq.Array(w.Labels()), portBindingsJSON,
			version.Committed[widget.Widget]().Number(),
		)
		if err != nil {
			return nil, fmt.Errorf("cannot insert new widget: %w", err)
		}
		rowsAffected, err := sqlResult.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("cannot get rows affected on insert: %w", err)
		}
		if rowsAffected != 1 {
			return nil, fmt.Errorf("expected 1 row affected on insert, got %d", rowsAffected)
		}
		return widget.NewWidget(
			w.ID(), w.Name(), w.Position(), w.Size(), w.Origin(), w.Rotation(),
			w.TypeID(), w.SceneID(), w.Labels(), w.PortBindings(),
			version.Committed[widget.Widget](),
		), nil
	}

	if w.Version().IsCommitted() {
		sqlResult, err := r.db.ExecContext(ctx,
			`UPDATE widgets
			 SET name = $1, x = $2, y = $3, z = $4, width = $5, height = $6,
			     origin_x = $7, origin_y = $8, rotation_degrees = $9,
			     type_id = $10, scene_id = $11, labels = $12, port_bindings = $13,
			     version = version + 1
			 WHERE id = $14 AND version = $15`,
			w.Name().String(),
			w.Position().X(), w.Position().Y(), w.Position().Z(),
			w.Size().Width(), w.Size().Height(),
			w.Origin().X(), w.Origin().Y(),
			w.Rotation().Degrees(),
			w.TypeID().UUID(), w.SceneID().UUID(),
			pq.Array(w.Labels()), portBindingsJSON,
			w.ID().UUID(), w.Version().Number(),
		)
		if err != nil {
			return nil, fmt.Errorf("cannot update widget: %w", err)
		}
		rowsAffected, err := sqlResult.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("cannot get rows affected on update: %w", err)
		}
		if rowsAffected != 1 {
			return nil, classifyUpdateConflict(ctx, r.db, tableNameWidgets, w.ID(), widget.ErrWidgetNotFound, widget.ErrWidgetConflict)
		}
		return widget.NewWidget(
			w.ID(), w.Name(), w.Position(), w.Size(), w.Origin(), w.Rotation(),
			w.TypeID(), w.SceneID(), w.Labels(), w.PortBindings(),
			w.Version().Next(),
		), nil
	}

	return nil, fmt.Errorf("undefined behavior with version %d", w.Version().Number())
}

const selectWidgetColumns = `id, name, x, y, z, width, height, origin_x, origin_y, rotation_degrees, type_id, scene_id, labels, port_bindings, version`

func (r widgetRepositoryPostgresImpl) FindByID(ctx context.Context, widgetID id.ID[widget.Widget]) (widget.Widget, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT "+selectWidgetColumns+" FROM widgets WHERE id = $1",
		widgetID.UUID(),
	)
	w, err := r.scanWidget(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, widget.ErrWidgetNotFound
		}
		return nil, fmt.Errorf("error scanning widget: %w", err)
	}
	return w, nil
}

func (r widgetRepositoryPostgresImpl) DeleteByID(ctx context.Context, widgetID id.ID[widget.Widget]) (widget.Widget, error) {
	row := r.db.QueryRowContext(ctx,
		"DELETE FROM widgets WHERE id = $1 RETURNING "+selectWidgetColumns,
		widgetID.UUID(),
	)
	w, err := r.scanWidget(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, widget.ErrWidgetNotFound
		}
		return nil, fmt.Errorf("cannot delete widget: %w", err)
	}
	return w, nil
}

func (r widgetRepositoryPostgresImpl) FindAll(ctx context.Context) ([]widget.Widget, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+selectWidgetColumns+" FROM widgets",
	)
	if err != nil {
		return nil, fmt.Errorf("error querying widgets: %w", err)
	}
	defer rows.Close()

	var result []widget.Widget
	for rows.Next() {
		w, err := r.scanWidget(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("error scanning widget: %w", err)
		}
		result = append(result, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over widgets: %w", err)
	}
	return result, nil
}

func (r widgetRepositoryPostgresImpl) FindBySceneID(ctx context.Context, sceneID id.ID[scene.Scene]) ([]widget.Widget, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+selectWidgetColumns+" FROM widgets WHERE scene_id = $1",
		sceneID.UUID(),
	)
	if err != nil {
		return nil, fmt.Errorf("error querying widgets by scene id: %w", err)
	}
	defer rows.Close()

	var result []widget.Widget
	for rows.Next() {
		w, err := r.scanWidget(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("error scanning widget: %w", err)
		}
		result = append(result, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over widgets: %w", err)
	}
	return result, nil
}

// rawWidgetRow holds the raw column values read from a widgets row before domain validation.
// The field order matches selectWidgetColumns.
type rawWidgetRow struct {
	id               uuid.UUID
	name             string
	x, y             float64
	z                int
	width            int
	height           int
	originX          float64
	originY          float64
	rotDegrees       float64
	typeID           uuid.UUID
	sceneID          uuid.UUID
	labels           pq.StringArray
	portBindingsJSON []byte
	version          int
}

// scanWidget reads a single widget row using the provided scan function.
// The column order must match selectWidgetColumns.
func (r widgetRepositoryPostgresImpl) scanWidget(
	scan func(...any) error,
) (widget.Widget, error) {
	var row rawWidgetRow
	if err := scan(
		&row.id, &row.name,
		&row.x, &row.y, &row.z,
		&row.width, &row.height,
		&row.originX, &row.originY,
		&row.rotDegrees,
		&row.typeID, &row.sceneID,
		&row.labels, &row.portBindingsJSON,
		&row.version,
	); err != nil {
		return nil, err
	}
	return r.reconstruct(row)
}

func (r widgetRepositoryPostgresImpl) reconstruct(row rawWidgetRow) (widget.Widget, error) {
	newID := id.NewID(id.IDWithUUID[widget.Widget](row.id))

	newName, err := widget.NewWidgetName(row.name)
	if err != nil {
		return nil, fmt.Errorf("cannot create widget name: %w", err)
	}

	pos := widget.NewPosition(row.x, row.y, row.z)

	size, err := widget.NewSize(row.width, row.height)
	if err != nil {
		return nil, fmt.Errorf("cannot create widget size: %w", err)
	}

	origin, err := widget.NewOrigin(row.originX, row.originY)
	if err != nil {
		return nil, fmt.Errorf("cannot create widget origin: %w", err)
	}

	rotation := widget.NewRotation(row.rotDegrees)

	typeID := id.NewID(id.IDWithUUID[widget.WidgetType](row.typeID))
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](row.sceneID))

	portBindings, err := unmarshalPortBindings(row.portBindingsJSON)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal port bindings: %w", err)
	}

	newVersion, err := version.New(version.WithNumber[widget.Widget](row.version))
	if err != nil {
		return nil, fmt.Errorf("cannot create widget version: %w", err)
	}

	return widget.NewWidget(newID, newName, pos, size, origin, rotation, typeID, sceneID, row.labels, portBindings, newVersion), nil
}

// portBindingJSON is the on-disk representation of a PortBinding.
type portBindingJSON struct {
	PortName string `json:"port_name"`
	TagID    string `json:"tag_id"`
}

func marshalPortBindings(bindings []widget.PortBinding) ([]byte, error) {
	rows := make([]portBindingJSON, len(bindings))
	for i, b := range bindings {
		rows[i] = portBindingJSON{
			PortName: b.PortName().String(),
			TagID:    b.TagID().UUID().String(),
		}
	}
	return json.Marshal(rows)
}

func unmarshalPortBindings(data []byte) ([]widget.PortBinding, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var rows []portBindingJSON
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("cannot decode port_bindings JSON: %w", err)
	}
	bindings := make([]widget.PortBinding, 0, len(rows))
	for _, row := range rows {
		portName, err := widget.NewInputPortName(row.PortName)
		if err != nil {
			return nil, fmt.Errorf("invalid stored port name %q: %w", row.PortName, err)
		}
		tagUUID, err := uuid.Parse(row.TagID)
		if err != nil {
			return nil, fmt.Errorf("invalid stored tag id %q: %w", row.TagID, err)
		}
		tagID := id.NewID(id.IDWithUUID[tag.Tag](tagUUID))
		bindings = append(bindings, widget.NewPortBinding(portName, tagID))
	}
	return bindings, nil
}
