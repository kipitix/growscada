// Package outbox stores the domain events of committed changes and delivers
// them after commit (ADR 0008). A repository appends an aggregate's pending
// events with Append in the transaction that saves the aggregate; the
// Dispatcher reads them back in the order of their transactions, hands each
// to its sink and deletes it, enqueuing a job for each policy that needs the
// event; the PolicyRunner runs those jobs apart.
package outbox

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// payload is the stored form of an event; the event type is stored beside it.
// It is internal to the server, not a contract (ADR 0005): a row lives
// seconds, and a change of this format must keep decoding the previous one
// until the outbox has drained.
type payload struct {
	Timestamp time.Time `json:"timestamp"`
	// ID is the ID of the aggregate (or Widget) the event is about.
	ID uuid.UUID `json:"id"`
	// ProjectID is the Project whose content (Tag, Scene, Widget) the event
	// is about (ADR 0006).
	ProjectID *uuid.UUID `json:"project_id,omitempty"`
	// SceneID is the Scene holding the Widget of a Widget event.
	SceneID *uuid.UUID `json:"scene_id,omitempty"`
	// WidgetTypeID is the WidgetType of the Widget of a widget_created or
	// widget_updated event; rows written before it was stored lack it
	// (untypedWidgetEvent).
	WidgetTypeID *uuid.UUID `json:"widget_type_id,omitempty"`
	// ByReconciliation marks a widget_updated event recorded by
	// Scene.ReconcileWith.
	ByReconciliation bool `json:"by_reconciliation,omitempty"`
	// LibraryID is the Library of a WidgetType's event, and the Library of a
	// Library Pin's (whose ID is the Library's too).
	LibraryID *uuid.UUID `json:"library_id,omitempty"`
	// Release is the number of the Library Release of library_released.
	Release int `json:"release,omitempty"`
	// EnvironmentID and DeviceID are the pair of an environment_device_*
	// event, whose ID is its Environment's.
	EnvironmentID *uuid.UUID `json:"environment_id,omitempty"`
	DeviceID      *uuid.UUID `json:"device_id,omitempty"`
	// EditLock is the state an edit_lock_* event announces (ADR 0015): its
	// holder (absent while free), the times, and the version. PreviousKey and
	// PreviousName are the holder an edit_lock_taken_over displaced.
	EditLock     *editLockPayload `json:"edit_lock,omitempty"`
	PreviousKey  *uuid.UUID       `json:"previous_key,omitempty"`
	PreviousName string           `json:"previous_name,omitempty"`
}

// editLockPayload is the state of an EditLock an event carries.
type editLockPayload struct {
	Held     bool       `json:"held"`
	Key      *uuid.UUID `json:"key,omitempty"`
	Name     string     `json:"name,omitempty"`
	Since    *time.Time `json:"since,omitempty"`
	LastSeen *time.Time `json:"last_seen,omitempty"`
	Version  int        `json:"version"`
}

