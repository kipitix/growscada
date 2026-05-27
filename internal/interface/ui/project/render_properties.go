package project

import (
	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

// ── Right panel: Properties ────────────────────────────────────────────────────

func (p *Project) renderPropertiesPanel() app.UI {
	var content app.UI
	if p.selectedWidgetID == "" {
		content = app.Div().
			Style("font-size", "13px").
			Style("color", "#aaa").
			Style("margin-top", "12px").
			Text("Select a widget to view its properties.")
	} else {
		var found widgetItem
		for _, w := range p.widgets {
			if w.ID == p.selectedWidgetID {
				found = w
				break
			}
		}
		if found.ID == "" {
			content = app.Div().Style("font-size", "13px").Style("color", "#aaa").Text("Widget not found.")
		} else {
			content = p.renderWidgetProperties(found)
		}
	}

	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("width", "260px").
		Style("flex-shrink", "0").
		Style("min-height", "0").
		Style("overflow-y", "auto").
		Style("border-left", "1px solid #ddd").
		Style("padding", "12px 10px").
		Body(
			app.H3().
				Style("margin", "0 0 10px 0").
				Style("font-size", "13px").
				Style("font-weight", "600").
				Style("color", "#333").
				Style("text-transform", "uppercase").
				Style("letter-spacing", "0.5px").
				Text("Properties"),
			content,
		)
}

func (p *Project) renderWidgetProperties(w widgetItem) app.UI {
	wid := w.ID

	tagNameOf := func(id string) string {
		for _, t := range p.tags {
			if t.ID == id {
				return t.Name
			}
		}
		return id
	}

	// ── Section header helper ─────────────────────────────────────────────
	sectionHeader := func(label string) app.UI {
		return app.Div().
			Style("font-size", "10px").
			Style("font-weight", "700").
			Style("color", "#888").
			Style("letter-spacing", "0.6px").
			Style("text-transform", "uppercase").
			Style("margin-bottom", "6px").
			Style("margin-top", "2px").
			Text(label)
	}

	inputStyle := func(el app.HTMLInput) app.HTMLInput {
		return el.
			Style("font-size", "12px").
			Style("padding", "3px 5px").
			Style("border", "1px solid #ccc").
			Style("border-radius", "3px").
			Style("box-sizing", "border-box")
	}

	smallLabel := func(txt string) app.UI {
		return app.Span().
			Style("font-size", "11px").
			Style("color", "#888").
			Style("min-width", "12px").
			Style("text-align", "center").
			Text(txt)
	}

	applyBtn := func(onClick func(ctx app.Context, e app.Event)) app.UI {
		return app.Button().
			Style("font-size", "11px").
			Style("padding", "3px 8px").
			Style("cursor", "pointer").
			Style("border", "1px solid #ccc").
			Style("border-radius", "3px").
			Style("background", "#f0f0f0").
			Style("white-space", "nowrap").
			Text("Apply").
			OnClick(onClick)
	}

	row2 := func(items ...app.UI) app.UI {
		return app.Div().
			Style("display", "flex").
			Style("align-items", "center").
			Style("gap", "4px").
			Style("margin-bottom", "8px").
			Body(items...)
	}

	section := func(items ...app.UI) app.UI {
		return app.Div().
			Style("margin-bottom", "14px").
			Style("padding-bottom", "12px").
			Style("border-bottom", "1px solid #f0f0f0").
			Body(items...)
	}

	// ── Name ─────────────────────────────────────────────────────────────
	nameSection := section(
		sectionHeader("Name"),
		row2(
			inputStyle(app.Input().
				Type("text").
				Value(p.editingWidgetName).
				Style("flex", "1").
				Style("min-width", "0").
				OnInput(func(ctx app.Context, e app.Event) {
					p.editingWidgetName = ctx.JSSrc().Get("value").String()
				}).
				OnKeyDown(func(ctx app.Context, e app.Event) {
					if e.Get("key").String() == "Enter" {
						p.saveWidgetName(ctx)
					}
				})),
			applyBtn(func(ctx app.Context, e app.Event) { p.saveWidgetName(ctx) }),
		),
	)

	// ── Position ─────────────────────────────────────────────────────────
	posSection := section(
		sectionHeader("Position (px)"),
		row2(
			smallLabel("X"),
			inputStyle(app.Input().Type("number").Value(p.editingPosX).
				Style("width", "68px").Style("text-align", "right").
				OnInput(func(ctx app.Context, e app.Event) {
					p.editingPosX = ctx.JSSrc().Get("value").String()
				})),
			smallLabel("Y"),
			inputStyle(app.Input().Type("number").Value(p.editingPosY).
				Style("width", "68px").Style("text-align", "right").
				OnInput(func(ctx app.Context, e app.Event) {
					p.editingPosY = ctx.JSSrc().Get("value").String()
				})),
			smallLabel("Z"),
			inputStyle(app.Input().Type("number").Value(p.editingPosZ).
				Style("width", "44px").Style("text-align", "right").
				OnInput(func(ctx app.Context, e app.Event) {
					p.editingPosZ = ctx.JSSrc().Get("value").String()
				})),
		),
	)

	// ── Size ─────────────────────────────────────────────────────────────
	sizeSection := section(
		sectionHeader("Size (px)"),
		row2(
			smallLabel("W"),
			inputStyle(app.Input().Type("number").Value(p.editingWidth).
				Style("width", "68px").Style("text-align", "right").
				OnInput(func(ctx app.Context, e app.Event) {
					p.editingWidth = ctx.JSSrc().Get("value").String()
				})),
			smallLabel("H"),
			inputStyle(app.Input().Type("number").Value(p.editingHeight).
				Style("width", "68px").Style("text-align", "right").
				OnInput(func(ctx app.Context, e app.Event) {
					p.editingHeight = ctx.JSSrc().Get("value").String()
				})),
		),
	)

	// ── Origin ────────────────────────────────────────────────────────────
	originSection := section(
		sectionHeader("Origin (0–1)"),
		app.Div().
			Style("font-size", "10px").
			Style("color", "#aaa").
			Style("margin-bottom", "5px").
			Text("Anchor for rotation. (0,0)=top-left, (0.5,0.5)=center"),
		row2(
			smallLabel("X"),
			inputStyle(app.Input().Type("number").Value(p.editingOriginX).
				Style("width", "72px").Style("text-align", "right").
				OnInput(func(ctx app.Context, e app.Event) {
					p.editingOriginX = ctx.JSSrc().Get("value").String()
				})),
			smallLabel("Y"),
			inputStyle(app.Input().Type("number").Value(p.editingOriginY).
				Style("width", "72px").Style("text-align", "right").
				OnInput(func(ctx app.Context, e app.Event) {
					p.editingOriginY = ctx.JSSrc().Get("value").String()
				})),
		),
	)

	// ── Rotation ──────────────────────────────────────────────────────────
	rotSection := section(
		sectionHeader("Rotation (°)"),
		row2(
			smallLabel("°"),
			inputStyle(app.Input().Type("number").Value(p.editingRotation).
				Style("flex", "1").Style("text-align", "right").
				OnInput(func(ctx app.Context, e app.Event) {
					p.editingRotation = ctx.JSSrc().Get("value").String()
				})),
		),
	)

	// ── Apply geometry button ─────────────────────────────────────────────
	applyGeomBtn := app.Div().
		Style("margin-bottom", "14px").
		Body(
			app.Button().
				Style("width", "100%").
				Style("padding", "5px 0").
				Style("font-size", "12px").
				Style("cursor", "pointer").
				Style("border", "1px solid #0066cc").
				Style("border-radius", "3px").
				Style("background", "#e8f0ff").
				Style("color", "#0044aa").
				Text("Apply Geometry").
				OnClick(func(ctx app.Context, e app.Event) {
					p.saveWidgetGeometry(ctx)
				}),
		)

	// ── Tags ──────────────────────────────────────────────────────────────
	tagRows := make([]app.UI, len(w.TagIDs))
	for i, tid := range w.TagIDs {
		tid := tid
		tagRows[i] = app.Div().
			Style("display", "flex").
			Style("align-items", "center").
			Style("justify-content", "space-between").
			Style("padding", "3px 6px").
			Style("background", "#f0f4ff").
			Style("border", "1px solid #c8d8f8").
			Style("border-radius", "3px").
			Style("margin-bottom", "4px").
			Style("font-size", "12px").
			Body(
				app.Span().Text(tagNameOf(tid)),
				app.Span().
					Style("cursor", "pointer").
					Style("color", "#c00").
					Style("font-size", "14px").
					Style("padding", "0 2px").
					Text("×").
					OnClick(func(ctx app.Context, e app.Event) {
						p.removeTagFromWidget(ctx, tid)
					}),
			)
	}

	assigned := make(map[string]bool, len(w.TagIDs))
	for _, tid := range w.TagIDs {
		assigned[tid] = true
	}
	addOptions := make([]app.UI, 0, len(p.tags)+1)
	addOptions = append(addOptions, app.Option().Value("").Selected(p.addingTagID == "").Text("— select tag —"))
	for _, t := range p.tags {
		if !assigned[t.ID] {
			t := t
			addOptions = append(addOptions, app.Option().Value(t.ID).Selected(t.ID == p.addingTagID).Text(t.Name))
		}
	}

	tagSection := app.Div().
		Style("margin-bottom", "14px").
		Body(
			sectionHeader("Tags"),
			app.Div().Body(tagRows...),
			app.Div().
				Style("display", "flex").
				Style("gap", "4px").
				Style("margin-top", "4px").
				Body(
					app.Select().
						Style("flex", "1").
						Style("min-width", "0").
						Style("font-size", "12px").
						Style("padding", "3px 4px").
						Style("border", "1px solid #ccc").
						Style("border-radius", "3px").
						Body(addOptions...).
						OnChange(func(ctx app.Context, e app.Event) {
							p.addingTagID = ctx.JSSrc().Get("value").String()
						}),
					app.Button().
						Style("font-size", "12px").
						Style("padding", "3px 8px").
						Style("cursor", "pointer").
						Style("border", "1px solid #ccc").
						Style("border-radius", "3px").
						Style("background", "#f5f5f5").
						Text("Add").
						OnClick(func(ctx app.Context, e app.Event) {
							p.addTagToWidget(ctx, p.addingTagID)
						}),
				),
		)

	// ── Transform matrix display (read-only) ──────────────────────────────
	matrix := widgetMatrixCSS(w)
	matrixDisplay := app.Div().
		Style("margin-bottom", "14px").
		Body(
			sectionHeader("Transform Matrix"),
			app.Div().
				Style("font-size", "10px").
				Style("font-family", "monospace").
				Style("color", "#666").
				Style("background", "#f8f8f8").
				Style("border", "1px solid #e8e8e8").
				Style("border-radius", "3px").
				Style("padding", "4px 6px").
				Style("word-break", "break-all").
				Text(matrix),
		)

	// ── Delete ────────────────────────────────────────────────────────────
	deleteBtn := app.Div().
		Style("padding-top", "4px").
		Body(
			app.Button().
				Style("width", "100%").
				Style("padding", "5px 0").
				Style("font-size", "12px").
				Style("cursor", "pointer").
				Style("border", "1px solid #e0b0b0").
				Style("border-radius", "3px").
				Style("background", "#fff5f5").
				Style("color", "#c00").
				Text("Delete Widget").
				OnClick(func(ctx app.Context, e app.Event) {
					p.deleteWidget(ctx, wid)
				}),
		)

	return app.Div().Body(
		nameSection,
		posSection,
		sizeSection,
		originSection,
		rotSection,
		applyGeomBtn,
		tagSection,
		matrixDisplay,
		deleteBtn,
	)
}
