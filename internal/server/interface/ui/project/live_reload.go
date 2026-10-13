package project

import (
	"context"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v10/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Live reload ───────────────────────────────────────────────────────────────
//
// Every server-backed list of the Project mode is (re)loaded through a
// uiutil.Reloader, both after the user's own mutations and on server events
// (widget types, scenes, widgets and tags changed by anyone — another tab,
// growctl, a Device). Editors merge fresh data field by field: a field the user
// has not edited follows the server, an unsaved edit is kept.

func (p *Project) handleServerEvents(ctx app.Context) {
	ctx.Handle(eventlog.ActionServerEvent, func(ctx app.Context, a app.Action) {
		ev, ok := a.Value.(eventlog.ServerEvent)
		if !ok {
			return
		}
		if ev.Type == eventlog.EventResync {
			p.sceneVersions.Forget() // events were lost: the server may have gone back
		}
		targets := eventlog.ReloadTargetsFor(ev.Type)
		if targets.WidgetTypes {
			p.loadWidgetTypes(ctx)
		}
		if targets.Scenes {
			p.loadScenes(ctx)
		}
		if targets.Widgets {
			p.loadWidgets(ctx)
		}
		if targets.Tags {
			p.loadTags(ctx)
		}
	})
}

// ── Widget types ──────────────────────────────────────────────────────────────

func (p *Project) loadWidgetTypes(ctx app.Context) {
	uiutil.Load(p.compoCtx, &p.widgetTypesReloader, p.client.ListWidgetTypes, func(ctx app.Context, list []apiv0.WidgetTypeResponse) {
		p.widgetTypes = list
	}, p.loadWidgetTypes)
}

// ── Scenes ────────────────────────────────────────────────────────────────────

func (p *Project) loadScenes(ctx app.Context) {
	uiutil.Load(p.compoCtx, &p.scenesReloader, p.client.ListScenes, p.setScenes, p.loadScenes)
}

// setScenes replaces the scene list, keeps a valid selection (falling back to
// the first scene) and merges the scene properties editor.
func (p *Project) setScenes(ctx app.Context, scenes []apiv0.SceneResponse) {
	old, known := p.findScene(p.selectedSceneID)
	prevSceneID := p.selectedSceneID

	for _, sc := range scenes {
		p.sceneVersions.Observe(sc.ID, sc.Version)
	}
	p.scenes = scenes

	if _, exists := p.findScene(p.selectedSceneID); p.selectedSceneID != uuid.Nil && !exists {
		p.selectedSceneID = uuid.Nil
		p.clearWidgetSelection(ctx)
	}
	if p.selectedSceneID == uuid.Nil && len(scenes) > 0 {
		p.selectedSceneID = scenes[0].ID
	}

	if current, ok := p.findScene(p.selectedSceneID); ok && known && p.selectedSceneID == prevSceneID {
		p.mergeSceneEditingFields(old, current)
	} else {
		p.syncSceneEditingFields()
	}

	if p.selectedSceneID != prevSceneID {
		uiutil.StoreID(ctx, sceneIDKey, p.selectedSceneID)
		if p.selectedSceneID != uuid.Nil {
			p.loadWidgets(ctx)
		}
	}
}

// mergeSceneEditingFields updates each scene property field that still holds
// the old server value.
func (p *Project) mergeSceneEditingFields(old, current apiv0.SceneResponse) {
	p.editingScenePropsName = uiutil.MergeField(p.editingScenePropsName, old.Name, current.Name)
	p.editingScenePropsWidth = uiutil.MergeField(p.editingScenePropsWidth, strconv.Itoa(old.Width), strconv.Itoa(current.Width))
	p.editingScenePropsHeight = uiutil.MergeField(p.editingScenePropsHeight, strconv.Itoa(old.Height), strconv.Itoa(current.Height))
	p.editingScenePropsBG = uiutil.MergeField(p.editingScenePropsBG, old.BackgroundHTML, current.BackgroundHTML)
}

// ── Widgets ───────────────────────────────────────────────────────────────────

// isInteracting reports whether a widget is being dragged, rotated or resized
// on the canvas; replacing p.widgets then would make it jump under the cursor.
func (p *Project) isInteracting() bool {
	return p.draggedWidgetID() != uuid.Nil
}

// loadWidgets (re)loads the widgets of the selected scene. During a canvas
// interaction the reload is deferred until it ends (finalizeAllDrags).
func (p *Project) loadWidgets(ctx app.Context) {
	ctx = p.compoCtx
	if p.selectedSceneID == uuid.Nil {
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
	uiutil.Do(ctx, func(c context.Context) ([]apiv0.WidgetResponse, error) {
		return p.client.ListWidgets(c, sceneID)
	}, func(ctx app.Context, widgets []apiv0.WidgetResponse, err error) {
		rerun := p.widgetsReloader.Done()
		switch {
		case p.selectedSceneID != sceneID:
			rerun = true // the selection changed meanwhile: load the new scene
		case err != nil:
			uiutil.ShowError(ctx, err)
		case p.isInteracting():
			p.widgetsReloadDeferred = true
		default:
			p.setWidgets(ctx, sceneID, widgets)
		}
		if rerun {
			p.loadWidgets(ctx)
		}
	})
}

// setWidgets replaces the selected scene's widgets and merges the widget
// properties editor.
func (p *Project) setWidgets(ctx app.Context, sceneID uuid.UUID, widgets []apiv0.WidgetResponse) {
	old, known := p.findWidget(p.selectedWidgetID)
	p.widgets = widgets

	// Widgets carry their Scene's version.
	for _, w := range widgets {
		p.sceneVersions.Observe(sceneID, w.SceneVersion)
	}

	if p.selectedWidgetID == uuid.Nil {
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

// findWidget returns the current live widget data by ID. Use it inside event
// handler closures instead of the closure-captured `w`, because go-app may
// not re-attach handlers when only the transform changes, leaving `w` stale
// across drags.
// widgetEditFields is the widget properties editor's text, formatted as the
// editor shows it.
type widgetEditFields struct {
	name, posX, posY, posZ, width, height, originX, originY, rotation string
}

func editFieldsOf(w apiv0.WidgetResponse) widgetEditFields {
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

func (p *Project) syncEditingFields(w apiv0.WidgetResponse) {
	p.setEditFields(editFieldsOf(w))
}

// mergeWidgetEditingFields updates each widget property field that still holds
// the old server value.
func (p *Project) mergeWidgetEditingFields(old, current apiv0.WidgetResponse) {
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
	uiutil.Load(p.compoCtx, &p.tagsReloader, p.client.ListTags, p.setTags, p.loadTags)
}

// setTags replaces the tag list and drops the selection if its tag is gone.
func (p *Project) setTags(ctx app.Context, tags []apiv0.TagResponse) {
	p.tags = tags
	if p.selectedTagID == uuid.Nil {
		return
	}
	for _, t := range tags {
		if t.ID == p.selectedTagID {
			return
		}
	}
	p.selectedTagID = uuid.Nil
	uiutil.StoreID(ctx, tagIDKey, uuid.Nil)
}
