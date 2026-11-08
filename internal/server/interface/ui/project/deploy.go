package project

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// The Project mode's deployment state (task 35, part 5): the Deployment the
// project runs, whether the Draft differs from it, and the Deploy and Discard
// actions. The state comes from the Deploy preview, which is the same document
// comparison a Deploy does; the Deployment itself is Operation's.

// loadDeployments (re)loads the Deployments the dialog lists, newest first.
func (p *Project) loadDeployments(ctx app.Context) {
	if p.content == nil {
		p.deployments, p.deploymentsLoaded = nil, false
		return
	}
	uiutil.LoadOfProject(p.compoCtx, &p.deploymentsReloader, p.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) ([]apiv0.DeploymentSummaryResponse, error) {
			return pc.Deployments(c)
		},
		func(ctx app.Context, deployments []apiv0.DeploymentSummaryResponse) {
			p.deployments, p.deploymentsLoaded = deployments, true
		}, p.loadDeployments)
}

// openDeploymentsDialog shows the Deployments of the project, read afresh: the
// dialog is available even before the first Deploy, where it is empty.
func (p *Project) openDeploymentsDialog(ctx app.Context) {
	p.deploymentsDialog = true
	p.selectDeployment(ctx, 0)
	p.loadDeployments(ctx)
	ctx.Update()
}

// closeDeploymentsDialog closes the list without rolling anything back.
func (p *Project) closeDeploymentsDialog() {
	p.deploymentsDialog = false
	p.clearDeploymentSelection()
}

// clearDeploymentSelection forgets the row whose document was shown.
func (p *Project) clearDeploymentSelection() {
	p.selectedDeployment = 0
	p.deploymentDetail, p.deploymentDetailLoaded = apiv0.DeploymentResponse{}, false
}

// selectDeployment shows the document of the Deployment number (0: none) as a
// summary for comparison. A response of a row no longer selected is dropped.
func (p *Project) selectDeployment(ctx app.Context, number int) {
	p.selectedDeployment = number
	p.deploymentDetail, p.deploymentDetailLoaded = apiv0.DeploymentResponse{}, false
	if number == 0 || p.content == nil {
		return
	}
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.DeploymentResponse, error) {
		return pc.DeploymentByNumber(c, number)
	}, func(ctx app.Context, detail apiv0.DeploymentResponse) {
		if p.selectedDeployment != number {
			return
		}
		p.deploymentDetail, p.deploymentDetailLoaded = detail, true
		ctx.Update()
	}, nil)
}

// isCurrentDeployment reports whether number is the Deployment the project runs.
// It is the newest of the list the dialog shows: the list is where the row is
// read from, so the mark and the offer to roll back agree even before the
// Deploy preview has answered.
func (p *Project) isCurrentDeployment(number int) bool {
	return len(p.deployments) > 0 && p.deployments[0].Number == number
}

// rollbackDeployment confirms rolling the project back to the selected Deployment
// and does it. The current Deployment's own document has nothing to roll back to
// and is refused before the request, as the dialog does not offer it.
func (p *Project) rollbackDeployment(ctx app.Context) {
	if p.content == nil || p.rollingBack || p.deploying || p.discarding {
		return
	}
	number := p.selectedDeployment
	if number == 0 || p.isCurrentDeployment(number) {
		return
	}
	message := fmt.Sprintf("Roll the project back to deployment %d? Its configuration becomes a new deployment.", number)
	if !app.Window().Call("confirm", message).Bool() {
		return
	}
	p.doRollback(ctx, number)
}

