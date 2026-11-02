package device

import (
	"context"

	"github.com/kipitix/growscada/internal/server/domain/id"
)

// EnvironmentRepository - repository interface for storing and managing the
// Environments of one Project's Draft (ADR 0006): every read sees only the
// Environments of that Project, and saving or deleting one of another Project
// is a programming error.
type EnvironmentRepository interface {
	// NextID returns a new unique identifier for an Environment.
	NextID() id.ID[Environment]

	// FindAll returns the Project's Environments in creation order.
	FindAll(context.Context) ([]Environment, error)

	// FindByID returns the Environment of the Project with the given ID.
	// Returns ErrEnvironmentNotFound if there is none.
	FindByID(context.Context, id.ID[Environment]) (Environment, error)

	// FindByName returns the Project's Environment with the given name,
	// unique in the Project: a Device Gateway names the Environment it serves,
	// and the name is resolved to a pair with a Device.
	// Returns ErrEnvironmentNotFound if there is none.
	FindByName(context.Context, EnvironmentName) (Environment, error)

	// Save stores an Environment and the events it recorded, in one
	// transaction: an Environment of the initial version is inserted, any
	// other replaces the stored one if the stored version is still the
	// Environment's version. The version is raised once per Save; the
	// returned Environment carries the new one and no events.
	// Returns ErrEnvironmentNotFound if the Environment does not exist on
	// update, ErrEnvironmentConflict if the stored version does not match,
	// ErrEnvironmentNameTaken if another Environment of the Project has the
	// name, or an error on failure.
	Save(context.Context, Environment) (Environment, error)

	// Delete removes an Environment if the stored version is still the
	// Environment's version and stores the events it recorded (see
	// Environment.Delete), in one transaction. An Environment still in use is
	// not removed: while it holds the pairs of its Devices, Delete refuses.
	// Returns ErrEnvironmentNotFound if the Environment does not exist,
	// ErrEnvironmentConflict if the stored version does not match, or an
	// error on failure.
	Delete(context.Context, Environment) error
}
