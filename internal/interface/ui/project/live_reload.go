package project

import (
	"fmt"
	"strconv"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

	"github.com/kipitix/growscada/internal/interface/ui/eventlog"
	"github.com/kipitix/growscada/internal/interface/ui/toast"
	"github.com/kipitix/growscada/internal/interface/ui/uiutil"
)

// ── Live reload ───────────────────────────────────────────────────────────────
//
// Every server-backed list of the Project mode is (re)loaded through a
// uiutil.Reloader, both after the user's own mutations and on server events
// (widget types, scenes, widgets and tags changed by anyone — another tab,
// growctl, a Device). Editors merge fresh data field by field: a field the user
// has not edited follows the server, an unsaved edit is kept.

// reloadTargets says which lists a server event makes stale.
type reloadTargets struct {
	widgetTypes, scenes, widgets, tags bool
}

func reloadTargetsFor(eventType string) reloadTargets {
	switch eventType {
	case "widget_type_created", "widget_type_updated", "widget_type_deleted":
		return reloadTargets{widgetTypes: true}
	case "scene_created", "scene_updated", "scene_deleted":
		return reloadTargets{scenes: true}
	case "widget_created", "widget_updated", "widget_deleted":
		// A widget change bumps its Scene's version (the shared optimistic
		// lock) without a scene event, so the scenes are reloaded as well.
		return reloadTargets{scenes: true, widgets: true}
	case "tag_created", "tag_updated", "tag_deleted":
		return reloadTargets{tags: true}
	}
	return reloadTargets{}
}

func (p *Project) handleServerEvents(ctx app.Context) {
	ctx.Handle(eventlog.ActionServerEvent, func(ctx app.Context, a app.Action) {
		ev, ok := a.Value.(eventlog.ServerEvent)
		if !ok {
			return
		}
		targets := reloadTargetsFor(ev.Type)
		if targets.widgetTypes {
			p.loadWidgetTypes(ctx)
		}
		if targets.scenes {
			p.loadScenes(ctx)
		}
		if targets.widgets {
			p.loadWidgets(ctx)
		}
		if targets.tags {
			p.loadTags(ctx)
		}
	})
}

// ── Widget types ──────────────────────────────────────────────────────────────

func (p *Project) loadWidgetTypes(ctx app.Context) {
	if !p.widgetTypesReloader.Start() {
		return
	}
	url := p.apiServerURL + "/api/v1/widget-types"
	ctx.Async(func() {
		result, problem := uiutil.FetchJSON[getWidgetTypesResponse](url)
		ctx.Dispatch(func(ctx app.Context) {
			if problem != nil {
				ctx.NewActionWithValue(toast.ActionAdd, *problem)
			} else {
				p.widgetTypes = result.WidgetTypes
			}
			if p.widgetTypesReloader.Done() {
				p.loadWidgetTypes(ctx)
			}
		})
	})
}

// ── Scenes ────────────────────────────────────────────────────────────────────

func (p *Project) loadScenes(ctx app.Context) {
	if !p.scenesReloader.Start() {
		return
	}
	url := p.apiServerURL + "/api/v1/scenes"
	ctx.Async(func() {
		result, problem := uiutil.FetchJSON[getScenesResponse](url)
		ctx.Dispatch(func(ctx app.Context) {
			if problem != nil {
				ctx.NewActionWithValue(toast.ActionAdd, *problem)
			} else {
				p.setScenes(ctx, result.Scenes)
			}
			if p.scenesReloader.Done() {
				p.loadScenes(ctx)
			}
		})
	})
}

