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
		if ev.Type == eventlog.EventResync {
			// Events were lost: the server may have gone back, the stack with
			// it.
			l.draftVersions.Forget()
			l.loadDraftChanges(ctx)
		}
		targets := eventlog.ReloadTargetsFor(ev)
		// A write of the Draft's content is one DraftChange, and a Release
		// clears the stack (ADR 0017): the undo and redo buttons follow the
		// events, as the release state does.
		if targets.DraftChanged() {
			l.loadDraftChanges(ctx)
		}
		if targets.Libraries {
			l.loadLibraries(ctx)
			l.loadReleases(ctx)
		}
		if targets.WidgetTypes {
			l.loadList(ctx)
		}
		if targets.Commits {
			// A commit event names the Library in ev.ID: another library's
			// leaves the history shown alone (task 72_2; task 72_7).
			if id := uiutil.ParseID(ev.ID); id == uuid.Nil || id == l.libraryID {
				l.loadCommits(ctx)
			}
		}
	})
}
