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

// loadDraftState (re)loads the state of the Draft: the Version every content
// write states (ADR 0016) and whether it holds changes its last saved state
// does not (task 74). It is read on mount, on a resync, after each content
// event and after each command — the points loadDraftChanges is read at.
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
			p.applyDraftState(mark, state)
		}, p.loadDraftState)
}

// applyDraftState stores the unsaved-changes flag a request sent at mark
// answered, under the same guard as applyDraftChanges: an answer of a read sent
// before a later write, or before a resync, would put a stale badge back.
func (p *Project) applyDraftState(mark uiutil.Mark, state apiv0.DraftStateResponse) {
	if p.draftVersions.Forgotten(mark, p.projectID) {
		return
	}
	if known := p.draftVersions.Of(p.projectID); known > state.Version {
		return
	}
	p.draftUnsaved, p.draftStateLoaded = state.HasUnsavedChanges, true
}

// loadUnsavedChanges (re)loads whether the Draft holds changes its last saved
// state does not, from the Draft's state, after a content event of another tab.
// The answer states the server's Version too, but that is not the Version the
// shown content was read at: advancing it would let a write of the stale
// content overwrite the other tab's change instead of being refused as a
// conflict (task 72_6), so only the flag is applied (task 74).
func (p *Project) loadUnsavedChanges(ctx app.Context) {
	if p.content == nil {
		return
	}
	content := p.content
	mark := p.markDraft()
	uiutil.LoadOfProject(p.compoCtx, &p.unsavedReloader, p.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) (apiv0.DraftStateResponse, error) {
			return pc.DraftState(c)
		},
		func(ctx app.Context, state apiv0.DraftStateResponse) {
			if content != p.content {
				return
			}
			p.applyDraftState(mark, state)
		}, p.loadUnsavedChanges)
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

// ── Widget types and Library Imports ──────────────────────────────────────────

// loadWidgetTypes (re)loads the WidgetTypes of the saved states the project
// imports: the palette, and what its Widgets are of (ADR 0007).
func (p *Project) loadWidgetTypes(ctx app.Context) {
	if p.content == nil {
		p.widgetTypes = nil
		return
	}
	uiutil.LoadOfProject(p.compoCtx, &p.widgetTypesReloader, p.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) ([]apiv0.WidgetTypeCopyResponse, error) {
			return pc.ImportedWidgetTypes(c)
		},
		func(ctx app.Context, list []apiv0.WidgetTypeCopyResponse) {
			p.widgetTypes = list
		}, p.loadWidgetTypes)
}

// loadImports (re)loads the project's Library Imports (task 85_6).
func (p *Project) loadImports(ctx app.Context) {
	if p.content == nil {
		p.imports = nil
		return
	}
	mark := p.markDraft()
	uiutil.LoadOfProject(p.compoCtx, &p.importsReloader, p.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) ([]apiv0.LibraryImportResponse, error) {
			return pc.Imports(c)
		},
		func(ctx app.Context, imports []apiv0.LibraryImportResponse) {
			p.imports = imports
			for _, imported := range imports {
				p.observeDraftVersion(mark, imported.Version)
			}
		}, p.loadImports)
}

