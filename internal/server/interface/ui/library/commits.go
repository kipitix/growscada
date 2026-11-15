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
		func(c context.Context, lib *apiclient.LibraryClient) ([]apiv0.SavedStateResponse, error) {
			return lib.SavedStates(c)
		},
		func(ctx app.Context, _ *apiclient.LibraryClient, commits []apiv0.SavedStateResponse) {
			l.commits, l.commitsLoaded = commits, true
		}, l.libraryGone, l.loadCommits)
}

// hasUnsavedChanges reports whether the Draft moved since its content last
// equalled its last saved state. The flag is the server's — the Draft's own
// Clean Version marker (ADR 0016, amendment 2026-11-10; task 74) —, so a
// Discard, a Revert and a Save all follow it without this tab remembering
// anything.
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

// saveCommit confirms the Save dialog: the Draft as it is becomes a saved
// state.
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
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.SavedStateResponse, error) {
		return draft.SaveSavedState(c, apiv0.CreateSavedStateRequest{Version: version, Title: title, Author: author})
	}, func(ctx app.Context, saved apiv0.SavedStateResponse) {
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
		l.loadDraftState(ctx) // the Save made the Draft its saved state's content
		l.loadLibraries(ctx)  // the status follows the new last saved state
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

// changeAuthor asks for the author name the Save dialog fills in. It is no lock:
// it blocks nobody and only fills in the dialog (task 72_7).
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
	l.loadHistoryLabels(l.compoCtx)
	l.compoCtx.Update()
}

// closeHistory returns from the saved states to the editor, dropping the name
// being edited.
func (l *Library) closeHistory() {
	l.historyScreen = false
	if l.history != nil {
		l.history.Reset()
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
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.SavedStateResponse, error) {
		return draft.RenameSavedState(c, number, apiv0.RenameSavedStateRequest{Title: title})
	}, func(ctx app.Context, _ apiv0.SavedStateResponse) {
		if draft != l.draft() {
			return
		}
		l.loadCommits(ctx)
		ctx.Update()
	}, nil)
}

// renderCommitBar is the panel's Save, its unsaved-changes indicator and the
// History button (task 72_7; task 84; ADR 0021).
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
	// a conditional one would shift History by a position, and
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

// renderAuthorBar shows the author the Save dialog fills in, with a button to
// change it: it is not a lock and blocks nobody (task 72_7).
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

// renderHistoryScreen is the history of the selected library's saved states
// (task 84): the screen both modes share, fed with this library's history.
func (l *Library) renderHistoryScreen() app.UI {
	screen := l.history
	screen.Rows = l.historyRows()
	screen.Loaded = l.commitsLoaded
	screen.Loading = l.loadingSavedState != 0
	screen.AllLabels = l.historyLabels
	screen.OnBack = func(ctx app.Context) { l.closeHistory() }
	screen.OnLoad = func(ctx app.Context, number int) { l.loadSavedState(number) }
	screen.OnRename = func(ctx app.Context, number int, title string) {
		l.renameSavedState(ctx, number, title)
	}
	screen.OnAssign = func(ctx app.Context, number int, labelID uuid.UUID) {
		l.assignLabel(ctx, number, labelID)
	}
	screen.OnUnassign = func(ctx app.Context, number int, labelID uuid.UUID) {
		l.unassignLabel(ctx, number, labelID)
	}
	screen.OnManageLabels = func(ctx app.Context) { l.manageLabels(ctx) }
	return screen
}

// historyRows maps the history to the screen's rows: each carries the labels
// that mark it (task 84, task 85_3).
func (l *Library) historyRows() []savedstates.Row {
	rows := make([]savedstates.Row, len(l.commits))
	for i, saved := range l.commits {
		rows[i] = savedstates.Row{
			Number:  saved.Number,
			Title:   saved.Title,
			Author:  saved.Author,
			SavedAt: saved.CreatedAt,
			Labels:  saved.Labels,
		}
	}
	return rows
}
