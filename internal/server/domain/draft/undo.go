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
		published, err := snapshotEvent(scope, snapshot, change)
		if err != nil {
			return nil, err
		}
		events = append(events, published)
	}
	for _, snapshot := range oldSide {
		if _, ok := findSnapshot(newSide, snapshot); ok {
			continue
		}
		published, err := snapshotEvent(scope, snapshot, entityDeleted)
		if err != nil {
			return nil, err
		}
		events = append(events, published)
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

// snapshotEvent is the event publishing snapshot with change: the restored row
// names the same entity, Scene and WidgetType as the row it replaces.
func snapshotEvent(scope editlock.Scope, snapshot Snapshot, change entityChange) (event.Event, error) {
	projectID := scope.ProjectID()
	libraryID := scope.LibraryID()
	switch state := snapshot.State.(type) {
	case TagState:
		tagID := id.NewID(id.IDWithUUID[tag.Tag](snapshot.ID))
		switch change {
		case entityCreated:
			return tag.NewTagCreatedEvent(tagID, projectID), nil
		case entityDeleted:
			return tag.NewTagDeletedEvent(tagID, projectID), nil
		}
		return nil, fmt.Errorf("a tag %s changed in place: there is no tag update event", snapshot.ID)
	case SceneState:
		sceneID := id.NewID(id.IDWithUUID[scene.Scene](snapshot.ID))
		switch change {
		case entityCreated:
			return scene.NewSceneCreatedEvent(sceneID, projectID), nil
		case entityUpdated:
			return scene.NewSceneUpdatedEvent(sceneID, projectID), nil
		case entityDeleted:
			return scene.NewSceneDeletedEvent(sceneID, projectID), nil
		}
	case WidgetState:
		widgetID := id.NewID(id.IDWithUUID[scene.Widget](snapshot.ID))
		sceneID := id.NewID(id.IDWithUUID[scene.Scene](state.SceneID))
		typeID := id.NewID(id.IDWithUUID[library.WidgetType](state.TypeID))
		switch change {
		case entityCreated:
			return scene.NewWidgetCreatedEvent(widgetID, sceneID, projectID, typeID), nil
		case entityUpdated:
			return scene.NewWidgetUpdatedEvent(widgetID, sceneID, projectID, typeID), nil
		case entityDeleted:
			return scene.NewWidgetDeletedEvent(widgetID, sceneID, projectID), nil
		}
	case DeviceState:
		deviceID := id.NewID(id.IDWithUUID[device.Device](snapshot.ID))
		switch change {
		case entityCreated:
			return device.NewDeviceCreatedEvent(deviceID, projectID), nil
		case entityUpdated:
			return device.NewDeviceUpdatedEvent(deviceID, projectID), nil
		case entityDeleted:
			return device.NewDeviceDeletedEvent(deviceID, projectID), nil
		}
	case EnvironmentState:
		environmentID := id.NewID(id.IDWithUUID[device.Environment](snapshot.ID))
		switch change {
		case entityCreated:
			return device.NewEnvironmentCreatedEvent(environmentID, projectID), nil
		case entityUpdated:
			return device.NewEnvironmentUpdatedEvent(environmentID, projectID), nil
		case entityDeleted:
			return device.NewEnvironmentDeletedEvent(environmentID, projectID), nil
		}
	case EnvironmentDeviceState:
		environmentID := id.NewID(id.IDWithUUID[device.Environment](state.EnvironmentID))
		deviceID := id.NewID(id.IDWithUUID[device.Device](state.DeviceID))
		switch change {
		case entityCreated:
			return device.NewEnvironmentDeviceCreatedEvent(environmentID, deviceID, projectID), nil
		case entityUpdated:
			return device.NewEnvironmentDeviceUpdatedEvent(environmentID, deviceID, projectID), nil
		case entityDeleted:
			return device.NewEnvironmentDeviceDeletedEvent(environmentID, deviceID, projectID), nil
		}
	case LibraryPinState:
		pinnedID := id.NewID(id.IDWithUUID[library.Library](state.LibraryID))
		switch change {
		case entityCreated:
			return librarypin.NewLibraryPinCreatedEvent(pinnedID, projectID), nil
		case entityUpdated:
			return librarypin.NewLibraryPinUpdatedEvent(pinnedID, projectID), nil
		case entityDeleted:
			return librarypin.NewLibraryPinDeletedEvent(pinnedID, projectID), nil
		}
	case WidgetTypeState:
		typeID := id.NewID(id.IDWithUUID[library.WidgetType](snapshot.ID))
		ownerID := libraryID
		if state.LibraryID != uuid.Nil {
			ownerID = id.NewID(id.IDWithUUID[library.Library](state.LibraryID))
		}
		switch change {
		case entityCreated:
			return library.NewWidgetTypeCreatedEvent(typeID, ownerID), nil
		case entityUpdated:
			return library.NewWidgetTypeUpdatedEvent(typeID, ownerID), nil
		case entityDeleted:
			return library.NewWidgetTypeDeletedEvent(typeID, ownerID), nil
		}
	default:
		return nil, fmt.Errorf("cannot publish a draft entity of unknown state %T", snapshot.State)
	}
	return nil, fmt.Errorf("cannot publish %s %s: unknown change %d", snapshot.Kind, snapshot.ID, change)
}
