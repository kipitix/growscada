package project

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Arrow-key move series and moves of the Draft (task 40) ────────────────────
//
// The arrows move the selection locally and send one PATCH when the series
// ends: an arrow keyup, 500 ms without a press, or any other write of the
// Draft. That write waits for the PATCH too (waitMoves), so the two never state
// the same Version of the Draft. A mouse drag of the selection sends its own
// one PATCH (finalizeAllDrags); a write queued behind it waits the same way.

// arrowSeriesDelay is how long the series waits for another arrow press before
// sending its PATCH.
const arrowSeriesDelay = 500 * time.Millisecond

// checkInTimeout bounds the Check in Root asks for when the Project mode is
// left or its project switched: the switch does not wait for it.
const checkInTimeout = 3 * time.Second

// beginArrowMove moves the selection by (dx, dy) — the arrows' 1 px or, with
// Shift, 10 px — starting the series if it is not running. It changes only the
// shown Widgets: the server learns the positions when the series ends.
func (p *Project) beginArrowMove(ctx app.Context, dx, dy float64) {
	if p.readOnly() || p.selectedSceneID == uuid.Nil || len(p.selectedWidgetIDs) == 0 {
		return
	}
	if !p.arrowActive {
		p.arrowActive = true
		p.arrowSceneID = p.selectedSceneID
		p.arrowBase = p.positionsOf(p.selectedWidgetIDs)
	}
	for _, id := range p.selectedWidgetIDs {
		i := uiutil.IdxByID(p.widgets, id, uiutil.WidgetID)
		if i < 0 {
			continue
		}
		p.widgets[i].Position.X += dx
		p.widgets[i].Position.Y += dy
	}
	p.syncMovedEditingFields()
	p.restartArrowTimer()
	ctx.Update()
}

// endArrowSeries sends the positions the series reached and stops it, if one is
// running. A selection change ends the series before it changes.
func (p *Project) endArrowSeries(ctx app.Context) {
	if p.arrowActive {
		p.flushArrowMoves(ctx)
	}
}

// flushArrowMoves sends the series' one PATCH: every still-selected Widget
// whose position differs from where the series started. A series that came
// back to its start sends nothing.
func (p *Project) flushArrowMoves(ctx app.Context) {
	p.stopArrowTimer()
	if !p.arrowActive {
		return
	}
	sceneID, base := p.arrowSceneID, p.arrowBase
	p.arrowActive, p.arrowBase = false, nil
	p.patchWidgets(ctx, sceneID, p.movedFrom(base, p.selectedWidgetIDs))
}

// restartArrowTimer arms the 500 ms that end the series when no arrow is
// pressed again.
func (p *Project) restartArrowTimer() {
	p.stopArrowTimer()
	if !p.mounted {
		return
	}
	p.arrowTimer = time.AfterFunc(arrowSeriesDelay, func() {
		p.compoCtx.Dispatch(func(ctx app.Context) { p.flushArrowMoves(ctx) })
	})
}

func (p *Project) stopArrowTimer() {
	if p.arrowTimer != nil {
		p.arrowTimer.Stop()
		p.arrowTimer = nil
	}
}

// flushMovesBeforeProjectChange sends a pending arrow series of the project
// being left and drops what waits for it: the selection, the positions and the
// Version it was built from are about to be replaced.
func (p *Project) flushMovesBeforeProjectChange(ctx app.Context) {
	p.stopArrowTimer()
	if p.arrowActive {
		sceneID, base := p.arrowSceneID, p.arrowBase
		p.arrowActive, p.arrowBase = false, nil
		moves := p.movedFrom(base, p.selectedWidgetIDs)
		if len(moves) > 0 {
			p.sendMoves(ctx, sceneID, moves)
		}
	}
	p.moveQueue = nil
	p.moveSending = false
	p.marquee = marqueeState{}
}

// GiveBackDraft gives the Draft of the project id back (ADR 0015) once the
// writes this mode still owes it are answered: a series of arrow moves still
// running is sent first, and its PATCH — and every move queued behind it —
// comes before the Check in, so the lock is never released under a move of its
// own Draft. Root calls it with its own context on a switch of project and on
// leaving the mode, before the project shown changes, so the Draft is given
// back even while this mode is not mounted; a mode that owes nothing sends the
// Check in at once. id names the project, never uuid.Nil: Root checks that.
func (p *Project) GiveBackDraft(ctx app.Context, id uuid.UUID) {
	content := p.client.Project(id)
	checkIn := func() {
		uiutil.Do(ctx, func(c context.Context) (struct{}, error) {
			ctx, cancel := context.WithTimeout(c, checkInTimeout)
			defer cancel()
			_, err := content.CheckInEditLock(ctx)
			return struct{}{}, err
		}, func(ctx app.Context, _ struct{}, err error) {
			// A lock another client holds is not a failure: this tab had lost it
			// already.
			if err != nil && !errors.Is(err, apiclient.ErrConflict) {
				uiutil.ShowError(ctx, err)
			}
		})
	}
	if !p.mounted || (!p.arrowActive && !p.moveSending) {
		checkIn()
		return
	}
	p.checkIns = append(p.checkIns, checkIn)
	if p.arrowActive {
		p.flushArrowMoves(p.compoCtx)
	}
	// Nothing had to be sent (or the series came back to its start): no answer
	// will come to send the Check in.
	p.runCheckIns()
}

