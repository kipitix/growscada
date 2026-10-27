package application

import (
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
}

// ProjectScopes opens the ProjectScope of a Project. It knows nothing of the
// Project's state: whoever opens a scope for the API first checks that the
// Project is live.
type ProjectScopes interface {
	// ForProject returns the repositories of the Project's content.
	ForProject(id.ID[project.Project]) ProjectScope
}
