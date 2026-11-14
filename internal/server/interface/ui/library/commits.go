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
	"github.com/kipitix/growscada/internal/server/interface/ui/savedstates"
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
	if l.draft() == nil {
		return
	}
	l.saveTitle = "Save " + uiutil.FormatTime(time.Now())
	l.saveDialog = true
	l.compoCtx.Update()
}

// saveFromShortcut is Ctrl+S: the Save dialog when the Draft has unsaved
// changes, nothing otherwise (task 72_7; ADR 0021).
func (l *Library) saveFromShortcut(ctx app.Context) {
	if !l.hasUnsavedChanges() || l.savingCommit {
		return
	}
	l.openSaveDialog()
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
	if l.history != nil {
		l.history.ResetRename()
	}
	if l.mounted {
		l.compoCtx.Update()
	}
}

// renameSavedState saves the name the history screen edited. It states no
// Draft Version: another Engineer's writes do not refuse it (ADR 0021).
func (l *Library) renameSavedState(ctx app.Context, number int, title string) {
	draft := l.draft()
	if draft == nil || number == 0 || title == "" {
		return
	}
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.CommitResponse, error) {
		return draft.RenameCommit(c, number, apiv0.RenameCommitRequest{Title: title})
	}, func(ctx app.Context, _ apiv0.CommitResponse) {
		if draft != l.draft() {
			return
		}
		l.loadCommits(ctx)
		ctx.Update()
	}, nil)
}

// releaseSavedState opens the publication of the saved state the screen named.
func (l *Library) releaseSavedState(ctx app.Context, number int) {
	for _, saved := range l.commits {
		if saved.Number == number {
			l.openReleaseDialog(saved)
			return
		}
	}
}

// editSavedStateReleaseNote changes the note of the Release that published the
// saved state, after asking for it: the note is metadata and states no Version
// (task 72_5, task 84).
func (l *Library) editSavedStateReleaseNote(ctx app.Context, number int) {
	lib, ok := l.selectedLibrary()
	if !ok {
		return
	}
	for _, released := range l.releases {
		if released.CommitNumber == nil || *released.CommitNumber != number {
			continue
		}
		note, ok := prompt(fmt.Sprintf("Note of release %d:", released.Number), released.Note, true)
		if !ok {
			return
		}
		uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.LibraryReleaseResponse, error) {
			return l.client.Library(lib.ID).UpdateReleaseNote(c, released.Number,
				apiv0.UpdateLibraryReleaseNoteRequest{Note: note})
		}, func(ctx app.Context, _ apiv0.LibraryReleaseResponse) {
			l.loadReleases(ctx)
			l.loadCommits(ctx)
			ctx.Update()
		}, l.reloadOnConflict)
		return
	}
}

// renderCommitBar is the panel's Save, its unsaved-changes indicator and the
// History button (task 72_7; task 84; ADR 0021). The Releases have no button of
// their own: a saved state is published from the history, where it is listed.
func (l *Library) renderCommitBar() []app.UI {
	editable := l.libraryID != uuid.Nil
	saveTitle := "Save the Draft as a saved state"
	switch {
	case l.savingCommit:
		saveTitle = "The Draft is being saved"
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
// (task 84): the screen both modes share, fed with this library's history and
// the Releases that published it.
func (l *Library) renderHistoryScreen() app.UI {
	screen := l.history
	screen.Rows = l.historyRows()
	screen.Loaded = l.commitsLoaded
	screen.Loading = l.loadingSavedState != 0
	screen.OnBack = func(ctx app.Context) { l.closeHistory() }
	screen.OnLoad = func(ctx app.Context, number int) { l.loadSavedState(number) }
	screen.OnRelease = func(ctx app.Context, number int) { l.releaseSavedState(ctx, number) }
	screen.OnRename = func(ctx app.Context, number int, title string) {
		l.renameSavedState(ctx, number, title)
	}
	screen.OnNote = func(ctx app.Context, number int) { l.editSavedStateReleaseNote(ctx, number) }
	return screen
}

// historyRows joins the history with the Releases that published it: a saved
// state a Release published carries that Release's number and note (task 84).
func (l *Library) historyRows() []savedstates.Row {
	rows := make([]savedstates.Row, len(l.commits))
	for i, saved := range l.commits {
		row := savedstates.Row{
			Number:   saved.Number,
			Title:    saved.Title,
			Author:   saved.Author,
			SavedAt:  saved.CreatedAt,
			Released: saved.Released,
		}
		for _, released := range l.releases {
			if released.CommitNumber != nil && *released.CommitNumber == saved.Number {
				row.Release, row.Note = released.Number, released.Note
				break
			}
		}
		rows[i] = row
	}
	return rows
}
