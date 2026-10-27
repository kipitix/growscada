package project

import (
	"context"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/tagstate"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Widget types and Library Pins ─────────────────────────────────────────────

// loadWidgetTypes (re)loads the WidgetTypes of the releases the project pins:
// the palette, and what its Widgets are of (ADR 0007).
func (p *Project) loadWidgetTypes(ctx app.Context) {
	if p.content == nil {
		p.widgetTypes = nil
		return
	}
	uiutil.LoadOfProject(p.compoCtx, &p.widgetTypesReloader, p.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) ([]apiv0.ReleasedWidgetTypeResponse, error) {
			return pc.ListWidgetTypes(c)
		},
		func(ctx app.Context, list []apiv0.ReleasedWidgetTypeResponse) {
			p.widgetTypes = list
		}, p.loadWidgetTypes)
}

// loadPins (re)loads the project's Library Pins.
func (p *Project) loadPins(ctx app.Context) {
	if p.content == nil {
		p.pins = nil
		return
	}
	uiutil.LoadOfProject(p.compoCtx, &p.pinsReloader, p.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) ([]apiv0.LibraryPinResponse, error) {
			return pc.ListLibraryPins(c)
		},
		func(ctx app.Context, pins []apiv0.LibraryPinResponse) {
			p.pins = pins
		}, p.loadPins)
}

// loadLibraries (re)loads the server's libraries: the names of the pinned
// ones and the releases a pin may take.
func (p *Project) loadLibraries(ctx app.Context) {
	uiutil.Load(p.compoCtx, &p.librariesReloader, p.client.ListLibraries, func(ctx app.Context, list []apiv0.LibraryResponse) {
		p.libraries = list
	}, p.loadLibraries)
}

// ── Scenes ────────────────────────────────────────────────────────────────────

// The lists of the project's content belong to its client (Project.content):
// a response of a project no longer shown is dropped.

// projectContent returns the client of the content of the project shown.
func (p *Project) projectContent() *apiclient.ProjectClient { return p.content }

func (p *Project) loadScenes(ctx app.Context) {
	if p.content == nil {
		p.scenes = nil
		return
	}
	mark := p.markScenes()
	uiutil.LoadOfProject(p.compoCtx, &p.scenesReloader, p.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) ([]apiv0.SceneResponse, error) {
			return pc.ListScenes(c)
		},
		func(ctx app.Context, scenes []apiv0.SceneResponse) {
			p.setScenes(ctx, mark, scenes)
		}, p.loadScenes)
}

// setScenes replaces the scene list, keeps a valid selection (falling back to
// the first scene) and merges the scene properties editor. The list decides
// which Scenes there are; a Scene's state held is kept unless the list's is
// newer (it may have been read before a save of it). mark is the moment the
// list was requested.
func (p *Project) setScenes(ctx app.Context, mark sceneMark, scenes []apiv0.SceneResponse) {
	old, known := p.findScene(p.selectedSceneID)
	prevSceneID := p.selectedSceneID

	merged := make([]apiv0.SceneResponse, len(scenes))
	for i, sc := range scenes {
		if !p.sceneStates.Advance(mark.states, sc.ID, sc.Version) {
			if cur, ok := p.findScene(sc.ID); ok {
				sc = cur
			}
		}
		merged[i] = sc
	}
	p.scenes = merged
	for _, sc := range scenes {
		p.observeSceneVersion(ctx, mark.versions, sc.ID, sc.Version)
	}

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
		uiutil.StoreID(ctx, p.selectionKey(sceneIDKey), p.selectedSceneID)
		if p.selectedSceneID != uuid.Nil {
			p.loadWidgets(ctx)
		}
	}
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
	if p.selectedSceneID == uuid.Nil || p.content == nil {
		p.widgets = nil
		return
	}
	if p.isInteracting() {
		p.widgetsReloadDeferred = true
		return
	}
	mark := p.sceneVersions.Mark()
	uiutil.LoadFor(ctx, &p.widgetsReloader,
		func() uiutil.SceneOfProject {
			return uiutil.SceneOfProject{Content: p.content, SceneID: p.selectedSceneID}
		},
		uiutil.ListWidgets,
		func(ctx app.Context, k uiutil.SceneOfProject, widgets []apiv0.WidgetResponse) {
			if p.isInteracting() {
				p.widgetsReloadDeferred = true
				return
			}
			p.setWidgets(ctx, mark, k.SceneID, widgets)
		}, p.loadWidgets)
}

// setWidgets replaces the selected scene's widgets and merges the widget
// properties editor. mark is the moment the list was requested.
func (p *Project) setWidgets(ctx app.Context, mark uiutil.Mark, sceneID uuid.UUID, widgets []apiv0.WidgetResponse) {
	old, known := p.findWidget(p.selectedWidgetID)
	p.widgets = widgets

	// Widgets carry their Scene's version; with none, the Scene is reread
	// for it.
	for _, w := range widgets {
		p.sceneVersions.Observe(mark, sceneID, w.SceneVersion)
	}
	if len(widgets) == 0 {
		p.loadScene(ctx, sceneID)
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

// ── Tags ──────────────────────────────────────────────────────────────────────

func (p *Project) loadTags(ctx app.Context) {
	if p.content == nil {
		p.tags = tagstate.Tags{}
		return
	}
	mark := p.tags.Mark()
	p.tagLists++
	n := p.tagLists
	uiutil.LoadOfProject(p.compoCtx, &p.tagsReloader, p.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) ([]apiv0.TagResponse, error) {
			return pc.ListTags(c)
		},
		func(ctx app.Context, tags []apiv0.TagResponse) {
			p.setTags(ctx, mark, n, tags)
		}, p.loadTags)
}

// setTags merges the loaded tag list into the known Tags (a newer state from
// a tag_updated wins) and drops the selection if its tag is gone. mark is the
// moment the list was requested, n its number (tagLists): a list sent before
// the selected Tag was created leaves the selection to the next one.
func (p *Project) setTags(ctx app.Context, mark tagstate.Mark, n uint64, tags []apiv0.TagResponse) {
	p.tags = p.tags.Merge(mark, tags)
	if _, ok := p.tags.Find(p.selectedTagID); ok || p.selectedTagID == uuid.Nil || n <= p.tagCreated {
		return
	}
	p.selectedTagID = uuid.Nil
	uiutil.StoreID(ctx, p.selectionKey(tagIDKey), uuid.Nil)
}
