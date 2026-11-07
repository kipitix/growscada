package project

import (
	"context"
	"fmt"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// The Project mode's deployment state (task 35, part 5): the Revision the
// project runs, whether the Draft differs from it, and the Deploy and Discard
// actions. The state comes from the Deploy preview, which is the same document
// comparison a Deploy does; the Revision itself is Operation's.

// loadRevisions (re)loads the Revisions the dialog lists, newest first.
func (p *Project) loadRevisions(ctx app.Context) {
	if p.content == nil {
		p.revisions, p.revisionsLoaded = nil, false
		return
	}
	uiutil.LoadOfProject(p.compoCtx, &p.revisionsReloader, p.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) ([]apiv0.RevisionSummaryResponse, error) {
			return pc.Revisions(c)
		},
		func(ctx app.Context, revisions []apiv0.RevisionSummaryResponse) {
			p.revisions, p.revisionsLoaded = revisions, true
		}, p.loadRevisions)
}

// openRevisionsDialog shows the Revisions of the project, read afresh: the
// dialog is available even before the first Deploy, where it is empty.
func (p *Project) openRevisionsDialog(ctx app.Context) {
	p.revisionsDialog = true
	p.selectRevision(ctx, 0)
	p.loadRevisions(ctx)
	ctx.Update()
}

// closeRevisionsDialog closes the list without rolling anything back.
func (p *Project) closeRevisionsDialog() {
	p.revisionsDialog = false
	p.clearRevisionSelection()
}

// clearRevisionSelection forgets the row whose document was shown.
func (p *Project) clearRevisionSelection() {
	p.selectedRevision = 0
	p.revisionDetail, p.revisionDetailLoaded = apiv0.RevisionResponse{}, false
}

// selectRevision shows the document of the Revision number (0: none) as a
// summary for comparison. A response of a row no longer selected is dropped.
func (p *Project) selectRevision(ctx app.Context, number int) {
	p.selectedRevision = number
	p.revisionDetail, p.revisionDetailLoaded = apiv0.RevisionResponse{}, false
	if number == 0 || p.content == nil {
		return
	}
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.RevisionResponse, error) {
		return pc.RevisionByNumber(c, number)
	}, func(ctx app.Context, detail apiv0.RevisionResponse) {
		if p.selectedRevision != number {
			return
		}
		p.revisionDetail, p.revisionDetailLoaded = detail, true
		ctx.Update()
	}, nil)
}

// isCurrentRevision reports whether number is the Revision the project runs.
// It is the newest of the list the dialog shows: the list is where the row is
// read from, so the mark and the offer to roll back agree even before the
// Deploy preview has answered.
func (p *Project) isCurrentRevision(number int) bool {
	return len(p.revisions) > 0 && p.revisions[0].Number == number
}

// rollbackRevision confirms rolling the project back to the selected Revision
// and does it. The current Revision's own document has nothing to roll back to
// and is refused before the request, as the dialog does not offer it.
func (p *Project) rollbackRevision(ctx app.Context) {
	if p.content == nil || p.rollingBack || p.deploying || p.discarding {
		return
	}
	number := p.selectedRevision
	if number == 0 || p.isCurrentRevision(number) {
		return
	}
	message := fmt.Sprintf("Roll the project back to revision %d? Its configuration becomes a new revision.", number)
	if !app.Window().Call("confirm", message).Bool() {
		return
	}
	p.doRollback(ctx, number)
}

// doRollback rolls the project back to the Revision number: a new Revision
// with the same document. Success closes the dialog and reloads the preview and
// the list; the server's refusal (nothing to roll back, the Draft being
// discarded, a Revision gone) is a toast and leaves the dialog open.
func (p *Project) doRollback(ctx app.Context, number int) {
	if p.content == nil || p.rollingBack {
		return
	}
	if p.waitMoves(ctx, func(ctx app.Context) { p.doRollback(ctx, number) }) {
		return
	}
	p.rollingBack = true
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.RevisionSummaryResponse, error) {
		return pc.Rollback(c, number)
	}, func(ctx app.Context, _ apiv0.RevisionSummaryResponse) {
		p.rollingBack = false
		p.closeRevisionsDialog()
		p.loadDeploy(ctx)
		p.loadRevisions(ctx)
		ctx.Update()
	}, func(ctx app.Context, err error) {
		p.rollingBack = false
		uiutil.ShowError(ctx, err)
		ctx.Update()
	})
}

