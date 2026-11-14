package library

import (
	"strconv"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

func itoa(n int) string { return strconv.Itoa(n) }

// ── Header ────────────────────────────────────────────────────────────────────

// renderHeader is the panel above the editor (task 84): the way back to the
// libraries with the name of the one shown, Save with its unsaved-changes
// indicator, Undo and Redo, History and Releases — icons, each with its hover
// text —, the library's state and the author. The dialogs the panel opens
// follow it.
func (l *Library) renderHeader() app.UI {
	if l.libraryID == uuid.Nil {
		return app.Div()
	}
	body := []app.UI{
		l.renderBackButton(),
		uiutil.BarDivider(),
	}
	body = append(body, l.renderCommitBar()...)
	body = append(body,
		uiutil.BarDivider(),
		l.renderDraftChangesBar(),
		uiutil.BarDivider(),
		l.renderLibraryStatus(),
		app.Div().Style("flex", "1"),
		l.renderAuthorBar(),
	)
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
			app.If(l.saveDialog, l.renderSaveDialog).Else(func() app.UI { return app.Div() }),
			app.If(l.releaseDialog, l.renderReleaseDialog).Else(func() app.UI { return app.Div() }),
		)
}

// renderBackButton is the way back to the selection page (task 84), with the
// name of the library shown. Root wires both — it owns the page and the name
// the selection announced —, so a mode nothing wired shows the name alone.
func (l *Library) renderBackButton() app.UI {
	name := ""
	if l.BackName != nil {
		name = l.BackName()
	}
	if name == "" {
		if lib, ok := l.selectedLibrary(); ok {
			name = lib.Name
		}
	}
	var back func(ctx app.Context, e app.Event)
	if l.OnBack != nil {
		back = func(ctx app.Context, e app.Event) { l.OnBack() }
	}
	return uiutil.PanelBack("Back to the libraries", name, back)
}

// releaseStateOf is one library's Release state (task 72_7; ADR 0021): the
// Draft changed since its last saved state (the server's own flag, which the
// selected one follows too — task 74), the last saved state is not published,
// or the published state is the saved one. The state is a short word for the
// panel and its kind says which.
func (l *Library) releaseStateOf(lib apiv0.LibraryResponse) (kind, text string) {
	switch {
	case lib.HasUnsavedChanges:
		return "unsaved", "unsaved changes"
	case lib.HasUnpublishedChanges:
		return "unpublished", "unpublished saved state"
	case lib.LatestCommit > 0:
		return "published", "published"
	case lib.LatestRelease > 0:
		return "legacy", "legacy release"
	default:
		return "new", "never saved"
	}
}

// renderLibraryStatus is the state of the library in words: what its last saved
// state and its last Release are.
func (l *Library) renderLibraryStatus() app.UI {
	lib, ok := l.selectedLibrary()
	if !ok {
		return app.Div()
	}
	kind, _ := l.releaseStateOf(lib)
	var status string
	switch {
	case lib.LatestRelease == 0 && lib.LatestCommit == 0:
		status = "Never saved or released."
	case lib.LatestCommit == 0:
		status = "Release " + itoa(lib.LatestRelease) + " · legacy, no saved states"
	case kind == "unsaved":
		status = "Saved state #" + itoa(lib.LatestCommit) + " · unsaved changes"
	case kind == "unpublished":
		status = "Saved state #" + itoa(lib.LatestCommit) + " · not published yet"
	case lib.LatestRelease == 0:
		status = "Saved state #" + itoa(lib.LatestCommit) + " · never released"
	default:
		status = "Release " + itoa(lib.LatestRelease) + " · published"
	}
	return app.Div().
		Attr("data-library-status", "true").
		Style("display", "flex").
		Style("align-items", "center").
		Body(
			app.Span().
				Class("library-status").
				Style("font-size", "12px").
				Style("color", "var(--text-muted)").
				Text(status),
		)
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
