package application

import (
	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// ProjectScope gives the repositories of one Project's content (ADR 0006):
// every query they make is scoped to that Project, so a query cannot forget
// it, and saving an aggregate of another Project through them is a
// programming error.
type ProjectScope interface {
	// ProjectID is the Project the repositories are scoped to.
	ProjectID() id.ID[project.Project]
	Tags() tag.TagRepository
	Scenes() scene.SceneRepository
	// LibraryPins are the Library Pins of the Project's Draft, and through
	// them the WidgetTypes its Widgets may be of (ADR 0007).
	LibraryPins() librarypin.Repository
	// Devices are the Devices of the Project's Draft: what its Tags may be
	// supplied by (ADR 0004, amendment 2026-11-01).
	Devices() device.DeviceRepository
	// Environments are the Environments of the Project's Draft: the
	// parameters by which its Devices are reached.
	Environments() device.EnvironmentRepository
	// EnvironmentDevices are the pairs of an Environment and a Device of the
	// Project's Draft, with their parameters.
	EnvironmentDevices() device.EnvironmentDeviceRepository
}

// ProjectScopes opens the ProjectScope of a Project. It knows nothing of the
// Project's state: whoever opens a scope for the API first checks that the
// Project is live.
type ProjectScopes interface {
	// ForProject returns the repositories of the Project's content, whose
	// writes are refused while a Discard of the Project is under way.
	ForProject(id.ID[project.Project]) ProjectScope
	// ForDiscard returns the repositories of the Project's content whose
	// writes are a Discard's: made only while one is under way (task 35,
	// Q10).
	ForDiscard(id.ID[project.Project]) ProjectScope
}
