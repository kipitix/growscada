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

// The Project mode's deployment state (task 35, part 5; task 72_4): the
// Deployment the project runs and the Release it came from, the Deployments
// history, Discard to the last Commit and Revert to the running Deployment. A
// Deploy takes a Release, so the header's Deploy button opens the Release list
// (releases.go); the Deployment itself is Operation's.

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

// closeDeploymentsDialog closes the list.
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
// The Deployment state is the truth; the list is the fallback while it has not
// answered, so the mark agrees with the history.
func (p *Project) isCurrentDeployment(number int) bool {
	if p.deployed {
		return p.deployment.Number == number
	}
	return len(p.deployments) > 0 && p.deployments[0].Number == number
}

// loadDeploy (re)loads the Deployment the project runs: the Release it came
// from is what the Release list marks as running. A failed read is not "not
// deployed": deploymentFailed makes the mode say the running Release cannot be
// checked rather than offer it for Deploy (task 72_4).
func (p *Project) loadDeploy(ctx app.Context) {
	if p.content == nil {
		p.deployment, p.deployed, p.deploymentLoaded, p.deploymentFailed = apiv0.DeploymentResponse{}, false, false, false
		return
	}
	if !p.deployReloader.Start() {
		return
	}
	content := p.content
	uiutil.Do(p.compoCtx, func(c context.Context) (apiclient.DeploymentState, error) {
		return content.Deployment(c)
	}, func(ctx app.Context, state apiclient.DeploymentState, err error) {
		rerun := p.deployReloader.Done()
		switch {
		case content != p.content:
			// A response of the project left: it says nothing of the one shown.
			rerun = true
		case err != nil:
			p.deploymentFailed = true
			uiutil.ShowError(ctx, err)
		default:
			p.deployment, p.deployed, p.deploymentLoaded, p.deploymentFailed = state.Deployment, state.Deployed, true, false
		}
		if rerun {
			p.loadDeploy(ctx)
		}
	})
}

// discard resets the Draft to the last Commit, dropping the unsaved changes,
// after the browser's confirmation. Without a Commit the server refuses (409),
// which is the toast.
func (p *Project) discard(ctx app.Context) {
	if p.content == nil || p.discarding || p.deploying || p.reverting {
		return
	}
	if p.waitMoves(ctx, func(ctx app.Context) { p.discard(ctx) }) {
		return
	}
	message := "Discard the unsaved changes of this project and restore the Draft from its last Commit?"
	if !app.Window().Call("confirm", message).Bool() {
		return
	}
	p.doDiscard(ctx)
}

// doDiscard performs the reset the confirmation asked for.
func (p *Project) doDiscard(ctx app.Context) {
	if p.content == nil || p.discarding || p.deploying || p.reverting {
		return
	}
	p.discarding = true
	mark := p.markDraft()
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.DiscardProjectResponse, error) {
		return pc.Discard(c)
	}, func(ctx app.Context, discarded apiv0.DiscardProjectResponse) {
		p.discarding = false
		p.observeDraftVersion(mark, discarded.Version)
		// The server's Draft state says whether the reset made the Draft its
		// last Commit's content; the flag is reread with the content (task 74).
		p.reloadDraftContent(ctx) // a Discard clears the stack (ADR 0017)
		p.showDiscardWarnings(ctx, discarded)
		ctx.Update()
	}, func(ctx app.Context, err error) {
		p.discarding = false
		uiutil.ShowError(ctx, err)
		p.loadDraftState(ctx)
		p.loadDraftChanges(ctx)
		ctx.Update()
	})
}

// revert resets the Draft to the Deployment the project runs, after the
// browser's confirmation. Without a Deployment the server refuses (409), which
// is the toast.
func (p *Project) revert(ctx app.Context) {
	if p.content == nil || p.reverting || p.deploying || p.discarding {
		return
	}
	if p.waitMoves(ctx, func(ctx app.Context) { p.revert(ctx) }) {
		return
	}
	message := "Reset the Draft to the Deployment the project runs?"
	if !app.Window().Call("confirm", message).Bool() {
		return
	}
	p.doRevert(ctx)
}

// doRevert performs the reset the confirmation asked for.
func (p *Project) doRevert(ctx app.Context) {
	if p.content == nil || p.reverting || p.deploying || p.discarding {
		return
	}
	p.reverting = true
	mark := p.markDraft()
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.DiscardProjectResponse, error) {
		return pc.RevertToDeployed(c)
	}, func(ctx app.Context, reverted apiv0.DiscardProjectResponse) {
		p.reverting = false
		p.observeDraftVersion(mark, reverted.Version)
		// The server's Draft state says whether the Deployment was the content
		// of the last Commit; a Revert to another state keeps the Draft
		// unsaved (task 74).
		p.reloadDraftContent(ctx)
		p.showDiscardWarnings(ctx, reverted)
		ctx.Update()
	}, func(ctx app.Context, err error) {
		p.reverting = false
		uiutil.ShowError(ctx, err)
		ctx.Update()
	})
}

// reloadDraftContent rereads what a Discard or a Revert replaced: the Draft's
// content, its Version and stack, the history and the running Deployment. The
// Scenes carry the Widgets of the selected one; the events do the same, but a
// reset is rare and the screen should not wait for them (task 72_6).
func (p *Project) reloadDraftContent(ctx app.Context) {
	// The whole content was replaced: the Scene list is the truth whatever
	// Version it states, and it rereads the Widgets of the selected Scene
	// (setScenes, task 72_6).
	p.contentReplacements++
	p.loadScenes(ctx)
	p.loadTags(ctx)
	p.loadPins(ctx)
	p.loadWidgetTypes(ctx)
	p.loadDraftState(ctx) // its Version and whether the new content is a Commit's
	p.loadDraftChanges(ctx)
	p.loadCommits(ctx)
	p.loadDeploy(ctx)
}

