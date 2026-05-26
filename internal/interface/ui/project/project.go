package project

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

// ── DTOs ─────────────────────────────────────────────────────────────────────

type widgetTypeItem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DefaultWidth  int    `json:"default_width"`
	DefaultHeight int    `json:"default_height"`
}

type getWidgetTypesResponse struct {
	WidgetTypes []widgetTypeItem `json:"widget_types"`
}

type sceneItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	BackgroundHTML string `json:"background_html"`
}

type getScenesResponse struct {
	Scenes []sceneItem `json:"scenes"`
}

type createSceneRequest struct {
	Name           string `json:"name"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	BackgroundHTML string `json:"background_html"`
}

type createSceneResponse struct {
	ID string `json:"id"`
}

type updateSceneRequest struct {
	Name           string `json:"name"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	BackgroundHTML string `json:"background_html"`
}

// Widget geometry sub-DTOs (match server restdto shape).

type positionDTO struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z int     `json:"z"`
}

type sizeDTO struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type originDTO struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type rotationDTO struct {
	Degrees float64 `json:"degrees"`
}

type transformMatrixDTO struct {
	A   float64 `json:"a"`
	B   float64 `json:"b"`
	C   float64 `json:"c"`
	D   float64 `json:"d"`
	E   float64 `json:"e"`
	F   float64 `json:"f"`
	CSS string  `json:"css"`
}

type widgetItem struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	Position        positionDTO        `json:"position"`
	Size            sizeDTO            `json:"size"`
	Origin          originDTO          `json:"origin"`
	Rotation        rotationDTO        `json:"rotation"`
	TransformMatrix transformMatrixDTO `json:"transform_matrix"`
	TypeID          string             `json:"type_id"`
	SceneID         string             `json:"scene_id"`
	Labels          []string           `json:"labels"`
	TagIDs          []string           `json:"tag_ids"`
	Version         int                `json:"version"`
}

type getWidgetsResponse struct {
	Widgets []widgetItem `json:"widgets"`
}

type createWidgetRequest struct {
	Name     string      `json:"name"`
	Position positionDTO `json:"position"`
	Size     sizeDTO     `json:"size"`
	Origin   originDTO   `json:"origin"`
	Rotation rotationDTO `json:"rotation"`
	TypeID   string      `json:"type_id"`
	SceneID  string      `json:"scene_id"`
	Labels   []string    `json:"labels"`
	TagIDs   []string    `json:"tag_ids"`
}

type createWidgetResponse struct {
	ID string `json:"id"`
}

type updateWidgetRequest struct {
	Name     string      `json:"name"`
	Position positionDTO `json:"position"`
	Size     sizeDTO     `json:"size"`
	Origin   originDTO   `json:"origin"`
	Rotation rotationDTO `json:"rotation"`
	TypeID   string      `json:"type_id"`
	SceneID  string      `json:"scene_id"`
	Labels   []string    `json:"labels"`
	TagIDs   []string    `json:"tag_ids"`
	Version  int         `json:"version"`
}

type updateWidgetResponse struct {
	Version int `json:"version"`
}

type tagItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type getTagsResponse struct {
	Tags []tagItem `json:"tags"`
}

// ── Matrix helper ─────────────────────────────────────────────────────────────

// computeMatrixCSS calculates the CSS matrix() string from widget geometric
// properties. Mirrors the domain TransformationMatrix formula:
//
//	T(pos) · T(+ox,+oy) · R(θ) · T(-ox,-oy)
func computeMatrixCSS(posX, posY, originX, originY, rotDeg float64, width, height int) string {
	rad := rotDeg * math.Pi / 180
	cosA := math.Cos(rad)
	sinA := math.Sin(rad)
	ox := originX * float64(width)
	oy := originY * float64(height)
	e := posX - cosA*ox + sinA*oy + ox
	f := posY - sinA*ox - cosA*oy + oy
	return fmt.Sprintf("matrix(%.6f,%.6f,%.6f,%.6f,%.6f,%.6f)",
		cosA, sinA, -sinA, cosA, e, f)
}

