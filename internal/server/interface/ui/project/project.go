package project

import (
	"math"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/tagstate"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Component ─────────────────────────────────────────────────────────────────

type projectSubTab string

const (
	subTabScenes projectSubTab = "scenes"
	subTabTags   projectSubTab = "tags"
)

type Project struct {
	app.Compo
	// compoCtx is the component's own context, captured in OnMount. Async work
	// dispatches its result through it: an event handler's ctx belongs to the
	// element that fired the event, and go-app silently drops Dispatch once that
	// element is unmounted (e.g. a tab that turns into its rename input), which
	// would lose the result and leave a Reloader loading forever.
	compoCtx  app.Context
	mounted   bool
	ThemeMode string // exported so go-app detects theme changes and re-renders
	client    *apiclient.Client
	// projectID is the project shown, uuid.Nil if none; content is the client
	// of its content, nil if none (SetProject). WidgetTypes are the server's,
	// shown whatever the project.
	projectID uuid.UUID
	content   *apiclient.ProjectClient

	activeSubTab projectSubTab

	widgetTypes []apiv0.WidgetTypeResponse
	scenes      []apiv0.SceneResponse
	widgets     []apiv0.WidgetResponse
	tags        tagstate.Tags
	// sceneVersions holds the version each write of a Scene or its Widgets
	// states; the Version and SceneVersion of the lists above are not read.
	// The selected Scene's is learned with its Widgets (observeSceneVersion).
	sceneVersions uiutil.Versions
	// sceneStates holds the version of each Scene's properties in p.scenes:
	// a reread replaces them only with a newer state (sceneVersions also
	// follows the Widgets' changes, which leave the properties as they are).
	sceneStates uiutil.Versions

	// Coalesce reloads of each server-backed list (see load.go).
	widgetTypesReloader uiutil.Reloader
	scenesReloader      uiutil.Reloader
	widgetsReloader     uiutil.Reloader
	tagsReloader        uiutil.Reloader
	// staleScenes are the Scenes to reread (loadScene) once the reread in
	// flight on staleScenesReloader is done.
	staleScenes         map[uuid.UUID]struct{}
	staleScenesReloader uiutil.Reloader
	// widgetsReloadDeferred is set when a widget reload arrives mid-drag; it
	// runs once the drag ends so the canvas does not jump under the cursor.
	widgetsReloadDeferred bool

	// ── Tags sub-tab state ──────────────────────────────────────────────────
	selectedTagID uuid.UUID
	// tagLists numbers the requests of the Tag list; one numbered up to
	// tagCreated was sent before the selected Tag was created and does not
	// hold it.
	tagLists, tagCreated uint64

	creatingTag bool
	newTagName  string
	newTagType  string

	selectedSceneID  uuid.UUID
	selectedWidgetID uuid.UUID

	editingSceneID   uuid.UUID
	editingSceneName string

	// ── Scene properties panel editing state ───────────────────────────────
	sceneEdit sceneEditFields

	draggedTypeID uuid.UUID
	dragJustEnded bool // suppresses the canvas OnClick that fires right after mouseup
	dragDidMove   bool // true if mouse moved during the drag; only then suppress the click
	// dragStartGeometry is the dragged widget's geometry before its first
	// move; a drag that ends where it started saves nothing.
	dragStartGeometry widgetGeometry

	// ── Drag: move widget body ──────────────────────────────────────────────
	draggingWidgetID uuid.UUID
	dragStartCliX    float64
	dragStartCliY    float64
	dragStartPosX    float64
	dragStartPosY    float64

	// ── Drag: move origin anchor ────────────────────────────────────────────
	// The anchor is moved within the widget in local space; the widget
	// position is adjusted simultaneously to keep the visual bounding-box
	// stationary on screen.
	draggingOriginID uuid.UUID
	originCliX       float64 // client X when drag started
	originCliY       float64 // client Y when drag started
	originStartOX    float64 // origin.X before drag
	originStartOY    float64 // origin.Y before drag
	originStartPosX  float64 // position.X before drag
	originStartPosY  float64 // position.Y before drag
	originDragW      int     // widget width during drag
	originDragH      int     // widget height during drag
	originDragRotDeg float64 // widget rotation during drag

	// ── Drag: rotation handle ───────────────────────────────────────────────
	// Dragging a circle positioned above the origin in local widget space.
	// We store the origin's client-space position and the initial angle so we
	// can compute the angular delta on every mousemove.
	rotatingWidgetID uuid.UUID
	rotOriginCliX    float64 // origin screen position X (client coords)
	rotOriginCliY    float64 // origin screen position Y (client coords)
	rotStartAngle    float64 // atan2 from origin to handle when drag started
	rotStartDeg      float64 // rotation.Degrees before drag

	// ── Drag: resize SE handle ──────────────────────────────────────────────
	resizingWidgetID uuid.UUID
	resizeStartCliX  float64
	resizeStartCliY  float64
	resizeStartW     int
	resizeStartH     int
	resizeStartDeg   float64 // for inverse-rotation delta
	resizeStartE     float64 // canvas X of widget top-left at resize start
	resizeStartF     float64 // canvas Y of widget top-left at resize start
	resizeOriginX    float64 // origin.X at resize start (for position compensation)
	resizeOriginY    float64 // origin.Y at resize start

	// ── Properties panel editing state ─────────────────────────────────────
	widgetEdit widgetEditFields

	// ── Input simulation (in-memory only, not persisted) ───────────────────
	// simInputs[widgetID][portName] = test value string
	simInputs map[uuid.UUID]map[string]string

	// ── Panel resize state ──────────────────────────────────────────────────
	widgetTypeWidth       int
	propertiesWidth       int
	resizingPanelSide     string // "left" | "right" | ""
	panelResizeStartX     float64
	panelResizeStartWidth int
}

// LocalStorage keys of the selection, kept per project (selectionKey).
const (
	sceneIDKey  = "project:sceneID"
	widgetIDKey = "project:widgetID"
	tagIDKey    = "project:tagID"
)

func NewProject(client *apiclient.Client) *Project {
	return &Project{client: client, activeSubTab: subTabScenes, simInputs: make(map[uuid.UUID]map[string]string)}
}

func (p *Project) OnMount(ctx app.Context) {
	p.compoCtx, p.mounted = ctx, true
	ctx.ObserveState("theme", &p.ThemeMode)
	p.widgetTypeWidth = 180
	p.propertiesWidth = 260
	ctx.LocalStorage().Get("project:widgetTypeWidth", &p.widgetTypeWidth)
	ctx.LocalStorage().Get("project:propertiesWidth", &p.propertiesWidth)

	var savedTab string
	ctx.LocalStorage().Get("project:subTab", &savedTab)
	if savedTab != "" {
		p.activeSubTab = projectSubTab(savedTab)
	}
	p.restoreSelection(ctx)
	p.loadWidgetTypes(ctx)
	p.loadScenes(ctx)
	if p.selectedSceneID != uuid.Nil {
		p.loadWidgets(ctx) // load widgets for the restored scene immediately
	}
	p.loadTags(ctx)

	p.handleServerEvents(ctx)
}

// SetProject shows the project id (uuid.Nil: none). What was shown of
// another project is dropped, its unsaved edits with it; the new project's
// selection is restored and its content loaded. Before the Project is
// mounted it only remembers the project, whose content OnMount loads.
func (p *Project) SetProject(id uuid.UUID) {
	if id == p.projectID {
		return
	}
	p.projectID, p.content = id, nil
	if id != uuid.Nil {
		p.content = p.client.Project(id)
	}
	if !p.mounted {
		return
	}
	ctx := p.compoCtx
	p.scenes, p.widgets, p.tags = nil, nil, tagstate.Tags{}
	p.sceneVersions.Forget()
	p.sceneStates.Forget()
	clear(p.staleScenes)
	p.widgetsReloadDeferred = false
	p.restoreSelection(ctx)
	p.widgetEdit = widgetEditFields{}
	p.sceneEdit = sceneEditFields{}
	p.editingSceneID, p.editingSceneName = uuid.Nil, ""
	p.creatingTag, p.newTagName, p.newTagType = false, "", ""
	p.simInputs = make(map[uuid.UUID]map[string]string)
	p.loadScenes(ctx)
	if p.selectedSceneID != uuid.Nil {
		p.loadWidgets(ctx)
	}
	p.loadTags(ctx)
	// Root calls it, so nothing marks this component for an update; with no
	// project left no load dispatches either.
	ctx.Update()
}

// selectionKey is the LocalStorage key base of the project shown.
func (p *Project) selectionKey(base string) string { return base + ":" + p.projectID.String() }

// restoreSelection reads the selection of the project shown from
// LocalStorage.
func (p *Project) restoreSelection(ctx app.Context) {
	p.selectedSceneID = uiutil.LoadID(ctx, p.selectionKey(sceneIDKey))
	p.selectedWidgetID = uiutil.LoadID(ctx, p.selectionKey(widgetIDKey))
	p.selectedTagID = uiutil.LoadID(ctx, p.selectionKey(tagIDKey))
}

// ── Input simulation ──────────────────────────────────────────────────────────

// setSimInput sets the Simulate Inputs value of port on the widget wid.
func (p *Project) setSimInput(wid uuid.UUID, port, value string) {
	if p.simInputs[wid] == nil {
		p.simInputs[wid] = make(map[string]string)
	}
	p.simInputs[wid][port] = value
}

// ── Widget selection helpers ──────────────────────────────────────────────────

func (p *Project) selectWidget(ctx app.Context, id uuid.UUID) {
	p.selectedWidgetID = id
	uiutil.StoreID(ctx, p.selectionKey(widgetIDKey), id)
	for _, w := range p.widgets {
		if w.ID == id {
			p.syncEditingFields(w)
			break
		}
	}
}

func (p *Project) clearWidgetSelection(ctx app.Context) {
	p.selectedWidgetID = uuid.Nil
	p.widgetEdit = widgetEditFields{}
	uiutil.StoreID(ctx, p.selectionKey(widgetIDKey), uuid.Nil)
}

// ── Lookups ───────────────────────────────────────────────────────────────────

func (p *Project) sceneIdx(id uuid.UUID) int { return uiutil.IdxByID(p.scenes, id, uiutil.SceneID) }

func (p *Project) findScene(id uuid.UUID) (apiv0.SceneResponse, bool) {
	return uiutil.FindByID(p.scenes, id, uiutil.SceneID)
}

func (p *Project) selectedWidgetIdx() int {
	return uiutil.IdxByID(p.widgets, p.selectedWidgetID, uiutil.WidgetID)
}

// findWidget returns the current live widget data by ID. Use it inside event
// handler closures instead of the closure-captured `w`, because go-app may
// not re-attach handlers when only the transform changes, leaving `w` stale
// across drags.
func (p *Project) findWidget(id uuid.UUID) (apiv0.WidgetResponse, bool) {
	return uiutil.FindByID(p.widgets, id, uiutil.WidgetID)
}

func (p *Project) widgetTypeByID(id uuid.UUID) (apiv0.WidgetTypeResponse, bool) {
	return uiutil.FindByID(p.widgetTypes, id, uiutil.WidgetTypeID)
}

func (p *Project) findTag(id uuid.UUID) (apiv0.TagResponse, bool) {
	return p.tags.Find(id)
}

// ── Render ────────────────────────────────────────────────────────────────────

func (p *Project) Render() app.UI {
	if p.content == nil {
		return uiutil.NoProject()
	}
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
			anyDrag := p.isInteracting() || p.resizingPanelSide != ""
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

			// Mark that a real drag occurred (not just a mousedown+mouseup click),
			// remembering the widget as it was before the first move.
			if id := p.draggedWidgetID(); id != uuid.Nil {
				if !p.dragDidMove {
					if w, ok := p.findWidget(id); ok {
						p.dragStartGeometry = geometryOf(w)
					}
				}
				p.dragDidMove = true
			}

			// Move widget body
			if p.draggingWidgetID != uuid.Nil {
				dx := clientX - p.dragStartCliX
				dy := clientY - p.dragStartCliY
				for i := range p.widgets {
					if p.widgets[i].ID == p.draggingWidgetID {
						p.widgets[i].Position.X = p.dragStartPosX + dx
						p.widgets[i].Position.Y = p.dragStartPosY + dy
						if p.selectedWidgetID == p.draggingWidgetID {
							f := widgetEditFieldsOf(p.widgets[i])
							p.widgetEdit.posX, p.widgetEdit.posY = f.posX, f.posY
						}
						break
					}
				}
			}

			// Move origin anchor; compensate position to keep widget visually stationary.
			// Screen delta (dx,dy) is rotated into widget-local space to get the local
			// origin delta; position is then compensated so the matrix e/f stays fixed.
			if p.draggingOriginID != uuid.Nil {
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
							f := widgetEditFieldsOf(p.widgets[i])
							p.widgetEdit.originX, p.widgetEdit.originY = f.originX, f.originY
							p.widgetEdit.posX, p.widgetEdit.posY = f.posX, f.posY
						}
						break
					}
				}
			}

			// Rotate widget around origin
			if p.rotatingWidgetID != uuid.Nil {
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
							p.widgetEdit.rotation = widgetEditFieldsOf(p.widgets[i]).rotation
						}
						break
					}
				}
			}

			// Resize widget via SE handle (inverse-rotation corrected).
			// After computing the new size we also recompute posX/posY so
			// that the canvas-space top-left corner (e, f) stays fixed,
			// preventing the widget from shifting when it is rotated and the
			// origin is not at (0, 0).
			if p.resizingWidgetID != uuid.Nil {
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
				// Compensate position so the top-left canvas corner is unchanged.
				newOx := p.resizeOriginX * float64(newW)
				newOy := p.resizeOriginY * float64(newH)
				newPosX := p.resizeStartE - newOx*(1-cosA) - sinA*newOy
				newPosY := p.resizeStartF + sinA*newOx - newOy*(1-cosA)
				for i := range p.widgets {
					if p.widgets[i].ID == p.resizingWidgetID {
						p.widgets[i].Size.Width = newW
						p.widgets[i].Size.Height = newH
						p.widgets[i].Position.X = newPosX
						p.widgets[i].Position.Y = newPosY
						if p.selectedWidgetID == p.resizingWidgetID {
							f := widgetEditFieldsOf(p.widgets[i])
							p.widgetEdit.width, p.widgetEdit.height = f.width, f.height
							p.widgetEdit.posX, p.widgetEdit.posY = f.posX, f.posY
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
