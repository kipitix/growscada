package project

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

// ── Component ─────────────────────────────────────────────────────────────────

type projectSubTab string

const (
	subTabScenes projectSubTab = "scenes"
	subTabTags   projectSubTab = "tags"
)

type Project struct {
	app.Compo
	apiServerURL string

	activeSubTab projectSubTab

	widgetTypes []widgetTypeItem
	scenes      []sceneItem
	widgets     []widgetItem
	tags        []tagItem

	// ── Tags sub-tab state ──────────────────────────────────────────────────
	selectedTagID string
	creatingTag   bool
	newTagName    string
	newTagType    string
	tagFetchErr   string

	selectedSceneID  string
	selectedWidgetID string

	editingSceneID   string
	editingSceneName string

	draggedTypeID  string
	dragJustEnded bool // suppresses the canvas OnClick that fires right after mouseup

	// ── Drag: move widget body ──────────────────────────────────────────────
	draggingWidgetID string
	dragStartCliX    float64
	dragStartCliY    float64
	dragStartPosX    float64
	dragStartPosY    float64

	// ── Drag: move origin anchor ────────────────────────────────────────────
	// The anchor is moved within the widget in local space; the widget
	// position is adjusted simultaneously to keep the visual bounding-box
	// stationary on screen.
	draggingOriginID string
	originCliX       float64 // client X when drag started
	originCliY       float64 // client Y when drag started
	originStartOX   float64 // origin.X before drag
	originStartOY   float64 // origin.Y before drag
	originStartPosX float64 // position.X before drag
	originStartPosY float64 // position.Y before drag
	originDragW     int     // widget width during drag
	originDragH     int     // widget height during drag
	originDragRotDeg float64 // widget rotation during drag

	// ── Drag: rotation handle ───────────────────────────────────────────────
	// Dragging a circle positioned above the origin in local widget space.
	// We store the origin's client-space position and the initial angle so we
	// can compute the angular delta on every mousemove.
	rotatingWidgetID string
	rotOriginCliX    float64 // origin screen position X (client coords)
	rotOriginCliY    float64 // origin screen position Y (client coords)
	rotStartAngle    float64 // atan2 from origin to handle when drag started
	rotStartDeg      float64 // rotation.Degrees before drag

	// ── Drag: resize SE handle ──────────────────────────────────────────────
	resizingWidgetID string
	resizeStartCliX  float64
	resizeStartCliY  float64
	resizeStartW     int
	resizeStartH     int
	resizeStartDeg   float64 // for inverse-rotation delta

	// ── Properties panel editing state ─────────────────────────────────────
	editingWidgetName string
	editingPosX       string
	editingPosY       string
	editingPosZ       string
	editingWidth      string
	editingHeight     string
	editingOriginX    string
	editingOriginY    string
	editingRotation string

	fetchErr string

	// ── Panel resize state ──────────────────────────────────────────────────
	widgetTypeWidth      int
	propertiesWidth      int
	resizingPanelSide    string // "left" | "right" | ""
	panelResizeStartX    float64
	panelResizeStartWidth int
}

func NewProject(apiServerURL string) *Project {
	return &Project{apiServerURL: apiServerURL, activeSubTab: subTabScenes}
}

func (p *Project) OnMount(ctx app.Context) {
	p.widgetTypeWidth = 180
	p.propertiesWidth = 260
	ctx.LocalStorage().Get("project:widgetTypeWidth", &p.widgetTypeWidth)
	ctx.LocalStorage().Get("project:propertiesWidth", &p.propertiesWidth)

	var savedTab string
	ctx.LocalStorage().Get("project:subTab", &savedTab)
	if savedTab != "" {
		p.activeSubTab = projectSubTab(savedTab)
	}
	ctx.LocalStorage().Get("project:sceneID", &p.selectedSceneID)
	ctx.LocalStorage().Get("project:widgetID", &p.selectedWidgetID)
	ctx.LocalStorage().Get("project:tagID", &p.selectedTagID)
	p.loadWidgetTypes(ctx)
	p.loadScenes(ctx)
	if p.selectedSceneID != "" {
		p.loadWidgets(ctx) // load widgets for the restored scene immediately
	}
	p.loadTags(ctx)
}

