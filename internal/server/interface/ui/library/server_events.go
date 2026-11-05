package library

import (
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
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
		// The selected library's lock changes with a Check out, a Check in, a
		// takeover or an expiry; the event carries its new state, the toast of
		// a lost lock included (ADR 0015). A Library's lock goes to the
		// server's stream, which the Bar holds always. An event without it (a
		// resync) rereads it.
		switch {
		case ev.EditLock != nil:
			if ev.ID == l.libraryID.String() {
				l.applyEditLock(ctx, *ev.EditLock)
			}
		case targets.EditLock:
			l.loadEditLock(ctx)
		}
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
	})
}
