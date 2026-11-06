package project

import (
	"fmt"
	"math"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
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
		// go-app keeps a handler of the same func literal on a reused
		// element: scoped by what the handlers capture, a tab never acts on
		// the scene it showed before the list changed.
		scope := sc.ID.String() + "/" + sc.Name

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
						p.editingSceneID = uuid.Nil
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
							p.selectScene(ctx, sc.ID)
						}, app.EventScope(scope)).
						OnDblClick(func(ctx app.Context, e app.Event) {
							if p.refuseReadOnly(ctx) {
								return
							}
							p.startEditingScene(sc.ID, sc.Name)
						}, app.EventScope(scope)),
					app.Span().
						Style("cursor", "pointer").
						Style("font-size", "14px").
						Style("line-height", "1").
						Style("color", "var(--text-muted)").
						Style("padding", "0 1px").
						Text("×").
						OnClick(func(ctx app.Context, e app.Event) {
							e.Call("stopPropagation")
							p.confirmDeleteScene(ctx, sc.ID, sc.Name)
						}, app.EventScope(scope)),
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

	addScene := app.Div().
		Attr("data-scene-add", "true").
		Style("display", "flex").
		Style("align-items", "center").
		Style("padding", "6px 10px").
		Style("cursor", "pointer").
		Style("font-size", "18px").
		Style("color", "var(--accent)").
		Style("line-height", "1").
		Text("+").
		OnClick(func(ctx app.Context, e app.Event) { p.createScene(ctx) })
	if p.readOnly() {
		addScene = addScene.
			Attr("title", readOnlyHint).
			Style("color", "var(--text-muted)").
			Style("cursor", "not-allowed")
	}
	tabs = append(tabs, addScene)

	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "row").
		Style("flex-shrink", "0").
		Style("border-bottom", "1px solid var(--border)").
		Style("background", "var(--bg-elevated)").
		Body(tabs...)
}

func (p *Project) renderSceneCanvas() app.UI {
	if p.selectedSceneID == uuid.Nil {
		return app.Div().
			Style("flex", "1").
			Style("display", "flex").
			Style("align-items", "center").
			Style("justify-content", "center").
			Style("color", "var(--text-muted)").
			Style("font-size", "14px").
			Body(app.Text("Create a scene to get started"))
	}

	sceneWidth, sceneHeight := p.sceneSize()
	var backgroundHTML string
	if sc, ok := p.findScene(p.selectedSceneID); ok {
		backgroundHTML = sc.BackgroundHTML
	}

	layers := p.canvasLayers(backgroundHTML)

	// Determine overall canvas cursor from active drag mode.
	canvasCursor := "default"
	switch {
	case p.draggingWidgetID != uuid.Nil:
		canvasCursor = "grabbing"
	case p.draggingOriginID != uuid.Nil:
		canvasCursor = "crosshair"
	case p.rotatingWidgetID != uuid.Nil:
		canvasCursor = "alias"
	case p.resizingWidgetID != uuid.Nil:
		canvasCursor = "se-resize"
	}

	canvas := app.Div().
		Style("position", "relative").
		Style("width", fmt.Sprintf("%.0fpx", sceneWidth)).
		Style("height", fmt.Sprintf("%.0fpx", sceneHeight)).
		Style("background-color", "var(--surface)").
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
			if typeID == uuid.Nil {
				typeID = uiutil.ParseID(e.Get("dataTransfer").Call("getData", "text/plain").String())
			}
			p.draggedTypeID = uuid.Nil
			if typeID == uuid.Nil {
				return
			}
			rect := e.Get("currentTarget").Call("getBoundingClientRect")
			x := e.Get("clientX").Float() - rect.Get("left").Float()
			y := e.Get("clientY").Float() - rect.Get("top").Float()
			p.createWidget(ctx, typeID, x, y)
		}).
		OnMouseDown(func(ctx app.Context, e app.Event) {
			// Any intentional mousedown on the canvas clears the post-drag
			// suppression flag so the subsequent click correctly deselects.
			p.dragJustEnded = false
			// A left press on the empty canvas starts a selection rectangle;
			// a Widget's overlay stops its own mousedown before this.
			if e.Get("button").Int() == 0 {
				e.PreventDefault()
				p.startMarquee(ctx, e)
			}
		}).
		OnClick(func(ctx app.Context, e app.Event) {
			if p.dragJustEnded {
				p.dragJustEnded = false
				return
			}
			p.clearWidgetSelection(ctx)
		}).
		Body(layers...)

	return app.Div().
		Style("flex", "1").
		Style("min-height", "0").
		Style("overflow", "auto").
		Style("background", "var(--bg-inset)").
		Style("padding", "24px").
		Body(canvas)
}

