package project

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// The Project mode's Save and its history (task 72_2; ADR 0021): Save fixes
// the Draft as a Commit, the history lists them and their titles are renamed.
// The Draft is still written at once — Save only records a named point of
// history, so the indicator follows the Version of the Draft against the one
// the last Commit fixed.

// loadCommits (re)loads the history of the project's Draft.
func (p *Project) loadCommits(ctx app.Context) {
	if p.content == nil {
		p.commits, p.commitsLoaded = nil, false
		return
	}
	uiutil.LoadOfProject(p.compoCtx, &p.commitsReloader, p.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) ([]apiv0.CommitResponse, error) {
			return pc.Commits(c)
		},
		func(ctx app.Context, commits []apiv0.CommitResponse) {
			p.commits, p.commitsLoaded = commits, true
		}, p.loadCommits)
}

// lastCommitVersion is the Version of the Draft the last Commit fixed, 0 for
// a Draft never saved.
func (p *Project) lastCommitVersion() int {
	if len(p.commits) == 0 {
		return 0
	}
	return p.commits[0].DraftVersion
}

// hasUnsavedChanges reports whether the Draft moved since its last Commit.
func (p *Project) hasUnsavedChanges() bool {
	return p.commitsLoaded && p.draftVersion() > p.lastCommitVersion()
}

// openSaveDialog opens the Save dialog with a default title and the stored
// author.
func (p *Project) openSaveDialog(ctx app.Context) {
	p.saveTitle = "Save " + time.Now().Format("02.01.2006 15:04")
	p.saveDialog = true
	ctx.Update()
}

// saveCommit confirms the Save dialog: the Draft as it is becomes a Commit.
func (p *Project) saveCommit(ctx app.Context) {
	if p.content == nil || p.savingCommit || p.deploying || p.discarding || p.rollingBack {
		return
	}
	if p.waitMoves(ctx, func(ctx app.Context) { p.saveCommit(ctx) }) {
		return
	}
	title, author := strings.TrimSpace(p.saveTitle), strings.TrimSpace(p.saveAuthor)
	if title == "" || author == "" {
		uiutil.ShowError(ctx, fmt.Errorf("a title and an author are required"))
		return
	}
	p.savingCommit = true
	ctx.LocalStorage().Set(uiutil.AuthorKey, author)
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.CommitResponse, error) {
		return pc.SaveCommit(c, apiv0.CreateCommitRequest{
			Version: p.draftVersion(), Title: title, Author: author,
		})
	}, func(ctx app.Context, _ apiv0.CommitResponse) {
		p.savingCommit = false
		p.saveDialog = false
		p.loadCommits(ctx)
		ctx.Update()
	}, func(ctx app.Context, err error) {
		p.savingCommit = false
		// A conflict drops the Draft's Version and rereads the content; a
		// nothing_to_save means the history moved.
		p.rejectedDraftWrite(ctx, err, p.loadCommits)
		ctx.Update()
	})
}

// openCommitsDialog shows the history, read afresh.
func (p *Project) openCommitsDialog(ctx app.Context) {
	p.commitsDialog = true
	p.loadCommits(ctx)
	ctx.Update()
}

// startRenamingCommit opens the inline editor of a Commit's title.
func (p *Project) startRenamingCommit(number int, current string) {
	p.renamingCommit, p.renameCommitTitle = number, current
}

// renameCommit saves the inline title of a Commit. It states no Draft
// Version: another Engineer's writes do not refuse it (ADR 0021).
func (p *Project) renameCommit(ctx app.Context) {
	number, title := p.renamingCommit, strings.TrimSpace(p.renameCommitTitle)
	if number == 0 || title == "" {
		return
	}
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.CommitResponse, error) {
		return pc.RenameCommit(c, number, apiv0.RenameCommitRequest{Title: title})
	}, func(ctx app.Context, _ apiv0.CommitResponse) {
		p.renamingCommit, p.renameCommitTitle = 0, ""
		p.loadCommits(ctx)
		ctx.Update()
	}, nil)
}

// commitTime renders a Commit's time in the browser's zone.
func commitTime(at time.Time) string { return at.Local().Format("2006-01-02 15:04") }

