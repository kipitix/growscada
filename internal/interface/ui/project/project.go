package project

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

// ── DTOs ────────────────────────────────────────────────────────────────────

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

type coordinatesItem struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type widgetItem struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Coordinates coordinatesItem `json:"coordinates"`
	Width       int             `json:"width"`
	Height      int             `json:"height"`
	TypeID      string          `json:"type_id"`
	SceneID     string          `json:"scene_id"`
	Labels      []string        `json:"labels"`
	TagIDs      []string        `json:"tag_ids"`
}

type getWidgetsResponse struct {
	Widgets []widgetItem `json:"widgets"`
}

type createWidgetRequest struct {
	Name        string          `json:"name"`
	Coordinates coordinatesItem `json:"coordinates"`
	Width       int             `json:"width"`
	Height      int             `json:"height"`
	TypeID      string          `json:"type_id"`
	SceneID     string          `json:"scene_id"`
	Labels      []string        `json:"labels"`
	TagIDs      []string        `json:"tag_ids"`
}

type createWidgetResponse struct {
	ID string `json:"id"`
}

type updateWidgetRequest struct {
	Name        string          `json:"name"`
	Coordinates coordinatesItem `json:"coordinates"`
	Width       int             `json:"width"`
	Height      int             `json:"height"`
	TypeID      string          `json:"type_id"`
	SceneID     string          `json:"scene_id"`
	Labels      []string        `json:"labels"`
	TagIDs      []string        `json:"tag_ids"`
}

type tagItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type getTagsResponse struct {
	Tags []tagItem `json:"tags"`
}

// ── Component ────────────────────────────────────────────────────────────────

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

	editingWidgetName   string
	editingWidgetWidth  string
	editingWidgetHeight string
	addingTagID         string

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

// ── Data loading ─────────────────────────────────────────────────────────────

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
		ctx.Dispatch(func(ctx app.Context) {
			p.widgetTypes = result.WidgetTypes
		})
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
			// auto-select first scene if none selected
			if p.selectedSceneID == "" && len(result.Scenes) > 0 {
				p.selectedSceneID = result.Scenes[0].ID
			}
			// deselect if scene was deleted
			found := false
			for _, sc := range result.Scenes {
				if sc.ID == p.selectedSceneID {
					found = true
					break
				}
			}
			if !found {
				p.selectedSceneID = ""
				p.selectedWidgetID = ""
				p.editingWidgetName = ""
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
		ctx.Dispatch(func(ctx app.Context) {
			p.tags = result.Tags
		})
	})
}

// ── Scene operations ──────────────────────────────────────────────────────────

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

	// optimistic local update
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

// ── Widget operations ─────────────────────────────────────────────────────────

func (p *Project) createWidget(ctx app.Context, typeID string, x, y float64) {
	if p.selectedSceneID == "" {
		return
	}
	name := "Widget"
	width, height := 100, 100
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
		Name:        name,
		Coordinates: coordinatesItem{X: x, Y: y},
		Width:       width,
		Height:      height,
		TypeID:      typeID,
		SceneID:     p.selectedSceneID,
		Labels:      []string{},
		TagIDs:      []string{},
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
			p.editingWidgetName = name
			p.editingWidgetWidth = strconv.Itoa(width)
			p.editingWidgetHeight = strconv.Itoa(height)
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
				p.selectedWidgetID = ""
				p.editingWidgetName = ""
				p.editingWidgetWidth = ""
				p.editingWidgetHeight = ""
			}
			p.loadWidgets(ctx)
		})
	})
}

func (p *Project) selectWidget(id string) {
	p.selectedWidgetID = id
	p.addingTagID = ""
	for _, w := range p.widgets {
		if w.ID == id {
			p.editingWidgetName = w.Name
			p.editingWidgetWidth = strconv.Itoa(w.Width)
			p.editingWidgetHeight = strconv.Itoa(w.Height)
			break
		}
	}
}

func (p *Project) saveWidgetName(ctx app.Context) {
	if p.selectedWidgetID == "" {
		return
	}
	idx := p.selectedWidgetIdx()
	if idx < 0 {
		return
	}
	p.widgets[idx].Name = p.editingWidgetName
	p.putWidget(ctx, p.widgets[idx])
}

