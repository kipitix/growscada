package project

import (
	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
)

// ── Server events ─────────────────────────────────────────────────────────────
//
// Every server-backed list of the Project mode is (re)loaded through a
// uiutil.Reloader, both after the user's own mutations and on server events
// (widget types, scenes, widgets and tags changed by anyone — another tab,
// growctl, a Device); the new state of a Tag carried by tag_updated is applied
// without a reload. Editors merge fresh data field by field (edit_fields.go):
// a field the user has not edited follows the server, an unsaved edit is kept.

func (p *Project) handleServerEvents(ctx app.Context) {
	ctx.Handle(eventlog.ActionServerEvent, func(ctx app.Context, a app.Action) {
		ev, ok := a.Value.(apiv0.Event)
		if !ok {
			return
		}
		p.tags = eventlog.ApplyToTags(p.tags, ev)
		if ev.Type == eventlog.EventResync {
			// Events were lost: the server may have gone back.
			p.sceneVersions.Forget()
			p.sceneStates.Forget()
		}
		targets := eventlog.ReloadTargetsFor(ev)
		if targets.WidgetTypes {
			p.loadWidgetTypes(ctx)
		}
		if targets.Scenes {
			p.loadScenes(ctx)
		}
		// The selected Scene's Widgets carry its version (and its own state
		// does not change with theirs); another Scene is reread for it.
		switch {
		case targets.WidgetsOf(p.selectedSceneID):
			p.loadWidgets(ctx)
		case targets.Scene != uuid.Nil:
			p.loadScene(ctx, targets.Scene)
		}
		if targets.Tags {
			p.loadTags(ctx)
		}
	})
}