// renderCommitBar is the project header's Save, its unsaved-changes
// indicator and the history button.
func (p *Project) renderCommitBar() []app.UI {
	body := []app.UI{
		p.deployButton("Save", "Save the Draft as a Commit", p.hasUnsavedChanges() && !p.savingCommit, func(ctx app.Context) {
			p.openSaveDialog(ctx)
		}),
	}
	if p.hasUnsavedChanges() {
		body = append(body, app.Span().
			Attr("data-commit-unsaved", "true").
			Style("font-size", "12px").
			Style("color", "var(--toast-warn-title)").
			Text("Unsaved changes"))
	} else if p.commitsLoaded {
		body = append(body, app.Span().
			Attr("data-commit-saved", "true").
			Style("font-size", "12px").
			Style("color", "var(--text-muted)").
			Text("Saved"))
	}
	body = append(body, p.deployButton("History", "The Commits of the project", !p.savingCommit, func(ctx app.Context) {
		p.openCommitsDialog(ctx)
	}))
	body = append(body, p.renderReleaseBar())
	return body
}

// renderSaveDialog is the Save confirmation: a title, the author, and the
// Save itself.
func (p *Project) renderSaveDialog() app.UI {
	return app.Div().
		Attr("data-save-dialog", "true").
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
			app.H3().Style("margin", "0 0 8px").Text("Save the project"),
			app.P().Style("margin", "0 0 6px").Style("color", "var(--text-muted)").
				Text("The whole Draft is fixed as an immutable Commit. Editing continues; Save only marks a point of history."),
			app.Input().
				Attr("data-save-title", "true").
				Type("text").
				Value(p.saveTitle).
				Placeholder("Title").
				Style("width", "100%").
				Style("box-sizing", "border-box").
				Style("margin-bottom", "6px").
				Style("padding", "4px 6px").
				Style("border", "1px solid var(--border-input)").
				Style("border-radius", "4px").
				Style("background", "var(--input-bg)").
				Style("color", "var(--text)").
				OnInput(func(ctx app.Context, e app.Event) {
					p.saveTitle = ctx.JSSrc().Get("value").String()
				}),
			app.Input().
				Attr("data-save-author", "true").
				Type("text").
				Value(p.saveAuthor).
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
					p.saveAuthor = ctx.JSSrc().Get("value").String()
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
							p.saveDialog = false
							ctx.Update()
						}),
					app.Button().
						Attr("data-save-confirm", "true").
						Disabled(p.savingCommit).
						Style("padding", "4px 12px").
						Style("font-size", "13px").
						Style("cursor", "pointer").
						Style("background", "var(--accent)").
						Style("color", "var(--accent-text)").
						Text("Save").
						OnClick(func(ctx app.Context, e app.Event) {
							p.saveCommit(ctx)
						}),
				),
		)
}

// renderCommitsDialog is the history: number, title, author, time and counts,
// each row offering its title's rename.
func (p *Project) renderCommitsDialog() app.UI {
	rows := []app.UI{}
	switch {
	case !p.commitsLoaded:
		rows = append(rows, app.P().Style("margin", "0 0 6px").Style("color", "var(--text-muted)").
			Text("Loading commits…"))
	case len(p.commits) == 0:
		rows = append(rows, app.P().Style("margin", "0 0 6px").
			Text("The project has not been saved yet."))
	default:
		for _, commit := range p.commits {
			rows = append(rows, p.renderCommitRow(commit))
		}
	}
	return app.Div().
		Attr("data-commits-dialog", "true").
		Style("position", "absolute").
		Style("top", "48px").
		Style("left", "16px").
		Style("z-index", "20").
		Style("width", "560px").
		Style("max-height", "70vh").
		Style("overflow-y", "auto").
		Style("padding", "14px").
		Style("border", "1px solid var(--border)").
		Style("border-radius", "6px").
		Style("background", "var(--bg)").
		Style("box-shadow", "var(--toast-shadow)").
		Style("font-size", "13px").
		Body(
			app.H3().Style("margin", "0 0 8px").Text("Commits"),
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
						p.commitsDialog = false
						ctx.Update()
					})),
		)
}

