package project

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/savedstates"
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// The Project mode's deployment state (task 35, part 5; task 85_4): the
// Deployment the project runs and the saved state it came from, the Deployments
// history, Discard to the last saved state and Revert to the running
// Deployment. A Deploy makes a saved state the running one, so the panel's
// Deploy button opens the saved states, narrowed by a label (task 85_5); the
// Deployment itself is Operation's.

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

// loadDeploy (re)loads the Deployment the project runs: the saved state it came
// from is what tells the Deploy window which of its rows is running. A failed
// read is not "not deployed": deploymentFailed makes the mode say the running
// saved state cannot be checked rather than offer it for Deploy (task 72_4;
// task 85_4).
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

// ── The Deploy window (task 85_5; ADR 0021, ADR 0025) ───────────────────────
//
// The window lists the saved states of the Draft, newest first, and makes one
// of them the running Deployment. The label filter is the server's:
// GET …/saved-states?label=<id>, so the same filter serves growctl and Bruno.

// openDeployDialog shows the saved states, read afresh, with the labels the
// filter offers: the window is the only place a Deploy is chosen from.
func (p *Project) openDeployDialog(ctx app.Context) {
	p.deployDialog = true
	p.loadDeploySavedStates(ctx)
	p.loadDeploy(ctx)
	p.loadHistoryLabels(ctx)
	ctx.Update()
}

// deployListKey names the list the window shows: the project and the label the
// filter states. The answer of another project or another label is dropped and
// the list reruns (uiutil.LoadFor).
type deployListKey struct {
	content *apiclient.ProjectClient
	label   uuid.UUID
}

// loadDeploySavedStates (re)loads the saved states the window lists: every
// one, or the ones the chosen label marks. The list is the Project's, so a 404
// is the Project's absence and goes to the switcher like the history's
// (task 85_5).
func (p *Project) loadDeploySavedStates(ctx app.Context) {
	if p.content == nil {
		p.deploySavedStates, p.deploySavedStatesLoaded = nil, false
		return
	}
	uiutil.LoadForGone(ctx, &p.deploySavedStatesReloader,
		func() deployListKey { return deployListKey{content: p.content, label: p.deployLabel} },
		func(c context.Context, k deployListKey) ([]apiv0.SavedStateResponse, error) {
			if k.content == nil {
				return nil, nil
			}
			if k.label == uuid.Nil {
				return k.content.SavedStates(c)
			}
			return k.content.SavedStatesByLabel(c, k.label)
		},
		func(ctx app.Context, _ deployListKey, savedStates []apiv0.SavedStateResponse) {
			p.deploySavedStates, p.deploySavedStatesLoaded = savedStates, true
		},
		func(ctx app.Context, k deployListKey) {
			if k.content != nil {
				ctx.NewActionWithValue(uiutil.ActionProjectGone, k.content.ID())
			}
		}, p.loadDeploySavedStates)
}

// setDeployLabel narrows the window to the saved states the label marks; the
// zero UUID asks for all of them.
func (p *Project) setDeployLabel(ctx app.Context, labelID uuid.UUID) {
	if labelID == p.deployLabel {
		return
	}
	p.deployLabel = labelID
	p.loadDeploySavedStates(ctx)
	ctx.Update()
}

// resetDeployLabelIfGone drops a filter whose label was deleted elsewhere: the
// menu no longer offers it, so the window shows every saved state again.
func (p *Project) resetDeployLabelIfGone(ctx app.Context) {
	if p.deployLabel == uuid.Nil {
		return
	}
	for _, label := range p.historyLabels {
		if label.ID == p.deployLabel {
			return
		}
	}
	p.deployLabel = uuid.Nil
	if p.deployDialog {
		p.loadDeploySavedStates(ctx)
	}
}