// Encode returns the stored form of a domain event. Events of delivery
// (connection lifecycle, system) are never stored and fail to encode, nor is
// tag_updated: the Runtime node publishes it itself (ADR 0011).
func Encode(e event.Event) ([]byte, error) {
	if e.Type() == event.EventTypeTagUpdated {
		return nil, fmt.Errorf("cannot encode event %s: not stored in the outbox", e.Type())
	}
	p := payload{Timestamp: e.Timestamp().Time()}
	if content, ok := e.(project.ContentEvent); ok {
		projectID := content.ProjectID().UUID()
		p.ProjectID = &projectID
	}
	switch ev := e.(type) {
	case tag.TagEvent:
		p.ID = ev.TagID().UUID()
	case library.WidgetTypeEvent:
		p.ID = ev.WidgetTypeID().UUID()
		libraryID := ev.LibraryID().UUID()
		p.LibraryID = &libraryID
	case library.LibraryReleasedEvent:
		p.ID = ev.LibraryID().UUID()
		p.Release = ev.Release().Int()
	case library.LibraryEvent:
		p.ID = ev.LibraryID().UUID()
	case librarypin.LibraryPinEvent:
		p.ID = ev.LibraryID().UUID()
	case device.EnvironmentDeviceEvent:
		p.ID = ev.EnvironmentID().UUID()
		environmentID := ev.EnvironmentID().UUID()
		deviceID := ev.DeviceID().UUID()
		p.EnvironmentID = &environmentID
		p.DeviceID = &deviceID
	case device.DeviceEvent:
		p.ID = ev.DeviceID().UUID()
	case device.EnvironmentEvent:
		p.ID = ev.EnvironmentID().UUID()
	case scene.WidgetEvent:
		p.ID = ev.WidgetID().UUID()
		sceneID := ev.SceneID().UUID()
		p.SceneID = &sceneID
		if typed, ok := ev.(scene.WidgetTypedEvent); ok {
			rawTypeID := typed.TypeID().UUID()
			p.WidgetTypeID = &rawTypeID
		}
		if updated, ok := ev.(scene.WidgetUpdatedEvent); ok {
			p.ByReconciliation = updated.ByReconciliation()
		}
	case scene.SceneEvent:
		p.ID = ev.SceneID().UUID()
	case project.ProjectEvent:
		p.ID = ev.ProjectID().UUID()
	case editlock.Event:
		if ev.Scope().IsLibrary() {
			rawLibraryID := ev.Scope().LibraryID().UUID()
			p.ID = rawLibraryID
			p.LibraryID = &rawLibraryID
		} else {
			rawProjectID := ev.Scope().ProjectID().UUID()
			p.ID = rawProjectID
			p.ProjectID = &rawProjectID
		}
		state := ev.State()
		p.EditLock = &editLockPayload{Held: state.Held, Name: state.Holder.Name(),
			Version: state.Version}
		if state.Held {
			key := state.Holder.Key().UUID()
			since, lastSeen := state.Since, state.LastSeen
			p.EditLock.Key, p.EditLock.Since, p.EditLock.LastSeen = &key, &since, &lastSeen
		}
		if taken, ok := ev.(editlock.EditLockTakenOverEvent); ok && !taken.Previous().IsZero() {
			key := taken.Previous().Key().UUID()
			p.PreviousKey, p.PreviousName = &key, taken.Previous().Name()
		}
	default:
		return nil, fmt.Errorf("cannot encode event %s: not a domain event", e.Type())
	}
	return json.Marshal(p)
}