// renderCommitRow is one Commit of the history, its title renamed in place.
func (p *Project) renderCommitRow(commit apiv0.CommitResponse) app.UI {
	var title app.UI
	if p.renamingCommit == commit.Number {
		title = app.Span().Body(
			app.Input().
				Attr("data-commit-rename-input", fmt.Sprint(commit.Number)).
				Type("text").
				Value(p.renameCommitTitle).
				AutoFocus(true).
				Style("font-size", "12px").
				OnInput(func(ctx app.Context, e app.Event) {
					p.renameCommitTitle = ctx.JSSrc().Get("value").String()
				}),
			app.Button().
				Attr("data-commit-rename-save", fmt.Sprint(commit.Number)).
				Style("margin-left", "4px").
				Text("Save").
				OnClick(func(ctx app.Context, e app.Event) {
					p.renameCommit(ctx)
				}),
			app.Button().
				Style("margin-left", "4px").
				Text("Cancel").
				OnClick(func(ctx app.Context, e app.Event) {
					p.renamingCommit, p.renameCommitTitle = 0, ""
					ctx.Update()
				}),
		)
	} else {
		title = app.Button().
			Attr("data-commit-rename", fmt.Sprint(commit.Number)).
			Title("Rename the title of this Commit").
			Style("padding", "1px 7px").
			Style("font-size", "11px").
			Style("cursor", "pointer").
			Style("border", "1px solid var(--border-input)").
			Style("border-radius", "4px").
			Style("background", "var(--bg)").
			Style("color", "var(--text)").
			Text("Rename").
			OnClick(func(ctx app.Context, e app.Event) {
				p.startRenamingCommit(commit.Number, commit.Title)
				ctx.Update()
			})
	}
	return app.Div().
		Attr("data-commit-row", fmt.Sprint(commit.Number)).
		Style("display", "flex").
		Style("align-items", "center").
		Style("justify-content", "space-between").
		Style("gap", "8px").
		Style("padding", "4px 6px").
		Style("border-bottom", "1px solid var(--border)").
		Body(
			app.Span().Body(
				app.Strong().Text(fmt.Sprintf("#%d ", commit.Number)),
				app.Span().Text(commit.Title),
				app.Span().Style("margin-left", "8px").Style("color", "var(--text-muted)").
					Text(fmt.Sprintf("%s · %s", commit.Author, commitTime(commit.CreatedAt))),
				app.Span().Style("margin-left", "8px").Style("color", "var(--text-muted)").
					Text(fmt.Sprintf("%d scenes, %d widgets, %d tags, %d devices, %d environments",
						commit.ContentCounts.Scenes, commit.ContentCounts.Widgets, commit.ContentCounts.Tags,
						commit.ContentCounts.Devices, commit.ContentCounts.Environments)),
			),
			app.Span().Style("display", "flex").Style("gap", "4px").Body(
				p.downloadCommitButton(commit.Number),
				p.releaseCommitButton(commit),
				title,
			),
		)
}

// downloadCommitButton offers the snapshot of a Commit as a download: the
// ProjectFile the Save fixed (task 72_2; ADR 0021).
func (p *Project) downloadCommitButton(number int) app.UI {
	return app.Button().
		Attr("data-commit-download", fmt.Sprint(number)).
		Title("Download the project file this Commit fixed").
		Style("padding", "1px 7px").
		Style("font-size", "11px").
		Style("cursor", "pointer").
		Style("border", "1px solid var(--border-input)").
		Style("border-radius", "4px").
		Style("background", "var(--bg)").
		Style("color", "var(--text)").
		Text("Project file").
		OnClick(func(ctx app.Context, e app.Event) {
			p.downloadCommitProjectFile(ctx, number)
		})
}

// downloadCommitProjectFile downloads the ProjectFile a Commit fixed.
func (p *Project) downloadCommitProjectFile(ctx app.Context, number int) {
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiclient.ProjectFileDownload, error) {
		return pc.CommitProjectFile(c, number)
	}, func(ctx app.Context, download apiclient.ProjectFileDownload) {
		saveProjectFile(download.Filename, download.Data)
		ctx.Update()
	}, nil)
}