// ── Data loading ──────────────────────────────────────────────────────────────

func (p *Project) loadWidgetTypes(ctx app.Context) {
	url := p.apiServerURL + "/api/v1/widget-types"
	ctx.Async(func() {
		resp, err := http.Get(url)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		defer resp.Body.Close()
		var result getWidgetTypesResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		ctx.Dispatch(func(ctx app.Context) { p.widgetTypes = result.WidgetTypes })
	})
}

func (p *Project) loadTags(ctx app.Context) {
	url := p.apiServerURL + "/api/v1/tags"
	ctx.Async(func() {
		resp, err := http.Get(url)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		defer resp.Body.Close()
		var result getTagsResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		ctx.Dispatch(func(ctx app.Context) {
			p.tags = result.Tags
			if p.selectedTagID != "" {
				found := false
				for _, t := range result.Tags {
					if t.ID == p.selectedTagID {
						found = true
						break
					}
				}
				if !found {
					p.selectedTagID = ""
					ctx.LocalStorage().Set("project:tagID", "")
				}
			}
		})
	})
}

// ── Widget selection helpers ──────────────────────────────────────────────────

func (p *Project) selectWidget(ctx app.Context, id string) {
	p.selectedWidgetID = id
	ctx.LocalStorage().Set("project:widgetID", id)
	for _, w := range p.widgets {
		if w.ID == id {
			p.syncEditingFields(w)
			break
		}
	}
}

func (p *Project) syncEditingFields(w widgetItem) {
	p.editingWidgetName = w.Name
	p.editingPosX = fmt.Sprintf("%.1f", w.Position.X)
	p.editingPosY = fmt.Sprintf("%.1f", w.Position.Y)
	p.editingPosZ = strconv.Itoa(w.Position.Z)
	p.editingWidth = strconv.Itoa(w.Size.Width)
	p.editingHeight = strconv.Itoa(w.Size.Height)
	p.editingOriginX = fmt.Sprintf("%.3f", w.Origin.X)
	p.editingOriginY = fmt.Sprintf("%.3f", w.Origin.Y)
	p.editingRotation = fmt.Sprintf("%.1f", w.Rotation.Degrees)
}

func (p *Project) clearWidgetSelection(ctx app.Context) {
	p.selectedWidgetID = ""
	p.editingWidgetName = ""
	p.editingPosX = ""
	p.editingPosY = ""
	p.editingPosZ = ""
	p.editingWidth = ""
	p.editingHeight = ""
	p.editingOriginX = ""
	p.editingOriginY = ""
	p.editingRotation = ""
	ctx.LocalStorage().Set("project:widgetID", "")
}

func (p *Project) selectedWidgetIdx() int {
	for i, w := range p.widgets {
		if w.ID == p.selectedWidgetID {
			return i
		}
	}
	return -1
}

// widgetByID returns the current live widget data by ID.
// Use this inside event handler closures instead of the closure-captured `w`,
// because go-app may not re-attach handlers when only the transform changes,
// leaving `w` stale across drags.
func (p *Project) widgetByID(id string) (widgetItem, bool) {
	for _, w := range p.widgets {
		if w.ID == id {
			return w, true
		}
	}
	return widgetItem{}, false
}

// ── Render ────────────────────────────────────────────────────────────────────

func (p *Project) Render() app.UI {
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex", "1").
		Style("min-height", "0").
		Style("overflow", "hidden").
		Body(
			p.renderSubTabs(),
			app.If(p.activeSubTab == subTabScenes, func() app.UI {
				return p.renderScenesContent()
			}).Else(func() app.UI {
				return p.renderTagsPanel()
			}),
		)
}

func (p *Project) renderSubTabs() app.UI {
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "row").
		Style("border-bottom", "1px solid var(--border)").
		Style("background", "var(--bg-elevated)").
		Style("padding", "0 4px").
		Body(
			p.subTab("Scenes", subTabScenes),
			p.subTab("Tags", subTabTags),
		)
}

