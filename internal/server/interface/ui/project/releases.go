package project

import (
	"context"
	"fmt"
	"strings"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// The Project mode's Releases (task 72_3; ADR 0021): a Commit is published as
// an immutable Release, the releases are listed, and a note is changed. A
// Release needs no Deploy and no Draft Version: it changes no Draft and keeps
// the history.

// loadReleases (re)loads the Releases of the project, newest first.
func (p *Project) loadReleases(ctx app.Context) {
	if p.content == nil {
		p.releases, p.releasesLoaded = nil, false
		return
	}
	uiutil.LoadOfProject(p.compoCtx, &p.releasesReloader, p.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) ([]apiv0.ReleaseResponse, error) {
			return pc.Releases(c)
		},
		func(ctx app.Context, releases []apiv0.ReleaseResponse) {
			p.releases, p.releasesLoaded = releases, true
		}, p.loadReleases)
}

// openReleasesDialog shows the Releases, read afresh: the list is where a
// Release is made the running Deployment (task 72_4).
func (p *Project) openReleasesDialog(ctx app.Context) {
	p.releasesDialog = true
	p.loadReleases(ctx)
	p.loadDeploy(ctx)
	ctx.Update()
}

// isRunningRelease reports whether the Release number is the one the running
// Deployment came from. A Deployment from a file, or one made before Releases
// existed, matches no Release (task 72_4).
func (p *Project) isRunningRelease(number int) bool {
	return p.deployed && p.deployment.ReleaseNumber != nil && *p.deployment.ReleaseNumber == number
}

// deployRelease makes Release number the running Deployment, after the
// browser's confirmation. Success reloads the Release list and the Deployment;
// the server's refusal (nothing to deploy, a Release gone) is a toast.
func (p *Project) deployRelease(ctx app.Context, number int) {
	if p.content == nil || p.deploying || p.discarding || p.reverting {
		return
	}
	message := fmt.Sprintf("Make Release %d the running one? Operation switches to it.", number)
	if !app.Window().Call("confirm", message).Bool() {
		return
	}
	p.doDeployRelease(ctx, number)
}

// doDeployRelease performs the Deploy the confirmation asked for.
func (p *Project) doDeployRelease(ctx app.Context, number int) {
	if p.content == nil || p.deploying || p.discarding || p.reverting {
		return
	}
	p.deploying = true
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.DeployResponse, error) {
		return pc.DeployRelease(c, number)
	}, func(ctx app.Context, deployed apiv0.DeployResponse) {
		p.deploying = false
		ctx.NewActionWithValue(toast.ActionAdd, toast.Problem{Problem: apiv0.Problem{
			Title:  fmt.Sprintf("Release %d deployed", number),
			Detail: fmt.Sprintf("Deployment %d is running", deployed.Number),
		}})
		p.loadReleases(ctx)
		p.loadDeploy(ctx)
		if p.deploymentsDialog {
			p.loadDeployments(ctx)
		}
		ctx.Update()
	}, func(ctx app.Context, err error) {
		p.deploying = false
		uiutil.ShowError(ctx, err)
		p.loadReleases(ctx)
		p.loadDeploy(ctx)
		ctx.Update()
	})
}

// openReleaseDialog opens the publication of a Commit: a note and the author.
func (p *Project) openReleaseDialog(commitNumber int) {
	p.releaseCommit = commitNumber
	p.releaseNote = ""
	p.releaseAuthor = p.saveAuthor
	p.releaseDialog = true
}