func (p *Project) saveWidgetSize(ctx app.Context) {
	if p.selectedWidgetID == "" {
		return
	}
	idx := p.selectedWidgetIdx()
	if idx < 0 {
		return
	}
	w, err := strconv.Atoi(p.editingWidgetWidth)
	if err != nil || w <= 0 {
		return
	}
	h, err := strconv.Atoi(p.editingWidgetHeight)
	if err != nil || h <= 0 {
		return
	}
	p.widgets[idx].Width = w
	p.widgets[idx].Height = h
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

func (p *Project) selectedWidgetIdx() int {
	for i, w := range p.widgets {
		if w.ID == p.selectedWidgetID {
			return i
		}
	}
	return -1
}

func (p *Project) putWidget(ctx app.Context, w widgetItem) {
	url := p.apiServerURL + "/api/v1/widgets/" + w.ID
	tagIDs := w.TagIDs
	if tagIDs == nil {
		tagIDs = []string{}
	}
	labels := w.Labels
	if labels == nil {
		labels = []string{}
	}
	body, _ := json.Marshal(updateWidgetRequest{
		Name:        w.Name,
		Coordinates: w.Coordinates,
		Width:       w.Width,
		Height:      w.Height,
		TypeID:      w.TypeID,
		SceneID:     w.SceneID,
		Labels:      labels,
		TagIDs:      tagIDs,
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

// ── Left panel: Widget Types ──────────────────────────────────────────────────

func (p *Project) renderWidgetTypePanel() app.UI {
	items := make([]app.UI, len(p.widgetTypes))
	for i, wt := range p.widgetTypes {
		id := wt.ID
		name := wt.Name
		w := wt.DefaultWidth
		h := wt.DefaultHeight
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

// ── Center panel: Scene ───────────────────────────────────────────────────────

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

		bg := "#f0f0f0"
		borderBottom := "2px solid transparent"
		color := "#555"
		fontWeight := "normal"
		if active {
			bg = "#fff"
			borderBottom = "2px solid #0066cc"
			color = "#0066cc"
			fontWeight = "600"
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
							p.selectedWidgetID = ""
							p.editingWidgetName = ""
							p.editingWidgetWidth = ""
							p.editingWidgetHeight = ""
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
		OnClick(func(ctx app.Context, e app.Event) {
			p.createScene(ctx)
		}),
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

	widgetEls := make([]app.UI, 0)
	for _, w := range p.widgets {
		if w.SceneID != p.selectedSceneID {
			continue
		}
		wid := w.ID
		wname := w.Name
		wx := w.Coordinates.X
		wy := w.Coordinates.Y
		ww := w.Width
		wh := w.Height
		if ww <= 0 {
			ww = 100
		}
		if wh <= 0 {
			wh = 100
		}
		isSelected := p.selectedWidgetID == wid

		border := "1px solid #bbb"
		shadow := "0 1px 3px rgba(0,0,0,0.15)"
		bg := "#fff"
		if isSelected {
			border = "2px solid #0066cc"
			shadow = "0 0 0 3px rgba(0,102,204,0.2)"
			bg = "#f0f5ff"
		}

		widgetEls = append(widgetEls, app.Div().
			Style("position", "absolute").
			Style("left", fmt.Sprintf("%.0fpx", wx)).
			Style("top", fmt.Sprintf("%.0fpx", wy)).
			Style("width", fmt.Sprintf("%dpx", ww)).
			Style("height", fmt.Sprintf("%dpx", wh)).
			Style("background", bg).
			Style("border", border).
			Style("border-radius", "4px").
			Style("font-size", "12px").
			Style("cursor", "pointer").
			Style("user-select", "none").
			Style("box-shadow", shadow).
			Style("display", "flex").
			Style("align-items", "center").
			Style("justify-content", "center").
			Style("overflow", "hidden").
			Style("padding", "0 6px").
			Style("box-sizing", "border-box").
			Body(
				app.Span().
					Style("white-space", "nowrap").
					Style("text-overflow", "ellipsis").
					Style("overflow", "hidden").
					Text(wname),
			).
			OnClick(func(ctx app.Context, e app.Event) {
				e.Call("stopPropagation")
				p.selectWidget(wid)
			}),
		)
	}

	canvas := app.Div().
		Style("position", "relative").
		Style("width", fmt.Sprintf("%dpx", sceneWidth)).
		Style("height", fmt.Sprintf("%dpx", sceneHeight)).
		Style("background-color", "#fafafa").
		Style("background-image", "radial-gradient(circle, #ccc 1px, transparent 1px)").
		Style("background-size", "24px 24px").
		Style("flex-shrink", "0").
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
			p.selectedWidgetID = ""
			p.editingWidgetName = ""
			p.editingWidgetWidth = ""
			p.editingWidgetHeight = ""
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

// ── Right panel: Properties ───────────────────────────────────────────────────

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
		Style("width", "240px").
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
	tagNameOf := func(id string) string {
		for _, t := range p.tags {
			if t.ID == id {
				return t.Name
			}
		}
		return id
	}

	// current tag rows
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
					Style("line-height", "1").
					Style("padding", "0 2px").
					Text("×").
					OnClick(func(ctx app.Context, e app.Event) {
						p.removeTagFromWidget(ctx, tid)
					}),
			)
	}

	// available tags (not yet assigned)
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

	wid := w.ID

	return app.Div().Body(
		// Name
		app.Div().
			Style("margin-bottom", "16px").
			Body(
				app.Div().
					Style("font-size", "11px").
					Style("font-weight", "600").
					Style("color", "#666").
					Style("margin-bottom", "4px").
					Text("NAME"),
				app.Div().
					Style("display", "flex").
					Style("gap", "4px").
					Body(
						app.Input().
							Type("text").
							Value(p.editingWidgetName).
							Style("flex", "1").
							Style("min-width", "0").
							Style("font-size", "13px").
							Style("padding", "4px 6px").
							Style("border", "1px solid #ccc").
							Style("border-radius", "3px").
							OnInput(func(ctx app.Context, e app.Event) {
								p.editingWidgetName = ctx.JSSrc().Get("value").String()
							}).
							OnKeyDown(func(ctx app.Context, e app.Event) {
								if e.Get("key").String() == "Enter" {
									p.saveWidgetName(ctx)
								}
							}),
						app.Button().
							Style("font-size", "12px").
							Style("padding", "4px 8px").
							Style("cursor", "pointer").
							Style("border", "1px solid #ccc").
							Style("border-radius", "3px").
							Style("background", "#f5f5f5").
							Text("Save").
							OnClick(func(ctx app.Context, e app.Event) {
								p.saveWidgetName(ctx)
							}),
					),
			),

		// Size
		app.Div().
			Style("margin-bottom", "16px").
			Body(
				app.Div().
					Style("font-size", "11px").
					Style("font-weight", "600").
					Style("color", "#666").
					Style("margin-bottom", "4px").
					Text("SIZE (px)"),
				app.Div().
					Style("display", "flex").
					Style("gap", "4px").
					Style("align-items", "center").
					Body(
						app.Input().
							Type("number").
							Value(p.editingWidgetWidth).
							Style("width", "60px").
							Style("font-size", "13px").
							Style("padding", "4px 6px").
							Style("border", "1px solid #ccc").
							Style("border-radius", "3px").
							Style("text-align", "center").
							OnInput(func(ctx app.Context, e app.Event) {
								p.editingWidgetWidth = ctx.JSSrc().Get("value").String()
							}),
						app.Span().
							Style("font-size", "12px").
							Style("color", "#888").
							Text("×"),
						app.Input().
							Type("number").
							Value(p.editingWidgetHeight).
							Style("width", "60px").
							Style("font-size", "13px").
							Style("padding", "4px 6px").
							Style("border", "1px solid #ccc").
							Style("border-radius", "3px").
							Style("text-align", "center").
							OnInput(func(ctx app.Context, e app.Event) {
								p.editingWidgetHeight = ctx.JSSrc().Get("value").String()
							}),
						app.Button().
							Style("font-size", "12px").
							Style("padding", "4px 8px").
							Style("cursor", "pointer").
							Style("border", "1px solid #ccc").
							Style("border-radius", "3px").
							Style("background", "#f5f5f5").
							Text("Apply").
							OnClick(func(ctx app.Context, e app.Event) {
								p.saveWidgetSize(ctx)
							}),
					),
			),

		// Tags
		app.Div().Body(
			app.Div().
				Style("font-size", "11px").
				Style("font-weight", "600").
				Style("color", "#666").
				Style("margin-bottom", "6px").
				Text("TAGS"),
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
		),

		// Delete
		app.Div().
			Style("margin-top", "20px").
			Style("padding-top", "16px").
			Style("border-top", "1px solid #eee").
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
			),
	)
}
