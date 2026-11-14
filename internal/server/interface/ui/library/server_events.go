package library

import (
	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Server events ─────────────────────────────────────────────────────────────
//
// The libraries, the history of the selected one's Draft and its WidgetTypes
// are reloaded on server events (changed by anyone — another tab, growctl, a
// load of a saved state); the editor merges the fresh data field by field
// (edit_fields.go), and takes it whole when the Draft itself was replaced.

func (l *Library) handleServerEvents(ctx app.Context) {
	ctx.Handle(eventlog.ActionServerEvent, func(ctx app.Context, a app.Action) {
		ev, ok := a.Value.(apiv0.Event)
		if !ok {
			return
		}
		if ev.Type == eventlog.EventProjectResync {
			// The Library shows nothing of a project: a resync of the Bar's
			// stream makes none of its lists stale (task 72_7).
			return
		}
		if ev.Type == eventlog.EventResync {
			// Events were lost: the server may have gone back, the stack with
			// it. Everything the mode shows is read again (task 72_7).
			l.draftVersions.Forget()
			l.loadDraftState(ctx)
			l.loadDraftChanges(ctx)
			l.loadLibraries(ctx)
			l.loadList(ctx)
			l.loadCommits(ctx)
			return
		}
		mine := l.eventOfSelectedLibrary(ev)
		targets := eventlog.ReloadTargetsFor(ev)
		// A write of the Draft's content is one DraftChange: the undo and redo
		// buttons follow the events (ADR 0017), as the release state does. A
		// Release publishes a Commit and clears no stack (task 72_5). The flag
		// follows every write of the content, the Draft's own state (task 74).
		if targets.DraftChanged() && mine {
			l.loadUnsavedChanges(ctx)
			l.loadDraftChanges(ctx)
		}
		if targets.Libraries {
			l.loadLibraries(ctx)
		}
		if targets.WidgetTypes && mine {
			// A WidgetType of the selected library changed: its Draft and the
			// unsaved indicator follow. The history is not reread, which a
			// Draft edit never changes (task 72_7).
			l.loadList(ctx)
		}
		if targets.Commits && mine {
			l.loadCommits(ctx)
			l.loadUnsavedChanges(ctx) // a Save makes the Draft its Commit's content
		}
		// A Release of the selected library, or a rename of the saved state it
		// published, changes the history's "released" mark and the library's
		// state — never another library's (task 72_7; task 84).
		if (targets.Releases || ev.Type == apiv0.EventCommitTitleUpdated) && mine {
			l.loadReleases(ctx) // the row names the Release that published it
			l.loadCommits(ctx)
			l.loadLibraries(ctx)
		}
		if eventlog.IsLabelEvent(ev.Type) {
			// The chips of the history and the labels its menu offers follow a
			// label of the Organization or one of its assignments, whichever
			// library's saved state it marks (task 85_3).
			l.loadHistoryLabels(ctx)
			if !mine {
				l.loadCommits(ctx)
			}
		}
	})
}

// eventOfSelectedLibrary reports whether an event of a Library's Commit, its
// Release or a WidgetType of its Draft concerns the selected library. The
// event names the library in ev.ID, and a WidgetType event in ev.LibraryID;
// an event that names none concerns every one (task 72_7).
func (l *Library) eventOfSelectedLibrary(ev apiv0.Event) bool {
	id := ev.ID
	if ev.LibraryID != "" {
		id = ev.LibraryID
	}
	parsed := uiutil.ParseID(id)
	return parsed == uuid.Nil || parsed == l.libraryID
}
