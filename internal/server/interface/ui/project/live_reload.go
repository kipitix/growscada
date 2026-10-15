package project

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v10/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Live reload ───────────────────────────────────────────────────────────────
//
// Every server-backed list of the Project mode is (re)loaded through a
// uiutil.Reloader, both after the user's own mutations and on server events
// (widget types, scenes, widgets and tags changed by anyone — another tab,
// growctl, a Device); the new state of a Tag carried by tag_updated is applied
// without a reload. Editors merge fresh data field by field: a field the user
// has not edited follows the server, an unsaved edit is kept.

func (p *Project) handleServerEvents(ctx app.Context) {
	ctx.Handle(eventlog.ActionServerEvent, func(ctx app.Context, a app.Action) {
		ev, ok := a.Value.(apiv0.Event)
		if !ok {
			return
		}
		if ev.Type == apiv0.EventTagUpdated && ev.Tag != nil {
			p.tags = p.tags.Apply(*ev.Tag)
		}
		if ev.Type == eventlog.EventResync {
			// Events were lost: the server may have gone back.
			p.sceneVersions.Forget()
			p.tags = p.tags.ForgetVersions()
		}
		targets := eventlog.ReloadTargetsFor(ev)
		if targets.WidgetTypes {
			p.loadWidgetTypes(ctx)
		}
		if targets.Scenes {
			p.loadScenes(ctx)
		}
		if targets.Scene != uuid.Nil {
			p.loadScene(ctx, targets.Scene)
		}
		if targets.Widgets || (targets.Scene != uuid.Nil && targets.Scene == p.selectedSceneID) {
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
	mark := p.sceneVersions.Mark()
	uiutil.Load(p.compoCtx, &p.scenesReloader, p.client.ListScenes, func(ctx app.Context, scenes []apiv0.SceneResponse) {
		p.setScenes(ctx, mark, scenes)
	}, p.loadScenes)
}

// setScenes replaces the scene list, keeps a valid selection (falling back to
// the first scene) and merges the scene properties editor. mark is the moment
// the list was requested.
func (p *Project) setScenes(ctx app.Context, mark uiutil.Mark, scenes []apiv0.SceneResponse) {
	old, known := p.findScene(p.selectedSceneID)
	prevSceneID := p.selectedSceneID

	for _, sc := range scenes {
		p.sceneVersions.Observe(mark, sc.ID, sc.Version)
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

// loadScene rereads the Scene sceneID, whose own state a change of one of its
// Widgets made stale (its version), and puts it in place in the list. Only
// scene events and a resync change which Scenes the list holds: a Scene not
// in it, or gone meanwhile (404), is left alone.
func (p *Project) loadScene(ctx app.Context, sceneID uuid.UUID) {
	if p.sceneReloaders == nil {
		p.sceneReloaders = make(map[uuid.UUID]*uiutil.Reloader)
	}
	r, ok := p.sceneReloaders[sceneID]
	if !ok {
		r = &uiutil.Reloader{}
		p.sceneReloaders[sceneID] = r
	}
	if !r.Start() {
		return
	}
	mark := p.sceneVersions.Mark()
	uiutil.Do(p.compoCtx, func(c context.Context) (apiv0.SceneResponse, error) {
		return p.client.GetScene(c, sceneID)
	}, func(ctx app.Context, sc apiv0.SceneResponse, err error) {
		rerun := r.Done()
		switch {
		case errors.Is(err, apiclient.ErrNotFound):
		case err != nil:
			uiutil.ShowError(ctx, err)
		default:
			p.setScene(mark, sc)
		}
		if rerun {
			p.loadScene(ctx, sceneID)
			return
		}
		delete(p.sceneReloaders, sceneID)
	})
}

// setScene puts the reread Scene sc in place in the list, if the list holds
// it, and merges the scene properties editor when it is the selected one.
// mark is the moment it was requested.
func (p *Project) setScene(mark uiutil.Mark, sc apiv0.SceneResponse) {
	i := uiutil.IdxByID(p.scenes, sc.ID, uiutil.SceneID)
	if i < 0 {
		return
	}
	p.sceneVersions.Observe(mark, sc.ID, sc.Version)
	old := p.scenes[i]
	scenes := slices.Clone(p.scenes)
	scenes[i] = sc
	p.scenes = scenes
	if sc.ID == p.selectedSceneID {
		p.mergeSceneEditingFields(old, sc)
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
	mark := p.sceneVersions.Mark()
	uiutil.LoadFor(ctx, &p.widgetsReloader, func() uuid.UUID { return p.selectedSceneID }, p.client.ListWidgets,
		func(ctx app.Context, sceneID uuid.UUID, widgets []apiv0.WidgetResponse) {
			if p.isInteracting() {
				p.widgetsReloadDeferred = true
				return
			}
			p.setWidgets(ctx, mark, sceneID, widgets)
		}, p.loadWidgets)
}

// setWidgets replaces the selected scene's widgets and merges the widget
// properties editor. mark is the moment the list was requested.
func (p *Project) setWidgets(ctx app.Context, mark uiutil.Mark, sceneID uuid.UUID, widgets []apiv0.WidgetResponse) {
	old, known := p.findWidget(p.selectedWidgetID)
	p.widgets = widgets

	// Widgets carry their Scene's version.
	for _, w := range widgets {
		p.sceneVersions.Observe(mark, sceneID, w.SceneVersion)
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

// setTags merges the loaded tag list into the known Tags (a newer state from
// a tag_updated wins) and drops the selection if its tag is gone.
func (p *Project) setTags(ctx app.Context, tags []apiv0.TagResponse) {
	p.tags = p.tags.Merge(tags)
	if _, ok := p.tags.Find(p.selectedTagID); ok || p.selectedTagID == uuid.Nil {
		return
	}
	p.selectedTagID = uuid.Nil
	uiutil.StoreID(ctx, tagIDKey, uuid.Nil)
}
