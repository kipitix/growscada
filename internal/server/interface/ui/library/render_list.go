package library

import (
	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
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
			app.H3().Style("margin", "0 0 8px 0").Text(l.listTitle()),
			l.renderListButtons(),
			app.Div().
				Style("flex", "1").
				Style("overflow-y", "auto").
				Style("margin-top", "4px").
				Body(l.renderList()),
		)
}

// listTitle names what the WidgetTypes shown are of.
func (l *Library) listTitle() string {
	if l.viewing != 0 {
		return "Release " + itoa(l.viewing) + " (read-only)"
	}
	return "Widget Types"
}

func (l *Library) renderListButtons() app.UI {
	if l.readOnly() {
		return app.Div()
	}
	deleteDisabled := l.selectedID == uuid.Nil

	deleteBtn := app.Button().
		Style("flex", "1").
		Style("padding", "4px 0").
		Style("font-size", "13px").
		Style("cursor", "pointer").
		Style("border", "1px solid var(--error-border)").
		Style("border-radius", "4px").
		Style("background", "var(--error-bg)").
		Style("color", "var(--error)").
		Text("Delete").
		OnClick(func(ctx app.Context, e app.Event) {
			if !app.Window().Call("confirm", "Are you sure you want to delete?").Bool() {
				return
			}
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
				Style("border", "1px solid var(--accent-border)").
				Style("border-radius", "4px").
				Style("background", "var(--accent-bg)").
				Style("color", "var(--accent)").
				Text("Create").
				OnClick(func(ctx app.Context, e app.Event) {
					l.createItem(ctx)
				}),
			deleteBtn,
		)
}

func (l *Library) renderList() app.UI {
	// Only the initial load shows a placeholder; event-driven reloads keep the
	// current list on screen until the fresh one arrives.
	shown := l.shownTypes()
	if l.libraryID == uuid.Nil {
		return app.Div().Style("font-size", "13px").Style("color", "var(--text-muted)").Text("Create a library first.")
	}
	if l.reloader.Loading() && len(shown) == 0 {
		return app.Div().Style("font-size", "13px").Style("color", "var(--text-muted)").Text("Loading...")
	}
	if len(shown) == 0 {
		return app.Div().Style("font-size", "13px").Style("color", "var(--text-muted)").Text("No widget types found.")
	}

	items := make([]app.UI, len(shown))
	for i, it := range shown {
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
						Style("border", "1px solid var(--accent)").
						Style("border-radius", "2px").
						Style("box-sizing", "border-box").
						Style("background", "var(--input-bg)").
						Style("color", "var(--text)").
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
								l.editingID = uuid.Nil
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
					uiutil.StoreID(ctx, selectedIDKey, id)
				}, app.EventScope(id.String())).
				OnDblClick(func(ctx app.Context, e app.Event) {
					if !l.readOnly() {
						l.startEditing(id, name)
					}
				}, app.EventScope(id.String()+"/"+name))
			if l.selectedID == id {
				item = item.(app.HTMLDiv).
					Style("background", "var(--accent)").
					Style("color", "var(--accent-text)")
			} else {
				item = item.(app.HTMLDiv).Style("color", "var(--text)")
			}
		}
		items[i] = item
	}
	return app.Div().Body(items...)
}
