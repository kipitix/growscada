package device

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// DeviceEvent - base interface for all events a Device records. It is a
// project.ContentEvent: it names the Device's Project, and the Device is what
// tells it apart there.
type DeviceEvent interface {
	project.ContentEvent
	DeviceID() id.ID[Device]
	deviceEvent()
}

type deviceEventImpl struct {
	event.Base
	deviceID id.ID[Device]
	project.Content
}

var _ DeviceEvent = (*deviceEventImpl)(nil)

// NO FABRIC METHOD

func (e deviceEventImpl) DeviceID() id.ID[Device] { return e.deviceID }

func (deviceEventImpl) deviceEvent() {}

func newDeviceEvent(anEventType event.EventType, aDeviceID id.ID[Device], aProjectID id.ID[project.Project], opts ...event.EventOption) deviceEventImpl {
	return deviceEventImpl{Base: event.NewBase(anEventType, opts...), deviceID: aDeviceID, Content: project.NewContent(aProjectID)}
}

// DeviceCreatedEvent is recorded by CreateDevice.
type DeviceCreatedEvent interface{ DeviceEvent }

// DeviceUpdatedEvent is recorded by Device.Update.
type DeviceUpdatedEvent interface{ DeviceEvent }

// DeviceDeletedEvent is recorded by Device.Delete.
type DeviceDeletedEvent interface{ DeviceEvent }

type deviceCreatedEventImpl struct{ deviceEventImpl }
type deviceUpdatedEventImpl struct{ deviceEventImpl }
type deviceDeletedEventImpl struct{ deviceEventImpl }

func NewDeviceCreatedEvent(aDeviceID id.ID[Device], aProjectID id.ID[project.Project], opts ...event.EventOption) DeviceCreatedEvent {
	return &deviceCreatedEventImpl{newDeviceEvent(event.EventTypeDeviceCreated, aDeviceID, aProjectID, opts...)}
}

func NewDeviceUpdatedEvent(aDeviceID id.ID[Device], aProjectID id.ID[project.Project], opts ...event.EventOption) DeviceUpdatedEvent {
	return &deviceUpdatedEventImpl{newDeviceEvent(event.EventTypeDeviceUpdated, aDeviceID, aProjectID, opts...)}
}

func NewDeviceDeletedEvent(aDeviceID id.ID[Device], aProjectID id.ID[project.Project], opts ...event.EventOption) DeviceDeletedEvent {
	return &deviceDeletedEventImpl{newDeviceEvent(event.EventTypeDeviceDeleted, aDeviceID, aProjectID, opts...)}
}