// isRunningSavedState reports whether number is the saved state the running
// Deployment came from. A Deployment from a file, or one made before saved
// states were deployed, matches none (task 85_4); neither does any saved state
// while the Deployment cannot be read: the window says so rather than mark a
// row from a stale answer (task 85_5).
func (p *Project) isRunningSavedState(number int) bool {
	return !p.deploymentFailed && p.deployed && p.deployment.SavedStateNumber != nil &&
		*p.deployment.SavedStateNumber == number
}

// deploySavedState makes saved state number the running Deployment, after the
// browser's confirmation. Success reloads the window and the Deployment; the
// server's refusal (nothing to deploy, a saved state gone) is a toast.
func (p *Project) deploySavedState(ctx app.Context, number int) {
	if p.content == nil || p.deploying || p.loadingSavedState != 0 {
		return
	}
	message := fmt.Sprintf("Make saved state %d the running one? Operation switches to it.", number)
	if !app.Window().Call("confirm", message).Bool() {
		return
	}
	p.doDeploySavedState(ctx, number)
}

// doDeploySavedState performs the Deploy the confirmation asked for.
func (p *Project) doDeploySavedState(ctx app.Context, number int) {
	if p.content == nil || p.deploying || p.loadingSavedState != 0 {
		return
	}
	p.deploying = true
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.DeployResponse, error) {
		return pc.DeploySavedState(c, number)
	}, func(ctx app.Context, deployed apiv0.DeployResponse) {
		p.deploying = false
		ctx.NewActionWithValue(toast.ActionAdd, toast.Problem{Problem: apiv0.Problem{
			Title:  fmt.Sprintf("Saved state %d deployed", number),
			Detail: fmt.Sprintf("Deployment %d is running", deployed.Number),
		}})
		p.loadDeploySavedStates(ctx)
		p.loadDeploy(ctx)
		if p.deploymentsDialog {
			p.loadDeployments(ctx)
		}
		ctx.Update()
	}, func(ctx app.Context, err error) {
		p.deploying = false
		if nothingToDeploy(err) {
			// Another tab deployed it meanwhile: the list is right, the
			// request was not (task 72_6).
			ctx.NewActionWithValue(toast.ActionAdd, toast.Problem{Problem: apiv0.Problem{
				Title:  "This saved state is running already",
				Detail: "The project runs it: nothing to deploy.",
			}})
		} else {
			uiutil.ShowError(ctx, err)
		}
		p.loadDeploySavedStates(ctx)
		p.loadDeploy(ctx)
		ctx.Update()
	})
}

// nothingToDeploy reports the server's refusal of a Deploy of the saved state
// the project runs already. The Problem carries no code, so its detail names
// it.
func nothingToDeploy(err error) bool {
	var statusErr *apiclient.StatusError
	return errors.Is(err, apiclient.ErrConflict) && errors.As(err, &statusErr) &&
		strings.Contains(statusErr.Problem.Detail, "Nothing to deploy")
}

// downloadSavedStateProjectFile downloads the ProjectFile a saved state fixed.
func (p *Project) downloadSavedStateProjectFile(ctx app.Context, number int) {
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiclient.ProjectFileDownload, error) {
		return pc.SavedStateProjectFile(c, number)
	}, func(ctx app.Context, download apiclient.ProjectFileDownload) {
		saveProjectFile(download.Filename, download.Data)
		ctx.Update()
	}, nil)
}