func widgetMatrixCSS(w widgetItem) string {
	return computeMatrixCSS(
		w.Position.X, w.Position.Y,
		w.Origin.X, w.Origin.Y,
		w.Rotation.Degrees,
		w.Size.Width, w.Size.Height,
	)
}

// ── Component ─────────────────────────────────────────────────────────────────

type Project struct {
	app.Compo
	apiServerURL string

	widgetTypes []widgetTypeItem
	scenes      []sceneItem
	widgets     []widgetItem
	tags        []tagItem

	selectedSceneID  string
	selectedWidgetID string

	editingSceneID   string
	editingSceneName string

	draggedTypeID string

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
	draggingOriginID  string
	originCliX        float64 // client X when drag started
	originCliY        float64 // client Y when drag started
	originStartOX     float64 // origin.X before drag
	originStartOY     float64 // origin.Y before drag
	originStartPosX   float64 // position.X before drag
	originStartPosY   float64 // position.Y before drag
	originDragW       int     // widget width during drag
	originDragH       int     // widget height during drag
	originDragRotDeg  float64 // widget rotation during drag

	// ── Drag: rotation handle ───────────────────────────────────────────────
	// Dragging a circle positioned above the origin in local widget space.
	// We store the origin's client-space position and the initial angle so we
	// can compute the angular delta on every mousemove.
	rotatingWidgetID  string
	rotOriginCliX     float64 // origin screen position X (client coords)
	rotOriginCliY     float64 // origin screen position Y (client coords)
	rotStartAngle     float64 // atan2 from origin to handle when drag started
	rotStartDeg       float64 // rotation.Degrees before drag

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
	editingRotation   string
	addingTagID       string

	fetchErr string
}

func NewProject(apiServerURL string) *Project {
	return &Project{apiServerURL: apiServerURL}
}

