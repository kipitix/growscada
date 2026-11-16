package project

import (
	"context"
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

// The Project mode's Save and its history (task 72_2; ADR 0021): Save fixes
// the Draft as a saved state, the history lists them and their names are renamed.
// The Draft is still written at once — Save only records a named point of
// history, so the indicator is the Draft's own unsaved-changes flag, its
// Version against its Clean Version (ADR 0016, amendments 2026-11-10 and
// 2026-11-16; task 74).

// loadSavedStates (re)loads the history of the project's Draft.
func (p *Project) loadSavedStates(ctx app.Context) {
	if p.content == nil {
		p.savedStates, p.savedStatesLoaded = nil, false
		return
	}
	uiutil.LoadOfProject(p.compoCtx, &p.savedStatesReloader, p.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) ([]apiv0.SavedStateResponse, error) {
			return pc.SavedStates(c)
		},
		func(ctx app.Context, savedStates []apiv0.SavedStateResponse) {
			p.savedStates, p.savedStatesLoaded = savedStates, true
		}, p.loadSavedStates)
}

// hasUnsavedChanges reports whether the Draft moved since its content last
// equalled a saved state's. The flag is the server's — the Draft's own Clean
// Version marker (ADR 0016, amendments 2026-11-10 and 2026-11-16; task 74) —,
// so a Discard, a Load, a Revert and a Save all follow it without this tab
// remembering anything.
func (p *Project) hasUnsavedChanges() bool {
	return p.draftUnsaved
}

// saveFromShortcut is Ctrl+S: the history screen with its Save form when the
// Draft has unsaved changes, nothing otherwise (task 72_6; task 91).
func (p *Project) saveFromShortcut(ctx app.Context) {
	if !p.hasUnsavedChanges() || p.savingSavedState {
		return
	}
	p.openHistory(ctx)
}

// saveSavedState confirms the Save form: the Draft as it is becomes a saved
// state with the title the form holds. The author is the stored user name,
// which the User name page sets and OnMount's Watch follows (task 90; task 91).
func (p *Project) saveSavedState(ctx app.Context, title string) {
	if p.content == nil || p.savingSavedState || p.deploying || p.loadingSavedState != 0 {
		return
	}
	if p.waitMoves(ctx, func(ctx app.Context) { p.saveSavedState(ctx, title) }) {
		return
	}
	title, author := strings.TrimSpace(title), strings.TrimSpace(p.saveAuthor)
	if title == "" || author == "" {
		uiutil.ShowError(ctx, fmt.Errorf("a title and an author are required"))
		return
	}
	p.savingSavedState = true
	ctx.LocalStorage().Set(uiutil.AuthorKey, author)
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.SavedStateResponse, error) {
		return pc.SaveSavedState(c, apiv0.CreateSavedStateRequest{
			Version: p.draftVersion(), Title: title, Author: author,
		})
	}, func(ctx app.Context, saved apiv0.SavedStateResponse) {
		p.savingSavedState = false
		p.history.StartNew("Save " + uiutil.FormatTime(time.Now()))
		ctx.NewActionWithValue(toast.ActionAdd, toast.Problem{Problem: apiv0.Problem{
			Title:  fmt.Sprintf("Saved state %d saved", saved.Number),
			Detail: saved.Title,
		}})
		p.loadSavedStates(ctx)
		p.loadDraftState(ctx) // the Save made the Draft its saved state's content
		ctx.Update()
	}, func(ctx app.Context, err error) {
		p.savingSavedState = false
		// A conflict drops the Draft's Version and rereads the content, the
		// stack and the history; a nothing_to_save means the history moved.
		p.rejectedDraftWrite(ctx, err, func(ctx app.Context) {
			p.loadSavedStates(ctx)
			p.loadDraftChanges(ctx)
		})
		ctx.Update()
	})
}

// openHistory shows the saved states of the project, read afresh (task 84): a
// screen of its own, in place of the content. The rows carry the saved states
// and their labels alone, and the form at the top starts on a fresh name, so
// the Save button and Ctrl+S land ready to fix a new one (task 85_5, task 85_7,
// task 91). The screen owns that name: the mode keeps none, so a name typed for
// one project never reaches another (task 91).
func (p *Project) openHistory(ctx app.Context) {
	p.historyScreen = true
	p.history.StartNew("Save " + uiutil.FormatTime(time.Now()))
	p.loadSavedStates(ctx)
	p.loadHistoryLabels(ctx)
	p.onHistory()
	ctx.Update()
}

