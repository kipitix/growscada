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
// Version against its Clean Version (ADR 0016, amendment 2026-11-10; task 74).

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

// hasUnsavedChanges reports whether the Draft moved since its content last
// equalled its last Commit. The flag is the server's — the Draft's own Clean
// Version marker (ADR 0016, amendment 2026-11-10; task 74) —, so a Discard, a
// Revert and a Save all follow it without this tab remembering anything.
func (p *Project) hasUnsavedChanges() bool {
	return p.draftUnsaved
}

// openSaveDialog opens the Save dialog with a default title and the stored
// author. A first Save with no author yet shows the field empty: the dialog is
// where the name is asked for (task 72_6).
func (p *Project) openSaveDialog(ctx app.Context) {
	p.saveTitle = "Save " + uiutil.FormatTime(time.Now())
	p.saveDialog = true
	ctx.Update()
}

// saveFromShortcut is Ctrl+S: the Save dialog when the Draft has unsaved
// changes, nothing otherwise (task 72_6).
func (p *Project) saveFromShortcut(ctx app.Context) {
	if !p.hasUnsavedChanges() || p.savingCommit {
		return
	}
	p.openSaveDialog(ctx)
}

// saveCommit confirms the Save dialog: the Draft as it is becomes a Commit.
func (p *Project) saveCommit(ctx app.Context) {
	if p.content == nil || p.savingCommit || p.deploying || p.loadingSavedState != 0 {
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
	}, func(ctx app.Context, saved apiv0.CommitResponse) {
		p.savingCommit = false
		p.saveDialog = false
		ctx.NewActionWithValue(toast.ActionAdd, toast.Problem{Problem: apiv0.Problem{
			Title:  fmt.Sprintf("Saved state %d saved", saved.Number),
			Detail: saved.Title,
		}})
		p.loadCommits(ctx)
		p.loadDraftState(ctx) // the Save made the Draft its Commit's content
		ctx.Update()
	}, func(ctx app.Context, err error) {
		p.savingCommit = false
		// A conflict drops the Draft's Version and rereads the content, the
		// stack and the history; a nothing_to_save means the history moved.
		p.rejectedDraftWrite(ctx, err, func(ctx app.Context) {
			p.loadCommits(ctx)
			p.loadDraftChanges(ctx)
		})
		ctx.Update()
	})
}

// changeAuthor asks for the author name the Save dialog fills in. It is no
// lock: it blocks nobody and only fills in the dialog (task 72_6).
func (p *Project) changeAuthor(ctx app.Context) {
	name, ok := prompt("Author:", p.saveAuthor)
	if !ok {
		return
	}
	p.saveAuthor = name
	ctx.LocalStorage().Set(uiutil.AuthorKey, name)
	ctx.Update()
}

// prompt asks the user for a text in the browser's dialog; false if they
// cancel or leave it empty.
func prompt(message, value string) (string, bool) {
	answer := app.Window().Call("prompt", message, value)
	if answer.IsNull() || answer.IsUndefined() {
		return "", false
	}
	text := strings.TrimSpace(answer.String())
	return text, text != ""
}

// openHistory shows the saved states of the project, read afresh (task 84): a
// screen of its own, in place of the content. The mode knows no Release, so the
// rows carry the saved states and their labels alone (task 85_5).
func (p *Project) openHistory(ctx app.Context) {
	p.historyScreen = true
	p.loadCommits(ctx)
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
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.CommitResponse, error) {
		return pc.RenameCommit(c, number, apiv0.RenameCommitRequest{Title: title})
	}, func(ctx app.Context, _ apiv0.CommitResponse) {
		p.loadCommits(ctx)
		ctx.Update()
	}, nil)
}

// renderCommitBar is the panel's Save, its unsaved-changes indicator and
// History (task 72_2, task 72_6; task 84).
func (p *Project) renderCommitBar() []app.UI {
	saveTitle := "Save the Draft as a saved state"
	switch {
	case p.savingCommit:
		saveTitle = "The Draft is being saved"
	case !p.hasUnsavedChanges():
		saveTitle = "Nothing to save since the last save"
	}
	body := []app.UI{
		p.panelButton("save", uiutil.IconSave(), saveTitle, p.hasUnsavedChanges() && !p.savingCommit,
			func(ctx app.Context) { p.openSaveDialog(ctx) }),
	}
	indicator := app.Span().Style("font-size", "12px")
	switch {
	case p.hasUnsavedChanges():
		indicator = indicator.
			Attr("data-commit-unsaved", "true").
			Style("color", "var(--toast-warn-title)").
			Text("Unsaved changes")
	case p.draftStateLoaded && len(p.commits) > 0:
		indicator = indicator.
			Attr("data-commit-saved", "true").
			Style("color", "var(--text-muted)").
			Text("Saved")
	default:
		indicator = indicator.Style("display", "none")
	}
	body = append(body, indicator)
	body = append(body,
		p.panelButton("history", uiutil.IconHistory(), "The saved states of the project", !p.savingCommit,
			func(ctx app.Context) { p.openHistory(ctx) }),
	)
	return body
}

// renderAuthorBar shows the author the Save dialog fills in, with
// a button to change it: it is not a lock and blocks nobody (task 72_6).
func (p *Project) renderAuthorBar() app.UI {
	name := strings.TrimSpace(p.saveAuthor)
	label := "Author: not set"
	if name != "" {
		label = "Author: " + name
	}
	return app.Div().
		Attr("data-author-bar", "true").
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "4px").
		Body(
			app.Span().
				Attr("data-author-name", name).
				Style("font-size", "12px").
				Style("color", "var(--text-muted)").
				Text(label),
			rowButton("Change", "", true, "Change the author name",
				func(ctx app.Context) { p.changeAuthor(ctx) }).
				Attr("data-author-change", "true"),
		)
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
						Disabled(p.savingCommit).
						Style("padding", "4px 12px").
						Style("font-size", "13px").
						Style("cursor", "pointer").
						Style("background", "var(--accent)").
						Style("color", "var(--accent-text)").
						Text(saveConfirmLabel(p.savingCommit)).
						OnClick(func(ctx app.Context, e app.Event) {
							p.saveCommit(ctx)
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
// A Release is not part of the Project mode any more, so the screen draws no
// Release and offers no Make Release here (task 85_5).
func (p *Project) renderHistoryScreen() app.UI {
	screen := p.history
	screen.Rows = p.historyRows()
	screen.Loaded = p.commitsLoaded
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
// labels that mark it (task 84, task 85_3). No Release is joined: a Deploy takes
// the saved state itself (task 85_5).
func (p *Project) historyRows() []savedstates.Row {
	rows := make([]savedstates.Row, len(p.commits))
	for i, saved := range p.commits {
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
