package project

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/tagstate"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// draftVersion is the Version of the Draft every write of its content states
// (ADR 0016): what the responses seen so far knew. 0 asks the client to read
// it.
func (p *Project) draftVersion() int { return p.draftVersions.Of(p.projectID) }

// markDraft is the moment a request carrying the Draft's Version is sent; its
// response observes the version with it (observeDraftVersion).
func (p *Project) markDraft() uiutil.Mark { return p.draftVersions.Mark() }

// observeDraftVersion records version of the Draft stated by a response to a
// request sent at mark. A response sent before a forget does not bring the
// forgotten version back, and a version never goes back on its own
// (uiutil.Versions): the next reread sets it after a resync or a conflict.
func (p *Project) observeDraftVersion(mark uiutil.Mark, version int) {
	p.draftVersions.Observe(mark, p.projectID, version)
}

// forgetDraftVersion forgets the Draft's Version, which the server may have
// taken back: a conflict showed this client an older one, or the stream
// resynced. The next response sets it anew.
func (p *Project) forgetDraftVersion() { p.draftVersions.ForgetID(p.projectID) }

// rejectedDraftVersion is the error handler of a write that states the Draft's
// Version: the toast, and on a conflict the Version dropped and reread, so the
// next try states the server's one.
func (p *Project) rejectedDraftVersion(ctx app.Context, err error) {
	p.rejectedDraftWrite(ctx, err, nil)
}

// rejectedDraftWrite is rejectedDraftVersion with reload, a list the conflict
// made stale, reread after the Draft's Version.
func (p *Project) rejectedDraftWrite(ctx app.Context, err error, reload func(app.Context)) {
	uiutil.ShowError(ctx, err)
	if !errors.Is(err, apiclient.ErrConflict) {
		return
	}
	p.forgetDraftVersion()
	p.loadDraftState(ctx)
	if reload != nil {
		reload(ctx)
	}
}

// loadDraftState (re)loads the Version of the Draft: what every content write
// states (ADR 0016).
func (p *Project) loadDraftState(ctx app.Context) {
	if p.content == nil {
		return
	}
	mark := p.markDraft()
	uiutil.LoadOfProject(p.compoCtx, &p.draftReloader, p.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) (apiv0.DraftStateResponse, error) {
			return pc.DraftState(c)
		},
		func(ctx app.Context, state apiv0.DraftStateResponse) {
			p.observeDraftVersion(mark, state.Version)
		}, p.loadDraftState)
}

// loadDraftChanges (re)loads the state of the Draft's DraftChange stack (ADR
// 0017): what an undo and a redo would apply, and the Version both state. It
// is read on mount, on a resync and after each content event; the answer is
// applied only when it is not behind what is known, so a read answered after a
// later write does not put an older stack back. A failed read is not a toast:
// the stack is coordination, and the next event brings the truth.
func (p *Project) loadDraftChanges(ctx app.Context) {
	if p.content == nil {
		p.changes, p.changesLoaded = apiv0.DraftChangesResponse{}, false
		return
	}
	if !p.changesReloader.Start() {
		return
	}
	content := p.content
	mark := p.markDraft()
	uiutil.Do(p.compoCtx, func(c context.Context) (apiv0.DraftChangesResponse, error) {
		return content.DraftChanges(c)
	}, func(ctx app.Context, state apiv0.DraftChangesResponse, err error) {
		rerun := p.changesReloader.Done()
		if content == p.content && err == nil {
			p.applyDraftChanges(mark, state)
		}
		if rerun {
			p.loadDraftChanges(ctx)
		}
	})
}

// applyDraftChanges stores the state of the stack that a request sent at mark
// answered, unless it is behind what is known: an answer of a read sent before
// a later write, or before a resync, would put stale buttons back. The state is
// read for the buttons and their descriptions only — its Version does not raise
// the one the shown content was read at, so an edit of a widget being dragged
// is still refused by the server rather than written over another client's
// change.
func (p *Project) applyDraftChanges(mark uiutil.Mark, state apiv0.DraftChangesResponse) {
	if p.draftVersions.Forgotten(mark, p.projectID) {
		return
	}
	if known := p.draftVersions.Of(p.projectID); known > state.Version {
		return
	}
	p.changes, p.changesLoaded = state, true
}

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
	mark := p.markDraft()
	uiutil.LoadOfProject(p.compoCtx, &p.pinsReloader, p.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) ([]apiv0.LibraryPinResponse, error) {
			return pc.ListLibraryPins(c)
		},
		func(ctx app.Context, pins []apiv0.LibraryPinResponse) {
			p.pins = pins
			for _, pin := range pins {
				p.observeDraftVersion(mark, pin.Version)
			}
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
	mark := p.markDraft()
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
// which Scenes there are; one Version serves the whole Draft (ADR 0016), so
// the list states it once and a list answered before a newer state of it does
// not replace what is shown. mark is the moment the list was requested.
func (p *Project) setScenes(ctx app.Context, mark uiutil.Mark, scenes []apiv0.SceneResponse) {
	old, known := p.findScene(p.selectedSceneID)
	prevSceneID := p.selectedSceneID

	// A newer Version while the selected Scene's Widgets are shown means they
	// may have changed: they are read again.
	fresh := len(scenes) > 0 && p.draftVersions.Advance(mark, p.projectID, scenes[0].Version)
	if len(scenes) > 0 {
		p.reloadShownWidgets(ctx, p.selectedSceneID, fresh)
	}

	merged := make([]apiv0.SceneResponse, len(scenes))
	for i, sc := range scenes {
		if !fresh {
			if cur, ok := p.findScene(sc.ID); ok {
				sc = cur
			}
		}
		merged[i] = sc
	}
	p.scenes = merged

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
	mark := p.markDraft()
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

	// Widgets carry the Draft's Version; with none, the Scene is reread for
	// it, which also keeps an empty Scene's state in step.
	for _, w := range widgets {
		p.observeDraftVersion(mark, w.Version)
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
	draftMark := p.markDraft()
	p.tagLists++
	n := p.tagLists
	uiutil.LoadOfProject(p.compoCtx, &p.tagsReloader, p.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) ([]apiv0.TagResponse, error) {
			return pc.ListTags(c)
		},
		func(ctx app.Context, tags []apiv0.TagResponse) {
			p.setTags(ctx, mark, draftMark, n, tags)
		}, p.loadTags)
}

// setTags merges the loaded tag list into the known Tags (a newer state from
// a tag_updated wins) and drops the selection if its tag is gone. mark is the
// moment the list was requested, draftMark that of its Draft Version, n its
// number (tagLists): a list sent before the selected Tag was created leaves
// the selection to the next one.
func (p *Project) setTags(ctx app.Context, mark tagstate.Mark, draftMark uiutil.Mark, n uint64, tags []apiv0.TagResponse) {
	p.tags = p.tags.Merge(mark, tags)
	for _, found := range tags {
		p.observeDraftVersion(draftMark, found.Version)
	}
	if _, ok := p.tags.Find(p.selectedTagID); ok || p.selectedTagID == uuid.Nil || n <= p.tagCreated {
		return
	}
	p.selectedTagID = uuid.Nil
	uiutil.StoreID(ctx, p.selectionKey(tagIDKey), uuid.Nil)
}
