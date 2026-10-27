package library

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// WidgetTypeEvent - base interface for all events a WidgetType records. It
// names the Library whose Draft the WidgetType belongs to.
type WidgetTypeEvent interface {
	event.Event
	WidgetTypeID() id.ID[WidgetType]
	LibraryID() id.ID[Library]
}

type widgetTypeEventImpl struct {
	event.Base
	widgetTypeID id.ID[WidgetType]
	libraryID    id.ID[Library]
}

var _ WidgetTypeEvent = (*widgetTypeEventImpl)(nil)

// NO FABRIC METHOD

func (e widgetTypeEventImpl) WidgetTypeID() id.ID[WidgetType] {
	return e.widgetTypeID
}

func (e widgetTypeEventImpl) LibraryID() id.ID[Library] {
	return e.libraryID
}