// publishRelease confirms the release dialog: the Commit becomes the next
// immutable Release. It changes no Draft, so no Version is stated.
func (p *Project) publishRelease(ctx app.Context) {
	if p.content == nil || p.releasing || p.releaseCommit == 0 {
		return
	}
	author := strings.TrimSpace(p.releaseAuthor)
	if author == "" {
		uiutil.ShowError(ctx, fmt.Errorf("an author is required"))
		return
	}
	number := p.releaseCommit
	p.releasing = true
	ctx.LocalStorage().Set(uiutil.AuthorKey, author)
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.ReleaseResponse, error) {
		return pc.CreateRelease(c, apiv0.CreateProjectReleaseRequest{Commit: number, Note: p.releaseNote, Author: author})
	}, func(ctx app.Context, created apiv0.ReleaseResponse) {
		p.releasing, p.releaseDialog, p.releaseCommit = false, false, 0
		ctx.NewActionWithValue(toast.ActionAdd, toast.Problem{Problem: apiv0.Problem{
			Title:  fmt.Sprintf("Release %d published", created.Number),
			Detail: created.Title,
		}})
		p.loadReleases(ctx)
		p.loadCommits(ctx)
		ctx.Update()
	}, func(ctx app.Context, err error) {
		p.releasing = false
		// A 409 is a Commit published meanwhile: the buttons follow the
		// history once it is reread.
		p.rejectedDraftWrite(ctx, err, func(ctx app.Context) {
			p.loadReleases(ctx)
			p.loadCommits(ctx)
		})
		ctx.Update()
	})
}

// startEditingReleaseNote opens the inline editor of a Release's note.
func (p *Project) startEditingReleaseNote(number int, current string) {
	p.editingRelease, p.releaseNoteDraft = number, current
}

// saveReleaseNote saves the inline note of a Release. It states no Draft
// Version: a note is metadata.
func (p *Project) saveReleaseNote(ctx app.Context) {
	number := p.editingRelease
	if number == 0 {
		return
	}
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.ReleaseResponse, error) {
		return pc.UpdateReleaseNote(c, number, apiv0.UpdateReleaseNoteRequest{Note: p.releaseNoteDraft})
	}, func(ctx app.Context, _ apiv0.ReleaseResponse) {
		p.editingRelease, p.releaseNoteDraft = 0, ""
		p.loadReleases(ctx)
		ctx.Update()
	}, nil)
}

// renderReleaseBar is the header's button of the Releases.
func (p *Project) renderReleaseBar() app.UI {
	return p.deployButton("Releases", "The Releases of the project", !p.releasing, func(ctx app.Context) {
		p.openReleasesDialog(ctx)
	})
}

// renderReleaseDialog is the publication confirmation: the Commit's number, a
// note and the author.
func (p *Project) renderReleaseDialog() app.UI {
	return app.Div().
		Attr("data-release-dialog", "true").
		Style("position", "absolute").
		Style("top", "48px").
		Style("left", "16px").
		Style("z-index", "20").
		Style("width", "380px").
		Style("padding", "14px").
		Style("border", "1px solid var(--border)").
		Style("border-radius", "6px").
		Style("background", "var(--bg)").
		Style("box-shadow", "var(--toast-shadow)").
		Style("font-size", "13px").
		Body(
			app.H3().Style("margin", "0 0 8px").Text(fmt.Sprintf("Release Commit %d", p.releaseCommit)),
			app.P().Style("margin", "0 0 6px").Style("color", "var(--text-muted)").
				Text("The Commit is published as an immutable Release: its content is copied and outlives the Commit."),
			app.Input().
				Attr("data-release-note", "true").
				Type("text").
				Value(p.releaseNote).
				Placeholder("Note (optional)").
				Style("width", "100%").
				Style("box-sizing", "border-box").
				Style("margin-bottom", "6px").
				Style("padding", "4px 6px").
				Style("border", "1px solid var(--border-input)").
				Style("border-radius", "4px").
				Style("background", "var(--input-bg)").
				Style("color", "var(--text)").
				OnInput(func(ctx app.Context, e app.Event) {
					p.releaseNote = ctx.JSSrc().Get("value").String()
				}),
			app.Input().
				Attr("data-release-author", "true").
				Type("text").
				Value(p.releaseAuthor).
				Placeholder("Author").
				Style("width", "100%").
				Style("box-sizing", "border-box").
				Style("margin-bottom", "10px").
				Style("padding", "4px 6px").
				Style("border", "1px solid var(--border-input)").
				Style("border-radius", "4px").
				Style("background", "var(--input-bg)").
				Style("color", "var(--text)").
				OnInput(func(ctx app.Context, e app.Event) {
					p.releaseAuthor = ctx.JSSrc().Get("value").String()
				}),
			app.Div().
				Style("display", "flex").
				Style("justify-content", "flex-end").
				Style("gap", "8px").
				Body(
					app.Button().
						Style("padding", "4px 12px").
						Style("font-size", "13px").
						Style("cursor", "pointer").
						Text("Cancel").
						OnClick(func(ctx app.Context, e app.Event) {
							p.releaseDialog, p.releaseCommit = false, 0
							ctx.Update()
						}),
					app.Button().
						Attr("data-release-confirm", "true").
						Disabled(p.releasing).
						Style("padding", "4px 12px").
						Style("font-size", "13px").
						Style("cursor", "pointer").
						Style("background", "var(--accent)").
						Style("color", "var(--accent-text)").
						Text("Release").
						OnClick(func(ctx app.Context, e app.Event) {
							p.publishRelease(ctx)
						}),
				),
		)
}

