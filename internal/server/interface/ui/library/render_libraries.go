package library

import (
	"strconv"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

func itoa(n int) string { return strconv.Itoa(n) }

// ── Libraries column ──────────────────────────────────────────────────────────

func (l *Library) renderLibrariesColumn() app.UI {
	items := make([]app.UI, 0, len(l.libraries))
	for _, lib := range l.libraries {
		id := lib.ID
		item := app.Div().
			Class("library-item").
			Style("padding", "6px 8px").
			Style("cursor", "pointer").
			Style("border-radius", "4px").
			Style("font-size", "13px").
			Text(lib.Name).
			OnClick(func(ctx app.Context, e app.Event) {
				l.selectLibrary(ctx, id)
			}, app.EventScope(id.String())).
			OnDblClick(func(ctx app.Context, e app.Event) {
				l.renameLibrary()
			})
		if id == l.libraryID {
			item = item.Style("background", "var(--accent)").Style("color", "var(--accent-text)")
		} else {
			item = item.Style("color", "var(--text)")
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		items = append(items, app.Div().Style("font-size", "13px").Style("color", "var(--text-muted)").Text("No libraries."))
	}

	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("width", "200px").
		Style("flex-shrink", "0").
		Style("min-height", "0").
		Body(
			app.H3().Style("margin", "0 0 8px 0").Text("Libraries"),
			l.renderDraftChangesBar(),
			app.Div().
				Style("display", "flex").
				Style("gap", "4px").
				Style("margin-bottom", "4px").
				Body(
					smallButton("Create", true, "accent", "Create a library", l.createLibrary),
					smallButton("Rename", l.libraryID != uuid.Nil, "", "Rename the selected library", l.renameLibrary),
					smallButton("Delete", l.libraryID != uuid.Nil, "error", "Delete the selected library with its Draft and releases", l.deleteLibrary),
				),
			app.Div().
				Style("flex", "0 1 auto").
				Style("max-height", "30%").
				Style("overflow-y", "auto").
				Body(items...),
			l.renderCommitBar(),
			app.If(l.saveDialog, l.renderSaveDialog).Else(func() app.UI { return app.Div() }),
			app.If(l.commitsDialog, l.renderCommitsDialog).Else(func() app.UI { return app.Div() }),
			l.renderLibraryStatus(),
			l.renderHistory(),
		)
}

// renderLibraryStatus renders the selected library's Commit and Release
// state, its Discard and its Revert.
func (l *Library) renderLibraryStatus() app.UI {
	lib, ok := l.selectedLibrary()
	if !ok {
		return app.Div()
	}
	var status string
	switch {
	case lib.LatestRelease == 0 && lib.LatestCommit == 0:
		status = "Never saved or released."
	case lib.LatestCommit == 0:
		status = "Release " + itoa(lib.LatestRelease) + " · legacy, no commits"
	case lib.LatestRelease == 0:
		status = "Commit #" + itoa(lib.LatestCommit) + " · never released"
	case lib.HasUnpublishedChanges:
		status = "Release " + itoa(lib.LatestRelease) + " · unpublished commits"
	default:
		status = "Release " + itoa(lib.LatestRelease) + " · up to date"
	}
	return app.Div().
		Style("margin-top", "8px").
		Style("padding-top", "8px").
		Style("border-top", "1px solid var(--border)").
		Body(
			app.Div().
				Class("library-status").
				Style("font-size", "12px").
				Style("color", "var(--text-muted)").
				Style("margin-bottom", "6px").
				Text(status),
			app.Div().
				Style("display", "flex").
				Style("gap", "4px").
				Body(
					smallButton("Discard", lib.LatestCommit > 0, "error",
						"Reset the Draft to its last Commit, dropping the unsaved changes", l.discard),
					smallButton("Revert", lib.LatestRelease > 0, "",
						"Reset the Draft to its latest Release, the published state", l.revert),
				),
		)
}

// renderHistory renders the Draft and the releases of the selected library;
// a release is shown read-only.
func (l *Library) renderHistory() app.UI {
	if l.libraryID == uuid.Nil {
		return app.Div()
	}
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
		r := l.releases[i]
		label := releaseHistoryLabel(r)
		title := r.Author + " · " + r.CreatedAt.Local().Format("2006-01-02 15:04")
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
		items = append(items, entry(r.Number, label, title, note))
	}
	return app.Div().
		Style("margin-top", "8px").
		Style("flex", "1").
		Style("min-height", "0").
		Style("overflow-y", "auto").
		Body(
			app.H3().Style("margin", "0 0 6px 0").Style("font-size", "13px").Text("History"),
			app.Div().Body(items...),
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

// smallButton renders a button of the columns; kind "accent", "error" or ""
// picks its colours, title is its hover text.
func smallButton(text string, enabled bool, kind, title string, onClick func()) app.UI {
	border, bg, fg := "var(--border-input)", "var(--bg-hover)", "var(--text)"
	switch kind {
	case "accent":
		border, bg, fg = "var(--accent-border)", "var(--accent-bg)", "var(--accent)"
	case "error":
		border, bg, fg = "var(--error-border)", "var(--error-bg)", "var(--error)"
	}
	b := app.Button().
		Title(title).
		Style("flex", "1").
		Style("padding", "4px 0").
		Style("font-size", "13px").
		Style("border", "1px solid "+border).
		Style("border-radius", "4px").
		Style("background", bg).
		Style("color", fg).
		Text(text)
	if !enabled {
		return b.Style("opacity", "0.4").Style("cursor", "default").Disabled(true)
	}
	return b.Style("cursor", "pointer").OnClick(func(ctx app.Context, e app.Event) { onClick() })
}
