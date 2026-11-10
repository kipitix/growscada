package library

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
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// The Library mode's Save and its history (task 72_2; ADR 0021): Save fixes
// the selected library's Draft as a Commit, the history lists them and their
// titles are renamed. The Draft is still written at once, so the indicator is
// the Draft's own unsaved-changes flag, its Version against its Clean Version
// (ADR 0016, amendment 2026-11-10; task 74).

// loadCommits (re)loads the history of the selected library's Draft. A 404
// means the library is gone and leaves it (libraryGone).
func (l *Library) loadCommits(ctx app.Context) {
	if l.libraryID == uuid.Nil {
		l.commits, l.commitsLoaded = nil, false
		return
	}
	uiutil.LoadForGone(l.compoCtx, &l.commitsReloader, l.draft,
		func(c context.Context, lib *apiclient.LibraryClient) ([]apiv0.CommitResponse, error) {
			return lib.Commits(c)
		},
		func(ctx app.Context, _ *apiclient.LibraryClient, commits []apiv0.CommitResponse) {
			l.commits, l.commitsLoaded = commits, true
		}, l.libraryGone, l.loadCommits)
}

// hasUnsavedChanges reports whether the Draft moved since its content last
// equalled its last Commit. The flag is the server's — the Draft's own Clean
// Version marker (ADR 0016, amendment 2026-11-10; task 74) —, so a Discard, a
// Revert and a Save all follow it without this tab remembering anything.
func (l *Library) hasUnsavedChanges() bool {
	return l.draftUnsaved
}

// openSaveDialog opens the Save dialog with a default title and the stored
// author.
func (l *Library) openSaveDialog() {
	if l.draft() == nil || l.readOnly() {
		return
	}
	l.saveTitle = "Save " + time.Now().Format("02.01.2006 15:04")
	l.saveDialog = true
	l.compoCtx.Update()
}

// saveFromShortcut is Ctrl+S: the Save dialog when the Draft has unsaved
// changes, nothing otherwise (task 72_7; ADR 0021).
func (l *Library) saveFromShortcut(ctx app.Context) {
	if l.readOnly() || !l.hasUnsavedChanges() || l.savingCommit {
		return
	}
	l.openSaveDialog()
}

// saveCommit confirms the Save dialog: the Draft as it is becomes a Commit.
func (l *Library) saveCommit() {
	draft := l.draft()
	if draft == nil || l.savingCommit || l.readOnly() {
		return
	}
	title, author := strings.TrimSpace(l.saveTitle), strings.TrimSpace(l.saveAuthor)
	if title == "" || author == "" {
		uiutil.ShowError(l.compoCtx, fmt.Errorf("a title and an author are required"))
		return
	}
	l.savingCommit = true
	l.compoCtx.LocalStorage().Set(uiutil.AuthorKey, author)
	version := l.draftVersion()
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.CommitResponse, error) {
		return draft.SaveCommit(c, apiv0.CreateCommitRequest{Version: version, Title: title, Author: author})
	}, func(ctx app.Context, saved apiv0.CommitResponse) {
		if draft != l.draft() {
			l.savingCommit = false
			return
		}
		l.savingCommit = false
		l.saveDialog = false
		ctx.NewActionWithValue(toast.ActionAdd, toast.Problem{Problem: apiv0.Problem{
			Title:  fmt.Sprintf("Commit %d saved", saved.Number),
			Detail: saved.Title,
		}})
		l.loadCommits(ctx)
		l.loadDraftState(ctx) // the Save made the Draft its Commit's content
		l.loadLibraries(ctx)  // the status follows the new last Commit
		ctx.Update()
	}, func(ctx app.Context, err error) {
		if draft != l.draft() {
			l.savingCommit = false
			return
		}
		l.savingCommit = false
		uiutil.ShowError(ctx, err)
		if errors.Is(err, apiclient.ErrConflict) {
			l.forgetDraftVersion()
			l.loadDraftState(ctx)
			l.loadDraftChanges(ctx)
			l.loadList(ctx)
		}
		l.loadCommits(ctx)
		l.loadLibraries(ctx)
		ctx.Update()
	})
}

