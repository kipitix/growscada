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
// later write does not put an older stack back. A 404 means the library is
// gone and leaves it (libraryGone); another failure shows its toast, and the
// next event reads the stack again.
func (l *Library) loadDraftChanges(ctx app.Context) {
	if l.draft() == nil {
		l.changes, l.changesLoaded = apiv0.DraftChangesResponse{}, false
		return
	}
	// The mark is taken before the request is sent: an answer of a read sent
	// before a forget must not put the forgotten state back (applyDraftChanges).
	mark := l.markDraft()
	uiutil.LoadForGone(l.compoCtx, &l.changesReloader, l.draft,
		func(c context.Context, lib *apiclient.LibraryClient) (apiv0.DraftChangesResponse, error) {
			return lib.DraftChanges(c)
		},
		func(ctx app.Context, _ *apiclient.LibraryClient, state apiv0.DraftChangesResponse) {
			l.applyDraftChanges(mark, state)
		}, l.libraryGone, l.loadDraftChanges)
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

// libraryGone handles a 404 of a read of the selected library — its Draft, the
// state of the Draft, its stack of DraftChanges or its saved states:
// the library was deleted, so it is left instead of an error being shown (ADR
// 0007). The read is the one that is 404 only for a deleted library; a
// command's 404 stays its own error. Root handles the action, leaves the
// library, shows the selection page and rereads its list (task 76), and the
// tab that opens a stale key goes the same way on its first read (task 76).
func (l *Library) libraryGone(ctx app.Context, lib *apiclient.LibraryClient) {
	ctx.NewActionWithValue(uiutil.ActionLibraryGone, lib.ID())
}

// loadList (re)loads the WidgetTypes of the selected library's Draft;
// concurrent requests are coalesced, a response of another library dropped.
func (l *Library) loadList(ctx app.Context) {
	if l.libraryID == uuid.Nil {
		l.widgetTypes = nil
		return
	}
	mark := l.markDraft()
	replacements := l.contentReplacements
	uiutil.LoadForGone(l.compoCtx, &l.reloader, l.draft,
		func(c context.Context, lib *apiclient.LibraryClient) ([]apiv0.WidgetTypeResponse, error) {
			return lib.ListWidgetTypes(c)
		},
		func(ctx app.Context, _ *apiclient.LibraryClient, items []apiv0.WidgetTypeResponse) {
			l.setWidgetTypes(ctx, mark, replacements, items)
		}, l.libraryGone, l.loadList)
}

// loadDraftState (re)loads the state of the selected library's Draft: its
// Version — the Draft of a library without a WidgetType carries it in no
// content DTO, and every write of the Draft states it (ADR 0016) — and whether
// it holds changes its last saved state does not (task 74). It is read on
// mount, on a resync, after each content event and after each command.
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
			l.applyDraftState(mark, state)
		}, l.libraryGone, l.loadDraftState)
}

// applyDraftState stores the unsaved-changes flag a request sent at mark
// answered, under the same guard as applyDraftChanges: an answer of a read sent
// before a later write, or before a resync, would put a stale badge back.
func (l *Library) applyDraftState(mark uiutil.Mark, state apiv0.DraftStateResponse) {
	if l.draftVersions.Forgotten(mark, l.libraryID) {
		return
	}
	if known := l.draftVersion(); known > state.Version {
		return
	}
	l.draftUnsaved, l.draftStateLoaded = state.HasUnsavedChanges, true
}

