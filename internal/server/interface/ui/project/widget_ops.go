package project

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v10/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
)

// ── Widget data loading ───────────────────────────────────────────────────────

// currentSceneVersion returns the last-known version of the selected scene,
// the sole optimistic-lock boundary shared by the scene and all its widgets.
func (p *Project) currentSceneVersion() int {
	for _, sc := range p.scenes {
		if sc.ID == p.selectedSceneID {
			return sc.Version
		}
	}
	return 0
}

// applyNewSceneVersion records a scene version bump (from a widget mutation)
// in both the scene list and every currently loaded widget, so the next
// mutation is checked against the up-to-date version.
func (p *Project) applyNewSceneVersion(sceneID string, newVersion int) {
	for i := range p.scenes {
		if p.scenes[i].ID == sceneID {
			p.scenes[i].Version = newVersion
			break
		}
	}
	if p.selectedSceneID == sceneID {
		for i := range p.widgets {
			p.widgets[i].SceneVersion = newVersion
		}
	}
}

// ── Widget CRUD ───────────────────────────────────────────────────────────────

func (p *Project) createWidget(ctx app.Context, typeID string, x, y float64) {
	ctx = p.compoCtx
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
	sceneID := p.selectedSceneID
	url := p.apiServerURL + apiv0.PathPrefix + "/scenes/" + sceneID + "/widgets"
	typeUUID, err := uuid.Parse(typeID)
	if err != nil {
		ctx.NewActionWithValue(toast.ActionAdd, toast.ClientError(err))
		return
	}
	body, _ := json.Marshal(apiv0.CreateWidgetRequest{
		Name:         name,
		Position:     apiv0.PositionRequest{X: x, Y: y, Z: 0},
		Size:         apiv0.SizeRequest{Width: width, Height: height},
		Origin:       apiv0.OriginRequest{X: 0.5, Y: 0.5},
		Rotation:     apiv0.RotationRequest{Degrees: 0},
		TypeID:       typeUUID,
		SceneVersion: p.currentSceneVersion(),
		Labels:       []string{},
		PortBindings: []apiv0.PortBinding{},
	})
	ctx.Async(func() {
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) {
				ctx.NewActionWithValue(toast.ActionAdd, toast.NetworkError(err))
			})
			return
		}
		if resp.StatusCode >= 400 {
			prob := toast.FromHTTPError(resp)
			ctx.Dispatch(func(ctx app.Context) {
				ctx.NewActionWithValue(toast.ActionAdd, prob)
			})
			return
		}
		defer resp.Body.Close()
		var result createWidgetResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			ctx.Dispatch(func(ctx app.Context) {
				ctx.NewActionWithValue(toast.ActionAdd, toast.NetworkError(err))
			})
			return
		}
		ctx.Dispatch(func(ctx app.Context) {
			p.applyNewSceneVersion(sceneID, result.SceneVersion)
			p.selectedWidgetID = result.ID
			ctx.LocalStorage().Set("project:widgetID", result.ID)
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
	ctx = p.compoCtx
	sceneID := p.selectedSceneID
	url := p.apiServerURL + apiv0.PathPrefix + "/scenes/" + sceneID + "/widgets/" + widgetID
	ctx.Async(func() {
		req, _ := http.NewRequest(http.MethodDelete, url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) {
				ctx.NewActionWithValue(toast.ActionAdd, toast.NetworkError(err))
			})
			return
		}
		if resp.StatusCode >= 400 {
			prob := toast.FromHTTPError(resp)
			ctx.Dispatch(func(ctx app.Context) {
				ctx.NewActionWithValue(toast.ActionAdd, prob)
			})
			return
		}
		defer resp.Body.Close()
		var result widgetItem
		_ = json.NewDecoder(resp.Body).Decode(&result)
		ctx.Dispatch(func(ctx app.Context) {
			if result.SceneVersion > 0 {
				p.applyNewSceneVersion(sceneID, result.SceneVersion)
			}
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

func (p *Project) saveWidgetGeometry(ctx app.Context) {
	idx := p.selectedWidgetIdx()
	if idx < 0 {
		return
	}
	var nf numberFields
	posX := nf.float("position X", p.editingPosX)
	posY := nf.float("position Y", p.editingPosY)
	posZ := nf.int("position Z", p.editingPosZ)
	width := nf.int("width", p.editingWidth)
	height := nf.int("height", p.editingHeight)
	originX := nf.float("origin X", p.editingOriginX)
	originY := nf.float("origin Y", p.editingOriginY)
	rotDeg := nf.float("rotation", p.editingRotation)
	if nf.err != nil {
		p.compoCtx.NewActionWithValue(toast.ActionAdd, toast.ClientError(nf.err))
		p.syncEditingFields(p.widgets[idx])
		return
	}

	p.widgets[idx].Position = positionDTO{X: posX, Y: posY, Z: posZ}
	p.widgets[idx].Size = sizeDTO{Width: width, Height: height}
	p.widgets[idx].Origin = originDTO{X: originX, Y: originY}
	p.widgets[idx].Rotation = rotationDTO{Degrees: rotDeg}
	// Show the fields as the widget now holds them, so that the reload after
	// a rejected save puts the server's values back (mergeWidgetEditingFields
	// keeps only fields the user changed since).
	p.syncEditingFields(p.widgets[idx])
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
	ctx = p.compoCtx
	sceneID := p.selectedSceneID
	url := p.apiServerURL + apiv0.PathPrefix + "/scenes/" + sceneID + "/widgets/" + w.ID
	typeUUID, err := uuid.Parse(w.TypeID)
	if err != nil {
		ctx.NewActionWithValue(toast.ActionAdd, toast.ClientError(err))
		return
	}
	portBindings := make([]apiv0.PortBinding, 0, len(w.PortBindings))
	for _, b := range w.PortBindings {
		tagID, err := uuid.Parse(b.TagID)
		if err != nil {
			ctx.NewActionWithValue(toast.ActionAdd, toast.ClientError(err))
			return
		}
		portBindings = append(portBindings, apiv0.PortBinding{PortName: b.PortName, TagID: tagID})
	}
	labels := w.Labels
	if labels == nil {
		labels = []string{}
	}
	body, err := json.Marshal(apiv0.UpdateWidgetRequest{
		Name:         w.Name,
		Position:     apiv0.PositionRequest(w.Position),
		Size:         apiv0.SizeRequest(w.Size),
		Origin:       apiv0.OriginRequest(w.Origin),
		Rotation:     apiv0.RotationRequest(w.Rotation),
		TypeID:       typeUUID,
		Labels:       labels,
		PortBindings: portBindings,
		SceneVersion: p.currentSceneVersion(),
	})
	if err != nil {
		ctx.NewActionWithValue(toast.ActionAdd, toast.ClientError(err))
		p.loadWidgets(ctx)
		return
	}
	ctx.Async(func() {
		req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) {
				ctx.NewActionWithValue(toast.ActionAdd, toast.NetworkError(err))
				p.loadWidgets(ctx)
			})
			return
		}
		if resp.StatusCode >= 400 {
			prob := toast.FromHTTPError(resp)
			ctx.Dispatch(func(ctx app.Context) {
				ctx.NewActionWithValue(toast.ActionAdd, prob)
				p.loadWidgets(ctx)
			})
			return
		}
		defer resp.Body.Close()
		var result updateWidgetResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && result.SceneVersion > 0 {
			ctx.Dispatch(func(ctx app.Context) {
				p.applyNewSceneVersion(sceneID, result.SceneVersion)
			})
		}
	})
}

// ── Drag finalisation ─────────────────────────────────────────────────────────

// widgetGeometry is the part of a widget a canvas drag changes.
type widgetGeometry struct {
	Position positionDTO
	Size     sizeDTO
	Origin   originDTO
	Rotation rotationDTO
}

func (w widgetItem) geometry() widgetGeometry {
	return widgetGeometry{Position: w.Position, Size: w.Size, Origin: w.Origin, Rotation: w.Rotation}
}

// draggedWidgetID returns the widget a canvas drag (move, origin, rotate or
// resize) is changing, or "" when none is.
func (p *Project) draggedWidgetID() string {
	for _, id := range []string{p.draggingWidgetID, p.draggingOriginID, p.rotatingWidgetID, p.resizingWidgetID} {
		if id != "" {
			return id
		}
	}
	return ""
}

// dragChangedWidget reports whether the drag that is ending changed w: a
// click (no move) or a drag back to the start leaves nothing to save, and
// saving it anyway would bump the scene version for every other client.
func (p *Project) dragChangedWidget(w widgetItem) bool {
	return p.dragDidMove && w.geometry() != p.dragStartGeometry
}

func (p *Project) finalizeAllDrags(ctx app.Context) {
	anyDrag := false
	for _, idPtr := range []*string{
		&p.draggingWidgetID,
		&p.draggingOriginID,
		&p.rotatingWidgetID,
		&p.resizingWidgetID,
	} {
		if *idPtr != "" {
			anyDrag = true
			id := *idPtr
			*idPtr = ""
			p.saveDraggedWidget(ctx, id)
		}
	}
	if anyDrag {
		p.dragJustEnded = p.dragDidMove
		p.dragDidMove = false
	}
	if p.widgetsReloadDeferred {
		p.widgetsReloadDeferred = false
		p.loadWidgets(ctx)
	}
}

func (p *Project) saveDraggedWidget(ctx app.Context, id string) {
	for i := range p.widgets {
		if p.widgets[i].ID == id {
			if !p.dragChangedWidget(p.widgets[i]) {
				break
			}
			if p.selectedWidgetID == id {
				p.syncEditingFields(p.widgets[i])
			}
			p.putWidget(ctx, p.widgets[i])
			break
		}
	}
}