func (p *Project) OnMount(ctx app.Context) {
	p.loadWidgetTypes(ctx)
	p.loadScenes(ctx)
	p.loadWidgets(ctx)
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

func (p *Project) loadScenes(ctx app.Context) {
	url := p.apiServerURL + "/api/v1/scenes"
	ctx.Async(func() {
		resp, err := http.Get(url)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		defer resp.Body.Close()
		var result getScenesResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		ctx.Dispatch(func(ctx app.Context) {
			p.scenes = result.Scenes
			if p.selectedSceneID == "" && len(result.Scenes) > 0 {
				p.selectedSceneID = result.Scenes[0].ID
			}
			found := false
			for _, sc := range result.Scenes {
				if sc.ID == p.selectedSceneID {
					found = true
					break
				}
			}
			if !found {
				p.selectedSceneID = ""
				p.clearWidgetSelection()
			}
		})
	})
}

func (p *Project) loadWidgets(ctx app.Context) {
	url := p.apiServerURL + "/api/v1/widgets"
	ctx.Async(func() {
		resp, err := http.Get(url)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		defer resp.Body.Close()
		var result getWidgetsResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		ctx.Dispatch(func(ctx app.Context) {
			p.widgets = result.Widgets
			// Re-sync editing fields if selected widget was refreshed
			if p.selectedWidgetID != "" {
				for _, w := range p.widgets {
					if w.ID == p.selectedWidgetID {
						p.syncEditingFields(w)
						break
					}
				}
			}
		})
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
		ctx.Dispatch(func(ctx app.Context) { p.tags = result.Tags })
	})
}

// ── Widget selection helpers ──────────────────────────────────────────────────

func (p *Project) selectWidget(id string) {
	p.selectedWidgetID = id
	p.addingTagID = ""
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

func (p *Project) clearWidgetSelection() {
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
	p.addingTagID = ""
}

func (p *Project) selectedWidgetIdx() int {
	for i, w := range p.widgets {
		if w.ID == p.selectedWidgetID {
			return i
		}
	}
	return -1
}

// ── Scene operations ───────────────────────────────────────────────────────────

func (p *Project) createScene(ctx app.Context) {
	url := p.apiServerURL + "/api/v1/scenes"
	body, _ := json.Marshal(createSceneRequest{Name: "New Scene", Width: 1920, Height: 1080})
	ctx.Async(func() {
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		defer resp.Body.Close()
		var result createSceneResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		ctx.Dispatch(func(ctx app.Context) {
			p.selectedSceneID = result.ID
			p.loadScenes(ctx)
		})
	})
}

func (p *Project) deleteScene(ctx app.Context, sceneID string) {
	url := p.apiServerURL + "/api/v1/scenes/" + sceneID
	ctx.Async(func() {
		req, _ := http.NewRequest(http.MethodDelete, url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		resp.Body.Close()
		ctx.Dispatch(func(ctx app.Context) {
			p.loadScenes(ctx)
			p.loadWidgets(ctx)
		})
	})
}

func (p *Project) startEditingScene(id, name string) {
	p.editingSceneID = id
	p.editingSceneName = name
}

func (p *Project) commitSceneEdit(ctx app.Context) {
	if p.editingSceneID == "" {
		return
	}
	id := p.editingSceneID
	name := p.editingSceneName
	p.editingSceneID = ""
	p.editingSceneName = ""

	var sc sceneItem
	for _, s := range p.scenes {
		if s.ID == id {
			sc = s
			break
		}
	}
	if sc.ID == "" {
		return
	}
	for i, s := range p.scenes {
		if s.ID == id {
			p.scenes[i].Name = name
			break
		}
	}
	url := p.apiServerURL + "/api/v1/scenes/" + id
	body, _ := json.Marshal(updateSceneRequest{
		Name:           name,
		Width:          sc.Width,
		Height:         sc.Height,
		BackgroundHTML: sc.BackgroundHTML,
	})
	ctx.Async(func() {
		req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		resp.Body.Close()
		ctx.Dispatch(func(ctx app.Context) { p.fetchErr = "" })
	})
}

// ── Widget operations ──────────────────────────────────────────────────────────

func (p *Project) createWidget(ctx app.Context, typeID string, x, y float64) {
	if p.selectedSceneID == "" {
		return
	}
	name := "Widget"
	width, height := 120, 60
	for _, wt := range p.widgetTypes {
		if wt.ID == typeID {
			name = wt.Name
			if wt.DefaultWidth > 0 {
				width = wt.DefaultWidth
			}
			if wt.DefaultHeight > 0 {
				height = wt.DefaultHeight
			}
			break
		}
	}
	url := p.apiServerURL + "/api/v1/widgets"
	body, _ := json.Marshal(createWidgetRequest{
		Name:     name,
		Position: positionDTO{X: x, Y: y, Z: 0},
		Size:     sizeDTO{Width: width, Height: height},
		Origin:   originDTO{X: 0.5, Y: 0.5},
		Rotation: rotationDTO{Degrees: 0},
		TypeID:   typeID,
		SceneID:  p.selectedSceneID,
		Labels:   []string{},
		TagIDs:   []string{},
	})
	ctx.Async(func() {
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		defer resp.Body.Close()
		var result createWidgetResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		ctx.Dispatch(func(ctx app.Context) {
			p.selectedWidgetID = result.ID
			// Pre-fill editing fields optimistically
			p.editingWidgetName = name
			p.editingPosX = fmt.Sprintf("%.1f", x)
			p.editingPosY = fmt.Sprintf("%.1f", y)
			p.editingPosZ = "0"
			p.editingWidth = strconv.Itoa(width)
			p.editingHeight = strconv.Itoa(height)
			p.editingOriginX = "0.500"
			p.editingOriginY = "0.500"
			p.editingRotation = "0.0"
			p.loadWidgets(ctx)
		})
	})
}

func (p *Project) deleteWidget(ctx app.Context, widgetID string) {
	url := p.apiServerURL + "/api/v1/widgets/" + widgetID
	ctx.Async(func() {
		req, _ := http.NewRequest(http.MethodDelete, url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		resp.Body.Close()
		ctx.Dispatch(func(ctx app.Context) {
			if p.selectedWidgetID == widgetID {
				p.clearWidgetSelection()
			}
			p.loadWidgets(ctx)
		})
	})
}

func (p *Project) saveWidgetName(ctx app.Context) {
	idx := p.selectedWidgetIdx()
	if idx < 0 {
		return
	}
	p.widgets[idx].Name = p.editingWidgetName
	p.putWidget(ctx, p.widgets[idx])
}

// saveWidgetGeometry parses all geometry editing fields and PUTs the widget.
func (p *Project) saveWidgetGeometry(ctx app.Context) {
	idx := p.selectedWidgetIdx()
	if idx < 0 {
		return
	}
	posX, _ := strconv.ParseFloat(p.editingPosX, 64)
	posY, _ := strconv.ParseFloat(p.editingPosY, 64)
	posZ, _ := strconv.Atoi(p.editingPosZ)
	width, _ := strconv.Atoi(p.editingWidth)
	height, _ := strconv.Atoi(p.editingHeight)
	originX, _ := strconv.ParseFloat(p.editingOriginX, 64)
	originY, _ := strconv.ParseFloat(p.editingOriginY, 64)
	rotDeg, _ := strconv.ParseFloat(p.editingRotation, 64)

	if width <= 0 {
		width = 10
	}
	if height <= 0 {
		height = 10
	}
	originX = math.Max(0, math.Min(1, originX))
	originY = math.Max(0, math.Min(1, originY))

	p.widgets[idx].Position = positionDTO{X: posX, Y: posY, Z: posZ}
	p.widgets[idx].Size = sizeDTO{Width: width, Height: height}
	p.widgets[idx].Origin = originDTO{X: originX, Y: originY}
	p.widgets[idx].Rotation = rotationDTO{Degrees: rotDeg}
	p.putWidget(ctx, p.widgets[idx])
}

func (p *Project) addTagToWidget(ctx app.Context, tagID string) {
	if p.selectedWidgetID == "" || tagID == "" {
		return
	}
	idx := p.selectedWidgetIdx()
	if idx < 0 {
		return
	}
	for _, tid := range p.widgets[idx].TagIDs {
		if tid == tagID {
			return
		}
	}
	p.widgets[idx].TagIDs = append(p.widgets[idx].TagIDs, tagID)
	p.addingTagID = ""
	p.putWidget(ctx, p.widgets[idx])
}

func (p *Project) removeTagFromWidget(ctx app.Context, tagID string) {
	if p.selectedWidgetID == "" {
		return
	}
	idx := p.selectedWidgetIdx()
	if idx < 0 {
		return
	}
	filtered := make([]string, 0, len(p.widgets[idx].TagIDs))
	for _, tid := range p.widgets[idx].TagIDs {
		if tid != tagID {
			filtered = append(filtered, tid)
		}
	}
	p.widgets[idx].TagIDs = filtered
	p.putWidget(ctx, p.widgets[idx])
}

func (p *Project) putWidget(ctx app.Context, w widgetItem) {
	url := p.apiServerURL + "/api/v1/widgets/" + w.ID
	wid := w.ID
	tagIDs := w.TagIDs
	if tagIDs == nil {
		tagIDs = []string{}
	}
	labels := w.Labels
	if labels == nil {
		labels = []string{}
	}
	body, _ := json.Marshal(updateWidgetRequest{
		Name:     w.Name,
		Position: w.Position,
		Size:     w.Size,
		Origin:   w.Origin,
		Rotation: w.Rotation,
		TypeID:   w.TypeID,
		SceneID:  w.SceneID,
		Labels:   labels,
		TagIDs:   tagIDs,
		Version:  w.Version,
	})
	ctx.Async(func() {
		req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) { p.fetchErr = err.Error() })
			return
		}
		defer resp.Body.Close()
		var result updateWidgetResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && result.Version > 0 {
			ctx.Dispatch(func(ctx app.Context) {
				for i := range p.widgets {
					if p.widgets[i].ID == wid {
						p.widgets[i].Version = result.Version
						break
					}
				}
				p.fetchErr = ""
			})
		}
	})
}

// ── Drag finalisation ─────────────────────────────────────────────────────────

// finalizeAllDrags saves any active drag operation to the server.
func (p *Project) finalizeAllDrags(ctx app.Context) {
	if id := p.draggingWidgetID; id != "" {
		p.draggingWidgetID = ""
		p.saveDraggedWidget(ctx, id)
	}
	if id := p.draggingOriginID; id != "" {
		p.draggingOriginID = ""
		p.saveDraggedWidget(ctx, id)
	}
	if id := p.rotatingWidgetID; id != "" {
		p.rotatingWidgetID = ""
		p.saveDraggedWidget(ctx, id)
	}
	if id := p.resizingWidgetID; id != "" {
		p.resizingWidgetID = ""
		p.saveDraggedWidget(ctx, id)
	}
}

func (p *Project) saveDraggedWidget(ctx app.Context, id string) {
	for i := range p.widgets {
		if p.widgets[i].ID == id {
			if p.selectedWidgetID == id {
				p.syncEditingFields(p.widgets[i])
			}
			p.putWidget(ctx, p.widgets[i])
			break
		}
	}
}

// ── Render ────────────────────────────────────────────────────────────────────

func (p *Project) Render() app.UI {
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "row").
		Style("flex", "1").
		Style("min-height", "0").
		Style("overflow", "hidden").
		Style("gap", "0").
		Body(
			p.renderWidgetTypePanel(),
			p.renderScenePanel(),
			p.renderPropertiesPanel(),
		)
}