// changeAuthor asks for the author name the Save and Release dialogs fill in.
// It is no lock: it blocks nobody and only fills in the dialogs (task 72_7).
func (l *Library) changeAuthor(ctx app.Context) {
	name, ok := prompt("Author:", l.saveAuthor, false)
	if !ok {
		return
	}
	l.saveAuthor = name
	ctx.LocalStorage().Set(uiutil.AuthorKey, name)
	ctx.Update()
}

// openCommitsDialog shows the history, read afresh.
func (l *Library) openCommitsDialog() {
	if l.libraryID == uuid.Nil {
		return
	}
	l.commitsDialog = true
	l.loadCommits(l.compoCtx)
	l.compoCtx.Update()
}

// renameCommit saves the inline title of a Commit. It states no Draft
// Version: another Engineer's writes do not refuse it (ADR 0021).
func (l *Library) renameCommit() {
	draft := l.draft()
	number, title := l.renamingCommit, strings.TrimSpace(l.renameCommitTitle)
	if draft == nil || number == 0 || title == "" {
		return
	}
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.CommitResponse, error) {
		return draft.RenameCommit(c, number, apiv0.RenameCommitRequest{Title: title})
	}, func(ctx app.Context, _ apiv0.CommitResponse) {
		if draft != l.draft() {
			return
		}
		l.renamingCommit, l.renameCommitTitle = 0, ""
		l.loadCommits(ctx)
		ctx.Update()
	}, nil)
}

// renderCommitBar is the header's Save, its unsaved-changes indicator and the
// History and Releases buttons (task 72_7; task 73; ADR 0021).
func (l *Library) renderCommitBar() []app.UI {
	editable := !l.readOnly() && l.libraryID != uuid.Nil
	saveLabel, saveTitle := "Save", "Save the Draft as a Commit"
	switch {
	case l.savingCommit:
		saveLabel, saveTitle = "Saving…", "The Draft is being fixed as a Commit"
	case l.readOnly():
		saveTitle = "A release or a commit is shown; open the Draft to edit it"
	case !l.hasUnsavedChanges():
		saveTitle = "Nothing to save since the last save"
	}
	body := []app.UI{
		barButton(saveLabel, editable && l.hasUnsavedChanges() && !l.savingCommit, "accent", saveTitle, l.openSaveDialog),
	}
	if l.hasUnsavedChanges() {
		body = append(body, app.Span().
			Attr("data-commit-unsaved", "true").
			Style("font-size", "12px").
			Style("color", "var(--toast-warn-title)").
			Text("Unsaved changes"))
	} else if l.draftStateLoaded && len(l.commits) > 0 {
		body = append(body, app.Span().
			Attr("data-commit-saved", "true").
			Style("font-size", "12px").
			Style("color", "var(--text-muted)").
			Text("Saved"))
	}
	body = append(body,
		barButton("History", l.libraryID != uuid.Nil, "", "The Commits of the library", l.openCommitsDialog),
		barButton("Releases", l.libraryID != uuid.Nil, "", "The Releases of the library", l.openReleasesDialog),
	)
	return body
}

