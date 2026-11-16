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
	"github.com/kipitix/growscada/internal/server/interface/ui/username"
)

// The Project mode's Save and its history (task 72_2; ADR 0021): Save fixes
// the Draft as a saved state, the history lists them and their names are renamed.
// The Draft is still written at once — Save only records a named point of
// history, so the indicator is the Draft's own unsaved-changes flag, its
// Version against its Clean Version (ADR 0016, amendment 2026-11-10; task 74).

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
// equalled its last saved state. The flag is the server's — the Draft's own
// Clean Version marker (ADR 0016, amendment 2026-11-10; task 74) —, so a
// Discard, a Revert and a Save all follow it without this tab remembering
// anything.
func (p *Project) hasUnsavedChanges() bool {
	return p.draftUnsaved
}

// openSaveDialog opens the Save dialog with a default title and the stored
// user name. A first Save with no name yet shows the field empty: the dialog
// still takes an author for that one save, while the User name page sets the
// name every dialog starts from (task 72_6, task 90).
func (p *Project) openSaveDialog(ctx app.Context) {
	p.saveTitle = "Save " + uiutil.FormatTime(time.Now())
	p.saveAuthor = username.Stored(ctx)
	p.saveDialog = true
	ctx.Update()
}

// saveFromShortcut is Ctrl+S: the Save dialog when the Draft has unsaved
// changes, nothing otherwise (task 72_6).
func (p *Project) saveFromShortcut(ctx app.Context) {
	if !p.hasUnsavedChanges() || p.savingSavedState {
		return
	}
	p.openSaveDialog(ctx)
}

// saveSavedState confirms the Save dialog: the Draft as it is becomes a saved
// state.
func (p *Project) saveSavedState(ctx app.Context) {
	if p.content == nil || p.savingSavedState || p.deploying || p.loadingSavedState != 0 {
		return
	}
	if p.waitMoves(ctx, func(ctx app.Context) { p.saveSavedState(ctx) }) {
		return
	}
	title, author := strings.TrimSpace(p.saveTitle), strings.TrimSpace(p.saveAuthor)
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
		p.saveDialog = false
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
// and their labels alone (task 85_5, task 85_7).
func (p *Project) openHistory(ctx app.Context) {
	p.historyScreen = true
	p.loadSavedStates(ctx)
	p.loadHistoryLabels(ctx)
	ctx.Update()
}

// closeHistory returns from the saved states to the content, dropping the name
// being edited.
func (p *Project) closeHistory() {
	p.historyScreen = false
	if p.history != nil {
		p.history.Reset()
	}
	if p.mounted {
		p.compoCtx.Update()
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

// renderSavedStateBar is the panel's Save, its unsaved-changes indicator and
// History (task 72_2, task 72_6; task 84).
func (p *Project) renderSavedStateBar() []app.UI {
	saveTitle := "Save the Draft as a saved state"
	switch {
	case p.savingSavedState:
		saveTitle = "The Draft is being saved"
	case !p.hasUnsavedChanges():
		saveTitle = "Nothing to save since the last save"
	}
	body := []app.UI{
		p.panelButton("save", uiutil.IconSave(), saveTitle, p.hasUnsavedChanges() && !p.savingSavedState,
			func(ctx app.Context) { p.openSaveDialog(ctx) }),
	}
	indicator := app.Span().Style("font-size", "12px")
	switch {
	case p.hasUnsavedChanges():
		indicator = indicator.
			Attr("data-saved-state-unsaved", "true").
			Style("color", "var(--toast-warn-title)").
			Text("Unsaved changes")
	case p.draftStateLoaded && len(p.savedStates) > 0:
		indicator = indicator.
			Attr("data-saved-state-saved", "true").
			Style("color", "var(--text-muted)").
			Text("Saved")
	default:
		indicator = indicator.Style("display", "none")
	}
	body = append(body, indicator)
	body = append(body,
		p.panelButton("history", uiutil.IconHistory(), "The saved states of the project", !p.savingSavedState,
			func(ctx app.Context) { p.openHistory(ctx) }),
	)
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
				Text("The whole Draft is fixed as an immutable saved state. Editing continues; Save only marks a point of history."),
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
						Disabled(p.savingSavedState).
						Style("padding", "4px 12px").
						Style("font-size", "13px").
						Style("cursor", "pointer").
						Style("background", "var(--accent)").
						Style("color", "var(--accent-text)").
						Text(saveConfirmLabel(p.savingSavedState)).
						OnClick(func(ctx app.Context, e app.Event) {
							p.saveSavedState(ctx)
						}),
				),
		)
}

// saveConfirmLabel is the Save dialog's button: Save, or that the request is in
// flight (a second click sends nothing).
func saveConfirmLabel(saving bool) string {
	if saving {
		return "Saving…"
	}
	return "Save"
}

// renderHistoryScreen is the history of the project's saved states (task 84):
// the screen both modes share, fed with this project's history and its labels.
// The screen draws saved states alone: every row is one of them, and a Deploy
// takes the row's saved state (task 85_5, task 85_7).
func (p *Project) renderHistoryScreen() app.UI {
	screen := p.history
	screen.Rows = p.historyRows()
	screen.Loaded = p.savedStatesLoaded
	screen.Loading = p.loadingSavedState != 0 || p.deploying
	screen.AllLabels = p.historyLabels
	screen.OnBack = func(ctx app.Context) { p.closeHistory() }
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
