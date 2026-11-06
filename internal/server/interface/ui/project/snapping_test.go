package project

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// ── Alignment guides and snapping (task 41) ───────────────────────────────────

func TestSnapAxis_PullsAnEdgeOntoAWidgetsEdgeAndCentre(t *testing.T) {
	// A neighbour's box: left edge 200, centre 250, right edge 300.
	targets := []axisTarget{{pos: 200}, {pos: 250}, {pos: 300}}

	// The moving box's right edge is 3 px short of the neighbour's left edge.
	delta, guide := snapAxis(0, 50, 197, targets)
	if delta != 3 || guide == nil || *guide != 200 {
		t.Errorf("edge to edge: delta %v, guide %v", delta, guide)
	}

	// … and 5 px short of its centre.
	delta, guide = snapAxis(0, 50, 245, targets)
	if delta != 5 || guide == nil || *guide != 250 {
		t.Errorf("edge to centre: delta %v, guide %v", delta, guide)
	}
}

func TestSnapAxis_PullsOntoTheScenesEdgeAndCentre(t *testing.T) {
	scene := sceneSnapTargets(800, 600)

	// Right edge 5 px short of the Scene's centre.
	delta, guide := snapAxis(295, 345, 395, scene.xs)
	if delta != 5 || guide == nil || *guide != 400 {
		t.Errorf("to the centre: delta %v, guide %v", delta, guide)
	}

	// Left edge 4 px past the Scene's left edge.
	delta, guide = snapAxis(4, 54, 104, scene.xs)
	if delta != -4 || guide == nil || *guide != 0 {
		t.Errorf("to the edge: delta %v, guide %v", delta, guide)
	}
}

func TestSnapAxis_LeavesWhatIsFartherThanSixPixels(t *testing.T) {
	targets := []axisTarget{{pos: 400}}

	// Right edge at 393: seven pixels from the target.
	if delta, guide := snapAxis(293, 343, 393, targets); delta != 0 || guide != nil {
		t.Errorf("snapped at 7 px: delta %v, guide %v", delta, guide)
	}
	if delta, guide := snapAxis(293, 343, 394, targets); delta != 6 || guide == nil {
		t.Errorf("a 6 px edge did not snap: delta %v, guide %v", delta, guide)
	}
}

// A tie between a Widget's target and the Scene's goes to the Scene.
func TestSnapAxis_ATieGoesToTheScene(t *testing.T) {
	targets := []axisTarget{{pos: 103}, {pos: 97, scene: true}}

	delta, guide := snapAxis(0, 50, 100, targets)
	if delta != -3 || guide == nil || *guide != 97 {
		t.Errorf("tie: delta %v, guide %v, want the Scene's 97", delta, guide)
	}
}

// A rotated Widget is a target by its axis-aligned bounding box, not by its
// unrotated rectangle: a quarter turn of the 100×50 Widget at (10, 20) around
// its top-left corner spans x −40…10, y 20…120.
func TestSnapMove_UsesARotatedWidgetsBoundingBox(t *testing.T) {
	turned := apiv0.WidgetResponse{
		ID:       uuid.New(),
		Position: apiv0.PositionResponse{X: 10, Y: 20},
		Size:     apiv0.SizeResponse{Width: 100, Height: 50},
		Rotation: apiv0.RotationResponse{Degrees: 90},
	}
	moving := uuid.New()
	targets := snapTargetsOf([]apiv0.WidgetResponse{turned}, []uuid.UUID{moving}, 1000, 1000)

	start := bounds{minX: -200, minY: 0, maxX: -160, maxY: 40}
	snapped := snapMove(start, 117, 0, targets)

	// The moving box's right edge was at −43, 3 px short of the turned box's
	// bbox left edge at −40.
	if snapped.dx != 120 || snapped.guideX == nil || *snapped.guideX != -40 {
		t.Errorf("dx %v, guide %v", snapped.dx, snapped.guideX)
	}
}

// dragProject is a Project of a 1000×1000 Scene with a, b and c on it, the
// first two selected and dragged together from where they are.
func dragProject(t *testing.T) (p *Project, a, b uuid.UUID) {
	t.Helper()
	sceneID := uuid.New()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	widget := func(id uuid.UUID, x float64) apiv0.WidgetResponse {
		return apiv0.WidgetResponse{
			ID:       id,
			SceneID:  sceneID,
			Position: apiv0.PositionResponse{X: x, Y: 100},
			Size:     apiv0.SizeResponse{Width: 100, Height: 50},
			Origin:   apiv0.OriginResponse{X: 0.5, Y: 0.5},
		}
	}
	p = &Project{
		scenes:           []apiv0.SceneResponse{{ID: sceneID, Name: "Main", Width: 1000, Height: 1000}},
		selectedSceneID:  sceneID,
		widgets:          []apiv0.WidgetResponse{widget(a, 10), widget(b, 150), widget(c, 520)},
		draggingWidgetID: a,
		dragGroup:        []uuid.UUID{a, b},
		dragStartPositions: map[uuid.UUID]apiv0.PositionResponse{
			a: {X: 10, Y: 100},
			b: {X: 150, Y: 100},
		},
	}
	return p, a, b
}

