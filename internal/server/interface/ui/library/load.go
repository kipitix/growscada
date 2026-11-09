package library

import (
	"context"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Data loading ──────────────────────────────────────────────────────────────

// draftVersion is the Version of the selected library's Draft every write of
// its content states (ADR 0016); 0 asks the client to read it.
func (l *Library) draftVersion() int { return l.draftVersions.Of(l.libraryID) }

// markDraft is the moment a request carrying the Draft's Version is sent; its
// response observes the version with it (observeDraftVersion).
func (l *Library) markDraft() uiutil.Mark { return l.draftVersions.Mark() }

// observeDraftVersion records version of the Draft, stated by a response to a
// request sent at mark. A response sent before a forget does not bring the
// forgotten version back, and a version never goes back on its own
// (uiutil.Versions): the next reread sets it after a resync or a conflict.
func (l *Library) observeDraftVersion(mark uiutil.Mark, version int) {
	l.draftVersions.Observe(mark, l.libraryID, version)
}

// forgetDraftVersion forgets the Draft's Version, which the server may have
// taken back: a conflict showed this client an older one, or the stream
// resynced. The next response sets it anew.
func (l *Library) forgetDraftVersion() { l.draftVersions.ForgetID(l.libraryID) }

// loadDraftChanges (re)loads the state of the Draft's DraftChange stack (ADR
// 0017): what an undo and a redo would apply, and the Version both state. It is
// read on mount, on a resync and after each content event; the answer is
// applied only when it is not behind what is known, so a read answered after a
// later write does not put an older stack back. A failed read is not a toast:
// the next event brings the truth.
func (l *Library) loadDraftChanges(ctx app.Context) {
	if l.draft() == nil {
		l.changes, l.changesLoaded = apiv0.DraftChangesResponse{}, false
		return
	}
	if !l.changesReloader.Start() {
		return
	}
	draft := l.draft()
	mark := l.markDraft()
	uiutil.Do(l.compoCtx, func(c context.Context) (apiv0.DraftChangesResponse, error) {
		return draft.DraftChanges(c)
	}, func(ctx app.Context, state apiv0.DraftChangesResponse, err error) {
		rerun := l.changesReloader.Done()
		if draft == l.draft() && err == nil {
			l.applyDraftChanges(mark, state)
		}
		if rerun {
			l.loadDraftChanges(ctx)
		}
	})
}

// applyDraftChanges stores the state of the stack that a request sent at mark
// answered, unless it is behind what is known: an answer of a read sent before
// a later write, or before a resync, would put stale buttons back. The state is
// read for the buttons and their descriptions only — its Version does not raise
// the one the shown WidgetTypes were read at.
func (l *Library) applyDraftChanges(mark uiutil.Mark, state apiv0.DraftChangesResponse) {
	if l.draftVersions.Forgotten(mark, l.libraryID) {
		return
	}
	if known := l.draftVersion(); known > state.Version {
		return
	}
	l.changes, l.changesLoaded = state, true
}

// loadLibraries (re)loads the library list: the header reads the selected
// one from it, and the selection page keeps its own list. A library the list
// no longer holds is left by the page, which announces another one or none
// (task 73).
func (l *Library) loadLibraries(ctx app.Context) {
	uiutil.Load(l.compoCtx, &l.librariesReloader, l.client.ListLibraries, func(ctx app.Context, list []apiv0.LibraryResponse) {
		l.libraries = list
	}, l.loadLibraries)
}

// libraryGone handles a 404 of the selected library's Draft or releases: the
// library was deleted, so it is left and the list reloaded instead of an
// error being shown (ADR 0007). The selection page picks another one, or
// none, and shows itself then (task 73).
func (l *Library) libraryGone(ctx app.Context, _ *apiclient.LibraryClient) {
	l.selectLibrary(ctx, uuid.Nil)
	l.loadLibraries(ctx)
}

// loadList (re)loads the WidgetTypes of the selected library's Draft;
// concurrent requests are coalesced, a response of another library dropped.
func (l *Library) loadList(ctx app.Context) {
	if l.libraryID == uuid.Nil {
		l.widgetTypes = nil
		return
	}
	mark := l.markDraft()
	uiutil.LoadForGone(l.compoCtx, &l.reloader, l.draft,
		func(c context.Context, lib *apiclient.LibraryClient) ([]apiv0.WidgetTypeResponse, error) {
			return lib.ListWidgetTypes(c)
		},
		func(ctx app.Context, _ *apiclient.LibraryClient, items []apiv0.WidgetTypeResponse) {
			l.setWidgetTypes(ctx, mark, items)
		}, l.libraryGone, l.loadList)
}

// loadDraftState (re)loads the Version of the selected library's Draft: the
// Draft of a library without a WidgetType carries it in no content DTO, and
// every write of the Draft states it (ADR 0016).
func (l *Library) loadDraftState(ctx app.Context) {
	if l.libraryID == uuid.Nil {
		return
	}
	mark := l.markDraft()
	uiutil.LoadForGone(l.compoCtx, &l.draftStateReloader, l.draft,
		func(c context.Context, lib *apiclient.LibraryClient) (apiv0.DraftStateResponse, error) {
			return lib.DraftState(c)
		},
		func(ctx app.Context, _ *apiclient.LibraryClient, state apiv0.DraftStateResponse) {
			l.observeDraftVersion(mark, state.Version)
		}, l.libraryGone, l.loadDraftState)
}

// loadReleases (re)loads the releases of the selected library.
func (l *Library) loadReleases(ctx app.Context) {
	if l.libraryID == uuid.Nil {
		l.releases = nil
		return
	}
	uiutil.LoadForGone(l.compoCtx, &l.releasesReloader, l.draft,
		func(c context.Context, lib *apiclient.LibraryClient) ([]apiv0.LibraryReleaseResponse, error) {
			return lib.ListReleases(c)
		},
		func(ctx app.Context, _ *apiclient.LibraryClient, releases []apiv0.LibraryReleaseResponse) {
			l.releases = releases
		}, l.libraryGone, l.loadReleases)
}

// setWidgetTypes replaces the list and brings the editor of the selected type
// up to date: a type seen for the first time fills the editor, a known one is
// merged field by field so unsaved edits survive changes made elsewhere, and a
// deleted one drops the selection. mark is the moment the list was requested.
func (l *Library) setWidgetTypes(ctx app.Context, mark uiutil.Mark, items []apiv0.WidgetTypeResponse) {
	old, known := l.findItem(l.selectedID)
	for _, it := range items {
		// Every DTO of the Draft carries its one Version (ADR 0016): the
		// writes state it under the Library, not under a WidgetType.
		l.observeDraftVersion(mark, it.Version)
	}
	l.widgetTypes = items
	if len(items) == 0 {
		// No WidgetType carries the Draft's Version, so the list states none:
		// read it, or the next write would state the one before the change
		// that emptied the Draft.
		l.loadDraftState(ctx)
	}
	if l.selectedID == uuid.Nil || l.readOnly() {
		return
	}
	current, exists := l.findItem(l.selectedID)
	switch {
	case !exists:
		l.clearSelection()
		uiutil.StoreID(ctx, selectedIDKey, uuid.Nil)
	case !known:
		l.selectItem(l.selectedID)
	default:
		l.mergeSelected(old, current)
	}
}