// renderAuthorBar shows the author the Save and Release dialogs fill in, with a
// button to change it: it is not a lock and blocks nobody (task 72_7).
func (l *Library) renderAuthorBar() app.UI {
	name := strings.TrimSpace(l.saveAuthor)
	label := "Author: not set"
	if name != "" {
		label = "Author: " + name
	}
	return app.Div().
		Attr("data-author-bar", "true").
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "4px").
		Style("margin-top", "2px").
		Body(
			app.Span().
				Attr("data-author-name", name).
				Style("font-size", "11px").
				Style("color", "var(--text-muted)").
				Text(label),
			app.Button().
				Attr("data-author-change", "true").
				Title("Change the author name").
				Style("padding", "1px 6px").
				Style("font-size", "11px").
				Style("cursor", "pointer").
				Style("border", "1px solid var(--border-input)").
				Style("border-radius", "3px").
				Style("background", "var(--bg)").
				Style("color", "var(--text)").
				Text("Change").
				OnClick(func(ctx app.Context, e app.Event) {
					l.changeAuthor(ctx)
				}),
		)
}

// renderSaveDialog is the Save confirmation.
func (l *Library) renderSaveDialog() app.UI {
	return app.Div().
		Attr("data-save-dialog", "true").
		Style("padding", "8px").
		Style("margin-bottom", "6px").
		Style("border", "1px solid var(--border)").
		Style("border-radius", "4px").
		Style("background", "var(--bg)").
		Style("font-size", "12px").
		Body(
			app.Div().Style("margin-bottom", "4px").Text("Save the library"),
			app.Input().
				Attr("data-save-title", "true").
				Type("text").
				Value(l.saveTitle).
				Placeholder("Title").
				Style("width", "100%").
				Style("box-sizing", "border-box").
				Style("margin-bottom", "4px").
				Style("font-size", "12px").
				OnInput(func(ctx app.Context, e app.Event) {
					l.saveTitle = ctx.JSSrc().Get("value").String()
				}),
			app.Input().
				Attr("data-save-author", "true").
				Type("text").
				Value(l.saveAuthor).
				Placeholder("Author").
				Style("width", "100%").
				Style("box-sizing", "border-box").
				Style("margin-bottom", "6px").
				Style("font-size", "12px").
				OnInput(func(ctx app.Context, e app.Event) {
					l.saveAuthor = ctx.JSSrc().Get("value").String()
				}),
			app.Div().Style("display", "flex").Style("gap", "4px").Body(
				smallButton("Save", !l.savingCommit, "accent", "Save the Draft as a Commit", l.saveCommit),
				smallButton("Cancel", true, "", "Close without saving", func() {
					l.saveDialog = false
					l.compoCtx.Update()
				}),
			),
		)
}

// renderReleaseDialog is the publication of a Commit: a note, the author and
// the Release itself (task 72_7; ADR 0021).
func (l *Library) renderReleaseDialog() app.UI {
	return app.Div().
		Attr("data-release-dialog", "true").
		Style("padding", "8px").
		Style("margin-top", "6px").
		Style("border", "1px solid var(--border)").
		Style("border-radius", "4px").
		Style("background", "var(--bg)").
		Style("font-size", "12px").
		Body(
			app.Div().Style("margin-bottom", "4px").Text(fmt.Sprintf("Release Commit %d", l.releaseCommit)),
			app.Input().
				Attr("data-release-note", "true").
				Type("text").
				Value(l.releaseNote).
				Placeholder("Note (optional)").
				Style("width", "100%").
				Style("box-sizing", "border-box").
				Style("margin-bottom", "4px").
				Style("font-size", "12px").
				OnInput(func(ctx app.Context, e app.Event) {
					l.releaseNote = ctx.JSSrc().Get("value").String()
				}),
			app.Input().
				Attr("data-release-author", "true").
				Type("text").
				Value(l.releaseAuthor).
				Placeholder("Author").
				Style("width", "100%").
				Style("box-sizing", "border-box").
				Style("margin-bottom", "6px").
				Style("font-size", "12px").
				OnInput(func(ctx app.Context, e app.Event) {
					l.releaseAuthor = ctx.JSSrc().Get("value").String()
				}),
			app.Div().Style("display", "flex").Style("gap", "4px").Body(
				smallButton("Release", !l.releasing, "accent", "Publish this Commit as the next Library Release", l.publishRelease),
				smallButton("Cancel", true, "", "Close without publishing", func() {
					l.releaseDialog, l.releaseCommit = false, 0
					l.compoCtx.Update()
				}),
			),
		)
}

