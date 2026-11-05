package project

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Widget CRUD ───────────────────────────────────────────────────────────────

// createWidget places a Widget of the WidgetType typeID at (x, y) on the
// selected Scene and selects it.
func (p *Project) createWidget(ctx app.Context, typeID uuid.UUID, x, y float64) {
	if p.refuseReadOnly(ctx) {
		return
	}
	if p.waitMoves(ctx, func(ctx app.Context) { p.createWidget(ctx, typeID, x, y) }) {
		return
	}
	sceneID := p.selectedSceneID
	if sceneID == uuid.Nil {
		return
	}
	w := apiv0.WidgetResponse{
		Name:     "Widget",
		Position: apiv0.PositionResponse{X: x, Y: y},
		Size:     apiv0.SizeResponse{Width: 120, Height: 60},
		Origin:   apiv0.OriginResponse{X: 0.5, Y: 0.5},
		TypeID:   typeID,
		SceneID:  sceneID,
	}
	if wt, ok := p.widgetTypeByID(typeID); ok {
		w.Name = wt.Name
		if wt.DefaultWidth > 0 {
			w.Size.Width = wt.DefaultWidth
		}
		if wt.DefaultHeight > 0 {
			w.Size.Height = wt.DefaultHeight
		}
	}
	req := apiv0.CreateWidgetRequest(widgetRequest(w, p.draftVersion()))
	mark := p.markDraft()
	callContent(p, func(c context.Context, content *apiclient.ProjectClient) (apiv0.CreateWidgetResponse, error) {
		return content.CreateWidget(c, sceneID, req)
	}, func(ctx app.Context, created apiv0.CreateWidgetResponse) {
		p.observeDraftVersion(mark, created.Version)
		w.ID = created.ID
		p.selectedWidgetIDs = []uuid.UUID{created.ID}
		uiutil.StoreID(ctx, p.selectionKey(widgetIDKey), created.ID)
		p.syncEditingFields(w)
		p.loadWidgets(ctx)
	}, p.rejectedSceneEdit(nil))
}

// deleteWidget deletes the Widget widgetID of the selected Scene. The
// response states the Draft's new Version; one that does not (a body the
// client could not read) leaves it unknown, so the known one, now stale, is
// forgotten and the Scenes are reread.
func (p *Project) deleteWidget(ctx app.Context, widgetID uuid.UUID) {
	if p.refuseReadOnly(ctx) {
		return
	}
	if p.waitMoves(ctx, func(ctx app.Context) { p.deleteWidget(ctx, widgetID) }) {
		return
	}
	sceneID := p.selectedSceneID
	mark := p.markDraft()
	callContent(p, func(c context.Context, content *apiclient.ProjectClient) (apiv0.WidgetResponse, error) {
		return content.DeleteWidget(c, sceneID, widgetID, p.draftVersion())
	}, func(ctx app.Context, deleted apiv0.WidgetResponse) {
		if deleted.Version > 0 {
			p.observeDraftVersion(mark, deleted.Version)
		} else {
			p.forgetDraftVersion()
			p.loadScenes(ctx)
		}
		if p.isWidgetSelected(widgetID) {
			p.removeWidgetFromSelection(ctx, widgetID)
		}
		filtered := p.widgets[:0]
		for _, w := range p.widgets {
			if w.ID != widgetID {
				filtered = append(filtered, w)
			}
		}
		p.widgets = filtered
	}, p.rejectedSceneEdit(nil))
}

func (p *Project) saveWidgetName(ctx app.Context) {
	if p.refuseReadOnly(ctx) {
		return
	}
	if p.waitMoves(ctx, func(ctx app.Context) { p.saveWidgetName(ctx) }) {
		return
	}
	if strings.TrimSpace(p.widgetEdit.name) == "" {
		return
	}
	idx := p.selectedWidgetIdx()
	if idx < 0 {
		return
	}
	p.widgets[idx].Name = p.widgetEdit.name
	p.putWidget(ctx, p.widgets[idx])
}

