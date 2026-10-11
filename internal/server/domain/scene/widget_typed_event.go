package scene

import (
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
)

// WidgetTypedEvent is a WidgetEvent that names the WidgetType of the widget:
// WidgetCreatedEvent and WidgetUpdatedEvent. A policy bringing Widgets into
// line with their WidgetType keys its job by it (ADR 0008).
type WidgetTypedEvent interface {
	WidgetEvent
	// TypeID is the WidgetType of the widget, as Widget.TypeID. It is not
	// named WidgetTypeID: that would make the event a library.WidgetTypeEvent
	// too, and a type switch would take it for an event of the WidgetType.
	TypeID() id.ID[library.WidgetType]
}

type widgetTypedEventImpl struct {
	widgetEventImpl
	widgetTypeID id.ID[library.WidgetType]
}

var _ WidgetTypedEvent = (*widgetTypedEventImpl)(nil)

// NO FABRIC METHOD

func (e widgetTypedEventImpl) TypeID() id.ID[library.WidgetType] {
	return e.widgetTypeID
}