// renderDeployDialog is the window a Deploy chooses from: the label filter, the
// warning of a running Deployment no saved state made, and each saved state's
// number, title, author, time, counts and labels, with the download of its
// ProjectFile and the Deploy itself (task 85_5).
func (p *Project) renderDeployDialog() app.UI {
	rows := []app.UI{}
	switch {
	case p.deploymentFailed:
		// The Deployment could not be read: the list is the Engineering node's
		// and stays, but which saved state runs cannot be checked.
		rows = append(rows, app.P().
			Attr("data-saved-state-running-unknown", "true").
			Style("margin", "0 0 6px").
			Style("color", "var(--toast-warn-title)").
			Text("The running saved state cannot be checked: the Deployment of the project cannot be read."))
	case p.deployed && p.deployment.SavedStateNumber == nil:
		rows = append(rows, app.P().
			Attr("data-saved-state-running-deployment", "true").
			Style("margin", "0 0 6px").
			Style("color", "var(--toast-warn-title)").
			Text(fmt.Sprintf("Running Deployment %d (no saved state): choose a saved state to deploy.",
				p.deployment.Number)))
	}
	switch {
	case !p.deploySavedStatesLoaded:
		rows = append(rows, app.P().Style("margin", "0 0 6px").Style("color", "var(--text-muted)").
			Text("Loading saved states…"))
	case len(p.deploySavedStates) == 0 && p.deployLabel != uuid.Nil:
		rows = append(rows, app.P().Style("margin", "0 0 6px").
			Text("No saved state carries this label."))
	case len(p.deploySavedStates) == 0:
		rows = append(rows, app.P().Style("margin", "0 0 6px").
			Text("Nothing saved yet. Save the Draft first: a saved state is what gets deployed."))
	default:
		for _, saved := range p.deploySavedStates {
			rows = append(rows, p.renderDeployRow(saved))
		}
	}
	return app.Div().
		Attr("data-deploy-dialog", "true").
		Style("position", "absolute").
		Style("top", "48px").
		Style("left", "16px").
		Style("z-index", "20").
		Style("width", "600px").
		Style("max-height", "70vh").
		Style("overflow-y", "auto").
		Style("padding", "14px").
		Style("border", "1px solid var(--border)").
		Style("border-radius", "6px").
		Style("background", "var(--bg)").
		Style("box-shadow", "var(--toast-shadow)").
		Style("font-size", "13px").
		Body(
			app.H3().Style("margin", "0 0 8px").Text("Deploy a saved state"),
			p.renderDeployLabelFilter(),
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
						p.deployDialog = false
						ctx.Update()
					})),
		)
}

// renderDeployLabelFilter is the window's header: the label the list is
// narrowed to. An Organization without labels has no filter, and says how to
// make one; while the labels are on their way the header claims neither, so a
// slow read never passes for an empty list (task 85_5).
func (p *Project) renderDeployLabelFilter() app.UI {
	switch {
	case !p.historyLabelsLoaded:
		return app.Div()
	case len(p.historyLabels) == 0:
		return app.Div().
			Attr("data-deploy-no-labels", "true").
			Style("display", "flex").
			Style("align-items", "center").
			Style("gap", "6px").
			Style("margin-bottom", "8px").
			Style("font-size", "12px").
			Style("color", "var(--text-muted)").
			Body(
				app.Span().Text("No labels yet"),
				rowButton("Manage labels", "", true, "Open the Labels page",
					func(ctx app.Context) { p.manageLabels(ctx) },
					app.EventScope("deploy-manage-labels")).
					Attr("data-deploy-manage-labels", "true"),
			)
	}
	options := []app.UI{
		app.Option().Value("").Selected(p.deployLabel == uuid.Nil).Text("All labels"),
	}
	for _, label := range p.historyLabels {
		options = append(options, app.Option().
			Value(label.ID.String()).
			Selected(label.ID == p.deployLabel).
			Text(label.Name))
	}
	return app.Div().
		Attr("data-deploy-filter", "true").
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "6px").
		Style("margin-bottom", "8px").
		Body(
			app.Span().
				Style("font-size", "12px").
				Style("color", "var(--text-muted)").
				Text("Label:"),
			app.Select().
				Attr("data-deploy-label", "true").
				Style("font-size", "12px").
				Style("background", "var(--input-bg)").
				Style("color", "var(--text)").
				Style("border", "1px solid var(--border-input)").
				OnChange(func(ctx app.Context, e app.Event) {
					p.setDeployLabel(ctx, uiutil.ParseID(ctx.JSSrc().Get("value").String()))
				}, app.EventScope("deploy-label")).
				Body(options...),
		)
}