// Decode rebuilds the event of the given type from its stored form.
func Decode(eventType event.EventType, data []byte) (event.Event, error) {
	var p payload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("cannot decode %s payload: %w", eventType, err)
	}
	ts := event.WithTimestamp(event.NewEventTimestamp(event.EventTimestampWithTime(p.Timestamp)))

	tagID := id.NewID(id.IDWithUUID[tag.Tag](p.ID))
	widgetTypeID := id.NewID(id.IDWithUUID[library.WidgetType](p.ID))
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](p.ID))
	widgetID := id.NewID(id.IDWithUUID[scene.Widget](p.ID))

	projectOfEvent := id.NewID(id.IDWithUUID[project.Project](p.ID))
	libraryOfEvent := id.NewID(id.IDWithUUID[library.Library](p.ID))
	deviceOfEvent := id.NewID(id.IDWithUUID[device.Device](p.ID))
	environmentOfEvent := id.NewID(id.IDWithUUID[device.Environment](p.ID))

	switch eventType {
	case event.EventTypeWidgetTypeCreated, event.EventTypeWidgetTypeUpdated, event.EventTypeWidgetTypeDeleted:
		if p.LibraryID == nil {
			return nil, fmt.Errorf("cannot decode %s payload: no library_id", eventType)
		}
		libraryID := id.NewID(id.IDWithUUID[library.Library](*p.LibraryID))
		switch eventType {
		case event.EventTypeWidgetTypeCreated:
			return library.NewWidgetTypeCreatedEvent(widgetTypeID, libraryID, ts), nil
		case event.EventTypeWidgetTypeUpdated:
			return library.NewWidgetTypeUpdatedEvent(widgetTypeID, libraryID, ts), nil
		default:
			return library.NewWidgetTypeDeletedEvent(widgetTypeID, libraryID, ts), nil
		}
	case event.EventTypeLibraryCreated:
		return library.NewLibraryCreatedEvent(libraryOfEvent, ts), nil
	case event.EventTypeLibraryUpdated:
		return library.NewLibraryUpdatedEvent(libraryOfEvent, ts), nil
	case event.EventTypeLibraryDeleted:
		return library.NewLibraryDeletedEvent(libraryOfEvent, ts), nil
	case event.EventTypeLibraryDiscardStarted:
		return library.NewLibraryDiscardStartedEvent(libraryOfEvent, ts), nil
	case event.EventTypeLibraryReleased:
		release, err := library.NewReleaseNumber(p.Release)
		if err != nil {
			return nil, fmt.Errorf("cannot decode %s payload: %w", eventType, err)
		}
		return library.NewLibraryReleasedEvent(libraryOfEvent, release, ts), nil
	case event.EventTypeProjectCreated:
		return project.NewProjectCreatedEvent(projectOfEvent, ts), nil
	case event.EventTypeProjectUpdated:
		return project.NewProjectUpdatedEvent(projectOfEvent, ts), nil
	case event.EventTypeProjectDeleted:
		return project.NewProjectDeletedEvent(projectOfEvent, ts), nil
	case event.EventTypeProjectDiscardStarted:
		return project.NewProjectDiscardStartedEvent(projectOfEvent, ts), nil
	case event.EventTypeEditLockAcquired, event.EventTypeEditLockReleased,
		event.EventTypeEditLockTakenOver, event.EventTypeEditLockExpired:
		return decodeEditLockEvent(eventType, p, ts)
	}

	// Events of a Project's content: the Project is stored beside them.
	if p.ProjectID == nil {
		return nil, fmt.Errorf("cannot decode %s payload: no project_id", eventType)
	}
	projectID := id.NewID(id.IDWithUUID[project.Project](*p.ProjectID))
	switch eventType {
	case event.EventTypeTagCreated:
		return tag.NewTagCreatedEvent(tagID, projectID, ts), nil
	case event.EventTypeTagDeleted:
		return tag.NewTagDeletedEvent(tagID, projectID, ts), nil
	case event.EventTypeSceneCreated:
		return scene.NewSceneCreatedEvent(sceneID, projectID, ts), nil
	case event.EventTypeSceneUpdated:
		return scene.NewSceneUpdatedEvent(sceneID, projectID, ts), nil
	case event.EventTypeSceneDeleted:
		return scene.NewSceneDeletedEvent(sceneID, projectID, ts), nil
	case event.EventTypeLibraryPinCreated:
		return librarypin.NewLibraryPinCreatedEvent(libraryOfEvent, projectID, ts), nil
	case event.EventTypeLibraryPinUpdated:
		return librarypin.NewLibraryPinUpdatedEvent(libraryOfEvent, projectID, ts), nil
	case event.EventTypeLibraryPinDeleted:
		return librarypin.NewLibraryPinDeletedEvent(libraryOfEvent, projectID, ts), nil
	case event.EventTypeDeviceCreated:
		return device.NewDeviceCreatedEvent(deviceOfEvent, projectID, ts), nil
	case event.EventTypeDeviceUpdated:
		return device.NewDeviceUpdatedEvent(deviceOfEvent, projectID, ts), nil
	case event.EventTypeDeviceDeleted:
		return device.NewDeviceDeletedEvent(deviceOfEvent, projectID, ts), nil
	case event.EventTypeEnvironmentCreated:
		return device.NewEnvironmentCreatedEvent(environmentOfEvent, projectID, ts), nil
	case event.EventTypeEnvironmentUpdated:
		return device.NewEnvironmentUpdatedEvent(environmentOfEvent, projectID, ts), nil
	case event.EventTypeEnvironmentDeleted:
		return device.NewEnvironmentDeletedEvent(environmentOfEvent, projectID, ts), nil
	case event.EventTypeEnvironmentDeviceCreated, event.EventTypeEnvironmentDeviceUpdated, event.EventTypeEnvironmentDeviceDeleted:
		if p.EnvironmentID == nil || p.DeviceID == nil {
			return nil, fmt.Errorf("cannot decode %s payload: no environment_id or device_id", eventType)
		}
		environmentID := id.NewID(id.IDWithUUID[device.Environment](*p.EnvironmentID))
		deviceID := id.NewID(id.IDWithUUID[device.Device](*p.DeviceID))
		switch eventType {
		case event.EventTypeEnvironmentDeviceCreated:
			return device.NewEnvironmentDeviceCreatedEvent(environmentID, deviceID, projectID, ts), nil
		case event.EventTypeEnvironmentDeviceUpdated:
			return device.NewEnvironmentDeviceUpdatedEvent(environmentID, deviceID, projectID, ts), nil
		default:
			return device.NewEnvironmentDeviceDeletedEvent(environmentID, deviceID, projectID, ts), nil
		}
	}

	// Widget events: the Scene that holds the widget is stored beside it.
	if p.SceneID == nil {
		return nil, fmt.Errorf("cannot decode %s payload: no scene_id", eventType)
	}
	holder := id.NewID(id.IDWithUUID[scene.Scene](*p.SceneID))
	switch eventType {
	case event.EventTypeWidgetCreated, event.EventTypeWidgetUpdated:
		if p.WidgetTypeID == nil {
			return untypedWidgetEvent{Base: event.NewBase(eventType, ts), widgetID: widgetID, sceneID: holder, Content: project.NewContent(projectID)}, nil
		}
	}
	switch eventType {
	case event.EventTypeWidgetCreated:
		return scene.NewWidgetCreatedEvent(widgetID, holder, projectID, id.NewID(id.IDWithUUID[library.WidgetType](*p.WidgetTypeID)), ts), nil
	case event.EventTypeWidgetUpdated:
		typeID := id.NewID(id.IDWithUUID[library.WidgetType](*p.WidgetTypeID))
		if p.ByReconciliation {
			return scene.NewWidgetUpdatedByReconciliationEvent(widgetID, holder, projectID, typeID, ts), nil
		}
		return scene.NewWidgetUpdatedEvent(widgetID, holder, projectID, typeID, ts), nil
	case event.EventTypeWidgetDeleted:
		return scene.NewWidgetDeletedEvent(widgetID, holder, projectID, ts), nil
	}
	return nil, fmt.Errorf("cannot decode event of type %s: not a domain event", eventType)
}

