package project

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Scene CRUD ────────────────────────────────────────────────────────────────

func (p *Project) createScene(ctx app.Context) {
	req := apiv0.CreateSceneRequest{Name: "New Scene", Width: 1920, Height: 1080}
	mark := p.markScenes()
	content := p.content
	if content == nil {
		return // no project shown
	}
	uiutil.Call(p.compoCtx, func(c context.Context) (apiv0.CreateSceneResponse, error) {
		return content.CreateScene(c, req)
	}, func(ctx app.Context, created apiv0.CreateSceneResponse) {
		// The new scene is the request as the server stored it: listed and
		// editable at once, before the list is reread.
		p.sceneVersions.Observe(mark.versions, created.ID, created.Version)
		p.sceneStates.Observe(mark.states, created.ID, created.Version)
		p.scenes = append(p.scenes, apiv0.SceneResponse{
			ID:             created.ID,
			Name:           req.Name,
			Width:          req.Width,
			Height:         req.Height,
			BackgroundHTML: req.BackgroundHTML,
			Version:        created.Version,
		})
		p.selectedSceneID = created.ID
		p.widgets = nil
		uiutil.StoreID(ctx, sceneIDKey, created.ID)
		p.clearWidgetSelection(ctx)
		p.syncSceneEditingFields()
		p.loadScenes(ctx)
		p.loadWidgets(ctx)
	}, nil)
}

// confirmDeleteScene fetches the scene's widget count so the confirmation
// dialog can warn the user how much they are about to delete along with the
// scene, then deletes it if they confirm.
func (p *Project) confirmDeleteScene(ctx app.Context, sceneID uuid.UUID, sceneName string) {
	confirm := func(ctx app.Context, widgetCount int) {
		message := fmt.Sprintf("Delete scene %q?", sceneName)
		if widgetCount > 0 {
			message = fmt.Sprintf("Scene %q contains %d widget(s). Delete the scene and all of its widgets?", sceneName, widgetCount)
		}
		if !app.Window().Call("confirm", message).Bool() {
			return
		}
		p.deleteScene(ctx, sceneID)
	}
	content := p.content
	if content == nil {
		return // no project shown
	}
	uiutil.Call(p.compoCtx, func(c context.Context) ([]apiv0.WidgetResponse, error) {
		return content.ListWidgets(c, sceneID)
	}, func(ctx app.Context, widgets []apiv0.WidgetResponse) {
		confirm(ctx, len(widgets))
	}, func(ctx app.Context, err error) {
		confirm(ctx, -1) // unknown
	})
}

func (p *Project) deleteScene(ctx app.Context, sceneID uuid.UUID) {
	content := p.content
	if content == nil {
		return // no project shown
	}
	uiutil.Call(p.compoCtx, func(c context.Context) (apiv0.SceneResponse, error) {
		return content.DeleteScene(c, sceneID)
	}, func(ctx app.Context, _ apiv0.SceneResponse) {
		p.loadScenes(ctx)
	}, p.rejectedSceneEdit(sceneID, nil))
}

// selectScene shows the Scene id, as a click on its tab does.
func (p *Project) selectScene(ctx app.Context, id uuid.UUID) {
	p.selectedSceneID = id
	uiutil.StoreID(ctx, sceneIDKey, id)
	p.clearWidgetSelection(ctx)
	p.syncSceneEditingFields()
	p.loadWidgets(ctx)
}

func (p *Project) saveSceneProperties(ctx app.Context) {
	ctx = p.compoCtx
	idx := p.sceneIdx(p.selectedSceneID)
	if idx < 0 {
		return
	}
	var nf numberFields
	w := nf.int("width", p.sceneEdit.width)
	h := nf.int("height", p.sceneEdit.height)
	if nf.err != nil {
		ctx.NewActionWithValue(toast.ActionAdd, toast.ClientError(nf.err))
		p.syncSceneEditingFields()
		return
	}

	sc := &p.scenes[idx]
	sc.Name = p.sceneEdit.name
	sc.Width = w
	sc.Height = h
	sc.BackgroundHTML = p.sceneEdit.background
	// Show the fields as the scene now holds them, so that the reload after
	// a rejected save puts the server's values back (mergeSceneEditingFields
	// keeps only fields the user changed since).
	p.syncSceneEditingFields()
	p.putScene(*sc)
}

func (p *Project) startEditingScene(id uuid.UUID, name string) {
	p.editingSceneID = id
	p.editingSceneName = name
}

func (p *Project) commitSceneEdit(ctx app.Context) {
	if p.editingSceneID == uuid.Nil {
		return
	}
	id := p.editingSceneID
	name := p.editingSceneName
	p.editingSceneID = uuid.Nil
	p.editingSceneName = ""

	idx := p.sceneIdx(id)
	if idx < 0 {
		return
	}
	p.scenes[idx].Name = name
	if p.selectedSceneID == id {
		p.sceneEdit.name = name
	}
	p.putScene(p.scenes[idx])
}

// putScene saves the Scene's properties at its known version.
func (p *Project) putScene(sc apiv0.SceneResponse) {
	req := apiv0.UpdateSceneRequest{
		Name:           sc.Name,
		Width:          sc.Width,
		Height:         sc.Height,
		BackgroundHTML: sc.BackgroundHTML,
		Version:        p.sceneVersions.Of(sc.ID),
	}
	mark := p.markScenes()
	content := p.content
	if content == nil {
		return // no project shown
	}
	uiutil.Call(p.compoCtx, func(c context.Context) (apiv0.UpdateSceneResponse, error) {
		return content.UpdateScene(c, sc.ID, req)
	}, func(ctx app.Context, updated apiv0.UpdateSceneResponse) {
		// The list holds the properties as saved.
		p.sceneVersions.Observe(mark.versions, sc.ID, updated.Version)
		p.sceneStates.Observe(mark.states, sc.ID, updated.Version)
	}, p.rejectedSceneEdit(sc.ID, p.loadScenes))
}