// renderCommitsDialog is the history of the selected library's Draft.
func (l *Library) renderCommitsDialog() app.UI {
	rows := []app.UI{}
	switch {
	case !l.commitsLoaded:
		rows = append(rows, app.Div().Style("color", "var(--text-muted)").Text("Loading commits…"))
	case len(l.commits) == 0:
		rows = append(rows, app.Div().Text("The library has not been saved yet."))
	default:
		for _, commit := range l.commits {
			rows = append(rows, l.renderCommitRow(commit))
		}
	}
	return app.Div().
		Attr("data-commits-dialog", "true").
		Style("padding", "8px").
		Style("margin-top", "6px").
		Style("border", "1px solid var(--border)").
		Style("border-radius", "4px").
		Style("background", "var(--bg)").
		Style("font-size", "12px").
		Style("max-height", "40vh").
		Style("overflow-y", "auto").
		Body(
			app.Div().Style("font-weight", "600").Style("margin-bottom", "4px").Text("Commits"),
			app.Div().Body(rows...),
			smallButton("Close", true, "", "Close the history", func() {
				l.commitsDialog = false
				l.compoCtx.Update()
			}),
		)
}

// renderCommitRow is one Commit of the history, its title renamed in place.
func (l *Library) renderCommitRow(commit apiv0.CommitResponse) app.UI {
	var rename app.UI
	if l.renamingCommit == commit.Number {
		rename = app.Div().Body(
			app.Input().
				Attr("data-commit-rename-input", fmt.Sprint(commit.Number)).
				Type("text").
				Value(l.renameCommitTitle).
				AutoFocus(true).
				Style("font-size", "11px").
				OnInput(func(ctx app.Context, e app.Event) {
					l.renameCommitTitle = ctx.JSSrc().Get("value").String()
				}),
			smallButton("Save", true, "accent", "Save the title", l.renameCommit),
			smallButton("Cancel", true, "", "Close the editor", func() {
				l.renamingCommit, l.renameCommitTitle = 0, ""
				l.compoCtx.Update()
			}),
		)
	} else {
		rename = app.Button().
			Attr("data-commit-rename", fmt.Sprint(commit.Number)).
			Style("padding", "0 6px").
			Style("font-size", "11px").
			Style("cursor", "pointer").
			Text("Rename").
			OnClick(func(ctx app.Context, e app.Event) {
				l.renamingCommit, l.renameCommitTitle = commit.Number, commit.Title
				ctx.Update()
			}, app.EventScope("rename/"+itoa(commit.Number)))
	}
	release := smallButton("Make Release", !commit.Released, "accent",
		"Publish this Commit as the next Library Release", func() { l.openReleaseDialog(commit) },
		app.EventScope("release/"+itoa(commit.Number)))
	if commit.Released {
		release = smallButton("Released", false, "", "A Release publishes this Commit", func() {})
	}
	view := smallButton("View", true, "", "Show the WidgetTypes of this Commit read-only",
		func() { l.viewCommit(commit.Number) }, app.EventScope("view/"+itoa(commit.Number)))
	return app.Div().
		Attr("data-commit-row", fmt.Sprint(commit.Number)).
		Style("padding", "3px 0").
		Style("border-bottom", "1px solid var(--border)").
		Body(
			app.Div().Text(fmt.Sprintf("#%d %s", commit.Number, commit.Title)),
			app.Div().Style("color", "var(--text-muted)").Text(fmt.Sprintf("%s · %s · %d widget types",
				commit.Author, commit.CreatedAt.Local().Format("2006-01-02 15:04"), commit.ContentCounts.WidgetTypes)),
			app.Div().Style("display", "flex").Style("gap", "4px").Body(view, rename, release),
		)
}
