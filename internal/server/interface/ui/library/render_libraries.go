package library

import (
	"strconv"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

func itoa(n int) string { return strconv.Itoa(n) }

// ── Header ────────────────────────────────────────────────────────────────────

// renderHeader is the panel above the editor (task 84, task 91): the way back,
// the floppy that opens the saved states, Undo and Redo, and the unsaved-changes
// marker after them. The entity's name lives in the navigation line above, and
// the panel names no saved state. The user name is set in the Misc section and
// is not repeated here (task 90).
func (l *Library) renderHeader() app.UI {
	if l.libraryID == uuid.Nil {
		return app.Div()
	}
	body := []app.UI{
		l.renderBackButton(),
		uiutil.BarDivider(),
		l.renderSaveButton(),
		uiutil.BarDivider(),
		l.renderDraftChangesBar(),
		l.renderUnsavedIndicator(),
		app.Div().Style("flex", "1"),
	}
	return app.Div().
		Attr("data-library-header", "true").
		Style("flex-shrink", "0").
		Body(
			app.Div().
				Style("display", "flex").
				Style("flex-wrap", "wrap").
				Style("align-items", "center").
				Style("gap", "8px").
				Style("padding", "6px 10px").
				Style("border-bottom", "1px solid var(--border)").
				Style("background", "var(--bg-elevated)").
				Body(body...),
		)
}

// renderBackButton is the panel's single way back (task 84, task 91): from the
// saved states to the editor, from the editor to the selection page. Root owns
// the page; a mode nothing wired shows no arrow. The entity's name is not
// repeated beside it.
func (l *Library) renderBackButton() app.UI {
	if l.historyScreen {
		return uiutil.PanelBackArrow("Back to the Draft", func(ctx app.Context, e app.Event) {
			l.closeHistory()
		})
	}
	var back func(ctx app.Context, e app.Event)
	if l.OnBack != nil {
		back = func(ctx app.Context, e app.Event) { l.OnBack() }
	}
	return uiutil.PanelBackArrow("Back to the libraries", back)
}

// ── Buttons ───────────────────────────────────────────────────────────────────

// smallButton renders a button of the dialogs in their shared-width layout;
// kind "accent", "error" or "" picks its colours, title is its hover text.
// options go to the click handler: a button of a list gives it the row's
// identity, so a reused element does not keep the handler of the row it showed
// before (app.EventScope).
func smallButton(text string, enabled bool, kind, title string, onClick func(), options ...app.EventOption) app.UI {
	return uiutil.Button(text, kind, enabled, title, uiutil.ButtonSmall,
		func(app.Context, app.Event) { onClick() }, options...)
}
