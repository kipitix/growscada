package device

import (
	"context"

	"github.com/kipitix/growscada/internal/server/domain/id"
)

// EnvironmentDeviceRepository reads the EnvironmentDevices of one Project's
// Draft (ADR 0006): every read sees only the pairs of the Environments and
// Devices of that Project. A pair is an entity of the Draft (ADR 0016): its
// writes go through the Draft's repository.
type EnvironmentDeviceRepository interface {
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
}
