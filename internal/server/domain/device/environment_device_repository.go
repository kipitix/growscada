package device

import (
	"context"

	"github.com/kipitix/growscada/internal/server/domain/id"
)

// EnvironmentDeviceRepository - repository interface for storing and managing
// the EnvironmentDevices of one Project's Draft (ADR 0006): every read sees
// only the pairs of the Environments and Devices of that Project, and saving
// or deleting a pair of another Project is a programming error.
type EnvironmentDeviceRepository interface {
	// NextID returns a new unique identifier for an EnvironmentDevice.
	NextID() id.ID[EnvironmentDevice]

	// FindAll returns the Project's pairs in creation order.
	FindAll(context.Context) ([]EnvironmentDevice, error)

	// FindByEnvironment returns the pairs of the Environment, in creation
	// order.
	// Returns ErrEnvironmentNotFound if the Project holds no such
	// Environment.
	FindByEnvironment(context.Context, id.ID[Environment]) ([]EnvironmentDevice, error)

	// FindByPair returns the pair of the Environment and the Device.
	// Returns ErrEnvironmentDeviceNotFound if the Environment holds no
	// parameters for the Device.
	FindByPair(context.Context, id.ID[Environment], id.ID[Device]) (EnvironmentDevice, error)

	// Save stores a pair and the events it recorded, in one transaction: a
	// pair of the initial version is inserted while both the Environment and
	// the Device are of the Project, any other replaces the stored one if the
	// stored version is still the pair's version. The version is raised once
	// per Save; the returned pair carries the new one and no events.
	// Returns ErrEnvironmentDeviceExists if the Environment already holds the
	// Device on insert, ErrEnvironmentDeviceNotFound if the pair does not
	// exist on update, ErrEnvironmentDeviceConflict if the stored version
	// does not match, ErrEnvironmentNotFound or ErrDeviceNotFound if either
	// side is not of the Project, or an error on failure.
	Save(context.Context, EnvironmentDevice) (EnvironmentDevice, error)

	// Delete removes a pair if the stored version is still the pair's
	// version, and stores the events it recorded (see
	// EnvironmentDevice.Delete), in one transaction.
	// Returns ErrEnvironmentDeviceNotFound if the pair does not exist,
	// ErrEnvironmentDeviceConflict if the stored version does not match, or
	// an error on failure.
	Delete(context.Context, EnvironmentDevice) error
}