// setScenes replaces the scene list, keeps a valid selection (falling back to
// the first scene) and merges the scene properties editor.
func (p *Project) setScenes(ctx app.Context, scenes []sceneItem) {
	old, known := p.findScene(p.selectedSceneID)
	prevSceneID := p.selectedSceneID

	// A version never goes back: a response that raced with our own mutation
	// must not undo the version bump that mutation already recorded.
	for i := range scenes {
		if prev, ok := p.findScene(scenes[i].ID); ok && prev.Version > scenes[i].Version {
			scenes[i].Version = prev.Version
		}
	}
	p.scenes = scenes

	if _, exists := p.findScene(p.selectedSceneID); p.selectedSceneID != "" && !exists {
		p.selectedSceneID = ""
		p.clearWidgetSelection(ctx)
	}
	if p.selectedSceneID == "" && len(scenes) > 0 {
		p.selectedSceneID = scenes[0].ID
	}

	if current, ok := p.findScene(p.selectedSceneID); ok && known && p.selectedSceneID == prevSceneID {
		p.mergeSceneEditingFields(old, current)
	} else {
		p.syncSceneEditingFields()
	}

	if p.selectedSceneID != prevSceneID {
		ctx.LocalStorage().Set("project:sceneID", p.selectedSceneID)
		if p.selectedSceneID != "" {
			p.loadWidgets(ctx)
		}
	}
}

func (p *Project) findScene(id string) (sceneItem, bool) {
	for _, sc := range p.scenes {
		if sc.ID == id {
			return sc, true
		}
	}
	return sceneItem{}, false
}

// mergeSceneEditingFields updates each scene property field that still holds
// the old server value.
func (p *Project) mergeSceneEditingFields(old, current sceneItem) {
	p.editingScenePropsName = uiutil.MergeField(p.editingScenePropsName, old.Name, current.Name)
	p.editingScenePropsWidth = uiutil.MergeField(p.editingScenePropsWidth, strconv.Itoa(old.Width), strconv.Itoa(current.Width))
	p.editingScenePropsHeight = uiutil.MergeField(p.editingScenePropsHeight, strconv.Itoa(old.Height), strconv.Itoa(current.Height))
	p.editingScenePropsBG = uiutil.MergeField(p.editingScenePropsBG, old.BackgroundHTML, current.BackgroundHTML)
}

// ── Widgets ───────────────────────────────────────────────────────────────────

// isInteracting reports whether a widget is being dragged, rotated or resized
// on the canvas; replacing p.widgets then would make it jump under the cursor.
func (p *Project) isInteracting() bool {
	return p.draggingWidgetID != "" || p.draggingOriginID != "" ||
		p.rotatingWidgetID != "" || p.resizingWidgetID != ""
}

// loadWidgets (re)loads the widgets of the selected scene. During a canvas
// interaction the reload is deferred until it ends (finalizeAllDrags).
func (p *Project) loadWidgets(ctx app.Context) {
	if p.selectedSceneID == "" {
		p.widgets = nil
		return
	}
	if p.isInteracting() {
		p.widgetsReloadDeferred = true
		return
	}
	if !p.widgetsReloader.Start() {
		return
	}
	sceneID := p.selectedSceneID
	url := p.apiServerURL + "/api/v1/scenes/" + sceneID + "/widgets"
	ctx.Async(func() {
		result, problem := uiutil.FetchJSON[getWidgetsResponse](url)
		ctx.Dispatch(func(ctx app.Context) {
			rerun := p.widgetsReloader.Done()
			switch {
			case p.selectedSceneID != sceneID:
				rerun = true // the selection changed meanwhile: load the new scene
			case problem != nil:
				ctx.NewActionWithValue(toast.ActionAdd, *problem)
			case p.isInteracting():
				p.widgetsReloadDeferred = true
			default:
				p.setWidgets(ctx, sceneID, result.Widgets)
			}
			if rerun {
				p.loadWidgets(ctx)
			}
		})
	})
}

// setWidgets replaces the selected scene's widgets and merges the widget
// properties editor.
func (p *Project) setWidgets(ctx app.Context, sceneID string, widgets []widgetItem) {
	old, known := p.findWidget(p.selectedWidgetID)
	p.widgets = widgets

	// Widgets carry their Scene's version; adopt it if it is newer.
	if len(widgets) > 0 {
		if sc, ok := p.findScene(sceneID); ok && widgets[0].SceneVersion > sc.Version {
			p.applyNewSceneVersion(sceneID, widgets[0].SceneVersion)
		}
	}

	if p.selectedWidgetID == "" {
		return
	}
	current, exists := p.findWidget(p.selectedWidgetID)
	switch {
	case !exists:
		p.clearWidgetSelection(ctx)
	case !known:
		p.syncEditingFields(current)
	default:
		p.mergeWidgetEditingFields(old, current)
	}
}

