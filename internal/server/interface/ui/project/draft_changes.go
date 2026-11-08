package project

import (
	"context"
	"errors"
	"strings"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// The DraftChange stack of the Project's Draft (ADR 0017): the Undo and Redo
// buttons of the header, the Ctrl+Z and Ctrl+Shift+Z shortcuts, and the state
// they read (the descriptions of the next steps) — read on mount, on a resync,
// after each content event and after each command (loadDraftChanges). Both
// commands are writes of the Draft: they state its Version like any other
// (ADR 0016).

// draftChangesVersion is the Version the last stack answer stated — what an
// undo and a redo state, since they apply the state the buttons were built
// from. Without an answer, the Version of the Draft the content was read at.
func (p *Project) draftChangesVersion() int {
	if p.changesLoaded {
		return p.changes.Version
	}
	return p.draftVersion()
}

// canUndo reports whether the Undo button acts now: the Draft is this tab's to
// edit and the stack names the change to undo. A state that offers a step but
// no description is no step either: the description is what the button shows
// as its hover text, and an unnamed step is not offered.
func (p *Project) canUndo() bool {
	return p.changesLoaded && p.changes.CanUndo && p.changes.UndoDescription != "" &&
		!p.undoing && !p.redoing
}

// canRedo reports whether the Redo button acts now.
func (p *Project) canRedo() bool {
	return p.changesLoaded && p.changes.CanRedo && p.changes.RedoDescription != "" &&
		!p.undoing && !p.redoing
}

// undoDraftChange undoes the change at the stack's position.
func (p *Project) undoDraftChange(ctx app.Context) { p.stepDraftChange(ctx, true) }

// redoDraftChange redoes the change after the stack's position.
func (p *Project) redoDraftChange(ctx app.Context) { p.stepDraftChange(ctx, false) }

// stepDraftChange runs an undo (undo true) or a redo of the Draft, stating the
// Version last seen: its answer is the state of the stack after it. The
// content itself comes with the events, as after any edit.
func (p *Project) stepDraftChange(ctx app.Context, undo bool) {
	if p.content == nil || p.undoing || p.redoing {
		return
	}
	// A command the buttons would not offer is not sent: a shortcut is the
	// button's own, and a step the stack does not name is no request.
	if (undo && !p.canUndo()) || (!undo && !p.canRedo()) {
		return
	}
	// A pending arrow series holds the Draft's Version: the undo states the
	// one it settles at.
	if p.waitMoves(ctx, func(ctx app.Context) { p.stepDraftChange(ctx, undo) }) {
		return
	}
	version := p.draftChangesVersion()
	mark := p.markDraft()
	if undo {
		p.undoing = true
	} else {
		p.redoing = true
	}
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.DraftChangesResponse, error) {
		if undo {
			return pc.UndoDraftChange(c, version)
		}
		return pc.RedoDraftChange(c, version)
	}, func(ctx app.Context, state apiv0.DraftChangesResponse) {
		p.undoing, p.redoing = false, false
		// The command raised the Draft's Version: the response states the new
		// one, which the next write states (the content follows by its events).
		p.observeDraftVersion(mark, state.Version)
		p.applyDraftChanges(mark, state)
		ctx.Update()
	}, func(ctx app.Context, err error) {
		p.undoing, p.redoing = false, false
		p.rejectedDraftChange(ctx, err)
	})
}

// rejectedDraftChange handles a refused undo or redo. A stale Version is what a
// refused edit is (the Problem carries the Draft's state): the Version is
// forgotten and the stack and the content are read as they are now. An empty
// stack (nothing_to_undo or nothing_to_redo, whose 409 carries no Draft) is a
// toast and a reread of the stack alone, with no claim on the content.
func (p *Project) rejectedDraftChange(ctx app.Context, err error) {
	var se *apiclient.StatusError
	problem := errors.Is(err, apiclient.ErrConflict) && errors.As(err, &se)
	if problem && se.Problem.Draft != nil {
		p.rejectedDraftWrite(ctx, err, func(ctx app.Context) {
			p.loadDraftChanges(ctx)
			p.loadScenes(ctx)
			p.loadWidgets(ctx)
			p.loadTags(ctx)
			p.loadPins(ctx)
			p.loadWidgetTypes(ctx)
			p.loadDeploy(ctx)
		})
		return
	}
	uiutil.ShowError(ctx, err)
	p.loadDraftChanges(ctx)
	ctx.Update()
}

