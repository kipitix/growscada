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
// the selected library's Draft as a saved state, the history lists them and their
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
			Title:  fmt.Sprintf("Saved state %d saved", saved.Number),
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

// openHistory shows the saved states of the selected library, read afresh
// (task 84): a screen of its own, in place of the editor.
func (l *Library) openHistory() {
	if l.libraryID == uuid.Nil {
		return
	}
	l.historyScreen = true
	l.loadCommits(l.compoCtx)
	l.compoCtx.Update()
}

// closeHistory returns from the saved states to the editor, dropping the name
// being edited.
func (l *Library) closeHistory() {
	l.historyScreen = false
	l.renamingCommit, l.renameCommitTitle = 0, ""
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

// renderCommitBar is the panel's Save, its unsaved-changes indicator and the
// History and Releases buttons (task 72_7; task 84; ADR 0021).
func (l *Library) renderCommitBar() []app.UI {
	editable := !l.readOnly() && l.libraryID != uuid.Nil
	saveTitle := "Save the Draft as a saved state"
	switch {
	case l.savingCommit:
		saveTitle = "The Draft is being saved"
	case l.readOnly():
		saveTitle = "A release is shown; open the Draft to edit it"
	case !l.hasUnsavedChanges():
		saveTitle = "Nothing to save since the last save"
	}
	// The indicator is always a child, hidden when there is nothing to show:
	// a conditional one would shift History and Releases by a position, and
	// go-app keeps the handler of a reused element whose handler is the same
	// func literal (task 79).
	indicator := app.Span().Style("font-size", "12px")
	switch {
	case l.hasUnsavedChanges():
		indicator = indicator.
			Attr("data-commit-unsaved", "true").
			Style("color", "var(--toast-warn-title)").
			Text("Unsaved changes")
	case l.draftStateLoaded && len(l.commits) > 0:
		indicator = indicator.
			Attr("data-commit-saved", "true").
			Style("color", "var(--text-muted)").
			Text("Saved")
	default:
		indicator = indicator.Style("display", "none")
	}
	// Each action also names its own app.EventScope, so a reused element can
	// never keep another action's handler.
	return []app.UI{
		uiutil.PanelButton(uiutil.IconSave(), editable && l.hasUnsavedChanges() && !l.savingCommit,
			saveTitle, func(app.Context, app.Event) { l.openSaveDialog() }, app.EventScope("save")),
		indicator,
		uiutil.PanelButton(uiutil.IconHistory(), l.libraryID != uuid.Nil,
			"The saved states of the library", func(app.Context, app.Event) { l.openHistory() },
			app.EventScope("history")),
		uiutil.PanelButton(uiutil.IconTag(), l.libraryID != uuid.Nil,
			"The Releases of the library", func(app.Context, app.Event) { l.openReleasesDialog() },
			app.EventScope("releases")),
	}
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
			uiutil.Button("Change", "", true, "Change the author name", uiutil.ButtonRow,
				func(ctx app.Context, e app.Event) {
					l.changeAuthor(ctx)
				}).Attr("data-author-change", "true"),
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
				smallButton("Save", !l.savingCommit, "accent", "Save the Draft as a saved state", l.saveCommit),
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
			app.Div().Style("margin-bottom", "4px").Text(fmt.Sprintf("Release saved state %d", l.releaseCommit)),
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
				smallButton("Release", !l.releasing, "accent", "Publish this saved state as the next Library Release", l.publishRelease),
				smallButton("Cancel", true, "", "Close without publishing", func() {
					l.releaseDialog, l.releaseCommit = false, 0
					l.compoCtx.Update()
				}),
			),
		)
}