func (p *Project) findWidget(id string) (widgetItem, bool) {
	for _, w := range p.widgets {
		if w.ID == id {
			return w, true
		}
	}
	return widgetItem{}, false
}

// widgetEditFields is the widget properties editor's text, formatted as the
// editor shows it.
type widgetEditFields struct {
	name, posX, posY, posZ, width, height, originX, originY, rotation string
}

func editFieldsOf(w widgetItem) widgetEditFields {
	return widgetEditFields{
		name:     w.Name,
		posX:     fmt.Sprintf("%.1f", w.Position.X),
		posY:     fmt.Sprintf("%.1f", w.Position.Y),
		posZ:     strconv.Itoa(w.Position.Z),
		width:    strconv.Itoa(w.Size.Width),
		height:   strconv.Itoa(w.Size.Height),
		originX:  fmt.Sprintf("%.3f", w.Origin.X),
		originY:  fmt.Sprintf("%.3f", w.Origin.Y),
		rotation: fmt.Sprintf("%.1f", w.Rotation.Degrees),
	}
}

func (p *Project) editFields() widgetEditFields {
	return widgetEditFields{
		name:     p.editingWidgetName,
		posX:     p.editingPosX,
		posY:     p.editingPosY,
		posZ:     p.editingPosZ,
		width:    p.editingWidth,
		height:   p.editingHeight,
		originX:  p.editingOriginX,
		originY:  p.editingOriginY,
		rotation: p.editingRotation,
	}
}

func (p *Project) setEditFields(f widgetEditFields) {
	p.editingWidgetName = f.name
	p.editingPosX = f.posX
	p.editingPosY = f.posY
	p.editingPosZ = f.posZ
	p.editingWidth = f.width
	p.editingHeight = f.height
	p.editingOriginX = f.originX
	p.editingOriginY = f.originY
	p.editingRotation = f.rotation
}

func (p *Project) syncEditingFields(w widgetItem) {
	p.setEditFields(editFieldsOf(w))
}

// mergeWidgetEditingFields updates each widget property field that still holds
// the old server value.
func (p *Project) mergeWidgetEditingFields(old, current widgetItem) {
	p.setEditFields(mergeEditFields(p.editFields(), editFieldsOf(old), editFieldsOf(current)))
}

func mergeEditFields(local, old, current widgetEditFields) widgetEditFields {
	m := uiutil.MergeField[string]
	return widgetEditFields{
		name:     m(local.name, old.name, current.name),
		posX:     m(local.posX, old.posX, current.posX),
		posY:     m(local.posY, old.posY, current.posY),
		posZ:     m(local.posZ, old.posZ, current.posZ),
		width:    m(local.width, old.width, current.width),
		height:   m(local.height, old.height, current.height),
		originX:  m(local.originX, old.originX, current.originX),
		originY:  m(local.originY, old.originY, current.originY),
		rotation: m(local.rotation, old.rotation, current.rotation),
	}
}

// ── Tags ──────────────────────────────────────────────────────────────────────

func (p *Project) loadTags(ctx app.Context) {
	if !p.tagsReloader.Start() {
		return
	}
	url := p.apiServerURL + "/api/v1/tags"
	ctx.Async(func() {
		result, problem := uiutil.FetchJSON[getTagsResponse](url)
		ctx.Dispatch(func(ctx app.Context) {
			if problem != nil {
				ctx.NewActionWithValue(toast.ActionAdd, *problem)
			} else {
				p.setTags(ctx, result.Tags)
			}
			if p.tagsReloader.Done() {
				p.loadTags(ctx)
			}
		})
	})
}

// setTags replaces the tag list and drops the selection if its tag is gone.
func (p *Project) setTags(ctx app.Context, tags []tagItem) {
	p.tags = tags
	if p.selectedTagID == "" {
		return
	}
	for _, t := range tags {
		if t.ID == p.selectedTagID {
			return
		}
	}
	p.selectedTagID = ""
	ctx.LocalStorage().Set("project:tagID", "")
}
