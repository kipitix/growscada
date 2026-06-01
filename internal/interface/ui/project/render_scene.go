package project

import (
	"fmt"
	"math"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

// ── Center panel: Scene ────────────────────────────────────────────────────────

func (p *Project) renderScenePanel() app.UI {
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex", "1").
		Style("min-width", "0").
		Style("min-height", "0").
		Body(
			p.renderSceneTabs(),
			p.renderSceneCanvas(),
		)
}

func (p *Project) renderSceneTabs() app.UI {
	tabs := make([]app.UI, 0, len(p.scenes)+1)

	for _, sc := range p.scenes {
		sc := sc
		active := p.selectedSceneID == sc.ID

		var bg, borderBottom, color, fontWeight string
		if active {
			bg = "var(--bg)"
			borderBottom = "2px solid var(--accent)"
			color = "var(--accent)"
			fontWeight = "600"
		} else {
			bg = "var(--bg-hover)"
			borderBottom = "2px solid transparent"
			color = "var(--text-2)"
			fontWeight = "normal"
		}

		var tabInner app.UI
		if p.editingSceneID == sc.ID {
			tabInner = app.Input().
				Type("text").
				Value(p.editingSceneName).
				AutoFocus(true).
				Style("font-size", "13px").
				Style("padding", "1px 4px").
				Style("border", "1px solid var(--accent)").
				Style("border-radius", "2px").
				Style("width", "90px").
				Style("background", "var(--input-bg)").
				Style("color", "var(--text)").
				OnInput(func(ctx app.Context, e app.Event) {
					p.editingSceneName = ctx.JSSrc().Get("value").String()
				}).
				OnBlur(func(ctx app.Context, e app.Event) {
					p.commitSceneEdit(ctx)
				}).
				OnKeyDown(func(ctx app.Context, e app.Event) {
					switch e.Get("key").String() {
					case "Enter":
						p.commitSceneEdit(ctx)
					case "Escape":
						p.editingSceneID = ""
						p.editingSceneName = ""
					}
				})
		} else {
			tabInner = app.Div().
				Style("display", "flex").
				Style("align-items", "center").
				Style("gap", "6px").
				Body(
					app.Span().
						Style("font-size", "13px").
						Style("cursor", "pointer").
						Text(sc.Name).
						OnClick(func(ctx app.Context, e app.Event) {
							p.selectedSceneID = sc.ID
							ctx.LocalStorage().Set("project:sceneID", sc.ID)
							p.clearWidgetSelection(ctx)
							p.loadWidgets(ctx)
						}).
						OnDblClick(func(ctx app.Context, e app.Event) {
							p.startEditingScene(sc.ID, sc.Name)
						}),
					app.Span().
						Style("cursor", "pointer").
						Style("font-size", "14px").
						Style("line-height", "1").
						Style("color", "var(--text-muted)").
						Style("padding", "0 1px").
						Text("×").
						OnClick(func(ctx app.Context, e app.Event) {
							e.Call("stopPropagation")
							if !app.Window().Call("confirm", "Are you sure you want to delete this scene?").Bool() {
								return
							}
							p.deleteScene(ctx, sc.ID)
						}),
				)
		}

		tabs = append(tabs, app.Div().
			Style("display", "flex").
			Style("align-items", "center").
			Style("padding", "6px 14px").
			Style("background", bg).
			Style("border-right", "1px solid var(--border)").
			Style("border-bottom", borderBottom).
			Style("color", color).
			Style("font-weight", fontWeight).
			Style("white-space", "nowrap").
			Body(tabInner),
		)
	}

	tabs = append(tabs, app.Div().
		Style("display", "flex").
		Style("align-items", "center").
		Style("padding", "6px 10px").
		Style("cursor", "pointer").
		Style("font-size", "18px").
		Style("color", "var(--accent)").
		Style("line-height", "1").
		Text("+").
		OnClick(func(ctx app.Context, e app.Event) { p.createScene(ctx) }),
	)

	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "row").
		Style("flex-shrink", "0").
		Style("border-bottom", "1px solid var(--border)").
		Style("background", "var(--bg-elevated)").
		Body(tabs...)
}

