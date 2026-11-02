package device

import (
	"context"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
)

// DeviceRepository - repository interface for storing and managing the
// Devices of one Project's Draft (ADR 0006): every read sees only the Devices
// of that Project, and saving or deleting one of another Project is a
// programming error.
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

	// Save stores a Device and the events it recorded, in one transaction: a
	// Device of the initial version is inserted, any other replaces the
	// stored one if the stored version is still the Device's version. The
	// version is raised once per Save; the returned Device carries the new
	// one and no events.
	// Returns ErrDeviceNotFound if the Device does not exist on update,
	// ErrDeviceConflict if the stored version does not match,
	// ErrDeviceNameTaken if another Device of the Project has the name, or an
	// error on failure.
	Save(context.Context, Device) (Device, error)

	// Delete removes a Device if the stored version is still the Device's
	// version and stores the events it recorded (see Device.Delete), in one
	// transaction. A Device still in use is not removed: while a Tag of the
	// Project is supplied by it, or while it stands in an EnvironmentDevice,
	// Delete refuses with ErrDeviceInUse (ADR 0008).
	// Returns ErrDeviceNotFound if the Device does not exist,
	// ErrDeviceConflict if the stored version does not match,
	// ErrDeviceInUse if it is still referred to, or an error on failure.
	Delete(context.Context, Device) error
}
