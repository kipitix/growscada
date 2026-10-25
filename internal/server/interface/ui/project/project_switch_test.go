package project

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

func TestSetProject_ShowsTheContentOfTheNewProjectOnly(t *testing.T) {
	url, api := newServer(t)
	inMain := createScene(t, api, "Main scene")
	createTag(t, api, "main.tag", "integer")
	f := mount(t, url, api)
	if f.p.selectedSceneID != inMain {
		t.Fatalf("selected %v, want Main's scene", f.p.selectedSceneID)
	}
	created, err := api.CreateProject(context.Background(), apiv0.CreateProjectRequest{Name: "Line 1"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	line := api.Project(created.ID)
	inLine, err := line.CreateScene(context.Background(), apiv0.CreateSceneRequest{Name: "Line scene", Width: 100, Height: 100})
	if err != nil {
		t.Fatalf("create scene: %v", err)
	}
	// A reload of Main's list is in flight when the project changes.
	f.p.loadScenes(f.p.compoCtx)

	f.p.SetProject(created.ID)
	f.run()

	f.noToasts()
	if len(f.p.scenes) != 1 || f.p.scenes[0].ID != inLine.ID || f.p.selectedSceneID != inLine.ID {
		t.Errorf("scenes %+v, selected %v: want Line 1's scene only", f.p.scenes, f.p.selectedSceneID)
	}
	if list := f.p.tags.List(); len(list) != 0 {
		t.Errorf("tags %+v, want Line 1's: none", list)
	}

	f.p.SetProject(uuid.Nil)
	f.run()
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, "data-no-project") {
		t.Errorf("no project: the mode does not say so: %.200s", html)
	}
	if len(f.p.scenes) != 0 || f.p.selectedSceneID != uuid.Nil {
		t.Errorf("no project: scenes %+v, selected %v", f.p.scenes, f.p.selectedSceneID)
	}
}
