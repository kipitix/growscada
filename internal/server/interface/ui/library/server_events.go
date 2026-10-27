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
			l.versions.Forget() // events were lost: the server may have gone back
		}
		targets := eventlog.ReloadTargetsFor(ev)
		if targets.Libraries {
			l.loadLibraries(ctx)
			l.loadReleases(ctx)
		}
		if targets.WidgetTypes {
			l.loadList(ctx)
		}
	})
}
