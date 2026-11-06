package project

import (
	"fmt"
	"math"
	"slices"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Marquee selection (task 40) ───────────────────────────────────────────────
//
// A mousedown on the empty canvas starts a rectangle; moving the pointer over
// the whole Project panel updates it, mouseup selects the Widgets it
// intersects — replacing the selection, or adding to it with Shift. A drag
// shorter than 3 px on both axes is a click, which clears the selection as
// clicking the canvas always did. The rectangle is drawn in the canvas's own
// coordinates: the canvas is not scaled.

// marqueeClickThreshold is how far a mousedown on the empty canvas may move
// and still be a click.
const marqueeClickThreshold = 3.0

// marqueeState is the rectangle being dragged and where it started in client
// coordinates, so the pointer may leave the canvas.
type marqueeState struct {
	active    bool
	additive  bool // Shift held: add to the selection instead of replacing it
	sceneID   uuid.UUID
	startX    float64 // canvas coordinates
	startY    float64
	currentX  float64
	currentY  float64
	startCliX float64
	startCliY float64
}

// startMarquee starts a selection rectangle at the pointer and remembers the
// canvas coordinates it begins at.
func (p *Project) startMarquee(ctx app.Context, e app.Event) {
	if p.selectedSceneID == uuid.Nil {
		return
	}
	rect := e.Get("currentTarget").Call("getBoundingClientRect")
	clientX := e.Get("clientX").Float()
	clientY := e.Get("clientY").Float()
	x := clientX - rect.Get("left").Float()
	y := clientY - rect.Get("top").Float()
	p.marquee = marqueeState{
		active:   true,
		additive: e.Get("shiftKey").Bool(),
		sceneID:  p.selectedSceneID,
		startX:   x, startY: y,
		currentX: x, currentY: y,
		startCliX: clientX, startCliY: clientY,
	}
}

// updateMarquee moves the rectangle's far corner by the pointer's offset from
// where the drag began.
func (p *Project) updateMarquee(e app.Event) {
	if !p.marquee.active {
		return
	}
	p.marquee.currentX = p.marquee.startX + e.Get("clientX").Float() - p.marquee.startCliX
	p.marquee.currentY = p.marquee.startY + e.Get("clientY").Float() - p.marquee.startCliY
}

// finishMarquee applies the rectangle's selection and reports whether it was a
// marquee (true) rather than a click (false, which leaves the click to clear the
// selection).
func (p *Project) finishMarquee(ctx app.Context) bool {
	m := p.marquee
	p.marquee = marqueeState{}
	if !m.active || m.sceneID != p.selectedSceneID {
		return false
	}
	if math.Abs(m.currentX-m.startX) < marqueeClickThreshold && math.Abs(m.currentY-m.startY) < marqueeClickThreshold {
		return false
	}
	hits := p.marqueeWidgetIDs(m)
	if m.additive {
		p.addToSelection(ctx, hits)
	} else {
		p.selectWidgets(ctx, hits)
	}
	return true
}

// cancelMarquee drops the rectangle without changing the selection (Esc).
func (p *Project) cancelMarquee() { p.marquee = marqueeState{} }

// marqueeWidgetIDs is the Widgets the rectangle intersects, in the order of
// p.widgets: the last one is primary.
func (p *Project) marqueeWidgetIDs(m marqueeState) []uuid.UUID {
	rect := marqueeBounds(m)
	ids := make([]uuid.UUID, 0, len(p.widgets))
	for _, w := range p.widgets {
		if widgetBounds(w).intersects(rect) {
			ids = append(ids, w.ID)
		}
	}
	return ids
}

// renderMarquee draws the rectangle over the Widgets' layer; it lets the
// pointer through.
func (p *Project) renderMarquee() app.UI {
	m := p.marquee
	x1, y1, x2, y2 := m.startX, m.startY, m.currentX, m.currentY
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	return app.Div().
		Attr("data-marquee", "true").
		Style("position", "absolute").
		Style("left", fmt.Sprintf("%.1fpx", x1)).
		Style("top", fmt.Sprintf("%.1fpx", y1)).
		Style("width", fmt.Sprintf("%.1fpx", x2-x1)).
		Style("height", fmt.Sprintf("%.1fpx", y2-y1)).
		Style("background", "color-mix(in srgb, var(--accent) 14%, transparent)").
		Style("border", "1px dashed var(--accent)").
		Style("box-sizing", "border-box").
		Style("pointer-events", "none").
		Style("z-index", "2147483647")
}

// bounds is an axis-aligned rectangle in canvas coordinates.
type bounds struct {
	minX, minY, maxX, maxY float64
}

// intersects reports whether two rectangles share any area (touching edges
// count: the Widget is on the edge of the marquee).
func (b bounds) intersects(o bounds) bool {
	return b.minX <= o.maxX && o.minX <= b.maxX && b.minY <= o.maxY && o.minY <= b.maxY
}

// marqueeBounds normalises the rectangle a marquee drag described.
func marqueeBounds(m marqueeState) bounds {
	return bounds{
		minX: math.Min(m.startX, m.currentX),
		minY: math.Min(m.startY, m.currentY),
		maxX: math.Max(m.startX, m.currentX),
		maxY: math.Max(m.startY, m.currentY),
	}
}

// widgetBounds is the axis-aligned bounding box of a Widget on the canvas: its
// four corners transformed by the Widget's matrix, min/max on each axis. A
// rotation therefore widens the box, as it does on screen.
func widgetBounds(w apiv0.WidgetResponse) bounds {
	m := uiutil.Matrix(
		w.Position.X, w.Position.Y,
		w.Origin.X, w.Origin.Y,
		w.Rotation.Degrees,
		w.Size.Width, w.Size.Height,
	)
	width, height := float64(w.Size.Width), float64(w.Size.Height)
	corners := [4][2]float64{{0, 0}, {width, 0}, {0, height}, {width, height}}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, c := range corners {
		x := m[0]*c[0] + m[2]*c[1] + m[4]
		y := m[1]*c[0] + m[3]*c[1] + m[5]
		minX, maxX = math.Min(minX, x), math.Max(maxX, x)
		minY, maxY = math.Min(minY, y), math.Max(maxY, y)
	}
	return bounds{minX: minX, minY: minY, maxX: maxX, maxY: maxY}
}

// ── Alignment guides and snapping (task 41) ───────────────────────────────────
//
// While a Widget's body is dragged — one Widget or the whole selection — its
// axis-aligned box is pulled onto the edges and centres of the other Widgets of
// the Scene and of the Scene itself, within 6 px on each axis independently.
// One guide line per axis marks the target the box snapped to. The geometry is
// pure, without DOM, so tables can test it.

// snapThreshold is how far a moving edge may be from a target to be pulled onto
// it, in canvas pixels.
const snapThreshold = 6.0

// The Scene sizes assumed when the Scene states none, as the canvas does.
const (
	defaultSceneWidth  = 1920.0
	defaultSceneHeight = 1080.0
)

// axisTarget is one guide target of an axis: a coordinate and whether it is the
// Scene's own, which wins a tie with a Widget's.
type axisTarget struct {
	pos   float64
	scene bool
}

// snapTargets are the guide targets of both axes.
type snapTargets struct {
	xs, ys []axisTarget
}

// dragGuides are the guide lines a snapped body drag draws, one per axis at
// most; a nil one means that axis is free.
type dragGuides struct {
	vertical   *float64
	horizontal *float64
}

// snapResult is what a body drag settles on: the delta it applies to the
// positions the drag started from and the guides to draw.
type snapResult struct {
	dx, dy         float64
	guideX, guideY *float64
}

// translated returns the box moved by (dx, dy).
func (b bounds) translated(dx, dy float64) bounds {
	return bounds{minX: b.minX + dx, minY: b.minY + dy, maxX: b.maxX + dx, maxY: b.maxY + dy}
}

// unionBounds returns the box that contains both.
func unionBounds(a, b bounds) bounds {
	return bounds{
		minX: math.Min(a.minX, b.minX),
		minY: math.Min(a.minY, b.minY),
		maxX: math.Max(a.maxX, b.maxX),
		maxY: math.Max(a.maxY, b.maxY),
	}
}

// boundsTargets are the guide targets of a box: its two edges and its centre.
func boundsTargets(b bounds) (xs, ys []axisTarget) {
	return []axisTarget{{pos: b.minX}, {pos: (b.minX + b.maxX) / 2}, {pos: b.maxX}},
		[]axisTarget{{pos: b.minY}, {pos: (b.minY + b.maxY) / 2}, {pos: b.maxY}}
}

// sceneSnapTargets are the Scene's own targets: its edges and its centre.
func sceneSnapTargets(width, height float64) snapTargets {
	if width <= 0 {
		width = defaultSceneWidth
	}
	if height <= 0 {
		height = defaultSceneHeight
	}
	return snapTargets{
		xs: []axisTarget{{pos: 0, scene: true}, {pos: width / 2, scene: true}, {pos: width, scene: true}},
		ys: []axisTarget{{pos: 0, scene: true}, {pos: height / 2, scene: true}, {pos: height, scene: true}},
	}
}

// snapTargetsOf collects the targets of the Widgets other than the moving ones
// and of the Scene.
func snapTargetsOf(widgets []apiv0.WidgetResponse, moving []uuid.UUID, sceneWidth, sceneHeight float64) snapTargets {
	targets := sceneSnapTargets(sceneWidth, sceneHeight)
	for _, w := range widgets {
		if slices.Contains(moving, w.ID) {
			continue
		}
		xs, ys := boundsTargets(widgetBounds(w))
		targets.xs = append(targets.xs, xs...)
		targets.ys = append(targets.ys, ys...)
	}
	return targets
}

// snapMove pulls the box, already moved by (dx, dy), onto the nearest target of
// each axis. A free axis keeps the delta it was given.
func snapMove(start bounds, dx, dy float64, targets snapTargets) snapResult {
	moved := start.translated(dx, dy)
	adjustX, guideX := snapAxis(moved.minX, (moved.minX+moved.maxX)/2, moved.maxX, targets.xs)
	adjustY, guideY := snapAxis(moved.minY, (moved.minY+moved.maxY)/2, moved.maxY, targets.ys)
	return snapResult{dx: dx + adjustX, dy: dy + adjustY, guideX: guideX, guideY: guideY}
}

// snapAxis picks the nearest target within snapThreshold for the box's two edges
// and centre, and reports the offset that puts the edge on it and the guide's
// coordinate. At equal distances the Scene's target wins.
func snapAxis(min, center, max float64, targets []axisTarget) (float64, *float64) {
	edges := [3]float64{min, center, max}
	bestDist := math.Inf(1)
	bestScene := false
	var bestDelta float64
	var best *float64
	for _, target := range targets {
		for _, edge := range edges {
			delta := target.pos - edge
			dist := math.Abs(delta)
			if dist > snapThreshold {
				continue
			}
			if dist > bestDist || (dist == bestDist && bestScene) {
				continue
			}
			pos := target.pos
			bestDist, bestScene, bestDelta, best = dist, target.scene, delta, &pos
		}
	}
	return bestDelta, best
}