func (p *Project) subTab(label string, tab projectSubTab) app.UI {
	active := p.activeSubTab == tab
	el := app.Div().
		Style("padding", "6px 16px").
		Style("cursor", "pointer").
		Style("font-size", "13px").
		Style("user-select", "none").
		Style("border-bottom", "2px solid transparent").
		Style("margin-bottom", "-1px").
		Text(label).
		OnClick(func(ctx app.Context, e app.Event) {
			p.activeSubTab = tab
			ctx.LocalStorage().Set("project:subTab", string(tab))
		})
	if active {
		return el.
			Style("border-bottom-color", "var(--accent)").
			Style("color", "var(--accent)").
			Style("font-weight", "600")
	}
	return el.Style("color", "var(--text-2)")
}

func (p *Project) renderPanelDivider(side string) app.UI {
	return app.Div().
		Style("width", "5px").
		Style("flex-shrink", "0").
		Style("cursor", "col-resize").
		Style("background", "var(--border)").
		Style("user-select", "none").
		OnMouseDown(func(ctx app.Context, e app.Event) {
			e.PreventDefault()
			p.resizingPanelSide = side
			p.panelResizeStartX = e.Get("clientX").Float()
			if side == "left" {
				p.panelResizeStartWidth = p.widgetTypeWidth
			} else {
				p.panelResizeStartWidth = p.propertiesWidth
			}
		})
}