// canvasLayers are the canvas's layers bottom to top: the Scene's background
// (the canvas's own colour when it has none), the editor's dot grid, the
// Widgets, the alignment guides of a body drag and the selection rectangle
// being dragged. The background, the grid and the guides let mouse events
// through to the canvas.
func (p *Project) canvasLayers(backgroundHTML string) []app.UI {
	textMuted := uiutil.IframeTextMuted()
	widgets := make([]app.UI, 0, len(p.widgets))
	for _, w := range p.widgets {
		widgets = append(widgets, p.renderWidget(w, textMuted))
	}
	layers := []app.UI{
		&uiutil.SceneBackground{HTML: backgroundHTML},
		renderCanvasGrid(),
		uiutil.SceneWidgetLayer(widgets),
	}
	if guides := p.renderDragGuides(); guides != nil {
		layers = append(layers, guides)
	}
	if p.marquee.active {
		layers = append(layers, p.renderMarquee())
	}
	return layers
}

// sceneSize is the selected Scene's canvas size in pixels: its own, or the
// canvas's default when the Scene states none.
func (p *Project) sceneSize() (width, height float64) {
	width, height = defaultSceneWidth, defaultSceneHeight
	if sc, ok := p.findScene(p.selectedSceneID); ok {
		if sc.Width > 0 {
			width = float64(sc.Width)
		}
		if sc.Height > 0 {
			height = float64(sc.Height)
		}
	}
	return width, height
}

// renderCanvasGrid renders the editor's dot grid over the Scene's background,
// so that it shows on any background. The dot is two-coloured on purpose (task
// 41): a light core inside a dark ring, one radial gradient each, so that at
// least one of the two contrasts with whatever is behind it — the core on a
// dark background, the ring on a light one. It therefore does not follow the
// theme, unlike every other colour of the UI.
func renderCanvasGrid() app.UI {
	return app.Div().
		Attr("data-canvas-grid", "true").
		Style("position", "absolute").
		Style("inset", "0").
		Style("background-image",
			"radial-gradient(circle, #f0f0f0 0, #f0f0f0 1px, transparent 1px), "+
				"radial-gradient(circle, #3c3c3c 0, #3c3c3c 1.75px, transparent 1.75px)").
		Style("background-size", "24px 24px").
		Style("pointer-events", "none")
}

// renderDragGuides draws the alignment guides of a body drag over the Widgets
// and under the selection rectangle: one 1 px line per snapped axis, the whole
// width or height of the canvas (task 41). Nothing is drawn while no axis is
// snapped, nor while Alt frees the drag.
func (p *Project) renderDragGuides() app.UI {
	guides := p.dragGuides
	if guides.vertical == nil && guides.horizontal == nil {
		return nil
	}
	lines := make([]app.UI, 0, 2)
	if guides.vertical != nil {
		lines = append(lines, app.Div().
			Attr("data-guide", "vertical").
			Style("position", "absolute").
			Style("left", fmt.Sprintf("%.1fpx", *guides.vertical)).
			Style("top", "0").
			Style("width", "1px").
			Style("height", "100%").
			Style("background", "var(--accent)"))
	}
	if guides.horizontal != nil {
		lines = append(lines, app.Div().
			Attr("data-guide", "horizontal").
			Style("position", "absolute").
			Style("top", fmt.Sprintf("%.1fpx", *guides.horizontal)).
			Style("left", "0").
			Style("width", "100%").
			Style("height", "1px").
			Style("background", "var(--accent)"))
	}
	return app.Div().
		Attr("data-guides", "true").
		Style("position", "absolute").
		Style("inset", "0").
		Style("pointer-events", "none").
		// Over every Widget (their z-index is the position's z), under the
		// selection rectangle.
		Style("z-index", "2147483645").
		Body(lines...)
}

