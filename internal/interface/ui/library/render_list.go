package library

import (
	"fmt"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

// ── List column ───────────────────────────────────────────────────────────────

func (l *Library) renderListColumn() app.UI {
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("width", "200px").
		Style("flex-shrink", "0").
		Style("min-height", "0").
		Body(
			app.H3().Style("margin", "0 0 8px 0").Text("Widget Types"),
			l.renderListButtons(),
			app.Div().
				Style("flex", "1").
				Style("overflow-y", "auto").
				Style("margin-top", "4px").
				Body(l.renderList()),
		)
}

func (l *Library) renderListButtons() app.UI {
	deleteDisabled := l.selectedID == ""

	deleteBtn := app.Button().
		Style("flex", "1").
		Style("padding", "4px 0").
		Style("font-size", "13px").
		Style("cursor", "pointer").
		Style("border", "1px solid #ccc").
		Style("border-radius", "4px").
		Text("Delete").
		OnClick(func(ctx app.Context, e app.Event) {
			l.deleteItem(ctx)
		})
	if deleteDisabled {
		deleteBtn = deleteBtn.
			Style("opacity", "0.4").
			Style("cursor", "default").
			Disabled(true)
	}

	return app.Div().
		Style("display", "flex").
		Style("gap", "4px").
		Style("margin-bottom", "4px").
		Body(
			app.Button().
				Style("flex", "1").
				Style("padding", "4px 0").
				Style("font-size", "13px").
				Style("cursor", "pointer").
				Style("border", "1px solid #ccc").
				Style("border-radius", "4px").
				Text("Create").
				OnClick(func(ctx app.Context, e app.Event) {
					l.createItem(ctx)
				}),
			deleteBtn,
		)
}

func (l *Library) renderList() app.UI {
	if l.loading {
		return app.Div().Style("font-size", "13px").Style("color", "#999").Text("Loading...")
	}
	if l.fetchErr != "" {
		return app.Div().Style("font-size", "13px").Style("color", "#c00").Text(fmt.Sprintf("Error: %s", l.fetchErr))
	}
	if len(l.widgetTypes) == 0 {
		return app.Div().Style("font-size", "13px").Style("color", "#999").Text("No widget types found.")
	}

	items := make([]app.UI, len(l.widgetTypes))
	for i, it := range l.widgetTypes {
		id := it.ID
		name := it.Name
		var item app.UI
		if l.editingID == id {
			item = app.Div().
				Style("padding", "2px 4px").
				Style("border-radius", "4px").
				Body(
					app.Input().
						Type("text").
						Value(l.editingName).
						AutoFocus(true).
						Style("width", "100%").
						Style("font-size", "13px").
						Style("padding", "3px 4px").
						Style("border", "1px solid #0066cc").
						Style("border-radius", "2px").
						Style("box-sizing", "border-box").
						OnInput(func(ctx app.Context, e app.Event) {
							l.editingName = ctx.JSSrc().Get("value").String()
						}).
						OnBlur(func(ctx app.Context, e app.Event) {
							l.commitEdit(ctx)
						}).
						OnKeyDown(func(ctx app.Context, e app.Event) {
							switch e.Get("key").String() {
							case "Enter":
								l.commitEdit(ctx)
							case "Escape":
								l.editingID = ""
								l.editingName = ""
							}
						}),
				)
		} else {
			item = app.Div().
				Style("padding", "6px 8px").
				Style("cursor", "pointer").
				Style("border-radius", "4px").
				Style("font-size", "13px").
				Body(app.Text(name)).
				OnClick(func(ctx app.Context, e app.Event) {
					l.selectItem(id)
					ctx.LocalStorage().Set("library:selectedID", id)
				}).
				OnDblClick(func(ctx app.Context, e app.Event) {
					l.startEditing(id, name)
				})
			if l.selectedID == id {
				item = item.(app.HTMLDiv).
					Style("background", "#0066cc").
					Style("color", "#fff")
			} else {
				item = item.(app.HTMLDiv).Style("color", "#333")
			}
		}
		items[i] = item
	}
	return app.Div().Body(items...)
}
