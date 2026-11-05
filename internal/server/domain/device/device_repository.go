package device

import (
	"context"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
)

// DeviceRepository reads the Devices of one Project's Draft (ADR 0006):
// every read sees only the Devices of that Project. A Device is an entity of
// the Draft (ADR 0016): its writes go through the Draft's repository.
type DeviceRepository interface {
	// NextID returns a new unique identifier for a Device.
	NextID() id.ID[Device]

	// FindAll returns the Project's Devices in creation order.
	FindAll(context.Context) ([]Device, error)

	// FindAllInLiveProject is FindAll that also checks, in the same query,
	// that the Project is live and of the Organization: for the list the UI
	// reloads on every event, which then needs no read of the Project of its
	// own. A Project not one of those has no Devices: the list is empty, as
	// an empty Project's, which the caller tells apart.
	FindAllInLiveProject(context.Context, id.ID[organization.Organization]) ([]Device, error)

	// FindByID returns the Device of the Project with the given ID.
	// Returns ErrDeviceNotFound if there is none.
	FindByID(context.Context, id.ID[Device]) (Device, error)

	// FindByName returns the Project's Device with the given name, unique in
	// the Project.
	// Returns ErrDeviceNotFound if there is none.
	FindByName(context.Context, DeviceName) (Device, error)
}
