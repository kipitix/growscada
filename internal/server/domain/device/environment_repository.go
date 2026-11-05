package device

import (
	"context"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
)

// EnvironmentRepository reads the Environments of one Project's Draft
// (ADR 0006): every read sees only the Environments of that Project. An
// Environment is an entity of the Draft (ADR 0016): its writes go through the
// Draft's repository.
type EnvironmentRepository interface {
	// NextID returns a new unique identifier for an Environment.
	NextID() id.ID[Environment]

	// FindAll returns the Project's Environments in creation order.
	FindAll(context.Context) ([]Environment, error)

	// FindAllInLiveProject is FindAll that also checks, in the same query,
	// that the Project is live and of the Organization: for the list the UI
	// reloads on every event. A Project not one of those has no Environments:
	// the list is empty, as an empty Project's, which the caller tells apart.
	FindAllInLiveProject(context.Context, id.ID[organization.Organization]) ([]Environment, error)

	// FindByID returns the Environment of the Project with the given ID.
	// Returns ErrEnvironmentNotFound if there is none.
	FindByID(context.Context, id.ID[Environment]) (Environment, error)

	// FindByName returns the Project's Environment with the given name,
	// unique in the Project: a Device Gateway names the Environment it serves,
	// and the name is resolved to a pair with a Device.
	// Returns ErrEnvironmentNotFound if there is none.
	FindByName(context.Context, EnvironmentName) (Environment, error)
}