// loadLibraries (re)loads the server's libraries: the names of the imported
// ones and the saved states an import may take.
func (p *Project) loadLibraries(ctx app.Context) {
	uiutil.Load(p.compoCtx, &p.librariesReloader, p.client.ListLibraries, func(ctx app.Context, list []apiv0.LibraryResponse) {
		p.libraries, p.librariesLoaded = list, true
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
	replacements := p.contentReplacements
	uiutil.LoadOfProject(p.compoCtx, &p.scenesReloader, p.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) ([]apiv0.SceneResponse, error) {
			return pc.ListScenes(c)
		},
		func(ctx app.Context, scenes []apiv0.SceneResponse) {
			p.setScenes(ctx, mark, replacements, scenes)
		}, p.loadScenes)
}

// setScenes replaces the scene list, keeps a valid selection (falling back to
// the first scene) and merges the scene properties editor. The list decides
// which Scenes there are; one Version serves the whole Draft (ADR 0016), so
// the list states it once and a list answered before a newer state of it does
// not replace what is shown. mark is the moment the list was requested;
// replacements is how many times the Draft's content had been replaced then —
// the list of a replacement (a Discard, a Revert, an opened project file) is
// the truth whatever Version it states, because the replacement answered that
// Version itself (task 72_6).
//
// A list older than the known state does not delete a Scene known to exist
// after it was requested (uiutil.Versions.KnownAfter): a Scene created
// meanwhile is kept at the end, and the selection with it (task 61).
func (p *Project) setScenes(ctx app.Context, mark uiutil.Mark, replacements uint64, scenes []apiv0.SceneResponse) {
	old, known := p.findScene(p.selectedSceneID)
	prevSceneID := p.selectedSceneID

	// The first list requested after the whole Draft was replaced (a Discard, a
	// Revert, an opened project file) is the truth whatever Version it states:
	// the replacement answered that Version itself, so the list advances nothing
	// and the Scenes shown and the Widgets of the selected one are replaced by
	// it. Every later list — and a second answer of that same replacement — is an
	// ordinary one again, judged by its Version, or a stale answer would put the
	// Scenes shown back (task 72_6).
	replaced := replacements == p.contentReplacements && replacements > p.appliedReplacements
	if replaced {
		p.appliedReplacements = replacements
	}
	// A newer Version while the selected Scene's Widgets are shown means they
	// may have changed: they are read again.
	fresh := len(scenes) > 0 && p.draftVersions.Advance(mark, p.projectID, scenes[0].Version)
	reread := fresh || replaced
	// The list replaces the Scenes shown unless it is behind the known state:
	// such a list only brings the ids it names. A list stating the known
	// Version is the server's own — another read may have stated that Version
	// first, the Draft's state on a resync, say — and replaces what is shown;
	// an empty list states no Version and is treated as behind.
	merge := !replaced && (len(scenes) == 0 || p.draftVersions.Behind(mark, p.projectID, scenes[0].Version))

	merged := make([]apiv0.SceneResponse, len(scenes))
	listed := make(map[uuid.UUID]struct{}, len(scenes))
	for i, sc := range scenes {
		if merge {
			// A local copy is as new as the response or newer: a Scene
			// written since states the newer name and size.
			if cur, ok := p.findScene(sc.ID); ok {
				sc = cur
			}
		}
		merged[i] = sc
		listed[sc.ID] = struct{}{}
	}
	if merge {
		// A Scene the response misses was created after it was requested: it
		// is kept, at the end, until a list that names it arrives.
		for _, sc := range p.scenes {
			if _, ok := listed[sc.ID]; ok {
				continue
			}
			if !p.draftVersions.KnownAfter(mark, sc.ID) {
				continue
			}
			merged = append(merged, sc)
		}
	}
	p.scenes = merged

	if _, exists := p.findScene(p.selectedSceneID); p.selectedSceneID != uuid.Nil && !exists &&
		!p.draftVersions.KnownAfter(mark, p.selectedSceneID) {
		// The Scene shown is gone with its Widgets: nothing is left to draw.
		p.selectedSceneID = uuid.Nil
		p.widgets = nil
		p.clearWidgetSelection(ctx)
	}
	if p.selectedSceneID == uuid.Nil && len(merged) > 0 {
		p.selectedSceneID = merged[0].ID
	}

	// The Widgets of the selected Scene are read once the selection says which
	// Scene that is: a reset may have deleted the one shown.
	if len(merged) > 0 {
		p.reloadShownWidgets(ctx, p.selectedSceneID, reread)
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

// isInteracting reports whether the canvas is changing a Widget — a body,
// anchor, rotation or resize drag, or an arrow series that has not been sent
// yet; replacing p.widgets then would make it jump under the cursor.
func (p *Project) isInteracting() bool {
	return p.draggedWidgetID() != uuid.Nil || p.arrowActive || p.moveSending
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

// setWidgets replaces the selected scene's widgets, prunes the selection of
// the IDs that are gone and merges the widget properties editor for the single
// selected Widget. mark is the moment the list was requested.
//
// A list older than the known state does not delete a Widget known to exist
// after it was requested (uiutil.Versions.KnownAfter): a Widget created
// meanwhile — the list misses it — is kept, and stays selected (task 61). The
// list of another Scene, the one shown meanwhile, changes nothing here.
func (p *Project) setWidgets(ctx app.Context, mark uiutil.Mark, sceneID uuid.UUID, widgets []apiv0.WidgetResponse) {
	if sceneID != p.selectedSceneID {
		return
	}
	oldID, oldSingle := p.singleSelected()
	var old apiv0.WidgetResponse
	var known bool
	if oldSingle {
		old, known = p.findWidget(oldID)
	}
	// A list behind the known state may be behind this client's Widgets too:
	// it only brings the ids it names, while one stating the known Version is
	// the server's own (a Scene reread or the Draft's state may have observed
	// that Version first). An empty list states no Version and is treated as
	// behind.
	behind := len(widgets) == 0 || p.draftVersions.Behind(mark, p.projectID, widgets[0].Version)
	p.widgets = p.mergedWithKnownWidgets(mark, !behind, widgets)

	// Widgets carry the Draft's Version; with none, the Scene is reread for
	// it, which also keeps an empty Scene's state in step.
	for _, w := range widgets {
		p.observeDraftVersion(mark, w.Version)
	}
	if len(widgets) == 0 {
		p.loadScene(ctx, sceneID)
	}

	p.pruneSelection(ctx)
	id, single := p.singleSelected()
	if !single {
		return
	}
	current, exists := p.findWidget(id)
	if !exists {
		return
	}
	if oldSingle && oldID == id && known {
		p.mergeWidgetEditingFields(old, current)
		return
	}
	p.syncEditingFields(current)
}

// mergedWithKnownWidgets returns the Widgets a list requested at mark holds: a
// list that is not behind replaces them, while one behind only brings the IDs
// it names, each as the local copy when there is one (a Widget written since
// states newer geometry and name). A Widget the list misses stays when it is
// known to exist after the list was requested; one known only from before it,
// or from no response at all, is deleted.
func (p *Project) mergedWithKnownWidgets(mark uiutil.Mark, replace bool, widgets []apiv0.WidgetResponse) []apiv0.WidgetResponse {
	if replace {
		return widgets
	}
	merged := make([]apiv0.WidgetResponse, len(widgets))
	listed := make(map[uuid.UUID]struct{}, len(widgets))
	for i, w := range widgets {
		if cur, ok := p.findWidget(w.ID); ok {
			w = cur
		}
		merged[i] = w
		listed[w.ID] = struct{}{}
	}
	for _, w := range p.widgets {
		if _, ok := listed[w.ID]; ok {
			continue
		}
		if !p.draftVersions.KnownAfter(mark, w.ID) {
			continue
		}
		merged = append(merged, w)
	}
	return merged
}

// ── Tags ──────────────────────────────────────────────────────────────────────

func (p *Project) loadTags(ctx app.Context) {
	if p.content == nil {
		p.tags = tagstate.Tags{}
		return
	}
	mark := p.tags.Mark()
	tagMark := p.tagVersions.Mark()
	draftMark := p.markDraft()
	uiutil.LoadOfProject(p.compoCtx, &p.tagsReloader, p.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) ([]apiv0.TagResponse, error) {
			return pc.ListTags(c)
		},
		func(ctx app.Context, tags []apiv0.TagResponse) {
			p.setTags(ctx, mark, tagMark, draftMark, tags)
		}, p.loadTags)
}

// setTags merges the loaded tag list into the known Tags (a newer state from
// a tag_updated wins) and drops the selection if its tag is gone. mark is the
// moment the list was requested, tagMark that of the tag existence marks and
// draftMark that of its Draft Version: a list sent before the selected Tag was
// created does not hold it, but the Tag is known to exist after the list
// (uiutil.Versions.KnownAfter), so the selection stays and the next list
// shows it (task 61).
func (p *Project) setTags(ctx app.Context, mark tagstate.Mark, tagMark, draftMark uiutil.Mark, tags []apiv0.TagResponse) {
	p.tags = p.tags.Merge(mark, tags)
	for _, found := range tags {
		p.observeDraftVersion(draftMark, found.Version)
	}
	if _, ok := p.tags.Find(p.selectedTagID); ok || p.selectedTagID == uuid.Nil ||
		p.tagVersions.KnownAfter(tagMark, p.selectedTagID) {
		return
	}
	p.selectedTagID = uuid.Nil
	uiutil.StoreID(ctx, p.selectionKey(tagIDKey), uuid.Nil)
}