// decodeEditLockEvent rebuilds an edit_lock_* event from its payload: the
// scope from the Project or Library it names, the state from its lock block.
func decodeEditLockEvent(eventType event.EventType, p payload, ts event.EventOption) (event.Event, error) {
	if p.EditLock == nil {
		return nil, fmt.Errorf("cannot decode %s payload: no edit_lock", eventType)
	}
	var scope editlock.Scope
	switch {
	case p.ProjectID != nil && p.LibraryID == nil:
		scope = editlock.ProjectScope(id.NewID(id.IDWithUUID[project.Project](*p.ProjectID)))
	case p.LibraryID != nil && p.ProjectID == nil:
		scope = editlock.LibraryScope(id.NewID(id.IDWithUUID[library.Library](*p.LibraryID)))
	default:
		return nil, fmt.Errorf("cannot decode %s payload: no single scope", eventType)
	}
	state := editlock.Snapshot{Version: p.EditLock.Version}
	if p.EditLock.Held {
		if p.EditLock.Key == nil || p.EditLock.Since == nil || p.EditLock.LastSeen == nil {
			return nil, fmt.Errorf("cannot decode %s payload: a held lock needs key and times", eventType)
		}
		key, err := editlock.ParseKey(p.EditLock.Key.String())
		if err != nil {
			return nil, fmt.Errorf("cannot decode %s payload: %w", eventType, err)
		}
		holder, err := editlock.NewHolder(key, p.EditLock.Name)
		if err != nil {
			return nil, fmt.Errorf("cannot decode %s payload: %w", eventType, err)
		}
		state.Held, state.Holder = true, holder
		state.Since, state.LastSeen = *p.EditLock.Since, *p.EditLock.LastSeen
	}
	switch eventType {
	case event.EventTypeEditLockAcquired:
		return editlock.NewEditLockAcquiredEvent(scope, state, ts), nil
	case event.EventTypeEditLockReleased:
		return editlock.NewEditLockReleasedEvent(scope, state, ts), nil
	case event.EventTypeEditLockTakenOver:
		var previous editlock.Holder
		if p.PreviousKey != nil {
			key, err := editlock.ParseKey(p.PreviousKey.String())
			if err == nil {
				previous, _ = editlock.NewHolder(key, p.PreviousName)
			}
		}
		return editlock.NewEditLockTakenOverEvent(scope, state, previous, ts), nil
	default:
		return editlock.NewEditLockExpiredEvent(scope, state, ts), nil
	}
}

// untypedWidgetEvent is a widget_created or widget_updated event read back
// from a row written before Widget events carried their WidgetType. It is a
// scene.WidgetEvent, enough for delivery, but no scene.WidgetTypedEvent, so
// no policy keyed by the WidgetType enqueues a job for it.
type untypedWidgetEvent struct {
	event.Base
	widgetID id.ID[scene.Widget]
	sceneID  id.ID[scene.Scene]
	project.Content
}

var _ scene.WidgetEvent = untypedWidgetEvent{}

func (e untypedWidgetEvent) WidgetID() id.ID[scene.Widget] { return e.widgetID }
func (e untypedWidgetEvent) SceneID() id.ID[scene.Scene]   { return e.sceneID }