// loadDeploy (re)loads the deployment state of the project.
func (p *Project) loadDeploy(ctx app.Context) {
	if p.content == nil {
		p.deployPreview, p.deployLoaded = apiv0.DeployPreviewResponse{}, false
		return
	}
	uiutil.LoadOfProject(p.compoCtx, &p.deployReloader, p.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) (apiv0.DeployPreviewResponse, error) {
			return pc.DeployPreview(c)
		},
		func(ctx app.Context, preview apiv0.DeployPreviewResponse) {
			p.deployPreview, p.deployLoaded = preview, true
		}, p.loadDeploy)
}

// openDeployDialog reads the preview afresh and opens the confirmation with
// it: what the Deploy would do, not what was last seen. The dialog keeps that
// preview (dialogPreview), so a reload of the bar that lands meanwhile does
// not rewrite it.
func (p *Project) openDeployDialog(ctx app.Context) {
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.DeployPreviewResponse, error) {
		return pc.DeployPreview(c)
	}, func(ctx app.Context, preview apiv0.DeployPreviewResponse) {
		p.dialogPreview = preview
		p.deployDialog = true
		ctx.Update()
	}, nil)
}

// closeDeployDialog closes the confirmation without deploying.
func (p *Project) closeDeployDialog() {
	p.deployDialog = false
}

// deploy confirms the open dialog: it makes the Draft the new Revision. The
// server's refusal (nothing to deploy, the Draft being discarded) is a toast.
func (p *Project) deploy(ctx app.Context) {
	if p.content == nil || p.deploying || p.discarding || p.rollingBack {
		return
	}
	if p.waitMoves(ctx, func(ctx app.Context) { p.deploy(ctx) }) {
		return
	}
	p.deploying = true
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.DeployResponse, error) {
		return pc.Deploy(c)
	}, func(ctx app.Context, _ apiv0.DeployResponse) {
		p.deploying = false
		p.deployDialog = false
		p.loadDeploy(ctx)
		p.loadDraftChanges(ctx) // a Deploy clears the stack (ADR 0017)
		if p.revisionsDialog {
			p.loadRevisions(ctx)
		}
		ctx.Update()
	}, func(ctx app.Context, err error) {
		// The dialog goes: its preview is what failed (the Draft may have
		// been deployed meanwhile), and the reloaded bar shows the truth.
		p.deploying = false
		p.deployDialog = false
		uiutil.ShowError(ctx, err)
		p.loadDeploy(ctx)
		ctx.Update()
	})
}

// discard resets the Draft to the Revision the project runs, after the
// browser's confirmation. Without a Revision the server refuses (409), which
// is the toast.
func (p *Project) discard(ctx app.Context) {
	if p.content == nil || p.discarding || p.deploying || p.rollingBack {
		return
	}
	if p.waitMoves(ctx, func(ctx app.Context) { p.discard(ctx) }) {
		return
	}
	message := "Discard the Draft of this project and restore it to the Revision it runs?"
	if !app.Window().Call("confirm", message).Bool() {
		return
	}
	p.discarding = true
	mark := p.markDraft()
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.DiscardProjectResponse, error) {
		return pc.Discard(c)
	}, func(ctx app.Context, discarded apiv0.DiscardProjectResponse) {
		p.discarding = false
		p.observeDraftVersion(mark, discarded.Version)
		p.loadDeploy(ctx)
		p.loadDraftChanges(ctx) // a Discard clears the stack (ADR 0017)
		ctx.Update()
	}, func(ctx app.Context, err error) {
		p.discarding = false
		uiutil.ShowError(ctx, err)
		ctx.Update()
	})
}