// ── Left panel: Widget Types ───────────────────────────────────────────────────

func (p *Project) renderWidgetTypePanel() app.UI {
	items := make([]app.UI, len(p.widgetTypes))
	for i, wt := range p.widgetTypes {
		id := wt.ID
		name := wt.Name
		w, h := wt.DefaultWidth, wt.DefaultHeight
		if w <= 0 {
			w = 100
		}
		if h <= 0 {
			h = 100
		}
		items[i] = app.Div().
			Style("padding", "7px 10px").
			Style("margin-bottom", "4px").
			Style("border", "1px solid #d0d0d0").
			Style("border-radius", "4px").
			Style("background", "#f5f5f5").
			Style("cursor", "grab").
			Style("user-select", "none").
			Body(
				app.Div().
					Style("font-size", "13px").
					Style("white-space", "nowrap").
					Style("overflow", "hidden").
					Style("text-overflow", "ellipsis").
					Text(name),
				app.Div().
					Style("font-size", "11px").
					Style("color", "#888").
					Style("margin-top", "2px").
					Text(fmt.Sprintf("%d × %d", w, h)),
			).
			Draggable(true).
			OnDragStart(func(ctx app.Context, e app.Event) {
				p.draggedTypeID = id
				e.Get("dataTransfer").Call("setData", "text/plain", id)
				e.Get("dataTransfer").Set("effectAllowed", "copy")
			})
	}

	var listBody app.UI
	if len(items) == 0 {
		listBody = app.Div().
			Style("font-size", "13px").
			Style("color", "#999").
			Text("No widget types. Create them in the Library tab.")
	} else {
		listBody = app.Div().Body(items...)
	}

	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("width", "180px").
		Style("flex-shrink", "0").
		Style("min-height", "0").
		Style("border-right", "1px solid #ddd").
		Style("padding", "12px 8px").
		Body(
			app.H3().
				Style("margin", "0 0 10px 0").
				Style("font-size", "13px").
				Style("font-weight", "600").
				Style("color", "#333").
				Style("text-transform", "uppercase").
				Style("letter-spacing", "0.5px").
				Text("Widget Types"),
			app.Div().
				Style("flex", "1").
				Style("overflow-y", "auto").
				Body(listBody),
		)
}

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
		bg, borderBottom, color, fontWeight := "#f0f0f0", "2px solid transparent", "#555", "normal"
		if active {
			bg, borderBottom, color, fontWeight = "#fff", "2px solid #0066cc", "#0066cc", "600"
		}

		var tabInner app.UI
		if p.editingSceneID == sc.ID {
			tabInner = app.Input().
				Type("text").
				Value(p.editingSceneName).
				AutoFocus(true).
				Style("font-size", "13px").
				Style("padding", "1px 4px").
				Style("border", "1px solid #0066cc").
				Style("border-radius", "2px").
				Style("width", "90px").
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
							p.clearWidgetSelection()
						}).
						OnDblClick(func(ctx app.Context, e app.Event) {
							p.startEditingScene(sc.ID, sc.Name)
						}),
					app.Span().
						Style("cursor", "pointer").
						Style("font-size", "14px").
						Style("line-height", "1").
						Style("color", "#aaa").
						Style("padding", "0 1px").
						Text("×").
						OnClick(func(ctx app.Context, e app.Event) {
							e.Call("stopPropagation")
							p.deleteScene(ctx, sc.ID)
						}),
				)
		}

		tabs = append(tabs, app.Div().
			Style("display", "flex").
			Style("align-items", "center").
			Style("padding", "6px 14px").
			Style("background", bg).
			Style("border-right", "1px solid #ddd").
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
		Style("color", "#0066cc").
		Style("line-height", "1").
		Text("+").
		OnClick(func(ctx app.Context, e app.Event) { p.createScene(ctx) }),
	)

	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "row").
		Style("flex-shrink", "0").
		Style("border-bottom", "1px solid #ddd").
		Style("background", "#f8f8f8").
		Body(tabs...)
}