func (p *Project) renderSceneCanvas() app.UI {
	if p.selectedSceneID == "" {
		return app.Div().
			Style("flex", "1").
			Style("display", "flex").
			Style("align-items", "center").
			Style("justify-content", "center").
			Style("color", "var(--text-muted)").
			Style("font-size", "14px").
			Body(app.Text("Create a scene to get started"))
	}

	sceneWidth, sceneHeight := 1920, 1080
	for _, sc := range p.scenes {
		if sc.ID == p.selectedSceneID {
			if sc.Width > 0 {
				sceneWidth = sc.Width
			}
			if sc.Height > 0 {
				sceneHeight = sc.Height
			}
			break
		}
	}

	widgetEls := make([]app.UI, 0, len(p.widgets))
	for _, w := range p.widgets {
		widgetEls = append(widgetEls, p.renderWidget(w))
	}

	// Determine overall canvas cursor from active drag mode.
	canvasCursor := "default"
	switch {
	case p.draggingWidgetID != "":
		canvasCursor = "grabbing"
	case p.draggingOriginID != "":
		canvasCursor = "crosshair"
	case p.rotatingWidgetID != "":
		canvasCursor = "alias"
	case p.resizingWidgetID != "":
		canvasCursor = "se-resize"
	}

	canvas := app.Div().
		Style("position", "relative").
		Style("width", fmt.Sprintf("%dpx", sceneWidth)).
		Style("height", fmt.Sprintf("%dpx", sceneHeight)).
		Style("background-color", "var(--surface)").
		Style("background-image", "radial-gradient(circle, var(--dot-color) 1px, transparent 1px)").
		Style("background-size", "24px 24px").
		Style("flex-shrink", "0").
		Style("cursor", canvasCursor).
		// OnMouseMove / OnMouseUp are intentionally on the root Render() div
		// so drags are not clipped by the canvas boundary (especially on
		// smaller scenes like 1280×720 where the canvas is smaller than the
		// scroll viewport).
		OnDragOver(func(ctx app.Context, e app.Event) {
			e.PreventDefault()
			e.Get("dataTransfer").Set("dropEffect", "copy")
		}).
		OnDrop(func(ctx app.Context, e app.Event) {
			e.PreventDefault()
			typeID := p.draggedTypeID
			if typeID == "" {
				typeID = e.Get("dataTransfer").Call("getData", "text/plain").String()
			}
			p.draggedTypeID = ""
			if typeID == "" {
				return
			}
			rect := e.Get("currentTarget").Call("getBoundingClientRect")
			x := e.Get("clientX").Float() - rect.Get("left").Float()
			y := e.Get("clientY").Float() - rect.Get("top").Float()
			p.createWidget(ctx, typeID, x, y)
		}).
		OnClick(func(ctx app.Context, e app.Event) {
			if p.dragJustEnded {
				p.dragJustEnded = false
				return
			}
			p.clearWidgetSelection(ctx)
		}).
		Body(widgetEls...)

	return app.Div().
		Style("flex", "1").
		Style("min-height", "0").
		Style("overflow", "auto").
		Style("background", "var(--bg-inset)").
		Style("padding", "24px").
		Body(canvas)
}

