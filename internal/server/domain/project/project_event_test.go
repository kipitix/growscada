package project_test

import (
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// TestProjectAndContentEvents_AreApart: both carry a ProjectID, but an event
// is of one kind only, so no check of one depends on checking the other
// first (the routing of the event streams, the outbox payload).
func TestProjectAndContentEvents_AreApart(t *testing.T) {
	projectID := id.NewID[project.Project]()
	own := []event.Event{
		project.NewProjectCreatedEvent(projectID),
		project.NewProjectUpdatedEvent(projectID),
		project.NewProjectDeletedEvent(projectID),
	}
	sceneID := id.NewID[scene.Scene]()
	content := []event.Event{
		tag.NewTagCreatedEvent(id.NewID[tag.Tag](), projectID),
		tag.NewTagDeletedEvent(id.NewID[tag.Tag](), projectID),
		scene.NewSceneCreatedEvent(sceneID, projectID),
		scene.NewWidgetDeletedEvent(id.NewID[scene.Widget](), sceneID, projectID),
		scene.NewWidgetUpdatedEvent(id.NewID[scene.Widget](), sceneID, projectID, id.NewID[library.WidgetType]()),
	}

	for _, e := range own {
		if _, ok := e.(project.ContentEvent); ok {
			t.Errorf("%s is a ContentEvent", e.Type())
		}
		if ev, ok := e.(project.ProjectEvent); !ok || ev.ProjectID() != projectID {
			t.Errorf("%s: not a ProjectEvent of the project", e.Type())
		}
	}
	for _, e := range content {
		if _, ok := e.(project.ProjectEvent); ok {
			t.Errorf("%s is a ProjectEvent", e.Type())
		}
		if ev, ok := e.(project.ContentEvent); !ok || ev.ProjectID() != projectID {
			t.Errorf("%s: not a ContentEvent of the project", e.Type())
		}
	}
}