func (p *Project) renderSceneCanvas() app.UI {
	if p.selectedSceneID == "" {
		return app.Div().
			Style("flex", "1").
			Style("display", "flex").
			Style("align-items", "center").
			Style("justify-content", "center").
			Style("color", "#aaa").
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
		if w.SceneID != p.selectedSceneID {
			continue
		}
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
		Style("background-color", "#fafafa").
		Style("background-image", "radial-gradient(circle, #ccc 1px, transparent 1px)").
		Style("background-size", "24px 24px").
		Style("flex-shrink", "0").
		Style("cursor", canvasCursor).
		// ── Mouse move: handle all drag modes ──────────────────────────────
		OnMouseMove(func(ctx app.Context, e app.Event) {
			anyDrag := p.draggingWidgetID != "" ||
				p.draggingOriginID != "" ||
				p.rotatingWidgetID != "" ||
				p.resizingWidgetID != ""
			if !anyDrag {
				return
			}
			e.PreventDefault()

			clientX := e.Get("clientX").Float()
			clientY := e.Get("clientY").Float()

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

			// Move origin anchor within widget (adjust position to keep content fixed)
			if p.draggingOriginID != "" {
				dx := clientX - p.originCliX
				dy := clientY - p.originCliY
				rad := p.originDragRotDeg * math.Pi / 180
				cosA := math.Cos(rad)
				sinA := math.Sin(rad)
				// Transform screen delta → local widget space (inverse rotation)
				dxLocal := dx*cosA + dy*sinA
				dyLocal := -dx*sinA + dy*cosA
				newOX := math.Max(0, math.Min(1, p.originStartOX+dxLocal/float64(p.originDragW)))
				newOY := math.Max(0, math.Min(1, p.originStartOY+dyLocal/float64(p.originDragH)))
				// Keep visual widget bounding-box in place by adjusting position
				newPosX := p.originStartPosX + (p.originStartOX-newOX)*float64(p.originDragW)
				newPosY := p.originStartPosY + (p.originStartOY-newOY)*float64(p.originDragH)
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
				newW := p.resizeStartW + int(dxLocal)
				newH := p.resizeStartH + int(dyLocal)
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
			p.finalizeAllDrags(ctx)
		}).
		OnMouseLeave(func(ctx app.Context, e app.Event) {
			p.finalizeAllDrags(ctx)
		}).
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
			p.clearWidgetSelection()
		}).
		Body(widgetEls...)

	return app.Div().
		Style("flex", "1").
		Style("min-height", "0").
		Style("overflow", "auto").
		Style("background", "#e8e8e8").
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
	border := "1.5px solid #bbb"
	bg := "rgba(240,240,240,0.85)"
	if isSelected {
		border = "2px solid #0066cc"
		bg = "rgba(235,245,255,0.92)"
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
				Text(w.Name),
		).
		OnMouseDown(func(ctx app.Context, e app.Event) {
			e.Call("stopPropagation")
			e.PreventDefault()
			p.selectWidget(wid)
			p.draggingWidgetID = wid
			p.dragStartCliX = e.Get("clientX").Float()
			p.dragStartCliY = e.Get("clientY").Float()
			p.dragStartPosX = w.Position.X
			p.dragStartPosY = w.Position.Y
		}).
		OnClick(func(ctx app.Context, e app.Event) {
			e.Call("stopPropagation")
		})

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
			Style("background", "#0066cc").
			Style("pointer-events", "none").
			Style("z-index", "1")

		// Rotation handle circle (above the origin in local space)
		rotHandle := app.Div().
			Style("position", "absolute").
			Style("left", fmt.Sprintf("%.1fpx", ox-8)).
			Style("top", fmt.Sprintf("%.1fpx", oy-rotHandleDist-8)).
			Style("width", "16px").
			Style("height", "16px").
			Style("background", "#0066cc").
			Style("border", "2px solid #fff").
			Style("border-radius", "50%").
			Style("cursor", "alias").
			Style("z-index", "3").
			Style("box-shadow", "0 1px 3px rgba(0,0,0,0.3)").
			OnMouseDown(func(ctx app.Context, e app.Event) {
				e.Call("stopPropagation")
				e.PreventDefault()

				rad := w.Rotation.Degrees * math.Pi / 180
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
				p.rotStartDeg = w.Rotation.Degrees
				p.rotatingWidgetID = wid
			})

		// Origin handle: orange circle at anchor point
		originHandle := app.Div().
			Style("position", "absolute").
			Style("left", fmt.Sprintf("%.1fpx", ox-7)).
			Style("top", fmt.Sprintf("%.1fpx", oy-7)).
			Style("width", "14px").
			Style("height", "14px").
			Style("background", "#ff8c00").
			Style("border", "2px solid #fff").
			Style("border-radius", "50%").
			Style("cursor", "crosshair").
			Style("z-index", "3").
			Style("box-shadow", "0 1px 3px rgba(0,0,0,0.3)").
			OnMouseDown(func(ctx app.Context, e app.Event) {
				e.Call("stopPropagation")
				e.PreventDefault()
				p.selectWidget(wid)
				p.draggingOriginID = wid
				p.originCliX = e.Get("clientX").Float()
				p.originCliY = e.Get("clientY").Float()
				p.originStartOX = w.Origin.X
				p.originStartOY = w.Origin.Y
				p.originStartPosX = w.Position.X
				p.originStartPosY = w.Position.Y
				p.originDragW = w.Size.Width
				p.originDragH = w.Size.Height
				p.originDragRotDeg = w.Rotation.Degrees
			})

		// SE resize handle: small square at bottom-right corner
		resizeHandle := app.Div().
			Style("position", "absolute").
			Style("right", "0").
			Style("bottom", "0").
			Style("width", "12px").
			Style("height", "12px").
			Style("background", "#0066cc").
			Style("border", "2px solid #fff").
			Style("border-radius", "3px 0 4px 0").
			Style("cursor", "se-resize").
			Style("z-index", "3").
			OnMouseDown(func(ctx app.Context, e app.Event) {
				e.Call("stopPropagation")
				e.PreventDefault()
				p.resizingWidgetID = wid
				p.resizeStartCliX = e.Get("clientX").Float()
				p.resizeStartCliY = e.Get("clientY").Float()
				p.resizeStartW = w.Size.Width
				p.resizeStartH = w.Size.Height
				p.resizeStartDeg = w.Rotation.Degrees
			})

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
		Body(bodyItems...)
}

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
