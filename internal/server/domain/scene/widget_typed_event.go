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
	// WidgetTypeID is the WidgetType of the widget. It is unknown (false) only
	// for an event read back from an outbox row written before events carried
	// it.
	WidgetTypeID() (id.ID[library.WidgetType], bool)
}

type widgetTypedEventImpl struct {
	widgetEventImpl
	widgetTypeID *id.ID[library.WidgetType]
}

var _ WidgetTypedEvent = (*widgetTypedEventImpl)(nil)

// NO FABRIC METHOD

func (e widgetTypedEventImpl) WidgetTypeID() (id.ID[library.WidgetType], bool) {
	if e.widgetTypeID == nil {
		return id.ID[library.WidgetType]{}, false
	}
	return *e.widgetTypeID, true
}
