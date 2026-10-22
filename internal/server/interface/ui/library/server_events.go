package library

import (
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
)

// ── Server events ─────────────────────────────────────────────────────────────
//
// The WidgetType list is reloaded on server events (WidgetTypes changed by
// anyone — another tab, the API); the editor merges the fresh data field by
// field (edit_fields.go).

func (l *Library) handleServerEvents(ctx app.Context) {
	ctx.Handle(eventlog.ActionServerEvent, func(ctx app.Context, a app.Action) {
		ev, ok := a.Value.(apiv0.Event)
		if !ok {
			return
		}
		if ev.Type == eventlog.EventResync {
			l.versions.Forget() // events were lost: the server may have gone back
		}
		if eventlog.ReloadTargetsFor(ev).WidgetTypes {
			l.loadList(ctx)
		}
	})
}