// runCheckIns sends the Check ins that waited for the Draft's last write. A
// move PATCH still in flight, or a series not sent yet, postpones them: the
// answer that settles those calls it again.
func (p *Project) runCheckIns() {
	if p.moveSending || p.arrowActive || len(p.checkIns) == 0 {
		return
	}
	waiting := p.checkIns
	p.checkIns = nil
	for _, checkIn := range waiting {
		checkIn()
	}
}

// waitMoves queues action until the arrow series and any move PATCH are done,
// reporting whether it queued it — the caller returns then, and the action runs
// the same method again once the Draft's Version is settled. This is how a
// write that follows a move never states the Version the move raised.
func (p *Project) waitMoves(ctx app.Context, action func(app.Context)) bool {
	if p.arrowActive {
		p.moveQueue = append(p.moveQueue, action)
		p.flushArrowMoves(ctx)
		return true
	}
	// A write run by runMoveQueue runs now: queueing it again would leave it
	// waiting for itself.
	if !p.moveDraining && (p.moveSending || len(p.moveQueue) > 0) {
		p.moveQueue = append(p.moveQueue, action)
		return true
	}
	return false
}

// patchWidgets sends the moves, or queues them behind the one in flight: the
// Draft has one Version and two changes of it are never sent at once.
func (p *Project) patchWidgets(ctx app.Context, sceneID uuid.UUID, moves []apiv0.WidgetMoveRequest) {
	if len(moves) == 0 || p.content == nil || sceneID == uuid.Nil {
		// Nothing to send: whatever waited for a move may go now.
		if p.content != nil {
			p.runMoveQueue(ctx)
		}
		return
	}
	if p.moveSending {
		p.moveQueue = append(p.moveQueue, func(ctx app.Context) { p.patchWidgets(ctx, sceneID, moves) })
		return
	}
	p.sendMoves(ctx, sceneID, moves)
}

// sendMoves sends one PATCH and, once it is answered, runs what waited for it.
func (p *Project) sendMoves(ctx app.Context, sceneID uuid.UUID, moves []apiv0.WidgetMoveRequest) {
	content := p.content
	if content == nil {
		return
	}
	p.moveSending = true
	mark := p.markDraft()
	req := apiv0.MoveWidgetsRequest{Version: p.draftVersion(), Moves: moves}
	uiutil.Call(p.compoCtx, func(c context.Context) (apiv0.MoveWidgetsResponse, error) {
		return content.MoveWidgets(c, sceneID, req)
	}, func(ctx app.Context, resp apiv0.MoveWidgetsResponse) {
		// The project may have been switched while the PATCH was in flight: its
		// flag and its queue belong to the project shown now, not to this
		// answer, while the Check in it waited for belongs to the Draft left.
		if content == p.content {
			p.moveSending = false
			p.observeDraftVersion(mark, resp.Version)
			p.runMoveQueue(ctx)
		}
		p.runCheckIns()
	}, func(ctx app.Context, err error) {
		if content == p.content {
			p.moveSending = false
			p.showRejectedSceneEdit(ctx, err, p.loadWidgets)
			p.runMoveQueue(ctx)
		}
		p.runCheckIns()
	})
}

// runMoveQueue starts the writes that waited for the move, one after another:
// a queued move leaves the others waiting for its own answer, while the other
// writes are simply run — they are what the move held back.
func (p *Project) runMoveQueue(ctx app.Context) {
	if p.moveDraining {
		return
	}
	p.moveDraining = true
	for len(p.moveQueue) > 0 && !p.moveSending && !p.arrowActive {
		action := p.moveQueue[0]
		p.moveQueue = p.moveQueue[1:]
		action(ctx)
	}
	p.moveDraining = false
	if !p.moveSending && !p.arrowActive && p.widgetsReloadDeferred {
		p.widgetsReloadDeferred = false
		p.loadWidgets(ctx)
	}
}

// positionsOf is the map of the positions of ids now.
func (p *Project) positionsOf(ids []uuid.UUID) map[uuid.UUID]apiv0.PositionResponse {
	out := make(map[uuid.UUID]apiv0.PositionResponse, len(ids))
	for _, id := range ids {
		if w, ok := p.findWidget(id); ok {
			out[id] = w.Position
		}
	}
	return out
}

// movedFrom builds the moves of ids whose position differs from base. The map
// is what a move changes; z, size and the rest stay untouched (ADR 0010).
func (p *Project) movedFrom(base map[uuid.UUID]apiv0.PositionResponse, ids []uuid.UUID) []apiv0.WidgetMoveRequest {
	moves := make([]apiv0.WidgetMoveRequest, 0, len(ids))
	for _, id := range ids {
		w, ok := p.findWidget(id)
		if !ok {
			continue
		}
		start, had := base[id]
		if had && start.X == w.Position.X && start.Y == w.Position.Y {
			continue
		}
		moves = append(moves, apiv0.WidgetMoveRequest{WidgetID: id, X: w.Position.X, Y: w.Position.Y})
	}
	return moves
}
