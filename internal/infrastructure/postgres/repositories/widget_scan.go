package repositories

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/tag"
	"github.com/kipitix/growscada/internal/domain/widget"
)

// selectWidgetColumns lists the widgets columns read by every SELECT/RETURNING
// in this package. scene_id is included so callers scoping a query by scene
// can still verify/route rows without a second lookup.
const selectWidgetColumns = `id, name, x, y, z, width, height, origin_x, origin_y, rotation_degrees, type_id, scene_id, labels, port_bindings`

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
}

// scanWidget reads a single widget row using the provided scan function.
// The column order must match selectWidgetColumns. It returns the domain
// widget alongside the raw scene_id column for callers that need it.
func scanWidget(scan func(...any) error) (widget.Widget, uuid.UUID, error) {
	var row rawWidgetRow
	if err := scan(
		&row.id, &row.name,
		&row.x, &row.y, &row.z,
		&row.width, &row.height,
		&row.originX, &row.originY,
		&row.rotDegrees,
		&row.typeID, &row.sceneID,
		&row.labels, &row.portBindingsJSON,
	); err != nil {
		return nil, uuid.Nil, err
	}
	w, err := reconstructWidget(row)
	return w, row.sceneID, err
}

func reconstructWidget(row rawWidgetRow) (widget.Widget, error) {
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

	portBindings, err := unmarshalPortBindings(row.portBindingsJSON)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal port bindings: %w", err)
	}

	return widget.NewWidget(newID, newName, pos, size, origin, rotation, typeID, row.labels, portBindings), nil
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
		portName, err := widget.NewInputPortNameFromStorage(row.PortName)
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
