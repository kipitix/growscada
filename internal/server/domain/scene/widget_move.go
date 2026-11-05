package scene

import "github.com/kipitix/growscada/internal/server/domain/id"

// WidgetMove is the new place of one Widget of a Scene: the entity's ID and
// the x and y of the Position it takes. A move changes the geometry alone
// (ADR 0010): the layer (z), the size, the origin, the rotation, the
// WidgetType, the labels and the PortBindings stay as they are stored.
// Value Object.
type WidgetMove struct {
	widgetID id.ID[Widget]
	x, y     float64
}

// NewWidgetMove creates a WidgetMove of the widget anID to (x, y) on the
// canvas.
func NewWidgetMove(anID id.ID[Widget], x, y float64) WidgetMove {
	return WidgetMove{widgetID: anID, x: x, y: y}
}

// WidgetID is the ID of the widget to move.
func (m WidgetMove) WidgetID() id.ID[Widget] { return m.widgetID }

// X is the new horizontal coordinate in canvas pixels.
func (m WidgetMove) X() float64 { return m.x }

// Y is the new vertical coordinate in canvas pixels.
func (m WidgetMove) Y() float64 { return m.y }
