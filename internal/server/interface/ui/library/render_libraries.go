package library

import (
	"strconv"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"
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
			}).
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
			app.Div().
				Style("display", "flex").
				Style("gap", "4px").
				Style("margin-bottom", "4px").
				Body(
					smallButton("Create", true, "accent", l.createLibrary),
					smallButton("Rename", l.libraryID != uuid.Nil, "", l.renameLibrary),
					smallButton("Delete", l.libraryID != uuid.Nil, "error", l.deleteLibrary),
				),
			app.Div().
				Style("flex", "0 1 auto").
				Style("max-height", "30%").
				Style("overflow-y", "auto").
				Body(items...),
			l.renderLibraryStatus(),
			l.renderHistory(),
		)
}

// renderLibraryStatus renders the selected library's release state, Release
// and Discard.
func (l *Library) renderLibraryStatus() app.UI {
	lib, ok := l.selectedLibrary()
	if !ok {
		return app.Div()
	}
	var status string
	switch {
	case lib.DiscardingTo != 0:
		status = "Discarding to release " + itoa(lib.DiscardingTo) + "… the Draft is read-only."
	case lib.LatestRelease == 0:
		status = "Never released."
	case lib.HasUnreleasedChanges:
		status = "Release " + itoa(lib.LatestRelease) + " · unreleased changes"
	default:
		status = "Release " + itoa(lib.LatestRelease) + " · up to date"
	}
	editable := lib.DiscardingTo == 0
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
					smallButton("Release", editable && lib.HasUnreleasedChanges, "accent", l.release),
					smallButton("Discard", editable && lib.LatestRelease > 0 && lib.HasUnreleasedChanges, "error", l.discard),
				),
		)
}

// renderHistory renders the Draft and the releases of the selected library;
// a release is shown read-only.
func (l *Library) renderHistory() app.UI {
	if l.libraryID == uuid.Nil {
		return app.Div()
	}
	entry := func(n int, label, title string) app.UI {
		d := app.Div().
			Class("library-history").
			Style("padding", "4px 8px").
			Style("cursor", "pointer").
			Style("border-radius", "4px").
			Style("font-size", "12px").
			Title(title).
			Text(label).
			OnClick(func(ctx app.Context, e app.Event) { l.view(n) })
		if n == l.viewing {
			return d.Style("background", "var(--bg-hover)").Style("font-weight", "600").Style("color", "var(--text)")
		}
		return d.Style("color", "var(--text)")
	}
	items := []app.UI{entry(0, "Draft", "The WidgetTypes being edited")}
	for i := len(l.releases) - 1; i >= 0; i-- {
		r := l.releases[i]
		label := "Release " + itoa(r.Number)
		if r.Note != "" {
			label += " — " + r.Note
		}
		items = append(items, entry(r.Number, label, r.CreatedAt.Local().Format("2006-01-02 15:04")))
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

// smallButton renders a button of the columns; kind "accent", "error" or ""
// picks its colours.
func smallButton(text string, enabled bool, kind string, onClick func()) app.UI {
	border, bg, fg := "var(--border-input)", "var(--bg-hover)", "var(--text)"
	switch kind {
	case "accent":
		border, bg, fg = "var(--accent-border)", "var(--accent-bg)", "var(--accent)"
	case "error":
		border, bg, fg = "var(--error-border)", "var(--error-bg)", "var(--error)"
	}
	b := app.Button().
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
