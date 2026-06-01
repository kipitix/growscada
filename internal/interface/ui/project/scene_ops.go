package project

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"

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
			// Validate restored/current scene ID; fall through to auto-select if invalid.
			if p.selectedSceneID != "" {
				found := false
				for _, sc := range result.Scenes {
					if sc.ID == p.selectedSceneID {
						found = true
						break
					}
				}
				if !found {
					p.selectedSceneID = ""
					p.clearWidgetSelection(ctx)
				}
			}
			if p.selectedSceneID == "" && len(result.Scenes) > 0 {
				p.selectedSceneID = result.Scenes[0].ID
			}
			p.syncSceneEditingFields()
			if p.selectedSceneID != prevSceneID {
				ctx.LocalStorage().Set("project:sceneID", p.selectedSceneID)
				if p.selectedSceneID != "" {
					p.loadWidgets(ctx)
				}
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
			ctx.LocalStorage().Set("project:sceneID", result.ID)
			p.clearWidgetSelection(ctx)
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
		})
	})
}

// syncSceneEditingFields populates the Properties-panel editing state from
// the currently selected scene. Call whenever the scene selection changes.
func (p *Project) syncSceneEditingFields() {
	for _, sc := range p.scenes {
		if sc.ID == p.selectedSceneID {
			p.editingScenePropsName = sc.Name
			p.editingScenePropsWidth = strconv.Itoa(sc.Width)
			p.editingScenePropsHeight = strconv.Itoa(sc.Height)
			p.editingScenePropsBG = sc.BackgroundHTML
			return
		}
	}
	p.editingScenePropsName = ""
	p.editingScenePropsWidth = ""
	p.editingScenePropsHeight = ""
	p.editingScenePropsBG = ""
}

// saveSceneProperties sends a PUT with the current Properties-panel state.
func (p *Project) saveSceneProperties(ctx app.Context) {
	if p.selectedSceneID == "" {
		return
	}
	id := p.selectedSceneID
	name := p.editingScenePropsName
	w, _ := strconv.Atoi(p.editingScenePropsWidth)
	h, _ := strconv.Atoi(p.editingScenePropsHeight)
	if w <= 0 {
		w = 1920
	}
	if h <= 0 {
		h = 1080
	}
	bg := p.editingScenePropsBG

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

	// Optimistic in-memory update.
	for i := range p.scenes {
		if p.scenes[i].ID == id {
			p.scenes[i].Name = name
			p.scenes[i].Width = w
			p.scenes[i].Height = h
			p.scenes[i].BackgroundHTML = bg
			break
		}
	}

	url := p.apiServerURL + "/api/v1/scenes/" + id
	body, _ := json.Marshal(updateSceneRequest{
		Name:           name,
		Width:          w,
		Height:         h,
		BackgroundHTML: bg,
		Version:        sc.Version,
	})
	ctx.Async(func() {
		req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		defer resp.Body.Close()
		var result updateSceneResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && result.Version > 0 {
			ctx.Dispatch(func(ctx app.Context) {
				for i := range p.scenes {
					if p.scenes[i].ID == id {
						p.scenes[i].Version = result.Version
						break
					}
				}
				p.fetchErr = ""
			})
		}
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
	if p.selectedSceneID == id {
		p.editingScenePropsName = name
	}
	url := p.apiServerURL + "/api/v1/scenes/" + id
	body, _ := json.Marshal(updateSceneRequest{
		Name:           name,
		Width:          sc.Width,
		Height:         sc.Height,
		BackgroundHTML: sc.BackgroundHTML,
		Version:        sc.Version,
	})
	sceneID := id
	ctx.Async(func() {
		req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		defer resp.Body.Close()
		var result updateSceneResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && result.Version > 0 {
			ctx.Dispatch(func(ctx app.Context) {
				for i := range p.scenes {
					if p.scenes[i].ID == sceneID {
						p.scenes[i].Version = result.Version
						break
					}
				}
				p.fetchErr = ""
			})
		}
	})
}
