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
// the selected library's Draft as a saved state, the history lists them and
// their titles are renamed. The Draft is still written at once, so the
// indicator is the Draft's own unsaved-changes flag, its Version against its
// Clean Version (ADR 0016, amendments 2026-11-10 and 2026-11-16; task 74).

// loadSavedStates (re)loads the history of the selected library's Draft. A 404
// means the library is gone and leaves it (libraryGone).
func (l *Library) loadSavedStates(ctx app.Context) {
	if l.libraryID == uuid.Nil {
		l.savedStates, l.savedStatesLoaded = nil, false
		return
	}
	uiutil.LoadForGone(l.compoCtx, &l.savedStatesReloader, l.draft,
		func(c context.Context, lib *apiclient.LibraryClient) ([]apiv0.SavedStateResponse, error) {
			return lib.SavedStates(c)
		},
		func(ctx app.Context, _ *apiclient.LibraryClient, savedStates []apiv0.SavedStateResponse) {
			l.savedStates, l.savedStatesLoaded = savedStates, true
		}, l.libraryGone, l.loadSavedStates)
}

// hasUnsavedChanges reports whether the Draft moved since its content last
// equalled a saved state's. The flag is the server's — the Draft's own Clean
// Version marker (ADR 0016, amendments 2026-11-10 and 2026-11-16; task 74) —,
// so a Discard, a Load, a Revert and a Save all follow it without this tab
// remembering anything.
func (l *Library) hasUnsavedChanges() bool {
	return l.draftUnsaved
}

// saveFromShortcut is Ctrl+S: the history screen with its Save form when the
// Draft has unsaved changes, nothing otherwise (task 72_7; task 91; ADR 0021).
func (l *Library) saveFromShortcut(ctx app.Context) {
	if !l.hasUnsavedChanges() || l.savingSavedState {
		return
	}
	l.openHistory()
}

// saveSavedState confirms the Save form: the Draft as it is becomes a saved
// state with the title the form holds. The author is the stored user name,
// which the User name page sets and OnMount's Watch follows (task 90; task 91).
func (l *Library) saveSavedState(title string) {
	draft := l.draft()
	if draft == nil || l.savingSavedState || l.loadingSavedState != 0 {
		return
	}
	title, author := strings.TrimSpace(title), strings.TrimSpace(l.saveAuthor)
	if title == "" || author == "" {
		uiutil.ShowError(l.compoCtx, fmt.Errorf("a title and an author are required"))
		return
	}
	l.savingSavedState = true
	l.compoCtx.LocalStorage().Set(uiutil.AuthorKey, author)
	version := l.draftVersion()
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.SavedStateResponse, error) {
		return draft.SaveSavedState(c, apiv0.CreateSavedStateRequest{Version: version, Title: title, Author: author})
	}, func(ctx app.Context, saved apiv0.SavedStateResponse) {
		if draft != l.draft() {
			l.savingSavedState = false
			return
		}
		l.savingSavedState = false
		l.history.StartNew("Save " + uiutil.FormatTime(time.Now()))
		ctx.NewActionWithValue(toast.ActionAdd, toast.Problem{Problem: apiv0.Problem{
			Title:  fmt.Sprintf("Saved state %d saved", saved.Number),
			Detail: saved.Title,
		}})
		l.loadSavedStates(ctx)
		l.loadDraftState(ctx) // the Save made the Draft its saved state's content
		l.loadLibraries(ctx)  // the status follows the new last saved state
		ctx.Update()
	}, func(ctx app.Context, err error) {
		if draft != l.draft() {
			l.savingSavedState = false
			return
		}
		l.savingSavedState = false
		uiutil.ShowError(ctx, err)
		if errors.Is(err, apiclient.ErrConflict) {
			l.forgetDraftVersion()
			l.loadDraftState(ctx)
			l.loadDraftChanges(ctx)
			l.loadList(ctx)
		}
		l.loadSavedStates(ctx)
		l.loadLibraries(ctx)
		ctx.Update()
	})
}