// renderDeployBar is the project's header: the Revision it runs (or "Not
// deployed"), whether the Draft has changes not deployed, and the Deploy and
// Discard buttons.
func (p *Project) renderDeployBar() app.UI {
	badge := "Not deployed"
	if p.deployPreview.CurrentRevision != nil {
		badge = fmt.Sprintf("Revision %d · deployed %s",
			p.deployPreview.CurrentRevision.Number,
			formatDeployedAt(p.deployPreview.CurrentRevision.DeployedAt))
	}

	body := []app.UI{
		p.renderDraftChangesBar(),
		app.Div().Style("width", "1px").Style("height", "18px").Style("background", "var(--border)"),
		app.Span().
			Attr("data-deploy-revision", "true").
			Style("font-size", "12px").
			Style("font-weight", "600").
			Style("color", "var(--text-2)").
			Text(badge),
	}
	if p.deployLoaded && p.deployPreview.CurrentRevision != nil && !p.deployPreview.Unchanged {
		body = append(body, app.Span().
			Attr("data-deploy-differs", "true").
			Style("font-size", "12px").
			Style("color", "var(--toast-warn-title)").
			Text("Unreleased changes"))
	}
	body = append(body,
		app.Div().Style("flex", "1"),
		p.deployButton("Project file…", "Download the Draft, open a file, or deploy one",
			!p.deploying && !p.discarding && !p.rollingBack && !p.projectFileBusy, func(ctx app.Context) {
				p.openProjectFileDialog(ctx)
			}),
		p.deployButton("Revisions", "The Revisions of the project", !p.deploying && !p.discarding && !p.rollingBack, func(ctx app.Context) {
			p.openRevisionsDialog(ctx)
		}),
		p.deployButton("Deploy", "Deploy the Draft as a new Revision",
			!p.deploying && !p.discarding && !p.rollingBack, func(ctx app.Context) {
				p.openDeployDialog(ctx)
			}),
		p.deployButton("Discard", "Reset the Draft to the Revision the project runs",
			!p.discarding && !p.deploying && !p.rollingBack, func(ctx app.Context) {
				p.discard(ctx)
			}),
	)

	if p.deployDialog {
		body = append(body, p.renderDeployDialog())
	}
	if p.revisionsDialog {
		body = append(body, p.renderRevisionsDialog())
	}
	if p.projectFileDialog {
		body = append(body, p.renderProjectFileDialog())
	}

	return app.Div().
		Attr("data-deploy-bar", "true").
		Style("display", "flex").
		Style("position", "relative").
		Style("align-items", "center").
		Style("gap", "10px").
		Style("padding", "6px 10px").
		Style("border-bottom", "1px solid var(--border)").
		Style("background", "var(--bg-elevated)").
		Body(body...)
}

func (p *Project) deployButton(label, title string, enabled bool, onClick func(app.Context)) app.UI {
	return app.Button().
		Title(title).
		Disabled(!enabled).
		Style("padding", "2px 9px").
		Style("font-size", "12px").
		Style("cursor", "pointer").
		Style("border", "1px solid var(--border-input)").
		Style("border-radius", "4px").
		Style("background", "var(--bg)").
		Style("color", "var(--text)").
		Text(label).
		OnClick(func(ctx app.Context, e app.Event) { onClick(ctx) }, app.EventScope(label))
}