// renderReleasesDialog is the list of Releases: number, the Commit's title,
// note, author, time and counts, each row offering its note's edit and the
// download of the copied ProjectFile.
func (p *Project) renderReleasesDialog() app.UI {
	rows := []app.UI{}
	switch {
	case p.deploymentFailed:
		// The Deployment could not be read: the list is the Engineering node's
		// and stays, but which Release runs cannot be checked (task 72_4).
		rows = append(rows, app.P().
			Attr("data-release-running-unknown", "true").
			Style("margin", "0 0 6px").
			Style("color", "var(--toast-warn-title)").
			Text("The running Release cannot be checked: the Deployment of the project cannot be read."))
	case p.deployed && p.deployment.ReleaseNumber == nil:
		rows = append(rows, app.P().
			Attr("data-release-running-deployment", "true").
			Style("margin", "0 0 6px").
			Style("color", "var(--toast-warn-title)").
			Text(fmt.Sprintf("Running Deployment %d (not a Release): choose a Release to deploy.",
				p.deployment.Number)))
	}
	switch {
	case !p.releasesLoaded:
		rows = append(rows, app.P().Style("margin", "0 0 6px").Style("color", "var(--text-muted)").
			Text("Loading releases…"))
	case len(p.releases) == 0:
		rows = append(rows, app.P().Style("margin", "0 0 6px").
			Text("The project has not been released yet."))
	default:
		for _, released := range p.releases {
			rows = append(rows, p.renderReleaseRow(released))
		}
	}
	return app.Div().
		Attr("data-releases-dialog", "true").
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
			app.H3().Style("margin", "0 0 8px").Text("Releases"),
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
						p.releasesDialog = false
						ctx.Update()
					})),
		)
}

// renderReleaseRow is one Release of the list, its note edited in place.
func (p *Project) renderReleaseRow(released apiv0.ReleaseResponse) app.UI {
	var note app.UI
	if p.editingRelease == released.Number {
		note = app.Span().Body(
			app.Input().
				Attr("data-release-note-input", fmt.Sprint(released.Number)).
				Type("text").
				Value(p.releaseNoteDraft).
				AutoFocus(true).
				Style("font-size", "12px").
				OnInput(func(ctx app.Context, e app.Event) {
					p.releaseNoteDraft = ctx.JSSrc().Get("value").String()
				}),
			app.Button().
				Attr("data-release-note-save", fmt.Sprint(released.Number)).
				Style("margin-left", "4px").
				Text("Save").
				OnClick(func(ctx app.Context, e app.Event) {
					p.saveReleaseNote(ctx)
				}),
			app.Button().
				Style("margin-left", "4px").
				Text("Cancel").
				OnClick(func(ctx app.Context, e app.Event) {
					p.editingRelease, p.releaseNoteDraft = 0, ""
					ctx.Update()
				}),
		)
	} else {
		note = app.Button().
			Attr("data-release-edit-note", fmt.Sprint(released.Number)).
			Title("Change the note of this Release").
			Style("padding", "1px 7px").
			Style("font-size", "11px").
			Style("cursor", "pointer").
			Style("border", "1px solid var(--border-input)").
			Style("border-radius", "4px").
			Style("background", "var(--bg)").
			Style("color", "var(--text)").
			Text("Edit note").
			OnClick(func(ctx app.Context, e app.Event) {
				p.startEditingReleaseNote(released.Number, released.Note)
				ctx.Update()
			})
	}
	return app.Div().
		Attr("data-release-row", fmt.Sprint(released.Number)).
		Style("display", "flex").
		Style("align-items", "center").
		Style("justify-content", "space-between").
		Style("gap", "8px").
		Style("padding", "4px 6px").
		Style("border-bottom", "1px solid var(--border)").
		Body(
			app.Span().Body(
				app.Strong().Text(fmt.Sprintf("#%d ", released.Number)),
				app.Span().Text(released.Title),
				app.Span().Style("margin-left", "8px").Style("color", "var(--text-muted)").
					Text(fmt.Sprintf("%s · %s", released.Author, commitTime(released.CreatedAt))),
				app.Span().Style("margin-left", "8px").Style("color", "var(--text-muted)").
					Text(fmt.Sprintf("%d scenes, %d widgets, %d tags, %d devices, %d environments",
						released.ContentCounts.Scenes, released.ContentCounts.Widgets, released.ContentCounts.Tags,
						released.ContentCounts.Devices, released.ContentCounts.Environments)),
				app.Span().
					Attr("data-release-note-text", fmt.Sprint(released.Number)).
					Style("margin-left", "8px").
					Text(released.Note),
			),
			app.Span().Style("display", "flex").Style("gap", "4px").Body(
				p.releaseDeployControl(released.Number),
				p.downloadReleaseButton(released.Number),
				note,
			),
		)
}

