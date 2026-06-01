package project

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

// ── Widget data loading ───────────────────────────────────────────────────────

func (p *Project) loadWidgets(ctx app.Context) {
	sceneID := p.selectedSceneID
	if sceneID == "" {
		ctx.Dispatch(func(ctx app.Context) { p.widgets = nil })
		return
	}
	url := p.apiServerURL + "/api/v1/scenes/" + sceneID + "/widgets"
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
			// Discard the response if the user has switched to a different scene
			// while the request was in flight.
			if p.selectedSceneID != sceneID {
				return
			}
			p.widgets = result.Widgets
			// Re-sync editing fields if selected widget was refreshed;
			// clear selection if it no longer exists in this scene.
			if p.selectedWidgetID != "" {
				found := false
				for _, w := range p.widgets {
					if w.ID == p.selectedWidgetID {
						p.syncEditingFields(w)
						found = true
						break
					}
				}
				if !found {
					p.clearWidgetSelection(ctx)
				}
			}
		})
	})
}

// ── Widget CRUD ───────────────────────────────────────────────────────────────

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
		Name:         name,
		Position:     positionDTO{X: x, Y: y, Z: 0},
		Size:         sizeDTO{Width: width, Height: height},
		Origin:       originDTO{X: 0.5, Y: 0.5},
		Rotation:     rotationDTO{Degrees: 0},
		TypeID:       typeID,
		SceneID:      p.selectedSceneID,
		Labels:       []string{},
		PortBindings: []portBindingDTO{},
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
			ctx.LocalStorage().Set("project:widgetID", result.ID)
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
				p.clearWidgetSelection(ctx)
			}
			filtered := p.widgets[:0]
			for _, w := range p.widgets {
				if w.ID != widgetID {
					filtered = append(filtered, w)
				}
			}
			p.widgets = filtered
		})
	})
}

func (p *Project) saveWidgetName(ctx app.Context) {
	if strings.TrimSpace(p.editingWidgetName) == "" {
		return
	}
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

func (p *Project) bindPort(ctx app.Context, portName, tagID string) {
	if p.selectedWidgetID == "" || portName == "" || tagID == "" {
		return
	}
	idx := p.selectedWidgetIdx()
	if idx < 0 {
		return
	}
	// Replace existing binding for this port, or append new one.
	found := false
	for i, b := range p.widgets[idx].PortBindings {
		if b.PortName == portName {
			p.widgets[idx].PortBindings[i].TagID = tagID
			found = true
			break
		}
	}
	if !found {
		p.widgets[idx].PortBindings = append(p.widgets[idx].PortBindings, portBindingDTO{
			PortName: portName,
			TagID:    tagID,
		})
	}
	p.putWidget(ctx, p.widgets[idx])
}

func (p *Project) unbindPort(ctx app.Context, portName string) {
	if p.selectedWidgetID == "" {
		return
	}
	idx := p.selectedWidgetIdx()
	if idx < 0 {
		return
	}
	filtered := make([]portBindingDTO, 0, len(p.widgets[idx].PortBindings))
	for _, b := range p.widgets[idx].PortBindings {
		if b.PortName != portName {
			filtered = append(filtered, b)
		}
	}
	p.widgets[idx].PortBindings = filtered
	p.putWidget(ctx, p.widgets[idx])
}

func (p *Project) putWidget(ctx app.Context, w widgetItem) {
	url := p.apiServerURL + "/api/v1/widgets/" + w.ID
	wid := w.ID
	portBindings := w.PortBindings
	if portBindings == nil {
		portBindings = []portBindingDTO{}
	}
	labels := w.Labels
	if labels == nil {
		labels = []string{}
	}
	body, _ := json.Marshal(updateWidgetRequest{
		Name:         w.Name,
		Position:     w.Position,
		Size:         w.Size,
		Origin:       w.Origin,
		Rotation:     w.Rotation,
		TypeID:       w.TypeID,
		SceneID:      w.SceneID,
		Labels:       labels,
		PortBindings: portBindings,
		Version:      w.Version,
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
	for _, idPtr := range []*string{
		&p.draggingWidgetID,
		&p.draggingOriginID,
		&p.rotatingWidgetID,
		&p.resizingWidgetID,
	} {
		if *idPtr != "" {
			id := *idPtr
			*idPtr = ""
			p.saveDraggedWidget(ctx, id)
		}
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