// The group is snapped by its common bounding box, and every Widget moves by
// the one delta it settles on (task 41).
func TestDrag_AGroupSnapsByItsCommonBoundingBox(t *testing.T) {
	p, a, b := dragProject(t)

	// The group spans 10…250; moved by 267 its right edge is 3 px short of c's
	// left edge at 520, so the whole group rides 3 px further.
	p.moveWidgetDrag(267, 0)
	if w, ok := p.findWidget(a); !ok || w.Position.X != 280 {
		t.Errorf("a at %+v, want x 280", w.Position)
	}
	if w, ok := p.findWidget(b); !ok || w.Position.X != 420 {
		t.Errorf("b at %+v, want x 420", w.Position)
	}
	if p.dragGuides.vertical == nil || *p.dragGuides.vertical != 520 {
		t.Errorf("no vertical guide at 520: %+v", p.dragGuides)
	}
}

// Alt frees the drag of snapping and guides; the position is the raw pointer
// delta.
func TestDrag_AltFreesTheWidgetsOfSnapping(t *testing.T) {
	p, _, _ := dragProject(t)
	p.dragAltHeld = true

	p.moveWidgetDrag(267, 0)

	if got := p.widgets[0].Position.X; got != 277 {
		t.Errorf("a at %v, want the raw 277", got)
	}
	if got := p.widgets[1].Position.X; got != 417 {
		t.Errorf("b at %v, want the raw 417", got)
	}
	if p.dragGuides.vertical != nil || p.dragGuides.horizontal != nil {
		t.Errorf("Alt drew guides: %+v", p.dragGuides)
	}
}

// Alt pressed and released before the pointer moved is not a drag yet: it must
// not pull the Widgets, or the canvas would show a position no PATCH ever sent.
func TestSetDragAlt_BeforeThePointerMoves_LeavesTheWidgetsAlone(t *testing.T) {
	p, a, _ := dragProject(t)
	// a starts 4 px from the Scene's left edge, so a snap would pull it to 0.
	p.widgets[0].Position.X = 4
	p.dragStartPositions[a] = apiv0.PositionResponse{X: 4, Y: 100}

	p.setDragAlt(p.compoCtx, true)
	p.setDragAlt(p.compoCtx, false)

	if got := p.widgets[0].Position.X; got != 4 {
		t.Errorf("a at %v, want the untouched 4", got)
	}
	if got := p.widgets[1].Position.X; got != 150 {
		t.Errorf("b at %v, want the untouched 150", got)
	}
	if p.dragGuides.vertical != nil || p.dragGuides.horizontal != nil {
		t.Errorf("an Alt press without a move drew guides: %+v", p.dragGuides)
	}
}

// Holding and releasing Alt re-applies the drag without another mousemove.
func TestSetDragAlt_ReappliesTheDragAtThePointer(t *testing.T) {
	p, _, _ := dragProject(t)
	p.dragStartCliX = 0
	p.moveWidgetDrag(267, 0)
	p.dragDidMove = true // as the mousemove of a real drag records it
	if got := p.widgets[0].Position.X; got != 280 {
		t.Fatalf("snapped a at %v, want 280", got)
	}

	p.setDragAlt(p.compoCtx, true)
	if got := p.widgets[0].Position.X; got != 277 {
		t.Errorf("Alt held: a at %v, want the raw 277", got)
	}
	p.setDragAlt(p.compoCtx, false)
	if got := p.widgets[0].Position.X; got != 280 {
		t.Errorf("Alt released: a at %v, want the snapped 280 again", got)
	}
	if p.dragGuides.vertical == nil {
		t.Error("Alt released left no guide")
	}
}

// ── The canvas ────────────────────────────────────────────────────────────────

// A body drag that snapped draws its guides in the canvas tree until mouseup.
func TestDrag_ShowsTheGuidesUntilMouseup(t *testing.T) {
	url, api := newServer(t)
	sceneID := createScene(t, api, "Main") // 800×600
	typeID := createWidgetType(t, api, "Gauge")
	a := createWidget(t, api, sceneID, typeID, "a")
	b := createWidget(t, api, sceneID, typeID, "b")
	spreadWidgets(t, api, sceneID, []uuid.UUID{a, b}) // a at 10,20; b at 210,20
	f := mount(t, url, api)
	f.run()

	f.p.selectOnly(f.p.compoCtx, a)
	// The same state a mousedown on the Widget's body records.
	f.p.dragSceneID = f.p.selectedSceneID
	f.p.dragGroup = []uuid.UUID{a}
	f.p.dragStartPositions = f.p.positionsOf(f.p.dragGroup)
	f.p.draggingWidgetID = a
	f.p.dragDidMove = true
	// b's left edge is at 210: the drag stops 3 px short of it and is pulled
	// onto it.
	f.p.moveWidgetDrag(97, 0)

	if f.p.dragGuides.vertical == nil || *f.p.dragGuides.vertical != 210 {
		t.Fatalf("guides after the move: %+v", f.p.dragGuides)
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, `data-guide="vertical"`) {
		t.Errorf("no vertical guide in the canvas tree")
	}

	f.p.finalizeAllDrags(f.p.compoCtx)
	f.run()

	if f.p.dragGuides.vertical != nil || f.p.dragGuides.horizontal != nil {
		t.Errorf("guides after mouseup: %+v", f.p.dragGuides)
	}
	if html := app.HTMLString(f.p.Render()); strings.Contains(html, "data-guides") {
		t.Errorf("the guides are still in the canvas tree")
	}
	f.noToasts()
	if w := f.widget(sceneID, a); w.Position.X != 110 {
		t.Errorf("server position x %v, want the snapped 110", w.Position.X)
	}
}
