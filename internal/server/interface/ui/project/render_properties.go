package project

import (
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Right panel: Properties ────────────────────────────────────────────────────

func (p *Project) renderPropertiesPanel() app.UI {
	var content app.UI
	if p.selectedWidgetID == uuid.Nil {
		if p.selectedSceneID == uuid.Nil {
			content = app.Div().
				Style("font-size", "13px").
				Style("color", "var(--text-muted)").
				Style("margin-top", "12px").
				Text("No scene selected.")
		} else {
			content = p.renderSceneProperties()
		}
	} else {
		if found, ok := p.findWidget(p.selectedWidgetID); ok {
			content = p.renderWidgetProperties(found)
		} else {
			content = app.Div().Style("font-size", "13px").Style("color", "var(--text-muted)").Text("Widget not found.")
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

func (p *Project) renderSceneProperties() app.UI {
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
			inputStyle(
				app.Input().
					Type("text").
					Value(p.sceneEdit.name).
					Style("flex", "1").
					Style("min-width", "0").
					OnInput(func(ctx app.Context, e app.Event) {
						p.sceneEdit.name = ctx.JSSrc().Get("value").String()
					}).
					OnBlur(func(ctx app.Context, e app.Event) {
						p.saveSceneProperties(ctx)
					}).
					OnKeyDown(func(ctx app.Context, e app.Event) {
						if e.Get("key").String() == "Enter" {
							p.saveSceneProperties(ctx)
						}
					}),
			),
		),
	)

	// ── Size ─────────────────────────────────────────────────────────────
	sizeSection := section(
		sectionHeader("Size (px)"),
		row2(
			smallLabel("W"),
			inputStyle(
				app.Input().Type("number").Value(p.sceneEdit.width).
					Style("width", "72px").Style("text-align", "right").
					OnInput(func(ctx app.Context, e app.Event) {
						p.sceneEdit.width = ctx.JSSrc().Get("value").String()
					}).
					OnBlur(func(ctx app.Context, e app.Event) {
						p.saveSceneProperties(ctx)
					}).
					OnKeyDown(func(ctx app.Context, e app.Event) {
						if e.Get("key").String() == "Enter" {
							p.saveSceneProperties(ctx)
						}
					}),
			),
			smallLabel("H"),
			inputStyle(
				app.Input().Type("number").Value(p.sceneEdit.height).
					Style("width", "72px").Style("text-align", "right").
					OnInput(func(ctx app.Context, e app.Event) {
						p.sceneEdit.height = ctx.JSSrc().Get("value").String()
					}).
					OnBlur(func(ctx app.Context, e app.Event) {
						p.saveSceneProperties(ctx)
					}).
					OnKeyDown(func(ctx app.Context, e app.Event) {
						if e.Get("key").String() == "Enter" {
							p.saveSceneProperties(ctx)
						}
					}),
			),
		),
	)

	// ── Background HTML ───────────────────────────────────────────────────
	bgSection := app.Div().
		Style("margin-bottom", "14px").
		Body(
			sectionHeader("Background HTML"),
			app.Textarea().
				Style("width", "100%").
				Style("font-size", "11px").
				Style("font-family", "monospace").
				Style("padding", "4px 5px").
				Style("border", "1px solid var(--border-input)").
				Style("border-radius", "3px").
				Style("box-sizing", "border-box").
				Style("resize", "vertical").
				Style("min-height", "80px").
				Style("background", "var(--bg)").
				Style("color", "var(--text)").
				Text(p.sceneEdit.background).
				OnInput(func(ctx app.Context, e app.Event) {
					p.sceneEdit.background = ctx.JSSrc().Get("value").String()
				}).
				OnBlur(func(ctx app.Context, e app.Event) {
					p.saveSceneProperties(ctx)
				}),
		)

	return app.Div().Body(nameSection, sizeSection, bgSection)
}

func (p *Project) renderWidgetProperties(w apiv0.WidgetResponse) app.UI {
	wid := w.ID

	// Defaults for reset buttons; InputPorts resolved in the same pass.
	defaultName := "Widget"
	defaultW, defaultH := 120, 60
	var wtInputPorts []apiv0.InputPort
	capturedWT, ok := p.widgetTypeByID(w.TypeID)
	if ok {
		defaultName = capturedWT.Name
		if capturedWT.DefaultWidth > 0 {
			defaultW = capturedWT.DefaultWidth
		}
		if capturedWT.DefaultHeight > 0 {
			defaultH = capturedWT.DefaultHeight
		}
		wtInputPorts = capturedWT.InputPorts
	}
	// The panel is reused for another widget: its handlers, scoped by the
	// widget and the defaults they capture, are replaced along with it (go-app
	// keeps a handler of the same func literal otherwise).
	scope := fmt.Sprintf("%s/%s/%d/%d", wid, defaultName, defaultW, defaultH)

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
			}, app.EventScope(scope))

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
					Value(p.widgetEdit.name).
					OnInput(func(ctx app.Context, e app.Event) {
						p.widgetEdit.name = ctx.JSSrc().Get("value").String()
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
					p.widgetEdit.name = defaultName
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
				app.Input().Type("number").Value(p.widgetEdit.posX).
					Style("width", "68px").Style("text-align", "right").
					OnInput(func(ctx app.Context, e app.Event) {
						p.widgetEdit.posX = ctx.JSSrc().Get("value").String()
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
					p.widgetEdit.posX = "0.0"
					p.saveWidgetGeometry(ctx)
				},
			),
			smallLabel("Y"),
			withReset(
				app.Input().Type("number").Value(p.widgetEdit.posY).
					Style("width", "68px").Style("text-align", "right").
					OnInput(func(ctx app.Context, e app.Event) {
						p.widgetEdit.posY = ctx.JSSrc().Get("value").String()
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
					p.widgetEdit.posY = "0.0"
					p.saveWidgetGeometry(ctx)
				},
			),
			smallLabel("Z"),
			withReset(
				app.Input().Type("number").Value(p.widgetEdit.posZ).
					Style("width", "44px").Style("text-align", "right").
					OnInput(func(ctx app.Context, e app.Event) {
						p.widgetEdit.posZ = ctx.JSSrc().Get("value").String()
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
					p.widgetEdit.posZ = "0"
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
				app.Input().Type("number").Value(p.widgetEdit.width).
					Style("width", "68px").Style("text-align", "right").
					OnInput(func(ctx app.Context, e app.Event) {
						p.widgetEdit.width = ctx.JSSrc().Get("value").String()
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
					p.widgetEdit.width = strconv.Itoa(defaultW)
					p.saveWidgetGeometry(ctx)
				},
			),
			smallLabel("H"),
			withReset(
				app.Input().Type("number").Value(p.widgetEdit.height).
					Style("width", "68px").Style("text-align", "right").
					OnInput(func(ctx app.Context, e app.Event) {
						p.widgetEdit.height = ctx.JSSrc().Get("value").String()
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
					p.widgetEdit.height = strconv.Itoa(defaultH)
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
				app.Input().Type("number").Value(p.widgetEdit.originX).
					Style("width", "72px").Style("text-align", "right").
					OnInput(func(ctx app.Context, e app.Event) {
						p.widgetEdit.originX = ctx.JSSrc().Get("value").String()
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
					p.widgetEdit.originX = "0.500"
					p.saveWidgetGeometry(ctx)
				},
			),
			smallLabel("Y"),
			withReset(
				app.Input().Type("number").Value(p.widgetEdit.originY).
					Style("width", "72px").Style("text-align", "right").
					OnInput(func(ctx app.Context, e app.Event) {
						p.widgetEdit.originY = ctx.JSSrc().Get("value").String()
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
					p.widgetEdit.originY = "0.500"
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
				app.Input().Type("number").Value(p.widgetEdit.rotation).
					Style("text-align", "right").
					OnInput(func(ctx app.Context, e app.Event) {
						p.widgetEdit.rotation = ctx.JSSrc().Get("value").String()
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
					p.widgetEdit.rotation = "0.0"
					p.saveWidgetGeometry(ctx)
				},
			),
		),
	)

	// ── Port Bindings ─────────────────────────────────────────────────────
	inputPorts := wtInputPorts

	// Build a map from port name → bound tag ID for quick lookup.
	boundTagID := make(map[string]uuid.UUID, len(w.PortBindings))
	for _, b := range w.PortBindings {
		boundTagID[b.PortName] = b.TagID
	}

	portBindingRows := make([]app.UI, 0, len(inputPorts))
	for _, port := range inputPorts {
		port := port
		currentTagID := boundTagID[port.Name]

		typeLabel := uiutil.TypeHintLabel(port.TypeHint)

		// Build tag options filtered by type hint
		tagOptions := make([]app.UI, 0, len(p.tags.List())+1)
		tagOptions = append(tagOptions, app.Option().Value("").Selected(currentTagID == uuid.Nil).Text("— unbound —"))
		offered := currentTagID == uuid.Nil
		for _, t := range p.tags.List() {
			t := t
			if port.TypeHint != "" && t.Type != port.TypeHint {
				continue
			}
			offered = offered || t.ID == currentTagID
			tagOptions = append(tagOptions, app.Option().Value(t.ID.String()).Selected(t.ID == currentTagID).Text(t.Name+" ("+t.Type+")"))
		}
		// A binding broken since it was made (its Tag deleted or of a type
		// the hint no longer accepts) stays until a policy removes it; it is
		// shown selected, so that choosing "— unbound —" unbinds it.
		if !offered {
			text := "⚠ missing tag"
			if t, ok := uiutil.FindByID(p.tags.List(), currentTagID, uiutil.TagID); ok {
				text = "⚠ " + t.Name + " (" + t.Type + ")"
			}
			tagOptions = append(tagOptions, app.Option().Value(currentTagID.String()).Selected(true).Text(text))
		}

		var descEl app.UI
		if port.Description != "" {
			descEl = app.Div().
				Style("font-size", "10px").
				Style("color", "var(--text-muted)").
				Text(port.Description)
		} else {
			descEl = app.Text("")
		}

		row := app.Div().
			Style("margin-bottom", "8px").
			Body(
				app.Div().
					Style("display", "flex").
					Style("align-items", "center").
					Style("gap", "4px").
					Style("margin-bottom", "2px").
					Body(
						app.Span().Style("font-size", "12px").Style("font-weight", "600").Text(port.Name),
						app.Span().
							Style("font-size", "10px").
							Style("color", "var(--text-muted)").
							Style("border", "1px solid var(--border)").
							Style("border-radius", "2px").
							Style("padding", "0 3px").
							Text(typeLabel),
					),
				descEl,
				app.Div().
					Style("display", "flex").
					Style("align-items", "center").
					Style("gap", "4px").
					Style("margin-top", "3px").
					Body(
						app.Select().
							Style("flex", "1").
							Style("font-size", "12px").
							Style("padding", "2px 4px").
							Style("border", "1px solid var(--border-input)").
							Style("border-radius", "3px").
							Body(tagOptions...).
							OnChange(func(ctx app.Context, e app.Event) {
								tagID := uiutil.ParseID(ctx.JSSrc().Get("value").String())
								if tagID == uuid.Nil {
									p.unbindPort(ctx, port.Name)
								} else {
									p.bindPort(ctx, port.Name, tagID)
								}
							}, app.EventScope(port.Name)),
					),
			)
		portBindingRows = append(portBindingRows, row)
	}

	var portBindingsBody app.UI
	if len(inputPorts) == 0 {
		portBindingsBody = app.Div().
			Style("font-size", "12px").
			Style("color", "var(--text-muted)").
			Text("This widget type has no input ports.")
	} else {
		portBindingsBody = app.Div().Body(portBindingRows...)
	}

	tagSection := app.Div().
		Style("margin-bottom", "14px").
		Style("padding-bottom", "12px").
		Style("border-bottom", "1px solid var(--border-subtle)").
		Body(
			sectionHeader("Port Bindings"),
			portBindingsBody,
		)

	// ── Simulate Inputs ───────────────────────────────────────────────────
	simVals := p.simInputs[wid]

	simRows := make([]app.UI, 0, len(inputPorts))
	for _, port := range inputPorts {
		port := port
		val := ""
		if simVals != nil {
			val = simVals[port.Name]
		}
		typeLabel := uiutil.TypeHintLabel(port.TypeHint)

		row := app.Div().
			Style("margin-bottom", "6px").
			Body(
				app.Div().
					Style("display", "flex").
					Style("align-items", "center").
					Style("gap", "4px").
					Style("margin-bottom", "2px").
					Body(
						app.Span().Style("font-size", "12px").Style("font-weight", "600").Text(port.Name),
						app.Span().
							Style("font-size", "10px").
							Style("color", "var(--text-muted)").
							Style("border", "1px solid var(--border)").
							Style("border-radius", "2px").
							Style("padding", "0 3px").
							Text(typeLabel),
					),
				&simInputField{
					FieldID:  "sim-" + wid.String() + "-" + port.Name,
					Val:      val,
					PortName: port.Name,
					OnChange: func(v string) {
						p.setSimInput(wid, port.Name, v)
						// Directly push the updated srcdoc so the canvas preview
						// reflects the new value without waiting for a parent re-render.
						srcdoc := uiutil.BuildSrcdoc(capturedWT.HtmlTemplate, capturedWT.Script, uiutil.PreviewInputs(p.simInputs[wid], capturedWT.InputPorts), capturedWT.InputPorts, uiutil.IframeBgColor())
						elem := app.Window().Get("document").Call("getElementById", widgetPreviewDOMID(wid))
						if !elem.IsNull() && !elem.IsUndefined() {
							elem.Set("srcdoc", srcdoc)
						}
					},
				},
			)
		simRows = append(simRows, row)
	}

	var simSection app.UI
	if len(inputPorts) > 0 {
		simSection = section(
			sectionHeader("Simulate Inputs"),
			app.Div().
				Style("font-size", "10px").
				Style("color", "var(--text-muted)").
				Style("margin-bottom", "6px").
				Text("Test values for the canvas preview — not saved."),
			app.Div().Body(simRows...),
		)
	} else {
		simSection = app.Text("")
	}

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
				}, app.EventScope(scope)),
		)

	return app.Div().Body(
		nameSection,
		posSection,
		sizeSection,
		originSection,
		rotSection,
		tagSection,
		simSection,
		matrixDisplay,
		deleteBtn,
	)
}