// loadUnsavedChanges (re)loads whether the selected library's Draft holds
// changes its last saved state does not, from the Draft's state, after a
// content event of another tab. The answer states the server's Version too, but
// that is not the Version the shown WidgetTypes were read at: advancing it
// would let an edit of the stale content overwrite the other tab's change
// instead of being refused as a conflict, so only the flag is applied (task
// 74).
func (l *Library) loadUnsavedChanges(ctx app.Context) {
	draft := l.draft()
	if draft == nil {
		return
	}
	mark := l.markDraft()
	uiutil.LoadForGone(l.compoCtx, &l.unsavedReloader, l.draft,
		func(c context.Context, lib *apiclient.LibraryClient) (apiv0.DraftStateResponse, error) {
			return lib.DraftState(c)
		},
		func(ctx app.Context, _ *apiclient.LibraryClient, state apiv0.DraftStateResponse) {
			l.applyDraftState(mark, state)
		}, l.libraryGone, l.loadUnsavedChanges)
}

// setWidgetTypes replaces the list and brings the editor of the selected type
// up to date: a type seen for the first time fills the editor, a known one is
// merged field by field so unsaved edits survive changes made elsewhere, and a
// deleted one drops the selection. mark is the moment the list was requested;
// replacements is how many times the whole Draft had been replaced then — the
// first list after a replacement (a load of a saved state) is the truth, so the
// editor is filled from it rather than merged, and a text typed but never
// applied does not survive the load (task 84).
//
// A list older than the known state does not delete a WidgetType known to
// exist after it was requested (uiutil.Versions.KnownAfter): one created
// meanwhile is kept at the end, with its selection (task 61).
func (l *Library) setWidgetTypes(ctx app.Context, mark uiutil.Mark, replacements uint64, items []apiv0.WidgetTypeResponse) {
	old, known := l.findItem(l.selectedID)
	// The first list requested after the whole Draft was replaced is the truth:
	// the editor takes it as it is, unsaved typing and all (task 84).
	replaced := replacements == l.contentReplacements && replacements > l.appliedReplacements
	if replaced {
		l.appliedReplacements = replacements
	}
	fresh := len(items) > 0 && l.draftVersions.Advance(mark, l.libraryID, items[0].Version)
	for _, it := range items {
		// Every DTO of the Draft carries its one Version (ADR 0016): the
		// writes state it under the Library, not under a WidgetType.
		l.observeDraftVersion(mark, it.Version)
	}
	if fresh || replaced {
		l.widgetTypes = items
	} else {
		l.widgetTypes = l.mergedWithKnownWidgetTypes(mark, items)
	}
	if len(items) == 0 {
		// No WidgetType carries the Draft's Version, so the list states none:
		// read it, or the next write would state the one before the change
		// that emptied the Draft.
		l.loadDraftState(ctx)
	}
	if l.selectedID == uuid.Nil {
		return
	}
	current, exists := l.findItem(l.selectedID)
	switch {
	case !exists && l.draftVersions.KnownAfter(mark, l.selectedID):
		// The WidgetType was created after the list was requested: it stays
		// selected and its editor keeps the text typed into it (task 61).
	case !exists:
		l.clearSelection()
		uiutil.StoreID(ctx, selectedIDKey, uuid.Nil)
	case !known || replaced:
		l.selectItem(l.selectedID)
	default:
		l.mergeSelected(old, current)
	}
}

// mergedWithKnownWidgetTypes returns the WidgetTypes a list requested at mark
// holds: the IDs it names, each as the local copy when there is one (a
// WidgetType written since states the newer name), and the IDs it misses that
// are known to exist after the list was requested, at the end.
func (l *Library) mergedWithKnownWidgetTypes(mark uiutil.Mark, items []apiv0.WidgetTypeResponse) []apiv0.WidgetTypeResponse {
	merged := make([]apiv0.WidgetTypeResponse, len(items))
	listed := make(map[uuid.UUID]struct{}, len(items))
	for i, it := range items {
		if cur, ok := l.findItem(it.ID); ok {
			it = cur
		}
		merged[i] = it
		listed[it.ID] = struct{}{}
	}
	for _, it := range l.widgetTypes {
		if _, ok := listed[it.ID]; ok {
			continue
		}
		if !l.draftVersions.KnownAfter(mark, it.ID) {
			continue
		}
		merged = append(merged, it)
	}
	return merged
}