// renderWidget renders a single widget element on the canvas using a CSS
// transformation matrix. When selected, overlay handles are shown:
//   - Orange circle: origin anchor (drag to move anchor within widget)
//   - Blue circle + line: rotation handle (drag to rotate)
//   - Blue square corner: SE resize handle (drag to resize)
func (p *Project) renderWidget(w apiv0.WidgetResponse, textMuted string) app.UI {
	wid := w.ID
	scope := wid.String() // go-app event scope
	isSelected := p.isWidgetSelected(wid)
	// The anchor, rotation and resize handles belong to the one Widget the
	// properties panel edits: a group has none (task 40).
	_, single := p.singleSelected()

	// Local pixel position of the origin anchor within the widget.
	ox := w.Origin.X * float64(w.Size.Width)
	oy := w.Origin.Y * float64(w.Size.Height)

	// ── Widget content: iframe preview + transparent drag overlay ───────────
	wt, wtOk := p.widgetTypeByID(w.TypeID)
	var srcdoc string
	if wtOk {
		srcdoc = uiutil.BuildSrcdoc(wt.HtmlTemplate, wt.Script, uiutil.PreviewInputs(p.simInputs[wid], wt.InputPorts), wt.InputPorts, uiutil.SceneWidgetBackground)
	} else {
		// Type not loaded yet or deleted — show widget name as a text fallback.
		srcdoc = uiutil.BuildSrcdoc(uiutil.WidgetNamePlaceholder(w.Name, textMuted), "", nil, nil, uiutil.SceneWidgetBackground)
	}

	content := app.Div().
		Style("position", "absolute").
		Style("inset", "0").
		Style("overflow", "hidden").
		Body(
			// Widget preview iframe — pointer-events:none so mouse hits overlay.
			&widgetPreviewFrame{
				ID:     widgetPreviewDOMID(wid),
				Srcdoc: srcdoc,
			},
			// Transparent overlay: captures drag/click for widget manipulation.
			app.Div().
				Style("position", "absolute").
				Style("inset", "0").
				Style("cursor", "move").
				Style("user-select", "none").
				OnMouseDown(func(ctx app.Context, e app.Event) {
					e.Call("stopPropagation")
					e.PreventDefault()
					// Shift/Ctrl/Cmd adds or removes the Widget without
					// dragging anything.
					if e.Get("shiftKey").Bool() || e.Get("ctrlKey").Bool() || e.Get("metaKey").Bool() {
						p.toggleWidgetSelection(ctx, wid)
						return
					}
					p.pressWidget(ctx, e, wid)
				}, app.EventScope(scope)).
				OnClick(func(ctx app.Context, e app.Event) {
					e.Call("stopPropagation")
				}, app.EventScope(scope)),
			// Selection border overlay (pointer-events:none so it doesn't block drag).
			app.If(isSelected, func() app.UI {
				return app.Div().
					Style("position", "absolute").
					Style("inset", "0").
					Style("border", "2px solid var(--accent)").
					Style("pointer-events", "none").
					Style("box-sizing", "border-box")
			}),
		)

	bodyItems := []app.UI{content}

	// ── Selection handles (only when this one Widget is selected) ───────────
	if single && isSelected {
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
				if p.refuseReadOnly(ctx) {
					return
				}

				cur, ok := p.findWidget(wid)
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
			}, app.EventScope(scope))

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
				p.selectOnly(ctx, wid)
				if p.refuseReadOnly(ctx) {
					return
				}
				p.draggingOriginID = wid
				p.originCliX = e.Get("clientX").Float()
				p.originCliY = e.Get("clientY").Float()
				if cur, ok := p.findWidget(wid); ok {
					p.originStartOX = cur.Origin.X
					p.originStartOY = cur.Origin.Y
					p.originStartPosX = cur.Position.X
					p.originStartPosY = cur.Position.Y
					p.originDragW = cur.Size.Width
					p.originDragH = cur.Size.Height
					p.originDragRotDeg = cur.Rotation.Degrees
				}
			}, app.EventScope(scope))

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
				if p.refuseReadOnly(ctx) {
					return
				}
				p.resizingWidgetID = wid
				p.resizeStartCliX = e.Get("clientX").Float()
				p.resizeStartCliY = e.Get("clientY").Float()
				if cur, ok := p.findWidget(wid); ok {
					p.resizeStartW = cur.Size.Width
					p.resizeStartH = cur.Size.Height
					p.resizeStartDeg = cur.Rotation.Degrees
					p.resizeOriginX = cur.Origin.X
					p.resizeOriginY = cur.Origin.Y
					// Record canvas-space top-left corner so it stays anchored
					// during resize even when origin is not at (0,0) and widget
					// is rotated.
					rad := cur.Rotation.Degrees * math.Pi / 180
					cosA := math.Cos(rad)
					sinA := math.Sin(rad)
					startOx := cur.Origin.X * float64(cur.Size.Width)
					startOy := cur.Origin.Y * float64(cur.Size.Height)
					p.resizeStartE = cur.Position.X + startOx*(1-cosA) + sinA*startOy
					p.resizeStartF = cur.Position.Y - sinA*startOx + startOy*(1-cosA)
				}
			}, app.EventScope(scope))

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
		Style("z-index", fmt.Sprint(w.Position.Z)).
		Style("overflow", "visible").
		OnClick(func(ctx app.Context, e app.Event) {
			e.Call("stopPropagation")
		}, app.EventScope(scope)).
		Body(bodyItems...)
}