func (p *Project) renderScenesContent() app.UI {
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "row").
		Style("flex", "1").
		Style("min-height", "0").
		Style("overflow", "hidden").
		Style("gap", "0").
		// ── Drag handlers live here so they capture mouse events across the
		// entire Project panel regardless of which scene/canvas is active.
		// Previously these were on the canvas div, which caused drag to break
		// on smaller scenes (e.g. 1280×720) when the mouse left the canvas.
		OnMouseMove(func(ctx app.Context, e app.Event) {
			anyDrag := p.draggingWidgetID != "" ||
				p.draggingOriginID != "" ||
				p.rotatingWidgetID != "" ||
				p.resizingWidgetID != "" ||
				p.resizingPanelSide != ""
			if !anyDrag {
				return
			}
			// Mouse button released outside this element — cancel all drags.
			if e.Get("buttons").Int() == 0 {
				if p.resizingPanelSide == "left" {
					ctx.LocalStorage().Set("project:widgetTypeWidth", p.widgetTypeWidth)
				} else if p.resizingPanelSide == "right" {
					ctx.LocalStorage().Set("project:propertiesWidth", p.propertiesWidth)
				}
				p.resizingPanelSide = ""
				p.finalizeAllDrags(ctx)
				return
			}
			e.PreventDefault()

			clientX := e.Get("clientX").Float()
			clientY := e.Get("clientY").Float()

			// Resize left or right panel
			if p.resizingPanelSide != "" {
				dx := int(clientX - p.panelResizeStartX)
				if p.resizingPanelSide == "left" {
					newW := p.panelResizeStartWidth + dx
					if newW < 100 {
						newW = 100
					} else if newW > 480 {
						newW = 480
					}
					p.widgetTypeWidth = newW
				} else {
					newW := p.panelResizeStartWidth - dx
					if newW < 160 {
						newW = 160
					} else if newW > 600 {
						newW = 600
					}
					p.propertiesWidth = newW
				}
			}

			// Move widget body
			if p.draggingWidgetID != "" {
				dx := clientX - p.dragStartCliX
				dy := clientY - p.dragStartCliY
				for i := range p.widgets {
					if p.widgets[i].ID == p.draggingWidgetID {
						p.widgets[i].Position.X = p.dragStartPosX + dx
						p.widgets[i].Position.Y = p.dragStartPosY + dy
						if p.selectedWidgetID == p.draggingWidgetID {
							p.editingPosX = fmt.Sprintf("%.1f", p.widgets[i].Position.X)
							p.editingPosY = fmt.Sprintf("%.1f", p.widgets[i].Position.Y)
						}
						break
					}
				}
			}

			// Move origin anchor; compensate position to keep widget visually stationary.
			// Screen delta (dx,dy) is rotated into widget-local space to get the local
			// origin delta; position is then compensated so the matrix e/f stays fixed.
			if p.draggingOriginID != "" {
				dx := clientX - p.originCliX
				dy := clientY - p.originCliY
				rad := p.originDragRotDeg * math.Pi / 180
				cosA := math.Cos(rad)
				sinA := math.Sin(rad)
				// Rotate screen delta into widget-local coordinate axes.
				dOxPx := cosA*dx + sinA*dy
				dOyPx := -sinA*dx + cosA*dy
				newOX := math.Max(0, math.Min(1, p.originStartOX+dOxPx/float64(p.originDragW)))
				newOY := math.Max(0, math.Min(1, p.originStartOY+dOyPx/float64(p.originDragH)))
				// Use clamped pixel deltas for position compensation.
				dOx := (newOX - p.originStartOX) * float64(p.originDragW)
				dOy := (newOY - p.originStartOY) * float64(p.originDragH)
				newPosX := p.originStartPosX - dOx*(1-cosA) - dOy*sinA
				newPosY := p.originStartPosY - dOy*(1-cosA) + dOx*sinA
				for i := range p.widgets {
					if p.widgets[i].ID == p.draggingOriginID {
						p.widgets[i].Origin.X = newOX
						p.widgets[i].Origin.Y = newOY
						p.widgets[i].Position.X = newPosX
						p.widgets[i].Position.Y = newPosY
						if p.selectedWidgetID == p.draggingOriginID {
							p.editingOriginX = fmt.Sprintf("%.3f", newOX)
							p.editingOriginY = fmt.Sprintf("%.3f", newOY)
							p.editingPosX = fmt.Sprintf("%.1f", newPosX)
							p.editingPosY = fmt.Sprintf("%.1f", newPosY)
						}
						break
					}
				}
			}

			// Rotate widget around origin
			if p.rotatingWidgetID != "" {
				dx := clientX - p.rotOriginCliX
				dy := clientY - p.rotOriginCliY
				currentAngle := math.Atan2(dy, dx)
				delta := currentAngle - p.rotStartAngle
				newDeg := math.Mod(p.rotStartDeg+delta*180/math.Pi, 360)
				if newDeg < 0 {
					newDeg += 360
				}
				for i := range p.widgets {
					if p.widgets[i].ID == p.rotatingWidgetID {
						p.widgets[i].Rotation.Degrees = newDeg
						if p.selectedWidgetID == p.rotatingWidgetID {
							p.editingRotation = fmt.Sprintf("%.1f", newDeg)
						}
						break
					}
				}
			}

			// Resize widget via SE handle (inverse-rotation corrected)
			if p.resizingWidgetID != "" {
				dx := clientX - p.resizeStartCliX
				dy := clientY - p.resizeStartCliY
				rad := p.resizeStartDeg * math.Pi / 180
				cosA := math.Cos(rad)
				sinA := math.Sin(rad)
				dxLocal := dx*cosA + dy*sinA
				dyLocal := -dx*sinA + dy*cosA
				const minSize = 20
				newW := p.resizeStartW + int(math.Round(dxLocal))
				newH := p.resizeStartH + int(math.Round(dyLocal))
				if newW < minSize {
					newW = minSize
				}
				if newH < minSize {
					newH = minSize
				}
				for i := range p.widgets {
					if p.widgets[i].ID == p.resizingWidgetID {
						p.widgets[i].Size.Width = newW
						p.widgets[i].Size.Height = newH
						if p.selectedWidgetID == p.resizingWidgetID {
							p.editingWidth = strconv.Itoa(newW)
							p.editingHeight = strconv.Itoa(newH)
						}
						break
					}
				}
			}
		}).
		OnMouseUp(func(ctx app.Context, e app.Event) {
			if p.resizingPanelSide == "left" {
				ctx.LocalStorage().Set("project:widgetTypeWidth", p.widgetTypeWidth)
			} else if p.resizingPanelSide == "right" {
				ctx.LocalStorage().Set("project:propertiesWidth", p.propertiesWidth)
			}
			p.resizingPanelSide = ""
			p.finalizeAllDrags(ctx)
		}).
		Body(
			p.renderWidgetTypePanel(),
			p.renderPanelDivider("left"),
			p.renderScenePanel(),
			p.renderPanelDivider("right"),
			p.renderPropertiesPanel(),
		)
}

