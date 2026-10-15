package restapi

import (
	"time"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
)

// apiEventTypes translates each domain EventType into the name the event
// stream publishes (ADR 0009). The names repeat the domain's on purpose: the
// domain's are the server's internal model (and what the outbox stores), these
// are the published API; a rename on one side must not change the other.
var apiEventTypes = map[event.EventType]string{
	event.EventTypeSystemReady:        apiv0.EventSystemReady,
	event.EventTypeTagCreated:         apiv0.EventTagCreated,
	event.EventTypeTagUpdated:         apiv0.EventTagUpdated,
	event.EventTypeTagDeleted:         apiv0.EventTagDeleted,
	event.EventTypeWidgetTypeCreated:  apiv0.EventWidgetTypeCreated,
	event.EventTypeWidgetTypeUpdated:  apiv0.EventWidgetTypeUpdated,
	event.EventTypeWidgetTypeDeleted:  apiv0.EventWidgetTypeDeleted,
	event.EventTypeWidgetCreated:      apiv0.EventWidgetCreated,
	event.EventTypeWidgetUpdated:      apiv0.EventWidgetUpdated,
	event.EventTypeWidgetDeleted:      apiv0.EventWidgetDeleted,
	event.EventTypeSceneCreated:       apiv0.EventSceneCreated,
	event.EventTypeSceneUpdated:       apiv0.EventSceneUpdated,
	event.EventTypeSceneDeleted:       apiv0.EventSceneDeleted,
	event.EventTypeClientConnected:    apiv0.EventClientConnected,
	event.EventTypeClientDisconnected: apiv0.EventClientDisconnected,
}

// newAPIEvent translates a domain (or delivery) event into the event stream's
// message: its type through apiEventTypes, and the fields its family carries
// — the affected aggregate's (or connection's) ID, the Scene of a Widget, the
// Tag's new state of tag_updated. An event type missing from the table gives
// an empty Type; such an event is not sent.
func newAPIEvent(e event.Event) apiv0.Event {
	ev := apiv0.Event{
		Type:      apiEventTypes[e.Type()],
		Timestamp: e.Timestamp().Time().Format(time.RFC3339),
	}
	switch e := e.(type) {
	case tag.TagEvent:
		ev.ID = e.TagID().String()
		if updated, ok := e.(tag.TagUpdatedEvent); ok {
			t := newTagResponse(appdto.NewTag(updated.Tag()))
			ev.Tag = &t
		}
	case scene.WidgetEvent:
		ev.ID = e.WidgetID().String()
		ev.SceneID = e.SceneID().String()
	case library.WidgetTypeEvent:
		ev.ID = e.WidgetTypeID().String()
	case scene.SceneEvent:
		ev.ID = e.SceneID().String()
	case eventbus.ClientEvent:
		ev.ID = e.ClientID().String()
	}
	return ev
}
