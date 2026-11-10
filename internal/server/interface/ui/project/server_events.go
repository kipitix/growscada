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
// (libraries, pins, scenes, widgets and tags changed by anyone — another tab,
// growctl, a Device); the new state of a Tag carried by tag_updated is applied
// without a reload. The events of the content are those of the stream of the
// project followed by the Bar (eventlog.Bar.SetProject), which may be another
// mode's: the events of another project are dropped. Editors merge fresh data field by field (edit_fields.go):
// a field the user has not edited follows the server, an unsaved edit is kept.

func (p *Project) handleServerEvents(ctx app.Context) {
	ctx.Handle(eventlog.ActionServerEvent, func(ctx app.Context, a app.Action) {
		ev, ok := a.Value.(apiv0.Event)
		if !ok || !eventlog.ForProject(a, p.projectID) {
			return
		}
		p.tags = eventlog.ApplyToTags(p.tags, ev)
		if ev.Type == eventlog.EventResync || ev.Type == eventlog.EventProjectResync {
			// Events were lost: the server may have gone back, the stack with
			// it.
			p.draftVersions.Forget()
			p.loadDraftState(ctx)
			p.loadDraftChanges(ctx)
		}
		targets := eventlog.ReloadTargetsFor(ev)
		// A write of the Draft's content is one DraftChange (ADR 0017): the
		// undo and redo buttons follow the events, as the counts they show do.
		// The unsaved-changes flag follows every write of the content, the
		// Draft's own state (task 74).
		if targets.DraftChanged() {
			p.loadUnsavedChanges(ctx)
			p.loadDraftChanges(ctx)
		}
		if targets.Deploy {
			// A Deploy, a Rollback or a file deploy: the running Deployment and
			// the Release it came from, which the list marks (task 72_4). A
			// write of the Draft changes neither.
			p.loadDeploy(ctx)
			p.loadReleases(ctx)
			if p.deploymentsDialog {
				p.loadDeployments(ctx)
			}
		}
		if targets.Libraries {
			p.loadLibraries(ctx)
		}
		if targets.LibraryPins {
			p.loadPins(ctx)
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
		if targets.Commits {
			p.loadCommits(ctx)
			p.loadUnsavedChanges(ctx) // a Save makes the Draft its Commit's content
		}
		if targets.Releases {
			p.loadReleases(ctx)
		}
	})
}
