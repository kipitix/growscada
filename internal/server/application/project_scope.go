package application

import (
	"context"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// ProjectScope gives the repositories of one Project's content (ADR 0006):
// every query they make is scoped to that Project, so a query cannot forget
// it, and saving an aggregate of another Project through them is a
// programming error.
type ProjectScope interface {
	Tags() tag.TagRepository
	Scenes() scene.SceneRepository
}

// ProjectScopes opens the ProjectScope of a Project. It knows nothing of the
// Project's state: whoever opens a scope for the API first checks that the
// Project is live.
type ProjectScopes interface {
	// ForProject returns the repositories of the Project's content.
	ForProject(id.ID[project.Project]) ProjectScope

	// FindProjectsUsingWidgetType returns the Projects, deleted ones included,
	// that hold Widgets of the WidgetType: a WidgetType is shared by every
	// Project until Libraries exist (ADR 0006, task 33).
	FindProjectsUsingWidgetType(context.Context, id.ID[library.WidgetType]) ([]id.ID[project.Project], error)
}
