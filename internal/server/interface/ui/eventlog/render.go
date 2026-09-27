package eventlog

import (
	"fmt"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

// visibleRows and entryRowHeightPx fix the entry list's height at exactly
// visibleRows rows, whether it holds zero entries or hundreds — the panel
// never grows/shrinks with content, only scrolls internally.
const (
	visibleRows      = 10
	entryRowHeightPx = 22
)

func (b *Bar) Render() app.UI {
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex-shrink", "0").
		Body(
			app.If(b.panelOpen, func() app.UI {
				return b.renderPanel()
			}),
			b.renderStatusBar(),
		)
}

func (b *Bar) renderStatusBar() app.UI {
	buttonBg := "var(--bg-elevated)"
	buttonColor := "var(--text-2)"
	if b.panelOpen {
		buttonBg = "var(--accent-bg)"
		buttonColor = "var(--accent)"
	}

	return app.Div().
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "10px").
		Style("padding", "3px 12px").
		Style("border-top", "1px solid var(--border)").
		Style("background", "var(--bg-elevated)").
		Style("font-size", "12px").
		Style("color", "var(--text-2)").
		Body(
			app.Span().
				Style("font-variant-numeric", "tabular-nums").
				Text(b.now),
			app.Div().Style("flex", "1"),
			app.Button().
				Style("padding", "3px 10px").
				Style("font-size", "12px").
				Style("cursor", "pointer").
				Style("border", "1px solid var(--border-input)").
				Style("border-radius", "4px").
				Style("background", buttonBg).
				Style("color", buttonColor).
				Text("Events").
				OnClick(func(ctx app.Context, e app.Event) {
					b.togglePanel()
				}),
		)
}

func (b *Bar) renderPanel() app.UI {
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("border-top", "1px solid var(--border)").
		Style("background", "var(--surface)").
		Body(
			b.renderFilters(),
			app.Div().
				Style("height", fmt.Sprintf("%dpx", visibleRows*entryRowHeightPx)).
				Style("overflow-y", "auto").
				Body(b.renderEntries()),
		)
}

func (b *Bar) renderFilters() app.UI {
	pills := make([]app.UI, len(allCategories))
	for i, c := range allCategories {
		pills[i] = b.renderFilterPill(c)
	}
	return app.Div().
		Style("display", "flex").
		Style("gap", "6px").
		Style("padding", "6px 12px").
		Style("border-bottom", "1px solid var(--border-subtle)").
		Body(pills...)
}

func (b *Bar) renderFilterPill(c category) app.UI {
	active := b.filters[c]

	bg := "var(--bg-elevated)"
	color := "var(--text-3)"
	border := "var(--border-input)"
	if active {
		bg = "var(--accent-bg)"
		color = "var(--accent)"
		border = "var(--accent-border)"
	}

	return app.Button().
		Style("padding", "2px 8px").
		Style("font-size", "11px").
		Style("cursor", "pointer").
		Style("border", "1px solid "+border).
		Style("border-radius", "10px").
		Style("background", bg).
		Style("color", color).
		Text(string(c)).
		OnClick(func(ctx app.Context, e app.Event) {
			b.toggleFilter(ctx, c)
		})
}

func (b *Bar) renderEntries() app.UI {
	visible := make([]app.UI, 0, len(b.entries))
	for _, entry := range b.entries {
		if !b.filters[entry.Category] {
			continue
		}
		visible = append(visible, b.renderEntry(entry))
	}

	if len(visible) == 0 {
		return app.Div().
			Style("height", fmt.Sprintf("%dpx", entryRowHeightPx)).
			Style("box-sizing", "border-box").
			Style("padding", "0 12px").
			Style("line-height", fmt.Sprintf("%dpx", entryRowHeightPx)).
			Style("font-size", "12px").
			Style("color", "var(--text-muted)").
			Text("No events.")
	}

	return app.Div().Body(visible...)
}

func (b *Bar) renderEntry(entry logEntry) app.UI {
	return app.Div().
		Style("box-sizing", "border-box").
		Style("height", fmt.Sprintf("%dpx", entryRowHeightPx)).
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "10px").
		Style("padding", "0 12px").
		Style("font-size", "12px").
		Style("border-bottom", "1px solid var(--border-subtle)").
		Body(
			app.Span().
				Style("color", "var(--text-3)").
				Style("font-variant-numeric", "tabular-nums").
				Style("flex-shrink", "0").
				Text(entry.Time),
			app.Span().
				Style("color", "var(--text)").
				Text(entry.Text),
		)
}
