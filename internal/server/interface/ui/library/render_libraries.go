package library

import (
	"fmt"
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
			app.If(l.releasesDialog, l.renderReleasesDialog).Else(func() app.UI { return app.Div() }),
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
	body := []app.UI{}
	if l.OnBack != nil {
		body = append(body, uiutil.PanelButton(uiutil.IconBack(), true, "Back to the libraries",
			func(app.Context, app.Event) { l.OnBack() }, app.EventScope("back")))
	}
	body = append(body, app.Span().
		Attr("data-editor-name", "true").
		Style("font-size", "13px").
		Style("font-weight", "600").
		Style("color", "var(--text)").
		// A long name is cut, never allowed to push the panel's actions out.
		Style("max-width", "260px").
		Style("overflow", "hidden").
		Style("text-overflow", "ellipsis").
		Style("white-space", "nowrap").
		Text(name))
	return app.Div().
		Attr("data-panel-back", "true").
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "6px").
		Body(body...)
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

// ── Releases ──────────────────────────────────────────────────────────────────

// openReleasesDialog shows the releases of the selected library, read afresh.
func (l *Library) openReleasesDialog() {
	if l.libraryID == uuid.Nil {
		return
	}
	l.releasesDialog = true
	l.loadReleases(l.compoCtx)
	l.compoCtx.Update()
}

// renderReleasesDialog is the releases of the selected library and a way back
// to the Draft; a release is shown read-only, with the Commit it published,
// its Author, time and the count of WidgetTypes it copied (task 72_7). The
// note of each is edited in place.
func (l *Library) renderReleasesDialog() app.UI {
	// The handlers are scoped by n: go-app keeps a handler of the same func
	// literal on a reused element, so a release prepended to the list would
	// leave each entry viewing the release it showed before.
	entry := func(n int, label, title string, extra app.UI) app.UI {
		d := app.Div().
			Class("library-history").
			Style("display", "flex").
			Style("align-items", "center").
			Style("gap", "4px").
			Style("padding", "4px 8px").
			Style("cursor", "pointer").
			Style("border-radius", "4px").
			Style("font-size", "12px").
			Title(title).
			OnClick(func(ctx app.Context, e app.Event) { l.view(n) }, app.EventScope(itoa(n)))
		text := app.Span().Style("flex", "1").Text(label)
		if n == l.viewing {
			d = d.Style("background", "var(--bg-hover)").Style("font-weight", "600").Style("color", "var(--text)")
		} else {
			d = d.Style("color", "var(--text)")
		}
		return d.Body(text, extra)
	}
	items := []app.UI{entry(0, "Draft", "The WidgetTypes being edited", app.Div())}
	for i := len(l.releases) - 1; i >= 0; i-- {
		items = append(items, l.renderReleaseRow(l.releases[i]))
	}
	return app.Div().
		Attr("data-releases-dialog", "true").
		Style("padding", "8px").
		Style("margin-top", "6px").
		Style("border", "1px solid var(--border)").
		Style("border-radius", "4px").
		Style("background", "var(--bg)").
		Style("font-size", "12px").
		Style("max-height", "40vh").
		Style("overflow-y", "auto").
		Body(
			app.Div().Style("font-weight", "600").Style("margin-bottom", "4px").Text("Releases"),
			app.Div().Body(items...),
			smallButton("Close", true, "", "Close the releases", func() {
				l.releasesDialog = false
				l.compoCtx.Update()
			}),
		)
}

// releaseHistoryLabel is the history line of a Release: its number, the Commit
// it published (or "legacy") and its note (task 72_5).
func releaseHistoryLabel(r apiv0.LibraryReleaseResponse) string {
	label := "Release " + itoa(r.Number)
	if r.CommitNumber != nil {
		label += " — saved state #" + itoa(*r.CommitNumber)
		if r.Title != nil && *r.Title != "" {
			label += " " + *r.Title
		}
	} else {
		label += " — legacy"
	}
	if r.Note != "" {
		label += " — " + r.Note
	}
	return label
}

// renderReleaseRow is one Release of the list: what it published and its
// Author, time and count, its note edited in place. Clicking it shows its
// copies read-only (task 72_7).
func (l *Library) renderReleaseRow(r apiv0.LibraryReleaseResponse) app.UI {
	// A bare inline action: it borrows the browser's own button look, so it
	// carries no palette of its own.
	note := app.Button().
		Attr("data-release-note", itoa(r.Number)).
		Style("padding", "0 6px").
		Style("font-size", "11px").
		Style("cursor", "pointer").
		Text("Note").
		OnClick(func(ctx app.Context, e app.Event) {
			// The row below shows the Release: editing its note does not
			// also switch the view to it.
			e.Call("stopPropagation")
			e.StopImmediatePropagation()
			l.editReleaseNote(r)
		}, app.EventScope("note/"+itoa(r.Number)))
	row := app.Div().
		Class("library-history").
		Attr("data-release-row", itoa(r.Number)).
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "4px").
		Style("padding", "4px 8px").
		Style("cursor", "pointer").
		Style("border-radius", "4px").
		Style("font-size", "12px").
		Title(r.Author+" · "+r.CreatedAt.Local().Format("2006-01-02 15:04")).
		OnClick(func(ctx app.Context, e app.Event) { l.view(r.Number) }, app.EventScope(itoa(r.Number))).
		Body(
			app.Div().
				Style("display", "flex").
				Style("align-items", "center").
				Style("gap", "4px").
				Body(app.Span().Style("flex", "1").Text(releaseHistoryLabel(r)), note),
			app.Div().
				Style("font-size", "10px").
				Style("opacity", "0.75").
				Text(fmt.Sprintf("%s · %s · %d widget types", r.Author,
					r.CreatedAt.Local().Format("2006-01-02 15:04"), r.ContentCounts.WidgetTypes)),
		)
	if r.Number == l.viewing {
		return row.Style("background", "var(--bg-hover)").Style("font-weight", "600").Style("color", "var(--text)")
	}
	return row.Style("color", "var(--text)")
}

// ── Buttons ───────────────────────────────────────────────────────────────────

// barButton renders one action of the header in its compact layout; kind
// "accent", "error" or "" picks its colours, title is its hover text. options
// go to the click handler: a button of a list gives it the row's identity, so
// a reused element does not keep the handler of the row it showed before
// (app.EventScope).
func barButton(text string, enabled bool, kind, title string, onClick func(), options ...app.EventOption) app.UI {
	return uiutil.Button(text, kind, enabled, title, uiutil.ButtonBar,
		func(app.Context, app.Event) { onClick() }, options...)
}

// smallButton renders a button of the dialogs in their shared-width layout;
// kind "accent", "error" or "" picks its colours, title is its hover text.
// options go to the click handler: a button of a list gives it the row's
// identity, so a reused element does not keep the handler of the row it showed
// before (app.EventScope).
func smallButton(text string, enabled bool, kind, title string, onClick func(), options ...app.EventOption) app.UI {
	return uiutil.Button(text, kind, enabled, title, uiutil.ButtonSmall,
		func(app.Context, app.Event) { onClick() }, options...)
}