// renderDeployRow is one saved state of the window: its number, title, author,
// time, counts and labels, the download of its ProjectFile and the Deploy —
// which the running one does not offer.
func (p *Project) renderDeployRow(saved apiv0.SavedStateResponse) app.UI {
	number := fmt.Sprint(saved.Number)
	body := []app.UI{
		app.Strong().Text(fmt.Sprintf("#%d ", saved.Number)),
		app.Span().Text(saved.Title),
		app.Span().Style("margin-left", "8px").Style("color", "var(--text-muted)").
			Text(fmt.Sprintf("%s · %s", saved.Author, uiutil.FormatTime(saved.CreatedAt))),
		app.Span().Style("margin-left", "8px").Style("color", "var(--text-muted)").
			Text(fmt.Sprintf("%d scenes, %d widgets, %d tags, %d devices, %d environments",
				saved.ContentCounts.Scenes, saved.ContentCounts.Widgets, saved.ContentCounts.Tags,
				saved.ContentCounts.Devices, saved.ContentCounts.Environments)),
	}
	if len(saved.Labels) > 0 {
		chips := []app.UI{}
		for _, label := range saved.Labels {
			chips = append(chips, savedstates.LabelChip(label))
		}
		body = append(body, app.Span().
			Style("display", "inline-flex").
			Style("flex-wrap", "wrap").
			Style("gap", "4px").
			Style("margin-left", "8px").
			Body(chips...))
	}

	controls := []app.UI{}
	if p.isRunningSavedState(saved.Number) {
		controls = append(controls, app.Span().
			Attr("data-saved-state-running", number).
			Style("padding", "1px 7px").
			Style("font-size", "11px").
			Style("font-weight", "600").
			Style("color", "var(--accent)").
			Text("Running"))
	} else {
		controls = append(controls, rowButton("Deploy", "accent", !(p.deploying || p.loadingSavedState != 0),
			"Make this saved state the running Deployment",
			func(ctx app.Context) { p.deploySavedState(ctx, saved.Number) },
			app.EventScope("deploy/"+number)).
			Attr("data-saved-state-deploy", number))
	}
	controls = append(controls, rowButton("Project file", "", true,
		"Download this saved state as a project file",
		func(ctx app.Context) { p.downloadSavedStateProjectFile(ctx, saved.Number) },
		app.EventScope("download-saved-state/"+number)).
		Attr("data-saved-state-download", number))

	return app.Div().
		Attr("data-saved-state-deploy-row", number).
		Style("display", "flex").
		Style("align-items", "center").
		Style("justify-content", "space-between").
		Style("gap", "8px").
		Style("padding", "4px 6px").
		Style("border-bottom", "1px solid var(--border)").
		Body(
			app.Span().Body(body...),
			app.Span().Style("display", "flex").Style("gap", "4px").Body(controls...),
		)
}

// loadSavedState makes the saved state number the Draft (task 84), after the
// browser's confirmation when the Draft holds unsaved changes: the load clears
// the undo stack, so the change is not one to take back.
func (p *Project) loadSavedState(ctx app.Context, number int) {
	if p.content == nil || p.deploying || p.loadingSavedState != 0 {
		return
	}
	if p.waitMoves(ctx, func(ctx app.Context) { p.loadSavedState(ctx, number) }) {
		return
	}
	if p.hasUnsavedChanges() && !app.Window().Call("confirm",
		fmt.Sprintf("Replace the unsaved changes of this project with saved state %d? The Draft becomes what was saved then.",
			number)).Bool() {
		return
	}
	p.doLoadSavedState(ctx, number)
}