// renderWidget renders a single widget element on the canvas using a CSS
// transformation matrix. When selected, overlay handles are shown:
//   - Orange circle: origin anchor (drag to move anchor within widget)
//   - Blue circle + line: rotation handle (drag to rotate)
//   - Blue square corner: SE resize handle (drag to resize)
func (p *Project) renderWidget(w widgetItem) app.UI {
	wid := w.ID
	isSelected := p.selectedWidgetID == wid

	// Local pixel position of the origin anchor within the widget.
	ox := w.Origin.X * float64(w.Size.Width)
	oy := w.Origin.Y * float64(w.Size.Height)

	// Visual style for selection state.
	border := "1.5px solid var(--border-input)"
	bg := "var(--widget-bg)"
	if isSelected {
		border = "2px solid var(--accent)"
		bg = "var(--widget-sel-bg)"
	}

	// ── Widget content (name label) ─────────────────────────────────────────
	content := app.Div().
		Style("position", "absolute").
		Style("inset", "0").
		Style("display", "flex").
		Style("align-items", "center").
		Style("justify-content", "center").
		Style("overflow", "hidden").
		Style("padding", "0 8px").
		Style("box-sizing", "border-box").
		Style("background", bg).
		Style("border", border).
		Style("border-radius", "4px").
		Style("cursor", "move").
		Style("user-select", "none").
		Body(
			app.Span().
				Style("font-size", "12px").
				Style("white-space", "nowrap").
				Style("text-overflow", "ellipsis").
				Style("overflow", "hidden").
				Style("color", "var(--text)").
				Text(w.Name),
		).
		OnMouseDown(func(ctx app.Context, e app.Event) {
			e.Call("stopPropagation")
			e.PreventDefault()
			p.selectWidget(ctx, wid)
			p.draggingWidgetID = wid
			p.dragStartCliX = e.Get("clientX").Float()
			p.dragStartCliY = e.Get("clientY").Float()
			// Read position from live p.widgets — the closure-captured w.Position
			// may be stale if go-app didn't re-attach this handler after a drag.
			if cur, ok := p.widgetByID(wid); ok {
				p.dragStartPosX = cur.Position.X
				p.dragStartPosY = cur.Position.Y
			}
		}, app.EventScope(wid)).
		OnClick(func(ctx app.Context, e app.Event) {
			e.Call("stopPropagation")
		}, app.EventScope(wid))

	bodyItems := []app.UI{content}

	// ── Selection handles (only when selected) ──────────────────────────────
	if isSelected {
		// Rotation arm: vertical line from origin toward the rotation handle
		const rotHandleDist = 40.0 // px above origin in local space
		rotLine := app.Div().
			Style("position", "absolute").
			Style("left", fmt.Sprintf("%.1fpx", ox-1)).
			Style("top", fmt.Sprintf("%.1fpx", oy-rotHandleDist)).
			Style("width", "2px").
			Style("height", fmt.Sprintf("%.0fpx", rotHandleDist)).
			Style("background", "var(--accent)").
			Style("pointer-events", "none").
			Style("z-index", "1")

		// Rotation handle circle (above the origin in local space)
		rotHandle := app.Div().
			Style("position", "absolute").
			Style("left", fmt.Sprintf("%.1fpx", ox-8)).
			Style("top", fmt.Sprintf("%.1fpx", oy-rotHandleDist-8)).
			Style("width", "16px").
			Style("height", "16px").
			Style("background", "var(--accent)").
			Style("border", "2px solid var(--bg)").
			Style("border-radius", "50%").
			Style("cursor", "alias").
			Style("z-index", "3").
			Style("box-shadow", "0 1px 3px rgba(0,0,0,0.3)").
			OnMouseDown(func(ctx app.Context, e app.Event) {
				e.Call("stopPropagation")
				e.PreventDefault()

				cur, ok := p.widgetByID(wid)
				if !ok {
					return
				}
				rotDeg := cur.Rotation.Degrees
				rad := rotDeg * math.Pi / 180
				sinA := math.Sin(rad)
				cosA := math.Cos(rad)

				// Compute origin position in client coordinates from handle rect.
				// The rotation handle is placed at local (ox, oy - rotHandleDist).
				// After the widget matrix transform, its client position is:
				//   handleClientX = (posX + ox + rotHandleDist*sinA) + canvasClientX
				//   handleClientY = (posY + oy - rotHandleDist*cosA) + canvasClientY
				// So: originClientX = handleClientX - rotHandleDist*sinA
				//     originClientY = handleClientY + rotHandleDist*cosA
				handleRect := e.Get("currentTarget").Call("getBoundingClientRect")
				handleCX := (handleRect.Get("left").Float() + handleRect.Get("right").Float()) / 2
				handleCY := (handleRect.Get("top").Float() + handleRect.Get("bottom").Float()) / 2

				p.rotOriginCliX = handleCX - rotHandleDist*sinA
				p.rotOriginCliY = handleCY + rotHandleDist*cosA
				p.rotStartAngle = math.Atan2(handleCY-p.rotOriginCliY, handleCX-p.rotOriginCliX)
				p.rotStartDeg = rotDeg
				p.rotatingWidgetID = wid
			}, app.EventScope(wid))

		// Origin handle: orange circle at anchor point
		originHandle := app.Div().
			Style("position", "absolute").
			Style("left", fmt.Sprintf("%.1fpx", ox-7)).
			Style("top", fmt.Sprintf("%.1fpx", oy-7)).
			Style("width", "14px").
			Style("height", "14px").
			Style("background", "#ff8c00").
			Style("border", "2px solid var(--bg)").
			Style("border-radius", "50%").
			Style("cursor", "crosshair").
			Style("z-index", "3").
			Style("box-shadow", "0 1px 3px rgba(0,0,0,0.3)").
			OnMouseDown(func(ctx app.Context, e app.Event) {
				e.Call("stopPropagation")
				e.PreventDefault()
				p.selectWidget(ctx, wid)
				p.draggingOriginID = wid
				p.originCliX = e.Get("clientX").Float()
				p.originCliY = e.Get("clientY").Float()
				if cur, ok := p.widgetByID(wid); ok {
					p.originStartOX = cur.Origin.X
					p.originStartOY = cur.Origin.Y
					p.originStartPosX = cur.Position.X
					p.originStartPosY = cur.Position.Y
					p.originDragW = cur.Size.Width
					p.originDragH = cur.Size.Height
					p.originDragRotDeg = cur.Rotation.Degrees
				}
			}, app.EventScope(wid))

		// SE resize handle: small square at bottom-right corner
		resizeHandle := app.Div().
			Style("position", "absolute").
			Style("right", "0").
			Style("bottom", "0").
			Style("width", "12px").
			Style("height", "12px").
			Style("background", "var(--accent)").
			Style("border", "2px solid var(--bg)").
			Style("border-radius", "3px 0 4px 0").
			Style("cursor", "se-resize").
			Style("z-index", "3").
			OnMouseDown(func(ctx app.Context, e app.Event) {
				e.Call("stopPropagation")
				e.PreventDefault()
				p.resizingWidgetID = wid
				p.resizeStartCliX = e.Get("clientX").Float()
				p.resizeStartCliY = e.Get("clientY").Float()
				if cur, ok := p.widgetByID(wid); ok {
					p.resizeStartW = cur.Size.Width
					p.resizeStartH = cur.Size.Height
					p.resizeStartDeg = cur.Rotation.Degrees
				}
			}, app.EventScope(wid))

		bodyItems = append(bodyItems, rotLine, rotHandle, originHandle, resizeHandle)
	}

	return app.Div().
		Style("position", "absolute").
		Style("left", "0").
		Style("top", "0").
		Style("width", fmt.Sprintf("%dpx", w.Size.Width)).
		Style("height", fmt.Sprintf("%dpx", w.Size.Height)).
		Style("transform-origin", "0 0").
		Style("transform", widgetMatrixCSS(w)).
		Style("overflow", "visible").
		OnClick(func(ctx app.Context, e app.Event) {
			e.Call("stopPropagation")
		}, app.EventScope(wid)).
		Body(bodyItems...)
}