// renderDeployDialog is the confirmation over the deploy bar: what the Deploy
// would make, and a warning for what it would drop.
func (p *Project) renderDeployDialog() app.UI {
	preview := p.dialogPreview

	lines := []app.UI{}
	if preview.CurrentRevision != nil {
		lines = append(lines, app.P().
			Style("margin", "0 0 6px").
			Text(fmt.Sprintf("Current revision: %d, deployed %s.",
				preview.CurrentRevision.Number, formatDeployedAt(preview.CurrentRevision.DeployedAt))))
	} else {
		lines = append(lines, app.P().
			Style("margin", "0 0 6px").
			Text("The project has not been deployed yet."))
	}
	if preview.Unchanged {
		lines = append(lines, app.P().
			Style("margin", "0 0 6px").
			Text("Nothing to deploy: the Draft is the revision it runs."))
	} else {
		lines = append(lines, app.P().
			Style("margin", "0 0 6px").
			Text("The whole Draft becomes a new Revision and Operation switches to it."))
	}
	if len(preview.DroppedWidgets) == 0 {
		lines = append(lines, app.P().
			Style("margin", "0 0 6px").
			Style("color", "var(--text-muted)").
			Text("No runtime widgets would be dropped."))
	} else {
		for _, dropped := range preview.DroppedWidgets {
			lines = append(lines, app.P().
				Style("margin", "0 0 6px").
				Style("color", "var(--toast-warn-title)").
				Text("Dropped widget: "+dropped.Name))
		}
	}

	buttons := []app.UI{
		app.Button().
			Style("padding", "4px 12px").
			Style("font-size", "13px").
			Style("cursor", "pointer").
			Text("Cancel").
			OnClick(func(ctx app.Context, e app.Event) {
				p.closeDeployDialog()
				ctx.Update()
			}),
	}
	if !preview.Unchanged {
		buttons = append(buttons, app.Button().
			Attr("data-deploy-confirm", "true").
			Disabled(p.deploying || p.rollingBack).
			Style("padding", "4px 12px").
			Style("font-size", "13px").
			Style("cursor", "pointer").
			Style("background", "var(--accent)").
			Style("color", "var(--accent-text)").
			Text("Deploy").
			OnClick(func(ctx app.Context, e app.Event) {
				p.deploy(ctx)
			}))
	}

	return app.Div().
		Attr("data-deploy-dialog", "true").
		Style("position", "absolute").
		Style("top", "48px").
		Style("right", "16px").
		Style("z-index", "20").
		Style("width", "420px").
		Style("padding", "14px").
		Style("border", "1px solid var(--border)").
		Style("border-radius", "6px").
		Style("background", "var(--bg)").
		Style("box-shadow", "var(--toast-shadow)").
		Style("font-size", "13px").
		Body(
			app.H3().Style("margin", "0 0 8px").Text("Deploy the project"),
			app.Div().Body(lines...),
			app.Div().
				Style("display", "flex").
				Style("justify-content", "flex-end").
				Style("gap", "8px").
				Body(buttons...),
		)
}

// renderRevisionsDialog is the list of the project's Revisions with the
// summary of the selected one: newest first, what caused each, and the running
// one marked, whose rollback is not offered (task 36, ADR 0014).
func (p *Project) renderRevisionsDialog() app.UI {
	rows := []app.UI{}
	switch {
	case !p.revisionsLoaded:
		rows = append(rows, app.P().
			Style("margin", "0 0 6px").
			Style("color", "var(--text-muted)").
			Text("Loading revisions…"))
	case len(p.revisions) == 0:
		rows = append(rows, app.P().
			Style("margin", "0 0 6px").
			Text("The project has not been deployed yet."))
	default:
		for _, revision := range p.revisions {
			rows = append(rows, p.renderRevisionRow(revision))
		}
	}
	if p.selectedRevision != 0 {
		rows = append(rows, p.renderRevisionSummary())
	}

	buttons := []app.UI{
		app.Button().
			Style("padding", "4px 12px").
			Style("font-size", "13px").
			Style("cursor", "pointer").
			Text("Close").
			OnClick(func(ctx app.Context, e app.Event) {
				p.closeRevisionsDialog()
				ctx.Update()
			}),
	}
	if p.selectedRevision != 0 && !p.isCurrentRevision(p.selectedRevision) {
		buttons = append(buttons, app.Button().
			Attr("data-revision-rollback", "true").
			Disabled(p.rollingBack || p.deploying || p.discarding).
			Style("padding", "4px 12px").
			Style("font-size", "13px").
			Style("cursor", "pointer").
			Style("background", "var(--accent)").
			Style("color", "var(--accent-text)").
			Text("Roll back").
			OnClick(func(ctx app.Context, e app.Event) {
				p.rollbackRevision(ctx)
			}))
	}

	return app.Div().
		Attr("data-revisions-dialog", "true").
		Style("position", "absolute").
		Style("top", "48px").
		Style("right", "16px").
		Style("z-index", "20").
		Style("width", "480px").
		Style("padding", "14px").
		Style("border", "1px solid var(--border)").
		Style("border-radius", "6px").
		Style("background", "var(--bg)").
		Style("box-shadow", "var(--toast-shadow)").
		Style("font-size", "13px").
		Body(
			app.H3().Style("margin", "0 0 8px").Text("Revisions"),
			app.Div().Body(rows...),
			app.Div().
				Style("display", "flex").
				Style("justify-content", "flex-end").
				Style("gap", "8px").
				Style("margin-top", "10px").
				Body(buttons...),
		)
}