// doLoadSavedState performs the reset the confirmation asked for: one
// transaction of the Draft, which the answer states.
func (p *Project) doLoadSavedState(ctx app.Context, number int) {
	if p.content == nil || p.loadingSavedState != 0 {
		return
	}
	p.loadingSavedState = number
	mark := p.markDraft()
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.LoadSavedStateResponse, error) {
		return pc.LoadSavedState(c, number)
	}, func(ctx app.Context, loaded apiv0.LoadSavedStateResponse) {
		p.loadingSavedState = 0
		p.observeDraftVersion(mark, loaded.Version)
		// The loaded content is a saved state's, so the server answers a clean
		// Draft whichever saved state it was; the flag is reread with the
		// content (ADR 0016, amendment 2026-11-16).
		p.reloadDraftContent(ctx) // a load clears the stack (ADR 0017)
		p.showLoadWarnings(ctx, loaded.Warnings)
		p.closeHistory()
		ctx.NewActionWithValue(toast.ActionAdd, toast.Problem{Problem: apiv0.Problem{
			Title: fmt.Sprintf("Saved state %d loaded", number),
		}})
		ctx.Update()
	}, func(ctx app.Context, err error) {
		p.loadingSavedState = 0
		uiutil.ShowError(ctx, err)
		p.loadDraftState(ctx)
		p.loadDraftChanges(ctx)
		ctx.Update()
	})
}

// reloadDraftContent rereads what a load of a saved state replaced: the
// Draft's content, its Version and stack, the history and the running
// Deployment. The
// Scenes carry the Widgets of the selected one; the events do the same, but a
// reset is rare and the screen should not wait for them (task 72_6).
func (p *Project) reloadDraftContent(ctx app.Context) {
	// The whole content was replaced: the Scene list is the truth whatever
	// Version it states, and it rereads the Widgets of the selected Scene
	// (setScenes, task 72_6).
	p.contentReplacements++
	p.loadScenes(ctx)
	p.loadTags(ctx)
	p.loadImports(ctx)
	p.loadWidgetTypes(ctx)
	p.loadDraftState(ctx) // its Version and whether the new content is a saved state's
	p.loadDraftChanges(ctx)
	p.loadSavedStates(ctx)
	p.loadDeploy(ctx)
}

// showLoadWarnings tells what a load of a saved state could not restore: the
// saved states of Libraries this server does not have went with the widgets of
// their types (task 71, task 84).
func (p *Project) showLoadWarnings(ctx app.Context, warnings []apiv0.DiscardWarningResponse) {
	if len(warnings) == 0 {
		return
	}
	parts := make([]string, len(warnings))
	for i, warning := range warnings {
		parts[i] = fmt.Sprintf("library %s saved state %d: %d widgets dropped",
			warning.LibraryID, warning.SavedState, warning.DroppedWidgets)
	}
	ctx.NewActionWithValue(toast.ActionAdd, toast.Problem{Problem: apiv0.Problem{
		Title:  "Restored without some Library Imports",
		Detail: strings.Join(parts, "; "),
	}})
}

// renderDeployUnknown is the panel's word on a Deployment it could not read: a
// failed read is not "not deployed", and without it the panel says nothing
// while the Deploy window refuses to name the running saved state (task 72_4;
// task 91). It is a warning, not the neutral state the panel no longer names.
func (p *Project) renderDeployUnknown() app.UI {
	if !p.deploymentFailed {
		return app.Span().Style("display", "none")
	}
	return app.Span().
		Attr("data-deploy-unknown", "true").
		Style("font-size", "12px").
		Style("color", "var(--toast-warn-title)").
		Text("Deployment unknown")
}

