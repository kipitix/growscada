package draft

import (
	"fmt"
	"reflect"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// Undo returns the Draft after undoing change (ADR 0017): the Version of its
// content is raised and its stack position steps back. The rows are written
// and the events published by the Session.
func (d *draftImpl) Undo(change DraftChange) (Draft, error) {
	if change.Seq != d.stackPosition {
		return nil, fmt.Errorf("%w: the stack is at %d, the change at %d", ErrNothingToUndo, d.stackPosition, change.Seq)
	}
	return d.WithVersion(d.version.Next()).WithStackPosition(d.stackPosition - 1), nil
}

// Redo returns the Draft after redoing change, the DraftChange after the
// stack's position (ADR 0017).
func (d *draftImpl) Redo(change DraftChange) (Draft, error) {
	if change.Seq != d.stackPosition+1 {
		return nil, fmt.Errorf("%w: the stack is at %d, the change at %d", ErrNothingToRedo, d.stackPosition, change.Seq)
	}
	return d.WithVersion(d.version.Next()).WithStackPosition(d.stackPosition + 1), nil
}

// entityChange is what applying one side of a DraftChange does to an entity.
type entityChange int

const (
	entityCreated entityChange = iota + 1
	entityUpdated
	entityDeleted
)

// inverseEvents returns the events that turning oldSide into newSide publishes
// (ADR 0017): an entity newSide has and oldSide lacks is created, one both have
// with a different row is updated, and one only oldSide has is deleted. The
// restored rows are published as the forward edit of the same effect would be,
// which is what the SSE stream and the UI reload on.
func inverseEvents(scope editlock.Scope, oldSide, newSide []Snapshot) ([]event.Event, error) {
	var events []event.Event
	for _, snapshot := range newSide {
		previous, ok := findSnapshot(oldSide, snapshot)
		change := entityCreated
		if ok {
			if sameState(previous.State, snapshot.State) {
				continue
			}
			change = entityUpdated
		}
		published, err := snapshotEvents(scope, snapshot, change)
		if err != nil {
			return nil, err
		}
		events = append(events, published...)
	}
	for _, snapshot := range oldSide {
		if _, ok := findSnapshot(newSide, snapshot); ok {
			continue
		}
		published, err := snapshotEvents(scope, snapshot, entityDeleted)
		if err != nil {
			return nil, err
		}
		events = append(events, published...)
	}
	return events, nil
}

// findSnapshot finds the state of the same entity — the same kind, ID and, for
// a pair (Environment, Device), the same Device — in list.
func findSnapshot(list []Snapshot, wanted Snapshot) (Snapshot, bool) {
	wantedPair, wantedIsPair := wanted.State.(EnvironmentDeviceState)
	for _, snapshot := range list {
		if snapshot.Kind != wanted.Kind || snapshot.ID != wanted.ID {
			continue
		}
		if wantedIsPair {
			pair, ok := snapshot.State.(EnvironmentDeviceState)
			if !ok || pair.DeviceID != wantedPair.DeviceID {
				continue
			}
		}
		return snapshot, true
	}
	return Snapshot{}, false
}

// sameState reports whether two stored states of one entity are the same row.
func sameState(a, b State) bool { return reflect.DeepEqual(a, b) }

// snapshotEvents is the events publishing snapshot with change: the restored
// row names the same entity, Scene and WidgetType as the row it replaces. A
// Tag definition has no update event, so a Tag that changed in place is a
// deletion and a creation, as the forward write records it.
func snapshotEvents(scope editlock.Scope, snapshot Snapshot, change entityChange) ([]event.Event, error) {
	projectID := scope.ProjectID()
	libraryID := scope.LibraryID()
	switch state := snapshot.State.(type) {
	case TagState:
		tagID := id.NewID(id.IDWithUUID[tag.Tag](snapshot.ID))
		switch change {
		case entityCreated:
			return []event.Event{tag.NewTagCreatedEvent(tagID, projectID)}, nil
		case entityUpdated:
			return []event.Event{
				tag.NewTagDeletedEvent(tagID, projectID),
				tag.NewTagCreatedEvent(tagID, projectID),
			}, nil
		case entityDeleted:
			return []event.Event{tag.NewTagDeletedEvent(tagID, projectID)}, nil
		}
	case SceneState:
		sceneID := id.NewID(id.IDWithUUID[scene.Scene](snapshot.ID))
		switch change {
		case entityCreated:
			return []event.Event{scene.NewSceneCreatedEvent(sceneID, projectID)}, nil
		case entityUpdated:
			return []event.Event{scene.NewSceneUpdatedEvent(sceneID, projectID)}, nil
		case entityDeleted:
			return []event.Event{scene.NewSceneDeletedEvent(sceneID, projectID)}, nil
		}
	case WidgetState:
		widgetID := id.NewID(id.IDWithUUID[scene.Widget](snapshot.ID))
		sceneID := id.NewID(id.IDWithUUID[scene.Scene](state.SceneID))
		typeID := id.NewID(id.IDWithUUID[library.WidgetType](state.TypeID))
		switch change {
		case entityCreated:
			return []event.Event{scene.NewWidgetCreatedEvent(widgetID, sceneID, projectID, typeID)}, nil
		case entityUpdated:
			return []event.Event{scene.NewWidgetUpdatedEvent(widgetID, sceneID, projectID, typeID)}, nil
		case entityDeleted:
			return []event.Event{scene.NewWidgetDeletedEvent(widgetID, sceneID, projectID)}, nil
		}
	case DeviceState:
		deviceID := id.NewID(id.IDWithUUID[device.Device](snapshot.ID))
		switch change {
		case entityCreated:
			return []event.Event{device.NewDeviceCreatedEvent(deviceID, projectID)}, nil
		case entityUpdated:
			return []event.Event{device.NewDeviceUpdatedEvent(deviceID, projectID)}, nil
		case entityDeleted:
			return []event.Event{device.NewDeviceDeletedEvent(deviceID, projectID)}, nil
		}
	case EnvironmentState:
		environmentID := id.NewID(id.IDWithUUID[device.Environment](snapshot.ID))
		switch change {
		case entityCreated:
			return []event.Event{device.NewEnvironmentCreatedEvent(environmentID, projectID)}, nil
		case entityUpdated:
			return []event.Event{device.NewEnvironmentUpdatedEvent(environmentID, projectID)}, nil
		case entityDeleted:
			return []event.Event{device.NewEnvironmentDeletedEvent(environmentID, projectID)}, nil
		}
	case EnvironmentDeviceState:
		environmentID := id.NewID(id.IDWithUUID[device.Environment](state.EnvironmentID))
		deviceID := id.NewID(id.IDWithUUID[device.Device](state.DeviceID))
		switch change {
		case entityCreated:
			return []event.Event{device.NewEnvironmentDeviceCreatedEvent(environmentID, deviceID, projectID)}, nil
		case entityUpdated:
			return []event.Event{device.NewEnvironmentDeviceUpdatedEvent(environmentID, deviceID, projectID)}, nil
		case entityDeleted:
			return []event.Event{device.NewEnvironmentDeviceDeletedEvent(environmentID, deviceID, projectID)}, nil
		}
	case LibraryPinState:
		pinnedID := id.NewID(id.IDWithUUID[library.Library](state.LibraryID))
		switch change {
		case entityCreated:
			return []event.Event{librarypin.NewLibraryPinCreatedEvent(pinnedID, projectID)}, nil
		case entityUpdated:
			return []event.Event{librarypin.NewLibraryPinUpdatedEvent(pinnedID, projectID)}, nil
		case entityDeleted:
			return []event.Event{librarypin.NewLibraryPinDeletedEvent(pinnedID, projectID)}, nil
		}
	case WidgetTypeState:
		typeID := id.NewID(id.IDWithUUID[library.WidgetType](snapshot.ID))
		ownerID := libraryID
		if state.LibraryID != uuid.Nil {
			ownerID = id.NewID(id.IDWithUUID[library.Library](state.LibraryID))
		}
		switch change {
		case entityCreated:
			return []event.Event{library.NewWidgetTypeCreatedEvent(typeID, ownerID)}, nil
		case entityUpdated:
			return []event.Event{library.NewWidgetTypeUpdatedEvent(typeID, ownerID)}, nil
		case entityDeleted:
			return []event.Event{library.NewWidgetTypeDeletedEvent(typeID, ownerID)}, nil
		}
	default:
		return nil, fmt.Errorf("cannot publish a draft entity of unknown state %T", snapshot.State)
	}
	return nil, fmt.Errorf("cannot publish %s %s: unknown change %d", snapshot.Kind, snapshot.ID, change)
}