// renderRevisionRow is one Revision of the list: its number, when it was
// deployed and why; the running one is marked.
func (p *Project) renderRevisionRow(revision apiv0.RevisionSummaryResponse) app.UI {
	body := []app.UI{
		app.Span().Text(fmt.Sprintf("#%d · %s · %s",
			revision.Number, formatDeployedAt(revision.DeployedAt), revisionCauseText(revision.Cause, revision.RolledBackFrom))),
	}
	if p.isCurrentRevision(revision.Number) {
		body = append(body, app.Span().
			Attr("data-revision-current", "true").
			Style("margin-left", "8px").
			Style("font-size", "11px").
			Style("color", "var(--accent)").
			Text("current"))
	}
	body = append(body, app.Button().
		Attr("data-revision-download", fmt.Sprint(revision.Number)).
		Title(fmt.Sprintf("Download revision %d as a project file", revision.Number)).
		Style("margin-left", "8px").
		Style("padding", "1px 7px").
		Style("font-size", "11px").
		Style("cursor", "pointer").
		Style("border", "1px solid var(--border-input)").
		Style("border-radius", "4px").
		Style("background", "var(--bg)").
		Style("color", "var(--text)").
		Text("Download").
		OnClick(func(ctx app.Context, e app.Event) {
			e.Call("stopPropagation")
			e.StopImmediatePropagation()
			p.downloadRevisionProjectFile(ctx, revision.Number)
		}))
	row := app.Div().
		Attr("data-revision-row", fmt.Sprint(revision.Number)).
		Style("display", "flex").
		Style("align-items", "center").
		Style("justify-content", "space-between").
		Style("padding", "4px 6px").
		Style("cursor", "pointer").
		Style("border-radius", "4px").
		Body(body...).
		OnClick(func(ctx app.Context, e app.Event) {
			p.selectRevision(ctx, revision.Number)
			ctx.Update()
		})
	if p.selectedRevision == revision.Number {
		row = row.
			Style("background", "var(--bg-elevated)").
			Style("font-weight", "600")
	}
	return row
}

// renderRevisionSummary is the selected Revision's document as counts, for
// comparison with what the project runs.
func (p *Project) renderRevisionSummary() app.UI {
	if !p.revisionDetailLoaded {
		return app.P().
			Style("margin", "8px 0 0").
			Style("color", "var(--text-muted)").
			Text(fmt.Sprintf("Loading revision %d…", p.selectedRevision))
	}
	detail := p.revisionDetail
	widgets := 0
	for _, scene := range detail.Content.Scenes {
		widgets += len(scene.Widgets)
	}
	return app.Div().
		Attr("data-revision-summary", "true").
		Style("margin", "8px 0 0").
		Style("padding", "8px").
		Style("border-top", "1px solid var(--border)").
		Body(
			app.P().
				Style("margin", "0 0 4px").
				Text(fmt.Sprintf("Revision %d · %s · %s",
					detail.Number, formatDeployedAt(detail.DeployedAt), revisionCauseText(detail.Cause, detail.RolledBackFrom))),
			app.P().
				Style("margin", "0").
				Style("color", "var(--text-2)").
				Text(fmt.Sprintf("%d scenes, %d widgets, %d tags, %d devices, %d environments",
					len(detail.Content.Scenes), widgets, len(detail.Content.Tags),
					len(detail.Content.Devices), len(detail.Content.Environments))),
		)
}

// revisionCauseText renders why a Revision exists: a Deploy, or a Rollback
// naming the Revision it copied. A cause this build does not know is shown as
// it came rather than passed off as a Deploy.
func revisionCauseText(cause apiv0.RevisionCause, rolledBackFrom *int) string {
	switch cause {
	case apiv0.RevisionCauseDeploy:
		return "Deployed"
	case apiv0.RevisionCauseRollback:
		if rolledBackFrom != nil {
			return fmt.Sprintf("Rolled back from #%d", *rolledBackFrom)
		}
		return "Rolled back"
	case apiv0.RevisionCauseFile:
		return "Deployed from file"
	default:
		return string(cause)
	}
}

// formatDeployedAt renders the deployment time of the contract (RFC 3339) for
// the badge; a value that does not parse is shown as it came.
func formatDeployedAt(value string) string {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}
