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
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// The Library mode's Save and its history (task 72_2; ADR 0021): Save fixes
// the selected library's Draft as a Commit, the history lists them and their
// titles are renamed. The Draft is still written at once, so the indicator
// follows the Version of the Draft against the one the last Commit fixed.

// authorKey is the LocalStorage key of the author name the dialogs prefill;
// the name is a declaration, not an account (task 72_2; tasks 49/50 bring
// Users).
const authorKey = "author:name"

// loadCommits (re)loads the history of the selected library's Draft.
func (l *Library) loadCommits(ctx app.Context) {
	if l.libraryID == uuid.Nil {
		l.commits, l.commitsLoaded = nil, false
		return
	}
	uiutil.LoadFor(l.compoCtx, &l.commitsReloader, l.draft,
		func(c context.Context, lib *apiclient.LibraryClient) ([]apiv0.CommitResponse, error) {
			return lib.Commits(c)
		},
		func(ctx app.Context, _ *apiclient.LibraryClient, commits []apiv0.CommitResponse) {
			l.commits, l.commitsLoaded = commits, true
		}, l.loadCommits)
}

// lastCommitVersion is the Version of the Draft the last Commit fixed, 0 for
// a Draft never saved.
func (l *Library) lastCommitVersion() int {
	if len(l.commits) == 0 {
		return 0
	}
	return l.commits[0].DraftVersion
}

// hasUnsavedChanges reports whether the Draft moved since its last Commit.
func (l *Library) hasUnsavedChanges() bool {
	return l.commitsLoaded && l.draftVersion() > l.lastCommitVersion()
}

// openSaveDialog opens the Save dialog with a default title and the stored
// author.
func (l *Library) openSaveDialog() {
	if l.draft() == nil || l.viewing != 0 {
		return
	}
	l.saveTitle = "Save " + time.Now().Format("02.01.2006 15:04")
	l.saveDialog = true
	l.compoCtx.Update()
}

// saveCommit confirms the Save dialog: the Draft as it is becomes a Commit.
func (l *Library) saveCommit() {
	draft := l.draft()
	if draft == nil || l.savingCommit {
		return
	}
	title, author := strings.TrimSpace(l.saveTitle), strings.TrimSpace(l.saveAuthor)
	if title == "" || author == "" {
		uiutil.ShowError(l.compoCtx, fmt.Errorf("a title and an author are required"))
		return
	}
	l.savingCommit = true
	l.compoCtx.LocalStorage().Set(authorKey, author)
	version := l.draftVersion()
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.CommitResponse, error) {
		return draft.SaveCommit(c, apiv0.CreateCommitRequest{Version: version, Title: title, Author: author})
	}, func(ctx app.Context, _ apiv0.CommitResponse) {
		if draft != l.draft() {
			l.savingCommit = false
			return
		}
		l.savingCommit = false
		l.saveDialog = false
		l.loadCommits(ctx)
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
		}
		l.loadCommits(ctx)
		ctx.Update()
	})
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

// renderCommitBar is the library column's Save, its unsaved-changes indicator
// and the history button.
func (l *Library) renderCommitBar() app.UI {
	editable := l.viewing == 0 && l.libraryID != uuid.Nil
	save := smallButton("Save", editable && l.hasUnsavedChanges() && !l.savingCommit,
		"accent", "Save the Draft as a Commit", l.openSaveDialog)
	history := smallButton("History", l.libraryID != uuid.Nil,
		"", "The Commits of the library", l.openCommitsDialog)
	indicator := app.Span().Style("font-size", "11px").Style("color", "var(--text-muted)")
	if l.hasUnsavedChanges() {
		indicator = indicator.Attr("data-commit-unsaved", "true").Style("color", "var(--toast-warn-title)").
			Text("Unsaved changes")
	} else if l.commitsLoaded {
		indicator = indicator.Attr("data-commit-saved", "true").Text("Saved")
	}
	return app.Div().
		Style("margin-bottom", "4px").
		Body(
			app.Div().Style("display", "flex").Style("gap", "4px").Body(save, history),
			indicator,
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
			})
	}
	return app.Div().
		Attr("data-commit-row", fmt.Sprint(commit.Number)).
		Style("padding", "3px 0").
		Style("border-bottom", "1px solid var(--border)").
		Body(
			app.Div().Text(fmt.Sprintf("#%d %s", commit.Number, commit.Title)),
			app.Div().Style("color", "var(--text-muted)").Text(fmt.Sprintf("%s · %s · %d widget types",
				commit.Author, commit.CreatedAt.Local().Format("2006-01-02 15:04"), commit.ContentCounts.WidgetTypes)),
			rename,
		)
}