// renderDeployBar is the project's panel: the way back, the floppy that opens
// the saved states, Undo and Redo with the unsaved-changes mark after them, and
// the Deploy, Deployments and project file actions (task 91).
func (p *Project) renderDeployBar() app.UI {
	busy := p.deploying || p.loadingSavedState != 0

	body := []app.UI{
		p.renderBackButton(),
		uiutil.BarDivider(),
		p.renderSaveButton(),
		uiutil.BarDivider(),
		p.renderDraftChangesBar(),
		p.renderUnsavedIndicator(),
		p.renderDeployUnknown(),
		app.Div().Style("flex", "1"),
		p.panelButton("project-file", uiutil.IconFile(), "Download the Draft, open a file, or deploy one",
			!busy && !p.projectFileBusy, func(ctx app.Context) {
				p.openProjectFileDialog(ctx)
			}),
		p.panelButton("deployments", uiutil.IconLayers(), "The Deployments of the project",
			!busy, func(ctx app.Context) {
				p.openDeploymentsDialog(ctx)
			}),
		p.panelButton("deploy", uiutil.IconDeploy(), p.deployButtonTitle(),
			!busy, func(ctx app.Context) {
				p.openDeployDialog(ctx)
			}),
	}

	if p.deploymentsDialog {
		body = append(body, p.renderDeploymentsDialog())
	}
	if p.projectFileDialog {
		body = append(body, p.renderProjectFileDialog())
	}
	if p.deployDialog {
		body = append(body, p.renderDeployDialog())
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

// renderBackButton is the panel's single way back (task 84, task 91): from the
// saved states to the content, from the content to the selection page. Root
// owns the page; a mode nothing wired shows no arrow. The project's name is not
// repeated beside it.
func (p *Project) renderBackButton() app.UI {
	if p.historyScreen {
		return uiutil.PanelBackArrow("Back to the Draft", func(ctx app.Context, e app.Event) {
			p.closeHistory()
		})
	}
	var back func(ctx app.Context, e app.Event)
	if p.OnBack != nil {
		back = func(ctx app.Context, e app.Event) { p.OnBack() }
	}
	return uiutil.PanelBackArrow("Back to the projects", back)
}

// deployButtonTitle is the Deploy button's hover text: a Draft with nothing
// saved says what to do first (task 72_6; task 85_5).
func (p *Project) deployButtonTitle() string {
	if p.savedStatesLoaded && len(p.savedStates) == 0 {
		return "Save the Draft first: only a saved state is deployed"
	}
	return "Choose a saved state to deploy"
}

// panelButton is one icon action of the panel (task 84). scope names the
// action for go-app — a reused element keeps the handler of the scope it had —
// and is never the hover text: rewording a title must not move a handler.
func (p *Project) panelButton(scope string, icon app.UI, title string, enabled bool, onClick func(app.Context)) app.UI {
	return uiutil.PanelButton(icon, enabled, title,
		func(ctx app.Context, e app.Event) { onClick(ctx) }, app.EventScope(scope))
}

// renderDeploymentsDialog is the list of the project's Deployments with the
// summary of the selected one: newest first, what caused each, the saved state
// it came from and the running one marked (task 36, ADR 0014; task 85_4).
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
// deployed, why and from which saved state; the running one is marked.
func (p *Project) renderDeploymentRow(deployment apiv0.DeploymentSummaryResponse) app.UI {
	body := []app.UI{
		app.Span().Text(fmt.Sprintf("#%d · %s · %s",
			deployment.Number, formatDeployedAt(deployment.DeployedAt), deploymentCauseText(deployment.Cause, deployment.RolledBackFrom))),
	}
	if deployment.SavedStateNumber != nil {
		body = append(body, app.Span().
			Style("margin-left", "8px").
			Style("font-size", "11px").
			Style("color", "var(--text-muted)").
			Text(fmt.Sprintf("Saved state #%d", *deployment.SavedStateNumber)))
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
	cause := fmt.Sprintf("Deployment %d · %s · %s",
		detail.Number, formatDeployedAt(detail.DeployedAt), deploymentCauseText(detail.Cause, detail.RolledBackFrom))
	if detail.SavedStateNumber != nil {
		cause += fmt.Sprintf(" · Saved state #%d", *detail.SavedStateNumber)
	}
	return app.Div().
		Attr("data-deployment-summary", "true").
		Style("margin", "8px 0 0").
		Style("padding", "8px").
		Style("border-top", "1px solid var(--border)").
		Body(
			app.P().
				Style("margin", "0 0 4px").
				Text(cause),
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