func (p *Project) saveWidgetGeometry(ctx app.Context) {
	if p.refuseReadOnly(ctx) {
		return
	}
	if p.waitMoves(ctx, func(ctx app.Context) { p.saveWidgetGeometry(ctx) }) {
		return
	}
	idx := p.selectedWidgetIdx()
	if idx < 0 {
		return
	}
	var nf numberFields
	posX := nf.float("position X", p.widgetEdit.posX)
	posY := nf.float("position Y", p.widgetEdit.posY)
	posZ := nf.int("position Z", p.widgetEdit.posZ)
	width := nf.int("width", p.widgetEdit.width)
	height := nf.int("height", p.widgetEdit.height)
	originX := nf.float("origin X", p.widgetEdit.originX)
	originY := nf.float("origin Y", p.widgetEdit.originY)
	rotDeg := nf.float("rotation", p.widgetEdit.rotation)
	if nf.err != nil {
		p.compoCtx.NewActionWithValue(toast.ActionAdd, toast.ClientError(nf.err))
		p.syncEditingFields(p.widgets[idx])
		return
	}

	p.widgets[idx].Position = apiv0.PositionResponse{X: posX, Y: posY, Z: posZ}
	p.widgets[idx].Size = apiv0.SizeResponse{Width: width, Height: height}
	p.widgets[idx].Origin = apiv0.OriginResponse{X: originX, Y: originY}
	p.widgets[idx].Rotation = apiv0.RotationResponse{Degrees: rotDeg}
	// Show the fields as the widget now holds them, so that the reload after
	// a rejected save puts the server's values back (mergeWidgetEditingFields
	// keeps only fields the user changed since).
	p.syncEditingFields(p.widgets[idx])
	p.putWidget(ctx, p.widgets[idx])
}

func (p *Project) bindPort(ctx app.Context, portName string, tagID uuid.UUID) {
	if p.refuseReadOnly(ctx) {
		return
	}
	if p.waitMoves(ctx, func(ctx app.Context) { p.bindPort(ctx, portName, tagID) }) {
		return
	}
	if _, single := p.singleSelected(); !single || portName == "" || tagID == uuid.Nil {
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
		p.widgets[idx].PortBindings = append(p.widgets[idx].PortBindings, apiv0.PortBinding{
			PortName: portName,
			TagID:    tagID,
		})
	}
	p.putWidget(ctx, p.widgets[idx])
}

func (p *Project) unbindPort(ctx app.Context, portName string) {
	if p.refuseReadOnly(ctx) {
		return
	}
	if p.waitMoves(ctx, func(ctx app.Context) { p.unbindPort(ctx, portName) }) {
		return
	}
	idx := p.selectedWidgetIdx()
	if idx < 0 {
		return
	}
	filtered := make([]apiv0.PortBinding, 0, len(p.widgets[idx].PortBindings))
	for _, b := range p.widgets[idx].PortBindings {
		if b.PortName != portName {
			filtered = append(filtered, b)
		}
	}
	p.widgets[idx].PortBindings = filtered
	p.putWidget(ctx, p.widgets[idx])
}

// putWidget saves w on the selected Scene at the Scene's known version.
func (p *Project) putWidget(ctx app.Context, w apiv0.WidgetResponse) {
	if p.refuseReadOnly(ctx) {
		return
	}
	if p.waitMoves(ctx, func(ctx app.Context) { p.putWidget(ctx, w) }) {
		return
	}
	sceneID := p.selectedSceneID
	req := apiv0.UpdateWidgetRequest(widgetRequest(w, p.draftVersion()))
	mark := p.markDraft()
	callContent(p, func(c context.Context, content *apiclient.ProjectClient) (apiv0.UpdateWidgetResponse, error) {
		return content.UpdateWidget(c, sceneID, w.ID, req)
	}, func(ctx app.Context, updated apiv0.UpdateWidgetResponse) {
		p.observeDraftVersion(mark, updated.Version)
	}, p.rejectedSceneEdit(p.loadWidgets))
}

// widgetRequest is the body that writes w at sceneVersion; it converts to
// both apiv0.CreateWidgetRequest and apiv0.UpdateWidgetRequest.
func widgetRequest(w apiv0.WidgetResponse, sceneVersion int) apiv0.UpdateWidgetRequest {
	// Copies, never nil: the body is marshalled later, on the Async goroutine,
	// while the UI may already change p.widgets in place (bindPort).
	labels := append([]string{}, w.Labels...)
	bindings := append([]apiv0.PortBinding{}, w.PortBindings...)
	return apiv0.UpdateWidgetRequest{
		Name:         w.Name,
		Position:     apiv0.PositionRequest(w.Position),
		Size:         apiv0.SizeRequest(w.Size),
		Origin:       apiv0.OriginRequest(w.Origin),
		Rotation:     apiv0.RotationRequest(w.Rotation),
		TypeID:       w.TypeID,
		Version:      sceneVersion,
		Labels:       labels,
		PortBindings: bindings,
	}
}