// showDiscardWarnings tells what a Discard or Revert could not restore: the
// Library Releases this server does not have went with the widgets of their
// types (task 71).
func (p *Project) showDiscardWarnings(ctx app.Context, answer apiv0.DiscardProjectResponse) {
	if len(answer.Warnings) == 0 {
		return
	}
	parts := make([]string, len(answer.Warnings))
	for i, warning := range answer.Warnings {
		parts[i] = fmt.Sprintf("library %s release %d: %d widgets dropped",
			warning.LibraryID, warning.Release, warning.DroppedWidgets)
	}
	ctx.NewActionWithValue(toast.ActionAdd, toast.Problem{Problem: apiv0.Problem{
		Title:  "Restored without some Library Releases",
		Detail: strings.Join(parts, "; "),
	}})
}

// renderDeployBar is the project's header: the Deployment it runs and its
// Release, and the Deploy, Discard changes, Revert to deployed, Deployments and
// project file actions.
func (p *Project) renderDeployBar() app.UI {
	badge := "Not deployed"
	switch {
	case p.deployed && p.deployment.ReleaseNumber != nil:
		// The Release is what people choose and name; the Deployment number
		// stays in the Deployments history and the detail (task 72_6).
		badge = fmt.Sprintf("Release %d · running", *p.deployment.ReleaseNumber)
	case p.deployed:
		badge = fmt.Sprintf("Deployment %d · not a Release", p.deployment.Number)
	case p.deploymentFailed:
		// The read failed: the mode does not claim the project is not
		// deployed (task 72_4).
		badge = "Deployment unknown"
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
	body = append(body,
		app.Div().Style("flex", "1"),
		p.deployButton("Project file…", "Download the Draft, open a file, or deploy one",
			!p.deploying && !p.discarding && !p.reverting && !p.projectFileBusy, func(ctx app.Context) {
				p.openProjectFileDialog(ctx)
			}),
		p.deployButton("Deployments", "The Deployments of the project",
			!p.deploying && !p.discarding && !p.reverting, func(ctx app.Context) {
				p.openDeploymentsDialog(ctx)
			}),
		p.deployButton("Deploy", p.deployButtonTitle(),
			!p.deploying && !p.discarding && !p.reverting, func(ctx app.Context) {
				p.openReleasesDialog(ctx)
			}),
		p.deployButton("Discard changes", "Reset the Draft to its last Commit, dropping the unsaved changes",
			!p.discarding && !p.deploying && !p.reverting, func(ctx app.Context) {
				p.discard(ctx)
			}),
		p.deployButton("Revert to deployed", "Reset the Draft to the Deployment the project runs",
			!p.reverting && !p.deploying && !p.discarding, func(ctx app.Context) {
				p.revert(ctx)
			}),
	)

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

// deployButtonTitle is the Deploy button's hover text: a Release list with
// nothing in it says what to do first (task 72_6).
func (p *Project) deployButtonTitle() string {
	if p.releasesLoaded && len(p.releases) == 0 {
		return "Save the Draft and publish a Release first: only a Release is deployed"
	}
	return "Choose a Release to deploy"
}

// deployButton is one action of the Project's header; the label is also the
// EventScope, so a reused element never keeps another action's handler (task
// 80). It is named for its first use and is the header's general button.
func (p *Project) deployButton(label, title string, enabled bool, onClick func(app.Context)) app.UI {
	return uiutil.Button(label, "", enabled, title, uiutil.ButtonBar,
		func(ctx app.Context, e app.Event) { onClick(ctx) }, app.EventScope(label))
}

// renderDeploymentsDialog is the list of the project's Deployments with the
// summary of the selected one: newest first, what caused each, the Release it
// came from and the running one marked (task 36, ADR 0014; task 72_4).
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
				Body(app.Button().
					Style("padding", "4px 12px").
					Style("font-size", "13px").
					Style("cursor", "pointer").
					Text("Close").
					OnClick(func(ctx app.Context, e app.Event) {
						p.closeDeploymentsDialog()
						ctx.Update()
					})),
		)
}

// renderDeploymentRow is one Deployment of the list: its number, when it was
// deployed, why and from which Release; the running one is marked.
func (p *Project) renderDeploymentRow(deployment apiv0.DeploymentSummaryResponse) app.UI {
	body := []app.UI{
		app.Span().Text(fmt.Sprintf("#%d · %s · %s",
			deployment.Number, formatDeployedAt(deployment.DeployedAt), deploymentCauseText(deployment.Cause, deployment.RolledBackFrom))),
	}
	if deployment.ReleaseNumber != nil {
		body = append(body, app.Span().
			Style("margin-left", "8px").
			Style("font-size", "11px").
			Style("color", "var(--text-muted)").
			Text(fmt.Sprintf("Release %d", *deployment.ReleaseNumber)))
	}
	if p.isCurrentDeployment(deployment.Number) {
		body = append(body, app.Span().
			Attr("data-deployment-current", "true").
			Style("margin-left", "8px").
			Style("font-size", "11px").
			Style("color", "var(--accent)").
			Text("current"))
	}
	body = append(body, uiutil.Button("Download", "", true,
		fmt.Sprintf("Download deployment %d as a project file", deployment.Number),
		uiutil.ButtonRow,
		func(ctx app.Context, e app.Event) {
			e.Call("stopPropagation")
			e.StopImmediatePropagation()
			p.downloadDeploymentProjectFile(ctx, deployment.Number)
		}, app.EventScope("download-deployment/"+fmt.Sprint(deployment.Number))).
		Attr("data-deployment-download", fmt.Sprint(deployment.Number)).
		Style("margin-left", "8px"))
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