// openHistory shows the saved states of the selected library, read afresh
// (task 84): a screen of its own, in place of the editor. The form at its top
// starts on a fresh name, so the Save button and Ctrl+S land ready to fix a new
// saved state; the screen owns that name and the mode keeps none, so a name
// typed for one library never reaches another (task 91).
func (l *Library) openHistory() {
	if l.libraryID == uuid.Nil {
		return
	}
	l.historyScreen = true
	l.history.StartNew("Save " + uiutil.FormatTime(time.Now()))
	l.loadSavedStates(l.compoCtx)
	l.loadHistoryLabels(l.compoCtx)
	l.onHistory()
	l.compoCtx.Update()
}

// toggleHistory opens the saved states or, when they are shown, closes them and
// returns to the editor: the panel's one button is both ways (task 91).
func (l *Library) toggleHistory() {
	if l.historyScreen {
		l.closeHistory()
		return
	}
	l.openHistory()
}

// HistoryShown reports whether the saved states screen is shown: Root's
// navigation line draws the step above it (task 91).
func (l *Library) HistoryShown() bool { return l.historyScreen }

// CloseHistory returns from the saved states to the editor, as the panel's
// way back does (task 91).
func (l *Library) CloseHistory() { l.closeHistory() }

// closeHistory returns from the saved states to the editor, dropping the name
// being edited.
func (l *Library) closeHistory() {
	l.historyScreen = false
	if l.history != nil {
		l.history.Reset()
	}
	l.onHistory()
	if l.mounted {
		l.compoCtx.Update()
	}
}

// onHistory tells Root the saved states opened or closed, so the navigation
// line above adds or drops their step (task 91).
func (l *Library) onHistory() {
	if l.OnHistory != nil {
		l.OnHistory()
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
		l.loadSavedStates(ctx)
		ctx.Update()
	}, nil)
}

// renderSaveButton is the panel's list: it opens the saved states of the
// library, where the Draft sits as the newest state and the form names the next
// one, and pressing it again returns to the editor (task 91). It stays enabled
// without changes, since it is also the way to browse them. The floppy belongs
// to that form's Save.
func (l *Library) renderSaveButton() app.UI {
	title := "The saved states of the library"
	if l.historyScreen {
		title = "Close the saved states"
	}
	return uiutil.PanelButton(uiutil.IconSavedStates(), l.libraryID != uuid.Nil,
		title, func(app.Context, app.Event) { l.toggleHistory() },
		app.EventScope("save"))
}

// renderUnsavedIndicator is the yellow mark after Undo and Redo: a Draft with
// nothing to save shows nothing (task 91).
func (l *Library) renderUnsavedIndicator() app.UI {
	if !l.hasUnsavedChanges() {
		return app.Span().Style("display", "none")
	}
	return app.Span().
		Attr("data-saved-state-unsaved", "true").
		Style("font-size", "12px").
		Style("color", "var(--toast-warn-title)").
		Text("Unsaved changes")
}

// renderHistoryScreen is the history of the selected library's saved states
// (task 84): the screen both modes share, fed with this library's history and
// its Save form (task 91).
func (l *Library) renderHistoryScreen() app.UI {
	screen := l.history
	screen.Rows = l.historyRows()
	screen.Loaded = l.savedStatesLoaded
	screen.Loading = l.loadingSavedState != 0
	screen.AllLabels = l.historyLabels
	screen.CanSave = l.hasUnsavedChanges()
	screen.Saving = l.savingSavedState
	screen.Busy = l.loadingSavedState != 0
	screen.Author = l.saveAuthor
	screen.OnSave = func(ctx app.Context, title string) { l.saveSavedState(title) }
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
	rows := make([]savedstates.Row, len(l.savedStates))
	for i, saved := range l.savedStates {
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
