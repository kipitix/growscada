package project

import (
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

func TestWidgetBounds_IsTheTransformedBoundingBox(t *testing.T) {
	w := apiv0.WidgetResponse{
		Position: apiv0.PositionResponse{X: 10, Y: 20},
		Size:     apiv0.SizeResponse{Width: 100, Height: 50},
		Origin:   apiv0.OriginResponse{X: 0, Y: 0},
	}
	if got := widgetBounds(w); got != (bounds{minX: 10, minY: 20, maxX: 110, maxY: 70}) {
		t.Errorf("unrotated: %+v", got)
	}

	// A quarter turn around the top-left corner swaps the sides: the bbox is
	// 50 wide and 100 tall, going up from the rotated widget.
	turned := w
	turned.Rotation = apiv0.RotationResponse{Degrees: 90}
	got := widgetBounds(turned)
	want := bounds{minX: -40, minY: 20, maxX: 10, maxY: 120}
	if !boundsNear(got, want) {
		t.Errorf("rotated: %+v, want %+v", got, want)
	}
}

// boundsNear compares bounding boxes; the matrix's floating point leaves the
// last bits of a turned corner open.
func boundsNear(a, b bounds) bool {
	const eps = 1e-9
	abs := func(v float64) float64 {
		if v < 0 {
			return -v
		}
		return v
	}
	return abs(a.minX-b.minX) < eps && abs(a.minY-b.minY) < eps &&
		abs(a.maxX-b.maxX) < eps && abs(a.maxY-b.maxY) < eps
}

func TestMarqueeWidgetIDs_IntersectionAndContainment(t *testing.T) {
	a := apiv0.WidgetResponse{ID: uuid.New(), Position: apiv0.PositionResponse{X: 0, Y: 0}, Size: apiv0.SizeResponse{Width: 100, Height: 50}}
	b := apiv0.WidgetResponse{ID: uuid.New(), Position: apiv0.PositionResponse{X: 200, Y: 0}, Size: apiv0.SizeResponse{Width: 100, Height: 50}}
	p := &Project{widgets: []apiv0.WidgetResponse{a, b}}

	// The rectangle crosses a and is inside b's left part? It ends at 250 so
	// it catches both: the first by intersection, the second partly too.
	ids := p.marqueeWidgetIDs(marqueeState{startX: 50, startY: 25, currentX: 250, currentY: 40})
	if len(ids) != 2 || ids[0] != a.ID || ids[1] != b.ID {
		t.Errorf("intersection: %v", ids)
	}

	// Containment of everything.
	ids = p.marqueeWidgetIDs(marqueeState{startX: -10, startY: -10, currentX: 400, currentY: 100})
	if len(ids) != 2 {
		t.Errorf("containment: %v", ids)
	}

	// Nothing.
	if ids := p.marqueeWidgetIDs(marqueeState{startX: 120, startY: 0, currentX: 180, currentY: 50}); len(ids) != 0 {
		t.Errorf("the gap caught %v", ids)
	}
}

func TestMarquee_ReplacesOrAddsAndIgnoresAClick(t *testing.T) {
	f, _, ids := selectionFixture(t, 2)
	f.p.selectOnly(f.p.compoCtx, ids[0])

	f.p.marquee = marqueeState{active: true, sceneID: f.p.selectedSceneID, startX: 200, startY: 0, currentX: 350, currentY: 100}
	if !f.p.finishMarquee(f.p.compoCtx) {
		t.Fatal("a drag across the second Widget was taken for a click")
	}
	if got := f.selection(); len(got) != 1 || got[0] != ids[1] {
		t.Errorf("replacing marquee: %v", got)
	}

	f.p.marquee = marqueeState{active: true, additive: true, sceneID: f.p.selectedSceneID, startX: 0, startY: 0, currentX: 150, currentY: 100}
	if !f.p.finishMarquee(f.p.compoCtx) {
		t.Fatal("the additive drag was taken for a click")
	}
	if got := f.selection(); len(got) != 2 || got[0] != ids[1] || got[1] != ids[0] {
		t.Errorf("additive marquee: %v", got)
	}
	if f.p.primaryWidgetID() != ids[0] {
		t.Errorf("primary %v, want the last caught %v", f.p.primaryWidgetID(), ids[0])
	}

	// A drag shorter than 3 px on both axes is a click: it leaves the
	// selection to the canvas click that follows.
	f.p.marquee = marqueeState{active: true, sceneID: f.p.selectedSceneID, startX: 10, startY: 10, currentX: 12, currentY: 12}
	if f.p.finishMarquee(f.p.compoCtx) {
		t.Error("a 2 px drag was taken for a marquee")
	}
	if got := f.selection(); len(got) != 2 {
		t.Errorf("the tiny drag changed the selection: %v", got)
	}
	if f.p.marquee.active {
		t.Error("the rectangle is still active")
	}
}

func TestMarquee_CancelDropsTheRectangleOnly(t *testing.T) {
	f, _, ids := selectionFixture(t, 1)
	f.p.selectOnly(f.p.compoCtx, ids[0])
	f.p.marquee = marqueeState{active: true, sceneID: f.p.selectedSceneID}

	f.p.cancelMarquee()

	if f.p.marquee.active {
		t.Error("Esc kept the rectangle")
	}
	if len(f.selection()) != 1 {
		t.Errorf("Esc changed the selection: %v", f.selection())
	}
}
