package restapi

import (
	"time"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/tagstate"
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
	event.EventTypeProjectCreated:     apiv0.EventProjectCreated,
	event.EventTypeProjectUpdated:     apiv0.EventProjectUpdated,
	event.EventTypeProjectDeleted:     apiv0.EventProjectDeleted,

	event.EventTypeLibraryCreated:    apiv0.EventLibraryCreated,
	event.EventTypeLibraryUpdated:    apiv0.EventLibraryUpdated,
	event.EventTypeLibraryDeleted:    apiv0.EventLibraryDeleted,
	event.EventTypeLibraryReleased:   apiv0.EventLibraryReleased,
	event.EventTypeLibraryPinCreated: apiv0.EventLibraryPinCreated,
	event.EventTypeLibraryPinUpdated: apiv0.EventLibraryPinUpdated,
	event.EventTypeLibraryPinDeleted: apiv0.EventLibraryPinDeleted,

	event.EventTypeDeviceCreated:            apiv0.EventDeviceCreated,
	event.EventTypeDeviceUpdated:            apiv0.EventDeviceUpdated,
	event.EventTypeDeviceDeleted:            apiv0.EventDeviceDeleted,
	event.EventTypeEnvironmentCreated:       apiv0.EventEnvironmentCreated,
	event.EventTypeEnvironmentUpdated:       apiv0.EventEnvironmentUpdated,
	event.EventTypeEnvironmentDeleted:       apiv0.EventEnvironmentDeleted,
	event.EventTypeEnvironmentDeviceCreated: apiv0.EventEnvironmentDeviceCreated,
	event.EventTypeEnvironmentDeviceUpdated: apiv0.EventEnvironmentDeviceUpdated,
	event.EventTypeEnvironmentDeviceDeleted: apiv0.EventEnvironmentDeviceDeleted,
	event.EventTypeDeploymentCreated:        apiv0.EventDeploymentCreated,
	event.EventTypeCommitCreated:            apiv0.EventCommitCreated,
	event.EventTypeCommitTitleUpdated:       apiv0.EventCommitTitleUpdated,
}

// newAPIEvent translates a domain (or delivery) event into the event stream's
// message: its type through apiEventTypes, and the fields its family carries
// — the affected aggregate's (or connection's) ID, the Scene of a Widget, the
// Library of a WidgetType, the number of a new release, the pair of an
// EnvironmentDevice, the Tag's new state of tag_updated, the number and time
// of a new Deployment. An event type missing from the table gives
// an empty Type; such an event is not sent.
func newAPIEvent(e event.Event) apiv0.Event {
	ev := apiv0.Event{
		Type:      apiEventTypes[e.Type()],
		Timestamp: e.Timestamp().Time().Format(time.RFC3339),
	}
	switch e := e.(type) {
	case draft.CommitEvent:
		if e.Scope().IsLibrary() {
			ev.ID = e.Scope().LibraryID().String()
		} else {
			ev.ID = e.Scope().ProjectID().String()
		}
		ev.Commit = e.CommitNumber()
	case tag.TagEvent:
		ev.ID = e.TagID().String()
		if updated, ok := e.(tagstate.TagUpdatedEvent); ok {
			t := newTagStateResponse(runtimeapp.NewTagState(updated.State()))
			ev.Tag = &t
		}
	case scene.WidgetEvent:
		ev.ID = e.WidgetID().String()
		ev.SceneID = e.SceneID().String()
	case library.WidgetTypeEvent:
		ev.ID = e.WidgetTypeID().String()
		ev.LibraryID = e.LibraryID().String()
	case library.LibraryEvent:
		ev.ID = e.LibraryID().String()
		if released, ok := e.(library.LibraryReleasedEvent); ok {
			ev.Release = released.Release().Int()
		}
	case librarypin.LibraryPinEvent:
		ev.ID = e.LibraryID().String()
	case device.EnvironmentDeviceEvent:
		ev.ID = e.EnvironmentID().String()
		ev.EnvironmentID = e.EnvironmentID().String()
		ev.DeviceID = e.DeviceID().String()
	case device.DeviceEvent:
		ev.ID = e.DeviceID().String()
	case device.EnvironmentEvent:
		ev.ID = e.EnvironmentID().String()
	case runtimeapp.DeploymentCreatedEvent:
		ev.ID = e.ProjectID().String()
		ev.Deployment = e.Number()
		ev.DeployedAt = deployedAt(e.DeployedAt())
	case scene.SceneEvent:
		ev.ID = e.SceneID().String()
	case project.ProjectEvent:
		ev.ID = e.ProjectID().String()
	case eventbus.ClientEvent:
		ev.ID = e.ClientID().String()
	}
	return ev
}
