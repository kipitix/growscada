package project

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

// ── Scene data loading ────────────────────────────────────────────────────────

func (p *Project) loadScenes(ctx app.Context) {
	url := p.apiServerURL + "/api/v1/scenes"
	ctx.Async(func() {
		resp, err := http.Get(url)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		defer resp.Body.Close()
		var result getScenesResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		ctx.Dispatch(func(ctx app.Context) {
			prevSceneID := p.selectedSceneID
			p.scenes = result.Scenes
			if p.selectedSceneID == "" && len(result.Scenes) > 0 {
				p.selectedSceneID = result.Scenes[0].ID
			}
			found := false
			for _, sc := range result.Scenes {
				if sc.ID == p.selectedSceneID {
					found = true
					break
				}
			}
			if !found {
				p.selectedSceneID = ""
				p.clearWidgetSelection()
			}
			// Reload widgets whenever the active scene changes (including initial load).
			if p.selectedSceneID != prevSceneID {
				p.loadWidgets(ctx)
			}
		})
	})
}

// ── Scene CRUD ────────────────────────────────────────────────────────────────

func (p *Project) createScene(ctx app.Context) {
	url := p.apiServerURL + "/api/v1/scenes"
	body, _ := json.Marshal(createSceneRequest{Name: "New Scene", Width: 1920, Height: 1080})
	ctx.Async(func() {
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		defer resp.Body.Close()
		var result createSceneResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		ctx.Dispatch(func(ctx app.Context) {
			p.selectedSceneID = result.ID
			p.widgets = nil // clear widgets from previous scene immediately
			p.loadScenes(ctx)
			p.loadWidgets(ctx) // new scene has no widgets; clears stale list
		})
	})
}

func (p *Project) deleteScene(ctx app.Context, sceneID string) {
	url := p.apiServerURL + "/api/v1/scenes/" + sceneID
	ctx.Async(func() {
		req, _ := http.NewRequest(http.MethodDelete, url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		resp.Body.Close()
		ctx.Dispatch(func(ctx app.Context) {
			p.loadScenes(ctx)
			p.loadWidgets(ctx)
		})
	})
}

func (p *Project) startEditingScene(id, name string) {
	p.editingSceneID = id
	p.editingSceneName = name
}

func (p *Project) commitSceneEdit(ctx app.Context) {
	if p.editingSceneID == "" {
		return
	}
	id := p.editingSceneID
	name := p.editingSceneName
	p.editingSceneID = ""
	p.editingSceneName = ""

	var sc sceneItem
	for _, s := range p.scenes {
		if s.ID == id {
			sc = s
			break
		}
	}
	if sc.ID == "" {
		return
	}
	for i, s := range p.scenes {
		if s.ID == id {
			p.scenes[i].Name = name
			break
		}
	}
	url := p.apiServerURL + "/api/v1/scenes/" + id
	body, _ := json.Marshal(updateSceneRequest{
		Name:           name,
		Width:          sc.Width,
		Height:         sc.Height,
		BackgroundHTML: sc.BackgroundHTML,
	})
	ctx.Async(func() {
		req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		resp.Body.Close()
		ctx.Dispatch(func(ctx app.Context) { p.fetchErr = "" })
	})
}
