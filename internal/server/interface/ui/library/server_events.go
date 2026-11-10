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
// The libraries, the selected one's releases and the WidgetTypes of its
// Draft are reloaded on server events (changed by anyone — another tab,
// growctl, a Discard); the editor merges the fresh data field by field
// (edit_fields.go).

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
			l.loadReleases(ctx)
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
			// unsaved indicator follow. The Releases are not reread with
			// their content, which a Draft edit never changes (task 72_7).
			l.loadList(ctx)
		}
		if targets.Commits && mine {
			l.loadCommits(ctx)
			l.loadUnsavedChanges(ctx) // a Save makes the Draft its Commit's content
		}
		// A Library Release names the Commit it published by that Commit's
		// current title — its own copy carries none —, so a rename changes the
		// Release list too. A Project Release keeps the title it was published
		// with and needs no such read (task 72_7; ADR 0021).
		if ev.Type == apiv0.EventCommitTitleUpdated && mine {
			l.loadReleases(ctx)
		}
		// A Release of the selected Library and its note changed in another
		// tab: the list of Releases follows, never another library's
		// (task 72_7).
		if targets.Releases && mine {
			l.loadReleases(ctx)
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
