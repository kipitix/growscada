package project

import (
	"context"
	"errors"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// The DraftChange stack of the Project's Draft (ADR 0017): the Undo and Redo
// buttons of the header, the Ctrl+Z and Ctrl+Shift+Z shortcuts, and the state
// they read (the descriptions of the next steps) — read on mount, on a resync,
// after each content event and after each command (loadDraftChanges). Both
// commands are writes of the Draft: they are gated by its EditLock and state
// its Version like any other (ADR 0015, ADR 0016).

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
// edit and the stack has a change to undo.
func (p *Project) canUndo() bool {
	return p.changesLoaded && !p.readOnly() && p.changes.CanUndo && !p.undoing && !p.redoing
}

// canRedo reports whether the Redo button acts now.
func (p *Project) canRedo() bool {
	return p.changesLoaded && !p.readOnly() && p.changes.CanRedo && !p.undoing && !p.redoing
}

// undoDraftChange undoes the change at the stack's position.
func (p *Project) undoDraftChange(ctx app.Context) { p.stepDraftChange(ctx, true) }

// redoDraftChange redoes the change after the stack's position.
func (p *Project) redoDraftChange(ctx app.Context) { p.stepDraftChange(ctx, false) }

// stepDraftChange runs an undo (undo true) or a redo of the Draft, stating the
// Version last seen: its answer is the state of the stack after it. The
// content itself comes with the events, as after any edit.
func (p *Project) stepDraftChange(ctx app.Context, undo bool) {
	if p.content == nil || p.undoing || p.redoing || p.refuseReadOnly(ctx) {
		return
	}
	// A command the buttons would not offer is not sent: a shortcut is the
	// button's own, and an empty stack is no request.
	if !p.changesLoaded || (undo && !p.changes.CanUndo) || (!undo && !p.changes.CanRedo) {
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
// refused edit is (the Problem names the Draft's state): the Version is
// forgotten and the stack and the content are read as they are now. An empty
// stack (the 409 carries no Problem.draft: nothing_to_undo or nothing_to_redo)
// is a toast and a reread of the stack alone, with no claim on the content.
func (p *Project) rejectedDraftChange(ctx app.Context, err error) {
	var se *apiclient.StatusError
	conflict := errors.Is(err, apiclient.ErrConflict)
	emptyStack := conflict && errors.As(err, &se) && se.Problem.Draft == nil
	if conflict && !emptyStack {
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

// renderDraftChangesBar is the Undo and Redo pair of the Project's header; it
// follows the EditLock row, which says whether the Draft may be edited at all.
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
// undo, why the button is off without the lock, or that the stack is empty.
func (p *Project) undoTitle() string {
	switch {
	case p.readOnly():
		return readOnlyTitle("Undo the last change")
	case !p.changesLoaded:
		return "Undo the last change"
	case !p.changes.CanUndo:
		return "Nothing to undo"
	default:
		return "Undo: " + p.changes.UndoDescription
	}
}

// redoTitle is the hover text of the Redo button.
func (p *Project) redoTitle() string {
	switch {
	case p.readOnly():
		return readOnlyTitle("Redo the change undone last")
	case !p.changesLoaded:
		return "Redo the change undone last"
	case !p.changes.CanRedo:
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
		ev := args[0]
		action := uiutil.KeyEventAction(ev)
		if action == uiutil.KeyNone || !p.shown() || !p.keyActionAllowed() {
			return nil
		}
		ev.Call("preventDefault") // the browser's own undo of the page
		p.compoCtx.Dispatch(func(ctx app.Context) { p.applyKeyAction(ctx, action) })
		return nil
	})
	app.Window().Call("addEventListener", "keydown", p.onKeyDown)
}

// noKeyDown releases the listener (OnDismount).
func (p *Project) noKeyDown() {
	if p.onKeyDown == nil {
		return
	}
	app.Window().Call("removeEventListener", "keydown", p.onKeyDown)
	p.onKeyDown.Release()
	p.onKeyDown = nil
}

// applyKeyAction runs the command a keydown asked for: the buttons' own, so a
// shortcut is refused exactly as they are (the lock, a command in flight).
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
// the dialog shows, and behind the name of a Check out or a takeover it would
// act on a Draft the user is only about to take.
func (p *Project) keyActionAllowed() bool {
	return !p.deployDialog && !p.revisionsDialog && !p.askingName && !p.takeOverDialog
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
