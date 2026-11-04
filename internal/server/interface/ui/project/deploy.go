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
// it: what the Deploy would do, not what was last seen.
func (p *Project) openDeployDialog(ctx app.Context) {
	if p.content == nil {
		return
	}
	uiutil.Call(p.compoCtx, p.content.DeployPreview,
		func(ctx app.Context, preview apiv0.DeployPreviewResponse) {
			p.deployPreview, p.deployLoaded = preview, true
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
	if p.content == nil || p.deploying {
		return
	}
	p.deploying = true
	uiutil.Call(p.compoCtx, p.content.Deploy,
		func(ctx app.Context, _ apiv0.DeployResponse) {
			p.deploying = false
			p.deployDialog = false
			p.loadDeploy(ctx)
			ctx.Update()
		},
		func(ctx app.Context, err error) {
			p.deploying = false
			uiutil.ShowError(ctx, err)
			ctx.Update()
		})
}

// discard resets the Draft to the Revision the project runs, after the
// browser's confirmation. Without a Revision the server refuses (409), which
// is the toast.
func (p *Project) discard(ctx app.Context) {
	if p.content == nil || p.discarding {
		return
	}
	message := "Discard the Draft of this project and restore it to the Revision it runs?"
	if !app.Window().Call("confirm", message).Bool() {
		return
	}
	p.discarding = true
	uiutil.Call(p.compoCtx, p.content.Discard,
		func(ctx app.Context, _ apiv0.DiscardProjectResponse) {
			p.discarding = false
			p.loadDeploy(ctx)
			ctx.Update()
		},
		func(ctx app.Context, err error) {
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
		p.deployButton("Deploy", "Deploy the Draft as a new Revision", !p.deploying, func(ctx app.Context) {
			p.openDeployDialog(ctx)
		}),
		p.deployButton("Discard", "Reset the Draft to the Revision the project runs", !p.discarding, func(ctx app.Context) {
			p.discard(ctx)
		}),
	)

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
	preview := p.deployPreview

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
			Disabled(p.deploying).
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

// formatDeployedAt renders the deployment time of the contract (RFC 3339) for
// the badge; a value that does not parse is shown as it came.
func formatDeployedAt(value string) string {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}