// renderHistoryScreen is the history of the selected library's saved states
// (task 84): a screen of its own with a way back to the editor and one row per
// saved state, the list scrolling in its own pane.
func (l *Library) renderHistoryScreen() app.UI {
	rows := []app.UI{}
	switch {
	case !l.commitsLoaded:
		rows = append(rows, app.Div().
			Style("padding", "8px").
			Style("color", "var(--text-muted)").
			Text("Loading saved states…"))
	case len(l.commits) == 0:
		rows = append(rows, app.Div().
			Style("padding", "8px").
			Text("The library has not been saved yet."))
	default:
		for _, saved := range l.commits {
			rows = append(rows, l.renderSavedStateRow(saved))
		}
	}
	return app.Div().
		Attr("data-history-screen", "true").
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex", "1").
		Style("min-height", "0").
		Body(
			app.Div().
				Style("display", "flex").
				Style("align-items", "center").
				Style("gap", "8px").
				Style("padding", "6px 10px").
				Style("border-bottom", "1px solid var(--border)").
				Body(
					uiutil.PanelButton(uiutil.IconBack(), true, "Back to the Draft",
						func(app.Context, app.Event) { l.closeHistory() }, app.EventScope("history-back")),
					app.Span().
						Attr("data-history-title", "true").
						Style("font-size", "13px").
						Style("font-weight", "600").
						Text("Saved states"),
				),
			app.Div().
				Attr("data-history-list", "true").
				Style("flex", "1").
				Style("min-height", "0").
				Style("overflow-y", "auto").
				Style("padding", "4px 10px").
				Body(rows...),
		)
}

// renderSavedStateRow is one saved state of the history (task 84): the time it
// was saved, its name — edited in place — and its author, and at the end the
// two actions: load it into the Draft and publish it as a Release.
func (l *Library) renderSavedStateRow(saved apiv0.CommitResponse) app.UI {
	number := itoa(saved.Number)
	title := app.Span().
		Attr("data-saved-state-title", number).
		Style("font-size", "13px").
		Text(saved.Title)
	var name app.UI = app.Div().
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "4px").
		Body(
			title,
			uiutil.PanelButton(uiutil.IconPencil(), true, "Rename the saved state",
				func(app.Context, app.Event) {
					l.renamingCommit, l.renameCommitTitle = saved.Number, saved.Title
					l.compoCtx.Update()
				}, app.EventScope("rename/"+number)),
		)
	if l.renamingCommit == saved.Number {
		name = app.Div().
			Style("display", "flex").
			Style("align-items", "center").
			Style("gap", "4px").
			Body(
				app.Input().
					Attr("data-saved-state-rename", number).
					Type("text").
					Value(l.renameCommitTitle).
					AutoFocus(true).
					Style("font-size", "12px").
					Style("padding", "2px 4px").
					OnInput(func(ctx app.Context, e app.Event) {
						l.renameCommitTitle = ctx.JSSrc().Get("value").String()
					}),
				uiutil.PanelButton(uiutil.IconCheck(), true, "Save the name",
					func(app.Context, app.Event) { l.renameCommit() }, app.EventScope("rename-ok/"+number)),
				uiutil.PanelButton(uiutil.IconClose(), true, "Drop the new name",
					func(app.Context, app.Event) {
						l.renamingCommit, l.renameCommitTitle = 0, ""
						l.compoCtx.Update()
					}, app.EventScope("rename-cancel/"+number)),
			)
	}
	release := uiutil.Button("Make Release", "accent", !saved.Released,
		"Publish this saved state as the next Library Release",
		uiutil.ButtonRow, func(ctx app.Context, e app.Event) { l.openReleaseDialog(saved) },
		app.EventScope("release/"+number))
	if saved.Released {
		release = uiutil.Button("Released", "", false, "A Release publishes this saved state",
			uiutil.ButtonRow, func(app.Context, app.Event) {}, app.EventScope("released/"+number))
	}
	load := uiutil.Button("Load", "", l.loadingSavedState == 0,
		"Replace the Draft with this saved state",
		uiutil.ButtonRow, func(ctx app.Context, e app.Event) { l.loadSavedState(saved.Number) },
		app.EventScope("load/"+number))
	return app.Div().
		Attr("data-saved-state-row", number).
		Style("display", "flex").
		Style("flex-wrap", "wrap").
		Style("align-items", "center").
		Style("gap", "10px").
		Style("padding", "6px 4px").
		Style("border-bottom", "1px solid var(--border)").
		Body(
			app.Span().
				Style("font-size", "11px").
				Style("color", "var(--text-muted)").
				Style("width", "118px").
				Style("flex-shrink", "0").
				Text(saved.CreatedAt.Local().Format("02.01.2006 15:04")),
			app.Div().Style("flex", "1").Style("min-width", "0").Body(name),
			app.Span().
				Style("font-size", "11px").
				Style("color", "var(--text-muted)").
				Text(saved.Author),
			load,
			release,
		)
}