// renderDraftChangesBar is the Undo and Redo pair of the Project's header.
func (p *Project) renderDraftChangesBar() app.UI {
	return app.Div().
		Attr("data-draft-changes-bar", "true").
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "6px").
		Body(
			p.draftChangeButton("Undo", "data-undo", p.undoTitle(), p.canUndo(), p.undoDraftChange),
			p.draftChangeButton("Redo", "data-redo", p.redoTitle(), p.canRedo(), p.redoDraftChange),
		)
}

// undoTitle is the hover text of the Undo button: the step the server would
// undo, or that the stack is empty.
func (p *Project) undoTitle() string {
	switch {
	case !p.changesLoaded:
		return "Undo the last change"
	case !p.changes.CanUndo || p.changes.UndoDescription == "":
		return "Nothing to undo"
	default:
		return "Undo: " + p.changes.UndoDescription
	}
}

// redoTitle is the hover text of the Redo button.
func (p *Project) redoTitle() string {
	switch {
	case !p.changesLoaded:
		return "Redo the change undone last"
	case !p.changes.CanRedo || p.changes.RedoDescription == "":
		return "Nothing to redo"
	default:
		return "Redo: " + p.changes.RedoDescription
	}
}

// draftChangeButton is one of the two: attr names it for the tests and the
// browser, title is its hover text.
func (p *Project) draftChangeButton(label, attr, title string, enabled bool, onClick func(app.Context)) app.UI {
	return app.Button().
		Attr(attr, "true").
		Title(title).
		Disabled(!enabled).
		Style("padding", "2px 9px").
		Style("font-size", "12px").
		Style("cursor", "pointer").
		Style("border", "1px solid var(--border-input)").
		Style("border-radius", "4px").
		Style("background", "var(--bg)").
		Style("color", "var(--text)").
		Text(label).
		OnClick(func(ctx app.Context, e app.Event) { onClick(ctx) }, app.EventScope(label))
}

// ── Keyboard shortcuts ────────────────────────────────────────────────────────

// listenShortcuts makes Ctrl+Z and Ctrl+Shift+Z (Cmd on macOS) undo and redo
// the Draft while the Project mode is shown.
func (p *Project) listenShortcuts() {
	if p.onKeyDown != nil {
		return
	}
	p.onKeyDown = app.FuncOf(func(this app.Value, args []app.Value) any {
		if len(args) == 0 {
			return nil
		}
		p.handleKeyDown(args[0])
		return nil
	})
	p.onKeyUp = app.FuncOf(func(this app.Value, args []app.Value) any {
		if len(args) == 0 {
			return nil
		}
		p.handleKeyUp(args[0])
		return nil
	})
	p.onBlur = app.FuncOf(func(this app.Value, args []app.Value) any {
		// The Alt+Tab that leaves the window also ends the series: its
		// keyup may never arrive here.
		if p.arrowActive && p.mounted {
			p.compoCtx.Dispatch(func(ctx app.Context) { p.flushArrowMoves(ctx) })
		}
		return nil
	})
	app.Window().Call("addEventListener", "keydown", p.onKeyDown)
	app.Window().Call("addEventListener", "keyup", p.onKeyUp)
	app.Window().Call("addEventListener", "blur", p.onBlur)
}

// noKeyDown releases the listeners (OnDismount).
func (p *Project) noKeyDown() {
	p.stopArrowTimer()
	for _, l := range []struct {
		event string
		fn    *app.Func
	}{{"keydown", &p.onKeyDown}, {"keyup", &p.onKeyUp}, {"blur", &p.onBlur}} {
		if *l.fn == nil {
			continue
		}
		app.Window().Call("removeEventListener", l.event, *l.fn)
		(*l.fn).Release()
		*l.fn = nil
	}
}