// toggleHistory opens the saved states or, when they are shown, closes them and
// returns to the content: the panel's one button is both ways (task 91).
func (p *Project) toggleHistory(ctx app.Context) {
	if p.historyScreen {
		p.closeHistory()
		return
	}
	p.openHistory(ctx)
}

// HistoryShown reports whether the saved states screen is shown: Root's
// navigation line draws the step above it (task 91).
func (p *Project) HistoryShown() bool { return p.historyScreen }

// CloseHistory returns from the saved states to the content, as the panel's
// way back does (task 91).
func (p *Project) CloseHistory() { p.closeHistory() }

// closeHistory returns from the saved states to the content, dropping the name
// being edited.
func (p *Project) closeHistory() {
	p.historyScreen = false
	if p.history != nil {
		p.history.Reset()
	}
	p.onHistory()
	if p.mounted {
		p.compoCtx.Update()
	}
}

// onHistory tells Root the saved states opened or closed, so the navigation
// line above adds or drops their step (task 91).
func (p *Project) onHistory() {
	if p.OnHistory != nil {
		p.OnHistory()
	}
}

// renameSavedState saves the name the history screen edited. It states no
// Draft Version: another Engineer's writes do not refuse it (ADR 0021).
func (p *Project) renameSavedState(ctx app.Context, number int, title string) {
	if number == 0 || title == "" {
		return
	}
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.SavedStateResponse, error) {
		return pc.RenameSavedState(c, number, apiv0.RenameSavedStateRequest{Title: title})
	}, func(ctx app.Context, _ apiv0.SavedStateResponse) {
		p.loadSavedStates(ctx)
		ctx.Update()
	}, nil)
}

// renderSaveButton is the panel's list: it opens the saved states of the
// project, where the Draft sits as the newest state and the form names the next
// one, and pressing it again returns to the content (task 91). It stays enabled
// without changes, since it is also the way to browse them. The floppy belongs
// to that form's Save.
func (p *Project) renderSaveButton() app.UI {
	title := "The saved states of the project"
	if p.historyScreen {
		title = "Close the saved states"
	}
	return p.panelButton("saved-states", uiutil.IconSavedStates(), title,
		true, func(ctx app.Context) { p.toggleHistory(ctx) })
}

// renderUnsavedIndicator is the yellow mark after Undo and Redo: a Draft with
// nothing to save shows nothing (task 91).
func (p *Project) renderUnsavedIndicator() app.UI {
	if !p.hasUnsavedChanges() {
		return app.Span().Style("display", "none")
	}
	return app.Span().
		Attr("data-saved-state-unsaved", "true").
		Style("font-size", "12px").
		Style("color", "var(--toast-warn-title)").
		Text("Unsaved changes")
}

// renderHistoryScreen is the history of the project's saved states (task 84):
// the screen both modes share, fed with this project's history, its labels and
// its Save form (task 91). The screen draws saved states alone: every row is
// one of them, and a Deploy takes the row's saved state (task 85_5, task 85_7).
func (p *Project) renderHistoryScreen() app.UI {
	screen := p.history
	screen.Rows = p.historyRows()
	screen.Loaded = p.savedStatesLoaded
	screen.Loading = p.loadingSavedState != 0 || p.deploying
	screen.AllLabels = p.historyLabels
	screen.CanSave = p.hasUnsavedChanges()
	screen.Saving = p.savingSavedState
	screen.Busy = p.deploying || p.loadingSavedState != 0
	screen.Author = p.saveAuthor
	screen.OnSave = func(ctx app.Context, title string) { p.saveSavedState(ctx, title) }
	screen.OnLoad = func(ctx app.Context, number int) { p.loadSavedState(ctx, number) }
	screen.OnRename = func(ctx app.Context, number int, title string) {
		p.renameSavedState(ctx, number, title)
	}
	screen.OnAssign = func(ctx app.Context, number int, labelID uuid.UUID) {
		p.assignLabel(ctx, number, labelID)
	}
	screen.OnUnassign = func(ctx app.Context, number int, labelID uuid.UUID) {
		p.unassignLabel(ctx, number, labelID)
	}
	screen.OnManageLabels = func(ctx app.Context) { p.manageLabels(ctx) }
	return screen
}

// historyRows is the history as the screen draws it: each saved state with the
// labels that mark it (task 84, task 85_3). A Deploy takes the saved state
// itself (task 85_5).
func (p *Project) historyRows() []savedstates.Row {
	rows := make([]savedstates.Row, len(p.savedStates))
	for i, saved := range p.savedStates {
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
