package project

import (
	"fmt"
	"strconv"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

// ── Right panel: Properties ────────────────────────────────────────────────────

func (p *Project) renderPropertiesPanel() app.UI {
	var content app.UI
	if p.selectedWidgetID == "" {
		content = app.Div().
			Style("font-size", "13px").
			Style("color", "var(--text-muted)").
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
			content = app.Div().Style("font-size", "13px").Style("color", "var(--text-muted)").Text("Widget not found.")
		} else {
			content = p.renderWidgetProperties(found)
		}
	}

	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("width", fmt.Sprintf("%dpx", p.propertiesWidth)).
		Style("flex-shrink", "0").
		Style("min-height", "0").
		Style("overflow-y", "auto").
		Style("padding", "12px 10px").
		Body(
			app.H3().
				Style("margin", "0 0 10px 0").
				Style("font-size", "13px").
				Style("font-weight", "600").
				Style("color", "var(--text)").
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

	// Defaults for reset buttons
	defaultName := "Widget"
	defaultW, defaultH := 120, 60
	for _, wt := range p.widgetTypes {
		if wt.ID == w.TypeID {
			defaultName = wt.Name
			if wt.DefaultWidth > 0 {
				defaultW = wt.DefaultWidth
			}
			if wt.DefaultHeight > 0 {
				defaultH = wt.DefaultHeight
			}
			break
		}
	}

	// ── Section header helper ─────────────────────────────────────────────
	sectionHeader := func(label string) app.UI {
		return app.Div().
			Style("font-size", "10px").
			Style("font-weight", "700").
			Style("color", "var(--text-3)").
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
			Style("border", "1px solid var(--border-input)").
			Style("border-radius", "3px").
			Style("box-sizing", "border-box")
	}

	smallLabel := func(txt string) app.UI {
		return app.Span().
			Style("font-size", "11px").
			Style("color", "var(--text-3)").
			Style("min-width", "12px").
			Style("text-align", "center").
			Text(txt)
	}

	// withReset wraps an input (applies inputStyle internally) with a small
	// inline reset button. expandFlex=true makes the wrapper grow to fill
	// available width (for name / rotation fields).
	withReset := func(el app.HTMLInput, expandFlex bool, onReset func(ctx app.Context)) app.HTMLDiv {
		styled := inputStyle(el).Style("padding-right", "18px")
		if expandFlex {
			styled = styled.Style("flex", "1").Style("min-width", "0")
		}

		resetBtn := app.Button().
			Style("position", "absolute").
			Style("right", "3px").
			Style("top", "50%").
			Style("transform", "translateY(-50%)").
			Style("width", "13px").
			Style("height", "13px").
			Style("padding", "0").
			Style("border", "none").
			Style("background", "transparent").
			Style("cursor", "pointer").
			Style("color", "var(--text-muted)").
			Style("font-size", "11px").
			Style("line-height", "1").
			Style("display", "flex").
			Style("align-items", "center").
			Style("justify-content", "center").
			Title("Reset to default").
			Text("↺").
			// PreventDefault on mousedown keeps focus on the input so OnBlur
			// doesn't fire before OnClick, avoiding a stale save racing the reset.
			OnMouseDown(func(ctx app.Context, e app.Event) {
				e.PreventDefault()
			}).
			OnClick(func(ctx app.Context, e app.Event) {
				onReset(ctx)
			})

		d := app.Div().
			Style("position", "relative").
			Style("display", "flex").
			Style("align-items", "center").
			Body(styled, resetBtn)
		if expandFlex {
			d = d.Style("flex", "1").Style("min-width", "0")
		}
		return d
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
			Style("border-bottom", "1px solid var(--border-subtle)").
			Body(items...)
	}

	// ── Name ─────────────────────────────────────────────────────────────
	nameSection := section(
		sectionHeader("Name"),
		row2(
			withReset(
				app.Input().
					Type("text").
					Value(p.editingWidgetName).
					OnInput(func(ctx app.Context, e app.Event) {
						p.editingWidgetName = ctx.JSSrc().Get("value").String()
					}).
					OnBlur(func(ctx app.Context, e app.Event) {
						p.saveWidgetName(ctx)
					}).
					OnKeyDown(func(ctx app.Context, e app.Event) {
						if e.Get("key").String() == "Enter" {
							p.saveWidgetName(ctx)
						}
					}),
				true,
				func(ctx app.Context) {
					p.editingWidgetName = defaultName
					p.saveWidgetName(ctx)
				},
			),
		),
	)

	// ── Position ─────────────────────────────────────────────────────────
	posSection := section(
		sectionHeader("Position (px)"),
		row2(
			smallLabel("X"),
			withReset(
				app.Input().Type("number").Value(p.editingPosX).
					Style("width", "68px").Style("text-align", "right").
					OnInput(func(ctx app.Context, e app.Event) {
						p.editingPosX = ctx.JSSrc().Get("value").String()
					}).
					OnBlur(func(ctx app.Context, e app.Event) {
						p.saveWidgetGeometry(ctx)
					}).
					OnKeyDown(func(ctx app.Context, e app.Event) {
						if e.Get("key").String() == "Enter" {
							p.saveWidgetGeometry(ctx)
						}
					}),
				false,
				func(ctx app.Context) {
					p.editingPosX = "0.0"
					p.saveWidgetGeometry(ctx)
				},
			),
			smallLabel("Y"),
			withReset(
				app.Input().Type("number").Value(p.editingPosY).
					Style("width", "68px").Style("text-align", "right").
					OnInput(func(ctx app.Context, e app.Event) {
						p.editingPosY = ctx.JSSrc().Get("value").String()
					}).
					OnBlur(func(ctx app.Context, e app.Event) {
						p.saveWidgetGeometry(ctx)
					}).
					OnKeyDown(func(ctx app.Context, e app.Event) {
						if e.Get("key").String() == "Enter" {
							p.saveWidgetGeometry(ctx)
						}
					}),
				false,
				func(ctx app.Context) {
					p.editingPosY = "0.0"
					p.saveWidgetGeometry(ctx)
				},
			),
			smallLabel("Z"),
			withReset(
				app.Input().Type("number").Value(p.editingPosZ).
					Style("width", "44px").Style("text-align", "right").
					OnInput(func(ctx app.Context, e app.Event) {
						p.editingPosZ = ctx.JSSrc().Get("value").String()
					}).
					OnBlur(func(ctx app.Context, e app.Event) {
						p.saveWidgetGeometry(ctx)
					}).
					OnKeyDown(func(ctx app.Context, e app.Event) {
						if e.Get("key").String() == "Enter" {
							p.saveWidgetGeometry(ctx)
						}
					}),
				false,
				func(ctx app.Context) {
					p.editingPosZ = "0"
					p.saveWidgetGeometry(ctx)
				},
			),
		),
	)

	// ── Size ─────────────────────────────────────────────────────────────
	sizeSection := section(
		sectionHeader("Size (px)"),
		row2(
			smallLabel("W"),
			withReset(
				app.Input().Type("number").Value(p.editingWidth).
					Style("width", "68px").Style("text-align", "right").
					OnInput(func(ctx app.Context, e app.Event) {
						p.editingWidth = ctx.JSSrc().Get("value").String()
					}).
					OnBlur(func(ctx app.Context, e app.Event) {
						p.saveWidgetGeometry(ctx)
					}).
					OnKeyDown(func(ctx app.Context, e app.Event) {
						if e.Get("key").String() == "Enter" {
							p.saveWidgetGeometry(ctx)
						}
					}),
				false,
				func(ctx app.Context) {
					p.editingWidth = strconv.Itoa(defaultW)
					p.saveWidgetGeometry(ctx)
				},
			),
			smallLabel("H"),
			withReset(
				app.Input().Type("number").Value(p.editingHeight).
					Style("width", "68px").Style("text-align", "right").
					OnInput(func(ctx app.Context, e app.Event) {
						p.editingHeight = ctx.JSSrc().Get("value").String()
					}).
					OnBlur(func(ctx app.Context, e app.Event) {
						p.saveWidgetGeometry(ctx)
					}).
					OnKeyDown(func(ctx app.Context, e app.Event) {
						if e.Get("key").String() == "Enter" {
							p.saveWidgetGeometry(ctx)
						}
					}),
				false,
				func(ctx app.Context) {
					p.editingHeight = strconv.Itoa(defaultH)
					p.saveWidgetGeometry(ctx)
				},
			),
		),
	)

	// ── Origin ────────────────────────────────────────────────────────────
	originSection := section(
		sectionHeader("Origin (0–1)"),
		app.Div().
			Style("font-size", "10px").
			Style("color", "var(--text-muted)").
			Style("margin-bottom", "5px").
			Text("Anchor for rotation. (0,0)=top-left, (0.5,0.5)=center"),
		row2(
			smallLabel("X"),
			withReset(
				app.Input().Type("number").Value(p.editingOriginX).
					Style("width", "72px").Style("text-align", "right").
					OnInput(func(ctx app.Context, e app.Event) {
						p.editingOriginX = ctx.JSSrc().Get("value").String()
					}).
					OnBlur(func(ctx app.Context, e app.Event) {
						p.saveWidgetGeometry(ctx)
					}).
					OnKeyDown(func(ctx app.Context, e app.Event) {
						if e.Get("key").String() == "Enter" {
							p.saveWidgetGeometry(ctx)
						}
					}),
				false,
				func(ctx app.Context) {
					p.editingOriginX = "0.500"
					p.saveWidgetGeometry(ctx)
				},
			),
			smallLabel("Y"),
			withReset(
				app.Input().Type("number").Value(p.editingOriginY).
					Style("width", "72px").Style("text-align", "right").
					OnInput(func(ctx app.Context, e app.Event) {
						p.editingOriginY = ctx.JSSrc().Get("value").String()
					}).
					OnBlur(func(ctx app.Context, e app.Event) {
						p.saveWidgetGeometry(ctx)
					}).
					OnKeyDown(func(ctx app.Context, e app.Event) {
						if e.Get("key").String() == "Enter" {
							p.saveWidgetGeometry(ctx)
						}
					}),
				false,
				func(ctx app.Context) {
					p.editingOriginY = "0.500"
					p.saveWidgetGeometry(ctx)
				},
			),
		),
	)

	// ── Rotation ──────────────────────────────────────────────────────────
	rotSection := section(
		sectionHeader("Rotation (°)"),
		row2(
			smallLabel("°"),
			withReset(
				app.Input().Type("number").Value(p.editingRotation).
					Style("text-align", "right").
					OnInput(func(ctx app.Context, e app.Event) {
						p.editingRotation = ctx.JSSrc().Get("value").String()
					}).
					OnBlur(func(ctx app.Context, e app.Event) {
						p.saveWidgetGeometry(ctx)
					}).
					OnKeyDown(func(ctx app.Context, e app.Event) {
						if e.Get("key").String() == "Enter" {
							p.saveWidgetGeometry(ctx)
						}
					}),
				true,
				func(ctx app.Context) {
					p.editingRotation = "0.0"
					p.saveWidgetGeometry(ctx)
				},
			),
		),
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
			Style("background", "var(--accent-bg)").
			Style("border", "1px solid var(--accent-border)").
			Style("border-radius", "3px").
			Style("margin-bottom", "4px").
			Style("font-size", "12px").
			Body(
				app.Span().Text(tagNameOf(tid)),
				app.Span().
					Style("cursor", "pointer").
					Style("color", "var(--error)").
					Style("font-size", "14px").
					Style("padding", "0 2px").
					Text("×").
					OnClick(func(ctx app.Context, e app.Event) {
						if !app.Window().Call("confirm", "Are you sure you want to remove this tag?").Bool() {
							return
						}
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
						Style("border", "1px solid var(--border-input)").
						Style("border-radius", "3px").
						Body(addOptions...).
						OnChange(func(ctx app.Context, e app.Event) {
							p.addingTagID = ctx.JSSrc().Get("value").String()
						}),
					app.Button().
						Style("font-size", "12px").
						Style("padding", "3px 8px").
						Style("cursor", "pointer").
						Style("border", "1px solid var(--border-input)").
						Style("border-radius", "3px").
						Style("background", "var(--bg-hover)").
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
				Style("color", "var(--text-2)").
				Style("background", "var(--bg-elevated)").
				Style("border", "1px solid var(--border)").
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
				Style("border", "1px solid var(--error-border)").
				Style("border-radius", "3px").
				Style("background", "var(--error-bg)").
				Style("color", "var(--error)").
				Text("Delete Widget").
				OnClick(func(ctx app.Context, e app.Event) {
					if !app.Window().Call("confirm", "Are you sure you want to delete this widget?").Bool() {
						return
					}
					p.deleteWidget(ctx, wid)
				}),
		)

	return app.Div().Body(
		nameSection,
		posSection,
		sizeSection,
		originSection,
		rotSection,
		tagSection,
		matrixDisplay,
		deleteBtn,
	)
}