// doRollback rolls the project back to the Deployment number: a new Deployment
// with the same document. Success closes the dialog and reloads the preview and
// the list; the server's refusal (nothing to roll back, the Draft being
// discarded, a Deployment gone) is a toast and leaves the dialog open.
func (p *Project) doRollback(ctx app.Context, number int) {
	if p.content == nil || p.rollingBack {
		return
	}
	if p.waitMoves(ctx, func(ctx app.Context) { p.doRollback(ctx, number) }) {
		return
	}
	p.rollingBack = true
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.DeploymentSummaryResponse, error) {
		return pc.RollbackDeployment(c, number)
	}, func(ctx app.Context, _ apiv0.DeploymentSummaryResponse) {
		p.rollingBack = false
		p.closeDeploymentsDialog()
		p.loadDeploy(ctx)
		p.loadDeployments(ctx)
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

// deploy confirms the open dialog: it makes the Draft the new Deployment. The
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
		if p.deploymentsDialog {
			p.loadDeployments(ctx)
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

// discard resets the Draft to the Deployment the project runs, after the
// browser's confirmation. Without a Deployment the server refuses (409), which
// is the toast.
func (p *Project) discard(ctx app.Context) {
	if p.content == nil || p.discarding || p.deploying || p.rollingBack {
		return
	}
	if p.waitMoves(ctx, func(ctx app.Context) { p.discard(ctx) }) {
		return
	}
	message := "Discard the Draft of this project and restore it to the Deployment it runs?"
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
		if len(discarded.Warnings) > 0 {
			// The Discard restored the Deployment as far as this server can: the
			// Library Releases it lacks went with the widgets of their types
			// (task 71).
			parts := make([]string, len(discarded.Warnings))
			for i, warning := range discarded.Warnings {
				parts[i] = fmt.Sprintf("library %s release %d: %d widgets dropped",
					warning.LibraryID, warning.Release, warning.DroppedWidgets)
			}
			ctx.NewActionWithValue(toast.ActionAdd, toast.Problem{Problem: apiv0.Problem{
				Title:  "Discarded without some Library Releases",
				Detail: strings.Join(parts, "; "),
			}})
		}
		ctx.Update()
	}, func(ctx app.Context, err error) {
		p.discarding = false
		uiutil.ShowError(ctx, err)
		ctx.Update()
	})
}