// handleKeyDown answers the keyboard of the shown mode: Ctrl/Cmd+A selects
// every Widget of the Scene, Esc drops the selection (and a rectangle being
// dragged), the arrows move the selection, and Ctrl+Z / Ctrl+Shift+Z undo and
// redo the Draft. All of them are ignored in a field the user types in, behind
// a dialog of the mode and while another mode is shown.
func (p *Project) handleKeyDown(ev app.Value) {
	if !p.shown() || !p.keyActionAllowed() || uiutil.KeyEventTyping(ev) {
		return
	}
	ctrl := ev.Get("ctrlKey").Bool() || ev.Get("metaKey").Bool()
	key := ev.Get("key").String()
	code := ev.Get("code").String()
	if key == "Alt" {
		// Alt of a body drag: it frees the Widgets of snapping and hides the
		// guides at once, and releasing it snaps them back (task 41).
		p.compoCtx.Dispatch(func(ctx app.Context) { p.setDragAlt(ctx, true) })
		return
	}
	switch {
	case ctrl && (strings.EqualFold(key, "a") || code == "KeyA"):
		ev.Call("preventDefault")
		p.compoCtx.Dispatch(func(ctx app.Context) { p.selectAllWidgets(ctx) })
		return
	case key == "Escape":
		ev.Call("preventDefault")
		p.compoCtx.Dispatch(func(ctx app.Context) { p.escapeSelection(ctx) })
		return
	}
	if dx, dy, arrow := arrowStep(key, ev.Get("shiftKey").Bool()); arrow {
		ev.Call("preventDefault") // the page does not scroll under the selection
		p.compoCtx.Dispatch(func(ctx app.Context) { p.beginArrowMove(ctx, dx, dy) })
		return
	}
	action := uiutil.KeyEventAction(ev)
	if action == uiutil.KeyNone {
		return
	}
	ev.Call("preventDefault") // the browser's own undo of the page
	p.compoCtx.Dispatch(func(ctx app.Context) { p.applyKeyAction(ctx, action) })
}

// handleKeyUp ends an arrow series when its arrow is released, and gives up Alt
// of a body drag when Alt is released (task 41).
func (p *Project) handleKeyUp(ev app.Value) {
	if ev.Get("key").String() == "Alt" {
		p.compoCtx.Dispatch(func(ctx app.Context) { p.setDragAlt(ctx, false) })
		return
	}
	if _, _, arrow := arrowStep(ev.Get("key").String(), false); !arrow || !p.arrowActive {
		return
	}
	p.compoCtx.Dispatch(func(ctx app.Context) { p.flushArrowMoves(ctx) })
}

// arrowStep is the move an arrow key asks for: 1 px, or 10 px with Shift.
func arrowStep(key string, shift bool) (dx, dy float64, ok bool) {
	step := 1.0
	if shift {
		step = 10
	}
	switch key {
	case "ArrowLeft":
		return -step, 0, true
	case "ArrowRight":
		return step, 0, true
	case "ArrowUp":
		return 0, -step, true
	case "ArrowDown":
		return 0, step, true
	}
	return 0, 0, false
}

// escapeSelection drops the selection and a rectangle being dragged with it.
func (p *Project) escapeSelection(ctx app.Context) {
	p.cancelMarquee()
	p.clearWidgetSelection(ctx)
}

// applyKeyAction runs the command a keydown asked for: the buttons' own, so a
// shortcut is refused exactly as they are (a command in flight).
func (p *Project) applyKeyAction(ctx app.Context, action uiutil.KeyAction) {
	if !p.keyActionAllowed() {
		return
	}
	switch action {
	case uiutil.KeyUndo:
		p.undoDraftChange(ctx)
	case uiutil.KeyRedo:
		p.redoDraftChange(ctx)
	}
}

// keyActionAllowed reports whether a shortcut may act now: no dialog of the
// mode is open. An undo behind the Deploy confirmation would change the Draft
// the dialog shows, and one behind the Save dialog would change the Draft the
// Save is about to fix as a Commit.
func (p *Project) keyActionAllowed() bool {
	return !p.deployDialog && !p.deploymentsDialog && !p.projectFileDialog &&
		!p.saveDialog && !p.commitsDialog
}

// shown reports whether the Project mode is the one displayed. Root keeps every
// mode mounted and hides the others with visibility, so a keydown anywhere
// reaches this listener even while another mode is shown.
func (p *Project) shown() bool {
	el := p.JSValue()
	if el.IsNull() || el.IsUndefined() {
		return false
	}
	return app.Window().Call("getComputedStyle", el).Get("visibility").String() != "hidden"
}