// ── Drag finalisation ─────────────────────────────────────────────────────────

// beginWidgetDrag starts moving the selection with the Widget id, recording
// where every selected Widget is now. A pending arrow series is sent first.
func (p *Project) beginWidgetDrag(ctx app.Context, e app.Event, id uuid.UUID) {
	if p.readOnly() || p.selectedSceneID == uuid.Nil {
		return
	}
	p.endArrowSeries(ctx)
	p.dragSceneID = p.selectedSceneID
	p.dragGroup = slices.Clone(p.selectedWidgetIDs)
	if !slices.Contains(p.dragGroup, id) {
		p.dragGroup = []uuid.UUID{id}
	}
	p.dragStartPositions = p.positionsOf(p.dragGroup)
	p.draggingWidgetID = id
	p.dragStartCliX = e.Get("clientX").Float()
	p.dragStartCliY = e.Get("clientY").Float()
	if w, ok := p.findWidget(id); ok {
		p.dragStartGeometry = geometryOf(w)
	}
	p.dragDidMove = false
}

// widgetGeometry is the part of a widget a canvas drag changes.
type widgetGeometry struct {
	Position apiv0.PositionResponse
	Size     apiv0.SizeResponse
	Origin   apiv0.OriginResponse
	Rotation apiv0.RotationResponse
}

func geometryOf(w apiv0.WidgetResponse) widgetGeometry {
	return widgetGeometry{Position: w.Position, Size: w.Size, Origin: w.Origin, Rotation: w.Rotation}
}

// draggedWidgetID returns the widget a canvas drag (move, origin, rotate or
// resize) is changing, or uuid.Nil when none is.
func (p *Project) draggedWidgetID() uuid.UUID {
	for _, id := range []uuid.UUID{p.draggingWidgetID, p.draggingOriginID, p.rotatingWidgetID, p.resizingWidgetID} {
		if id != uuid.Nil {
			return id
		}
	}
	return uuid.Nil
}

// dragChangedWidget reports whether the drag that is ending changed w: a
// click (no move) or a drag back to the start leaves nothing to save, and
// saving it anyway would bump the Draft's Version for every other client.
func (p *Project) dragChangedWidget(w apiv0.WidgetResponse) bool {
	return p.dragDidMove && geometryOf(w) != p.dragStartGeometry
}

func (p *Project) finalizeAllDrags(ctx app.Context) {
	groupDidMove := false
	if p.draggingWidgetID != uuid.Nil {
		id := p.draggingWidgetID
		p.draggingWidgetID = uuid.Nil
		groupDidMove = p.dragDidMove
		if groupDidMove {
			// One PATCH for the whole group, even for a single Widget; a drag
			// that came back to the start sends nothing.
			moves := p.movedFrom(p.dragStartPositions, p.dragGroup)
			p.patchWidgets(ctx, p.dragSceneID, moves)
			p.syncMovedEditingFields()
		} else {
			// A plain click on a selected Widget: now that no drag follows,
			// the selection is that Widget alone.
			p.selectOnly(ctx, id)
		}
		p.dragGroup, p.dragStartPositions, p.dragSceneID = nil, nil, uuid.Nil
	}

	anyDrag := false
	for _, idPtr := range []*uuid.UUID{
		&p.draggingOriginID,
		&p.rotatingWidgetID,
		&p.resizingWidgetID,
	} {
		if *idPtr != uuid.Nil {
			anyDrag = true
			id := *idPtr
			*idPtr = uuid.Nil
			p.saveDraggedWidget(ctx, id)
		}
	}
	if groupDidMove || anyDrag {
		p.dragJustEnded = p.dragDidMove
	}
	p.dragDidMove = false
	if p.widgetsReloadDeferred {
		p.waitMoves(ctx, p.loadWidgets)
	}
}

func (p *Project) saveDraggedWidget(ctx app.Context, id uuid.UUID) {
	for i := range p.widgets {
		if p.widgets[i].ID == id {
			if !p.dragChangedWidget(p.widgets[i]) {
				break
			}
			if sid, single := p.singleSelected(); single && sid == id {
				p.syncEditingFields(p.widgets[i])
			}
			p.putWidget(ctx, p.widgets[i])
			break
		}
	}
}
