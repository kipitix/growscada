package project

import (
	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

var tagTypes = []string{"string", "boolean", "integer"}

func (p *Project) renderTagsPanel() app.UI {
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "row").
		Style("flex", "1").
		Style("min-height", "0").
		Style("gap", "12px").
		Body(
			p.renderTagListColumn(),
			p.renderTagPropertiesPanel(),
		)
}

// ── Left column: list ─────────────────────────────────────────────────────────

func (p *Project) renderTagListColumn() app.UI {
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("width", "200px").
		Style("flex-shrink", "0").
		Style("min-height", "0").
		Body(
			app.H3().Style("margin", "0 0 8px 0").Text("Tags"),
			p.renderTagListButtons(),
			app.Div().
				Style("flex", "1").
				Style("overflow-y", "auto").
				Style("margin-top", "4px").
				Body(p.renderTagList()),
		)
}

func (p *Project) renderTagListButtons() app.UI {
	deleteDisabled := p.selectedTagID == "" || p.creatingTag

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
			p.deleteTag(ctx)
		})
	if deleteDisabled {
		deleteBtn = deleteBtn.
			Style("opacity", "0.4").
			Style("cursor", "default").
			Disabled(true)
	}

	createDisabled := p.creatingTag
	createBtn := app.Button().
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
			p.creatingTag = true
			p.newTagName = "New Tag"
			p.newTagType = "string"
			p.selectedTagID = ""
			ctx.LocalStorage().Set("project:tagID", "")
		})
	if createDisabled {
		createBtn = createBtn.
			Style("opacity", "0.4").
			Style("cursor", "default").
			Disabled(true)
	}

	return app.Div().
		Style("display", "flex").
		Style("gap", "4px").
		Style("margin-bottom", "4px").
		Body(createBtn, deleteBtn)
}

func (p *Project) renderTagList() app.UI {
	if p.tagFetchErr != "" {
		return app.Div().Style("font-size", "13px").Style("color", "var(--error)").Text("Error: " + p.tagFetchErr)
	}
	if len(p.tags) == 0 {
		return app.Div().Style("font-size", "13px").Style("color", "var(--text-muted)").Text("No tags.")
	}

	items := make([]app.UI, len(p.tags))
	for i, t := range p.tags {
		id := t.ID
		name := t.Name
		tagType := t.Type
		var item app.HTMLDiv = app.Div().
			Style("padding", "6px 8px").
			Style("cursor", "pointer").
			Style("border-radius", "4px").
			Style("font-size", "13px").
			Body(
				app.Div().Text(name),
				app.Div().Style("font-size", "11px").Style("opacity", "0.65").Text(tagType),
			).
			OnClick(func(ctx app.Context, e app.Event) {
				p.selectedTagID = id
				p.creatingTag = false
				ctx.LocalStorage().Set("project:tagID", id)
			})
		if p.selectedTagID == id {
			item = item.Style("background", "var(--accent)").Style("color", "var(--accent-text)")
		} else {
			item = item.Style("color", "var(--text)")
		}
		items[i] = item
	}
	return app.Div().Body(items...)
}

// ── Right panel: properties ───────────────────────────────────────────────────

func (p *Project) renderTagPropertiesPanel() app.UI {
	if p.creatingTag {
		return p.renderTagCreateForm()
	}
	if p.selectedTagID == "" {
		return app.Div().
			Style("flex", "1").
			Style("display", "flex").
			Style("align-items", "center").
			Style("justify-content", "center").
			Style("color", "var(--text-muted)").
			Style("font-size", "14px").
			Body(app.Text("Select a tag or create a new one."))
	}

	var selected tagItem
	for _, t := range p.tags {
		if t.ID == p.selectedTagID {
			selected = t
			break
		}
	}

	return app.Div().
		Style("flex", "1").
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("gap", "12px").
		Style("padding", "4px 0").
		Body(
			app.H3().Style("margin", "0 0 4px 0").Text("Tag Properties"),
			p.renderTagPropRow("Name", selected.Name),
			p.renderTagPropRow("Type", selected.Type),
			p.renderTagPropRow("Value", selected.Value),
			p.renderTagPropRow("Quality", selected.Quality),
		)
}

func (p *Project) renderTagPropRow(label, value string) app.UI {
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("gap", "2px").
		Body(
			app.Div().
				Style("font-size", "11px").
				Style("color", "var(--text-3)").
				Style("text-transform", "uppercase").
				Style("letter-spacing", "0.04em").
				Text(label),
			app.Div().
				Style("font-size", "14px").
				Style("color", "var(--text)").
				Text(value),
		)
}

func (p *Project) renderTagCreateForm() app.UI {
	typeOptions := make([]app.UI, len(tagTypes))
	for i, t := range tagTypes {
		opt := app.Option().Value(t).Text(t)
		if t == p.newTagType {
			opt = opt.Selected(true)
		}
		typeOptions[i] = opt
	}

	return app.Div().
		Style("flex", "1").
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("gap", "16px").
		Style("padding", "4px 0").
		Style("max-width", "320px").
		Body(
			app.H3().Style("margin", "0 0 4px 0").Text("New Tag"),
			app.Div().
				Style("display", "flex").
				Style("flex-direction", "column").
				Style("gap", "4px").
				Body(
					app.Label().Style("font-size", "12px").Style("color", "var(--text-2)").Text("Name"),
					app.Input().
						Type("text").
						Value(p.newTagName).
						AutoFocus(true).
						Style("font-size", "14px").
						Style("padding", "5px 8px").
						Style("border", "1px solid var(--border-input)").
						Style("border-radius", "4px").
						Style("width", "100%").
						Style("box-sizing", "border-box").
						OnInput(func(ctx app.Context, e app.Event) {
							p.newTagName = ctx.JSSrc().Get("value").String()
						}),
				),
			app.Div().
				Style("display", "flex").
				Style("flex-direction", "column").
				Style("gap", "4px").
				Body(
					app.Label().Style("font-size", "12px").Style("color", "var(--text-2)").Text("Type"),
					app.Select().
						Style("font-size", "14px").
						Style("padding", "5px 8px").
						Style("border", "1px solid var(--border-input)").
						Style("border-radius", "4px").
						Style("width", "100%").
						Style("box-sizing", "border-box").
						OnChange(func(ctx app.Context, e app.Event) {
							p.newTagType = ctx.JSSrc().Get("value").String()
						}).
						Body(typeOptions...),
				),
			app.Div().
				Style("display", "flex").
				Style("gap", "8px").
				Body(
					app.Button().
						Style("padding", "6px 18px").
						Style("font-size", "13px").
						Style("cursor", "pointer").
						Style("border", "1px solid var(--accent)").
						Style("border-radius", "4px").
						Style("background", "var(--accent)").
						Style("color", "var(--accent-text)").
						Text("Create").
						OnClick(func(ctx app.Context, e app.Event) {
							p.createTag(ctx)
						}),
					app.Button().
						Style("padding", "6px 18px").
						Style("font-size", "13px").
						Style("cursor", "pointer").
						Style("border", "1px solid var(--border-input)").
						Style("border-radius", "4px").
						Style("background", "var(--bg)").
						Text("Cancel").
						OnClick(func(ctx app.Context, e app.Event) {
							p.creatingTag = false
							p.newTagName = ""
							p.newTagType = ""
						}),
				),
		)
}
