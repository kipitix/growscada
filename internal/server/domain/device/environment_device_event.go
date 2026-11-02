package device

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// EnvironmentDeviceEvent - base interface for all events an EnvironmentDevice
// records. It is a project.ContentEvent: it names the Project of the
// Environment — and so of the Device, which belongs to the same Project —
// and the pair is what tells it apart there.
type EnvironmentDeviceEvent interface {
	project.ContentEvent
	EnvironmentID() id.ID[Environment]
	DeviceID() id.ID[Device]
	environmentDeviceEvent()
}

type environmentDeviceEventImpl struct {
	event.Base
	environmentID id.ID[Environment]
	deviceID      id.ID[Device]
	project.Content
}

var _ EnvironmentDeviceEvent = (*environmentDeviceEventImpl)(nil)

// NO FABRIC METHOD

func (e environmentDeviceEventImpl) EnvironmentID() id.ID[Environment] { return e.environmentID }
func (e environmentDeviceEventImpl) DeviceID() id.ID[Device]           { return e.deviceID }

func (environmentDeviceEventImpl) environmentDeviceEvent() {}

func newEnvironmentDeviceEvent(
	anEventType event.EventType,
	anEnvironmentID id.ID[Environment],
	aDeviceID id.ID[Device],
	aProjectID id.ID[project.Project],
	opts ...event.EventOption,
) environmentDeviceEventImpl {
	return environmentDeviceEventImpl{
		Base:          event.NewBase(anEventType, opts...),
		environmentID: anEnvironmentID,
		deviceID:      aDeviceID,
		Content:       project.NewContent(aProjectID),
	}
}

// EnvironmentDeviceCreatedEvent is recorded by CreateEnvironmentDevice.
type EnvironmentDeviceCreatedEvent interface{ EnvironmentDeviceEvent }

// EnvironmentDeviceUpdatedEvent is recorded by EnvironmentDevice.UpdateParams.
type EnvironmentDeviceUpdatedEvent interface{ EnvironmentDeviceEvent }

// EnvironmentDeviceDeletedEvent is recorded by EnvironmentDevice.Delete.
type EnvironmentDeviceDeletedEvent interface{ EnvironmentDeviceEvent }

type environmentDeviceCreatedEventImpl struct{ environmentDeviceEventImpl }
type environmentDeviceUpdatedEventImpl struct{ environmentDeviceEventImpl }
type environmentDeviceDeletedEventImpl struct{ environmentDeviceEventImpl }

func NewEnvironmentDeviceCreatedEvent(anEnvironmentID id.ID[Environment], aDeviceID id.ID[Device], aProjectID id.ID[project.Project], opts ...event.EventOption) EnvironmentDeviceCreatedEvent {
	return &environmentDeviceCreatedEventImpl{newEnvironmentDeviceEvent(event.EventTypeEnvironmentDeviceCreated, anEnvironmentID, aDeviceID, aProjectID, opts...)}
}

func NewEnvironmentDeviceUpdatedEvent(anEnvironmentID id.ID[Environment], aDeviceID id.ID[Device], aProjectID id.ID[project.Project], opts ...event.EventOption) EnvironmentDeviceUpdatedEvent {
	return &environmentDeviceUpdatedEventImpl{newEnvironmentDeviceEvent(event.EventTypeEnvironmentDeviceUpdated, anEnvironmentID, aDeviceID, aProjectID, opts...)}
}

func NewEnvironmentDeviceDeletedEvent(anEnvironmentID id.ID[Environment], aDeviceID id.ID[Device], aProjectID id.ID[project.Project], opts ...event.EventOption) EnvironmentDeviceDeletedEvent {
	return &environmentDeviceDeletedEventImpl{newEnvironmentDeviceEvent(event.EventTypeEnvironmentDeviceDeleted, anEnvironmentID, aDeviceID, aProjectID, opts...)}
}