// renderDeployBar is the project's header: the Deployment it runs (or "Not
// deployed"), whether the Draft has changes not deployed, and the Deploy and
// Discard buttons.
func (p *Project) renderDeployBar() app.UI {
	badge := "Not deployed"
	if p.deployPreview.CurrentDeployment != nil {
		badge = fmt.Sprintf("Deployment %d · deployed %s",
			p.deployPreview.CurrentDeployment.Number,
			formatDeployedAt(p.deployPreview.CurrentDeployment.DeployedAt))
	}

	body := p.renderCommitBar()
	body = append(body,
		p.renderDraftChangesBar(),
		app.Div().Style("width", "1px").Style("height", "18px").Style("background", "var(--border)"),
		app.Span().
			Attr("data-deploy-running", "true").
			Style("font-size", "12px").
			Style("font-weight", "600").
			Style("color", "var(--text-2)").
			Text(badge),
	)
	if p.deployLoaded && p.deployPreview.CurrentDeployment != nil && !p.deployPreview.Unchanged {
		body = append(body, app.Span().
			Attr("data-deploy-differs", "true").
			Style("font-size", "12px").
			Style("color", "var(--toast-warn-title)").
			Text("Unreleased changes"))
	}
	if p.deployLoaded && len(p.deployPreview.MissingReleases) > 0 {
		body = append(body, app.Span().
			Attr("data-deploy-missing-releases", "true").
			Title("The running Deployment names Library Releases this server does not have; a Discard drops their widgets").
			Style("font-size", "12px").
			Style("color", "var(--toast-warn-title)").
			Text("Missing Library Releases"))
	}
	body = append(body,
		app.Div().Style("flex", "1"),
		p.deployButton("Project file…", "Download the Draft, open a file, or deploy one",
			!p.deploying && !p.discarding && !p.rollingBack && !p.projectFileBusy, func(ctx app.Context) {
				p.openProjectFileDialog(ctx)
			}),
		p.deployButton("Deployments", "The Deployments of the project", !p.deploying && !p.discarding && !p.rollingBack, func(ctx app.Context) {
			p.openDeploymentsDialog(ctx)
		}),
		p.deployButton("Deploy", "Deploy the Draft as a new Deployment",
			!p.deploying && !p.discarding && !p.rollingBack, func(ctx app.Context) {
				p.openDeployDialog(ctx)
			}),
		p.deployButton("Discard", "Reset the Draft to the Deployment the project runs",
			!p.discarding && !p.deploying && !p.rollingBack, func(ctx app.Context) {
				p.discard(ctx)
			}),
	)

	if p.deployDialog {
		body = append(body, p.renderDeployDialog())
	}
	if p.deploymentsDialog {
		body = append(body, p.renderDeploymentsDialog())
	}
	if p.projectFileDialog {
		body = append(body, p.renderProjectFileDialog())
	}
	if p.saveDialog {
		body = append(body, p.renderSaveDialog())
	}
	if p.commitsDialog {
		body = append(body, p.renderCommitsDialog())
	}
	if p.releasesDialog {
		body = append(body, p.renderReleasesDialog())
	}
	if p.releaseDialog {
		body = append(body, p.renderReleaseDialog())
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
	if preview.CurrentDeployment != nil {
		lines = append(lines, app.P().
			Style("margin", "0 0 6px").
			Text(fmt.Sprintf("Current deployment: %d, deployed %s.",
				preview.CurrentDeployment.Number, formatDeployedAt(preview.CurrentDeployment.DeployedAt))))
	} else {
		lines = append(lines, app.P().
			Style("margin", "0 0 6px").
			Text("The project has not been deployed yet."))
	}
	if preview.Unchanged {
		lines = append(lines, app.P().
			Style("margin", "0 0 6px").
			Text("Nothing to deploy: the Draft is the deployment it runs."))
	} else {
		lines = append(lines, app.P().
			Style("margin", "0 0 6px").
			Text("The whole Draft becomes a new Deployment and Operation switches to it."))
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
	for _, missing := range preview.MissingReleases {
		lines = append(lines, app.P().
			Style("margin", "0 0 6px").
			Style("color", "var(--toast-warn-title)").
			Text(fmt.Sprintf("The running Deployment names Library Release %d of library %s, which this server does not have: a Discard drops its widgets.",
				missing.Release, missing.LibraryID)))
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

// renderDeploymentsDialog is the list of the project's Deployments with the
// summary of the selected one: newest first, what caused each, and the running
// one marked, whose rollback is not offered (task 36, ADR 0014).
func (p *Project) renderDeploymentsDialog() app.UI {
	rows := []app.UI{}
	switch {
	case !p.deploymentsLoaded:
		rows = append(rows, app.P().
			Style("margin", "0 0 6px").
			Style("color", "var(--text-muted)").
			Text("Loading deployments…"))
	case len(p.deployments) == 0:
		rows = append(rows, app.P().
			Style("margin", "0 0 6px").
			Text("The project has not been deployed yet."))
	default:
		for _, deployment := range p.deployments {
			rows = append(rows, p.renderDeploymentRow(deployment))
		}
	}
	if p.selectedDeployment != 0 {
		rows = append(rows, p.renderDeploymentSummary())
	}

	buttons := []app.UI{
		app.Button().
			Style("padding", "4px 12px").
			Style("font-size", "13px").
			Style("cursor", "pointer").
			Text("Close").
			OnClick(func(ctx app.Context, e app.Event) {
				p.closeDeploymentsDialog()
				ctx.Update()
			}),
	}
	if p.selectedDeployment != 0 && !p.isCurrentDeployment(p.selectedDeployment) {
		buttons = append(buttons, app.Button().
			Attr("data-deployment-rollback", "true").
			Disabled(p.rollingBack || p.deploying || p.discarding).
			Style("padding", "4px 12px").
			Style("font-size", "13px").
			Style("cursor", "pointer").
			Style("background", "var(--accent)").
			Style("color", "var(--accent-text)").
			Text("Roll back").
			OnClick(func(ctx app.Context, e app.Event) {
				p.rollbackDeployment(ctx)
			}))
	}

	return app.Div().
		Attr("data-deployments-dialog", "true").
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
			app.H3().Style("margin", "0 0 8px").Text("Deployments"),
			app.Div().Body(rows...),
			app.Div().
				Style("display", "flex").
				Style("justify-content", "flex-end").
				Style("gap", "8px").
				Style("margin-top", "10px").
				Body(buttons...),
		)
}

// renderDeploymentRow is one Deployment of the list: its number, when it was
// deployed and why; the running one is marked.
func (p *Project) renderDeploymentRow(deployment apiv0.DeploymentSummaryResponse) app.UI {
	body := []app.UI{
		app.Span().Text(fmt.Sprintf("#%d · %s · %s",
			deployment.Number, formatDeployedAt(deployment.DeployedAt), deploymentCauseText(deployment.Cause, deployment.RolledBackFrom))),
	}
	if p.isCurrentDeployment(deployment.Number) {
		body = append(body, app.Span().
			Attr("data-deployment-current", "true").
			Style("margin-left", "8px").
			Style("font-size", "11px").
			Style("color", "var(--accent)").
			Text("current"))
	}
	body = append(body, app.Button().
		Attr("data-deployment-download", fmt.Sprint(deployment.Number)).
		Title(fmt.Sprintf("Download deployment %d as a project file", deployment.Number)).
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
			p.downloadDeploymentProjectFile(ctx, deployment.Number)
		}))
	row := app.Div().
		Attr("data-deployment-row", fmt.Sprint(deployment.Number)).
		Style("display", "flex").
		Style("align-items", "center").
		Style("justify-content", "space-between").
		Style("padding", "4px 6px").
		Style("cursor", "pointer").
		Style("border-radius", "4px").
		Body(body...).
		OnClick(func(ctx app.Context, e app.Event) {
			p.selectDeployment(ctx, deployment.Number)
			ctx.Update()
		})
	if p.selectedDeployment == deployment.Number {
		row = row.
			Style("background", "var(--bg-elevated)").
			Style("font-weight", "600")
	}
	return row
}

// renderDeploymentSummary is the selected Deployment's document as counts, for
// comparison with what the project runs.
func (p *Project) renderDeploymentSummary() app.UI {
	if !p.deploymentDetailLoaded {
		return app.P().
			Style("margin", "8px 0 0").
			Style("color", "var(--text-muted)").
			Text(fmt.Sprintf("Loading deployment %d…", p.selectedDeployment))
	}
	detail := p.deploymentDetail
	widgets := 0
	for _, scene := range detail.Content.Scenes {
		widgets += len(scene.Widgets)
	}
	return app.Div().
		Attr("data-deployment-summary", "true").
		Style("margin", "8px 0 0").
		Style("padding", "8px").
		Style("border-top", "1px solid var(--border)").
		Body(
			app.P().
				Style("margin", "0 0 4px").
				Text(fmt.Sprintf("Deployment %d · %s · %s",
					detail.Number, formatDeployedAt(detail.DeployedAt), deploymentCauseText(detail.Cause, detail.RolledBackFrom))),
			app.P().
				Style("margin", "0").
				Style("color", "var(--text-2)").
				Text(fmt.Sprintf("%d scenes, %d widgets, %d tags, %d devices, %d environments",
					len(detail.Content.Scenes), widgets, len(detail.Content.Tags),
					len(detail.Content.Devices), len(detail.Content.Environments))),
		)
}

// deploymentCauseText renders why a Deployment exists: a Deploy, or a Rollback
// naming the Deployment it copied. A cause this build does not know is shown as
// it came rather than passed off as a Deploy.
func deploymentCauseText(cause apiv0.DeploymentCause, rolledBackFrom *int) string {
	switch cause {
	case apiv0.DeploymentCauseDeploy:
		return "Deployed"
	case apiv0.DeploymentCauseRollback:
		if rolledBackFrom != nil {
			return fmt.Sprintf("Rolled back from #%d", *rolledBackFrom)
		}
		return "Rolled back"
	case apiv0.DeploymentCauseFile:
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