// releaseDeployControl marks the running Release and offers every other one
// for Deploy (task 72_4).
func (p *Project) releaseDeployControl(number int) app.UI {
	if p.isRunningRelease(number) {
		return app.Span().
			Attr("data-release-running", fmt.Sprint(number)).
			Style("padding", "1px 7px").
			Style("font-size", "11px").
			Style("font-weight", "600").
			Style("color", "var(--accent)").
			Text("Running")
	}
	return app.Button().
		Attr("data-release-deploy", fmt.Sprint(number)).
		Title("Make this Release the running Deployment").
		Disabled(p.deploying || p.discarding || p.reverting).
		Style("padding", "1px 7px").
		Style("font-size", "11px").
		Style("cursor", "pointer").
		Style("border", "1px solid var(--border-input)").
		Style("border-radius", "4px").
		Style("background", "var(--accent)").
		Style("color", "var(--accent-text)").
		Text("Deploy").
		OnClick(func(ctx app.Context, e app.Event) {
			p.deployRelease(ctx, number)
		})
}

// releaseCommitButton offers a Commit's publication as a Release. A Commit
// published already has the button disabled with the reason in its tooltip
// (task 72_3; ADR 0021).
func (p *Project) releaseCommitButton(commit apiv0.CommitResponse) app.UI {
	button := app.Button().
		Attr("data-commit-release", fmt.Sprint(commit.Number)).
		Style("padding", "1px 7px").
		Style("font-size", "11px").
		Style("cursor", "pointer").
		Style("border", "1px solid var(--border-input)").
		Style("border-radius", "4px").
		Style("background", "var(--bg)").
		Style("color", "var(--text)").
		Text("Make Release")
	if commit.Released {
		return button.Disabled(true).Title("This Commit is released already")
	}
	return button.Title("Publish this Commit as a Release").OnClick(func(ctx app.Context, e app.Event) {
		p.openReleaseDialog(commit.Number)
		ctx.Update()
	})
}

// downloadReleaseButton offers the ProjectFile a Release copied as a download.
func (p *Project) downloadReleaseButton(number int) app.UI {
	return app.Button().
		Attr("data-release-download", fmt.Sprint(number)).
		Title("Download the project file this Release copied").
		Style("padding", "1px 7px").
		Style("font-size", "11px").
		Style("cursor", "pointer").
		Style("border", "1px solid var(--border-input)").
		Style("border-radius", "4px").
		Style("background", "var(--bg)").
		Style("color", "var(--text)").
		Text("Project file").
		OnClick(func(ctx app.Context, e app.Event) {
			p.downloadReleaseProjectFile(ctx, number)
		})
}

// downloadReleaseProjectFile downloads the ProjectFile a Release copied.
func (p *Project) downloadReleaseProjectFile(ctx app.Context, number int) {
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiclient.ProjectFileDownload, error) {
		return pc.ReleaseProjectFile(c, number)
	}, func(ctx app.Context, download apiclient.ProjectFileDownload) {
		saveProjectFile(download.Filename, download.Data)
		ctx.Update()
	}, nil)
}
