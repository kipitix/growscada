package library

import (
	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

// ── Widget Types column ──────────────────────────────────────────────────────

func (l *Library) renderWidgetTypesPanel() app.UI {
	return app.Div().
		Class("panel").
		Style("flex", "0 0 224px").
		Body(
			app.Div().Class("panel-head").Body(
				app.Div().Class("panel-title").Body(
					app.Span().Class("panel-title-icon").Text("▣"),
					app.Text("Widget Types"),
				),
				app.Span().Class("panel-count").Text(len(l.widgetTypes)),
				app.Div().Class("panel-head-actions").Body(
					app.Button().
						Class("btn", "btn-accent", "btn-sm").
						Title("Create widget type").
						Text("+").
						OnClick(func(ctx app.Context, e app.Event) {
							l.createItem(ctx)
						}),
				),
			),
			app.Div().Class("panel-body").Body(l.renderList()),
			app.Div().Class("panel-foot").Body(
				l.renderDeleteButton(),
			),
		)
}

func (l *Library) renderDeleteButton() app.UI {
	disabled := l.selectedID == ""
	return app.Button().
		Class("btn", "btn-ghost", "btn-danger", "btn-sm", "btn-block").
		Text("Delete widget").
		Disabled(disabled).
		OnClick(func(ctx app.Context, e app.Event) {
			if !app.Window().Call("confirm", "Are you sure you want to delete?").Bool() {
				return
			}
			l.deleteItem(ctx)
		})
}

func (l *Library) renderList() app.UI {
	if l.loading {
		return app.Div().Class("empty").Text("Loading...")
	}
	if len(l.widgetTypes) == 0 {
		return app.Div().Class("empty").Text("No widget types found.")
	}

	items := make([]app.UI, len(l.widgetTypes))
	for i, it := range l.widgetTypes {
		id := it.ID
		name := it.Name
		kind := it.ScriptLanguage
		if kind == "" {
			kind = "—"
		}

		var item app.UI
		if l.editingID == id {
			item = app.Div().
				Style("padding", "2px").
				Body(
					app.Input().
						Class("inp").
						Type("text").
						Value(l.editingName).
						AutoFocus(true).
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
			class := "witem"
			if l.selectedID == id {
				class += " active"
			}
			item = app.Div().
				Class(class).
				Body(
					app.Div().Class("witem-ico").Text("▣"),
					app.Div().Class("witem-main").Body(
						app.Div().Class("witem-name").Text(name),
						app.Div().Class("witem-kind").Text(kind),
					),
				).
				OnClick(func(ctx app.Context, e app.Event) {
					l.selectItem(id)
					ctx.LocalStorage().Set("library:selectedID", id)
				}).
				OnDblClick(func(ctx app.Context, e app.Event) {
					l.startEditing(id, name)
				})
		}
		items[i] = item
	}
	return app.Div().Class("wlist").Body(items...)
}
