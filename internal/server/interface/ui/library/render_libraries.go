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

// renderHeader is the slim header above the editor (task 73): Save with its
// unsaved-changes indicator, History, Releases, the Undo and Redo pair, the
// library's Commit and Release state with Discard and Revert, and the author —
// the actions that used to sit in the libraries column, which is gone. The
// dialogs the header opens follow it.
func (l *Library) renderHeader() app.UI {
	if l.libraryID == uuid.Nil {
		return app.Div()
	}
	body := l.renderCommitBar()
	body = append(body,
		barDivider(),
		l.renderDraftChangesBar(),
		barDivider(),
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
			app.If(l.commitsDialog, l.renderCommitsDialog).Else(func() app.UI { return app.Div() }),
			app.If(l.releasesDialog, l.renderReleasesDialog).Else(func() app.UI { return app.Div() }),
			app.If(l.releaseDialog, l.renderReleaseDialog).Else(func() app.UI { return app.Div() }),
		)
}

// barDivider separates the groups of the header.
func barDivider() app.UI {
	return app.Div().Style("width", "1px").Style("height", "18px").Style("background", "var(--border)")
}

// releaseStateOf is one library's Release state (task 72_7; ADR 0021): the
// Draft changed since its last Commit (the server's own flag, which the
// selected one follows too — task 74), the last Commit is not published, or the
// published state is the saved one. The state is a short word for the header
// and its kind says which.
func (l *Library) releaseStateOf(lib apiv0.LibraryResponse) (kind, text string) {
	switch {
	case lib.HasUnsavedChanges:
		return "unsaved", "unsaved changes"
	case lib.HasUnpublishedChanges:
		return "unpublished", "unpublished commit"
	case lib.LatestCommit > 0:
		return "published", "published"
	case lib.LatestRelease > 0:
		return "legacy", "legacy release"
	default:
		return "new", "never saved"
	}
}

// renderLibraryStatus renders the selected library's Commit and Release state
// and its Discard and Revert.
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
		status = "Release " + itoa(lib.LatestRelease) + " · legacy, no commits"
	case kind == "unsaved":
		status = "Commit #" + itoa(lib.LatestCommit) + " · unsaved changes"
	case kind == "unpublished":
		status = "Commit #" + itoa(lib.LatestCommit) + " · not published yet"
	case lib.LatestRelease == 0:
		status = "Commit #" + itoa(lib.LatestCommit) + " · never released"
	default:
		status = "Release " + itoa(lib.LatestRelease) + " · published"
	}
	return app.Div().
		Attr("data-library-status", "true").
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "6px").
		Body(
			app.Span().
				Class("library-status").
				Style("font-size", "12px").
				Style("color", "var(--text-muted)").
				Text(status),
			barButton("Discard", lib.LatestCommit > 0, "error",
				"Reset the Draft to its last Commit, dropping the unsaved changes", l.discard,
				app.EventScope("discard")),
			barButton("Revert", lib.LatestRelease > 0, "",
				"Reset the Draft to its latest Release, the published state", l.revert,
				app.EventScope("revert")),
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
		if n == 0 && l.viewingCommit != 0 {
			d = d.Style("color", "var(--text)")
		} else if n == l.viewing {
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
		label += " — commit #" + itoa(*r.CommitNumber)
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
