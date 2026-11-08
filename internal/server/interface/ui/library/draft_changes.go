package library

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// The DraftChange stack of the selected library's Draft (ADR 0017): the Undo
// and Redo buttons of the libraries column, the Ctrl+Z and Ctrl+Shift+Z
// shortcuts, and the state they read (the descriptions of the next steps) —
// read on mount, on a resync, after each content event and after each command
// (loadDraftChanges). Both commands are writes of the Draft: they are gated by
// its Version like any other (ADR 0016).

// draftChangesVersion is the Version the last stack answer stated — what an
// undo and a redo state, since they apply the state the buttons were built
// from. Without an answer, the Version of the Draft the WidgetTypes were read
// at.
func (l *Library) draftChangesVersion() int {
	if l.changesLoaded {
		return l.changes.Version
	}
	return l.draftVersion()
}

// canUndo reports whether the Undo button acts now: a Draft this tab may edit —
// not a release shown read-only — with a change to undo. A state that offers a
// step but no description is no step either: the description is what the button
// shows as its hover text, and an unnamed step is not offered.
func (l *Library) canUndo() bool {
	return l.changesLoaded && !l.readOnly() && l.changes.CanUndo && l.changes.UndoDescription != "" &&
		!l.undoing && !l.redoing
}

// canRedo reports whether the Redo button acts now.
func (l *Library) canRedo() bool {
	return l.changesLoaded && !l.readOnly() && l.changes.CanRedo && l.changes.RedoDescription != "" &&
		!l.undoing && !l.redoing
}

// undoDraftChange undoes the change at the stack's position.
func (l *Library) undoDraftChange(ctx app.Context) { l.stepDraftChange(ctx, true) }

// redoDraftChange redoes the change after the stack's position.
func (l *Library) redoDraftChange(ctx app.Context) { l.stepDraftChange(ctx, false) }

// stepDraftChange runs an undo (undo true) or a redo of the Draft, stating the
// Version last seen: its answer is the state of the stack after it. The
// WidgetTypes themselves come with the events, as after any edit.
func (l *Library) stepDraftChange(ctx app.Context, undo bool) {
	if l.draft() == nil || l.undoing || l.redoing || l.readOnly() {
		return
	}
	// A command the buttons would not offer is not sent: a shortcut is the
	// button's own, and a step the stack does not name is no request.
	if (undo && !l.canUndo()) || (!undo && !l.canRedo()) {
		return
	}
	version := l.draftChangesVersion()
	mark := l.markDraft()
	if undo {
		l.undoing = true
	} else {
		l.redoing = true
	}
	callDraft(l, func(c context.Context, lib *apiclient.LibraryClient) (apiv0.DraftChangesResponse, error) {
		if undo {
			return lib.UndoDraftChange(c, version)
		}
		return lib.RedoDraftChange(c, version)
	}, func(ctx app.Context, state apiv0.DraftChangesResponse) {
		l.undoing, l.redoing = false, false
		// The command raised the Draft's Version: the response states the new
		// one, which the next write states (the WidgetTypes follow by their
		// events).
		l.observeDraftVersion(mark, state.Version)
		l.applyDraftChanges(mark, state)
		ctx.Update()
	}, func(ctx app.Context, err error) {
		l.undoing, l.redoing = false, false
		l.rejectedDraftChange(ctx, err)
	})
}

// rejectedDraftChange handles a refused undo or redo. A stale Version is what a
// refused edit is (the Problem carries the Draft's state): the Version is
// forgotten and the list and the stack are read as they are now. An empty stack
// (nothing_to_undo or nothing_to_redo, whose 409 carries no Draft) is a toast
// and a reread of the stack alone, with no claim on the WidgetTypes.
func (l *Library) rejectedDraftChange(ctx app.Context, err error) {
	var se *apiclient.StatusError
	problem := errors.Is(err, apiclient.ErrConflict) && errors.As(err, &se)
	if problem && se.Problem.Draft != nil {
		uiutil.ShowError(ctx, err)
		l.forgetDraftVersion()
		l.loadDraftChanges(ctx)
		l.loadList(ctx)
		l.loadReleases(ctx)
		l.loadLibraries(ctx)
		ctx.Update()
		return
	}
	uiutil.ShowError(ctx, err)
	l.loadDraftChanges(ctx)
	ctx.Update()
}

// renderDraftChangesBar is the Undo and Redo pair of the libraries column.
func (l *Library) renderDraftChangesBar() app.UI {
	if l.libraryID == uuid.Nil {
		return app.Div()
	}
	return app.Div().
		Attr("data-draft-changes-bar", "true").
		Style("display", "flex").
		Style("gap", "4px").
		Style("margin-bottom", "4px").
		Body(
			smallButton("Undo", l.canUndo(), "", l.undoTitle(), l.undoDraftChangeFromButton),
			smallButton("Redo", l.canRedo(), "", l.redoTitle(), l.redoDraftChangeFromButton),
		)
}

// undoDraftChangeFromButton is the Undo button's handler: smallButton takes a
// handler without the context, and the command runs with the component's own
// (the button's element may be reused while the answer is on its way).
func (l *Library) undoDraftChangeFromButton() { l.undoDraftChange(l.compoCtx) }

// redoDraftChangeFromButton is the Redo button's handler.
func (l *Library) redoDraftChangeFromButton() { l.redoDraftChange(l.compoCtx) }

// undoTitle is the hover text of the Undo button: the step the server would
// undo, or that the stack is empty.
func (l *Library) undoTitle() string {
	switch {
	case l.viewing != 0:
		return "Undo the last change — a release is shown; open the Draft to edit it"
	case !l.changesLoaded:
		return "Undo the last change"
	case !l.changes.CanUndo || l.changes.UndoDescription == "":
		return "Nothing to undo"
	default:
		return "Undo: " + l.changes.UndoDescription
	}
}

// redoTitle is the hover text of the Redo button.
func (l *Library) redoTitle() string {
	switch {
	case l.viewing != 0:
		return "Redo the change undone last — a release is shown; open the Draft to edit it"
	case !l.changesLoaded:
		return "Redo the change undone last"
	case !l.changes.CanRedo || l.changes.RedoDescription == "":
		return "Nothing to redo"
	default:
		return "Redo: " + l.changes.RedoDescription
	}
}

// ── Keyboard shortcuts ────────────────────────────────────────────────────────

// listenShortcuts makes Ctrl+Z and Ctrl+Shift+Z (Cmd on macOS) undo and redo
// the Draft while the Library mode is shown.
func (l *Library) listenShortcuts() {
	if l.onKeyDown != nil {
		return
	}
	l.onKeyDown = app.FuncOf(func(this app.Value, args []app.Value) any {
		if len(args) == 0 {
			return nil
		}
		ev := args[0]
		action := uiutil.KeyEventAction(ev)
		if action == uiutil.KeyNone || !l.shown() || !l.keyActionAllowed() {
			return nil
		}
		ev.Call("preventDefault") // the browser's own undo of the page
		l.compoCtx.Dispatch(func(ctx app.Context) { l.applyKeyAction(ctx, action) })
		return nil
	})
	app.Window().Call("addEventListener", "keydown", l.onKeyDown)
}

// keyActionAllowed reports whether a shortcut may act now: no dialog of the
// mode is open. An undo behind the Save dialog would change the Draft the Save
// is about to fix as a Commit.
func (l *Library) keyActionAllowed() bool {
	return !l.saveDialog && !l.commitsDialog
}

// noKeyDown releases the listener (OnDismount).
func (l *Library) noKeyDown() {
	if l.onKeyDown == nil {
		return
	}
	app.Window().Call("removeEventListener", "keydown", l.onKeyDown)
	l.onKeyDown.Release()
	l.onKeyDown = nil
}

// applyKeyAction runs the command a keydown asked for: the buttons' own, so a
// shortcut is refused exactly as they are (a release shown, a command in
// flight).
func (l *Library) applyKeyAction(ctx app.Context, action uiutil.KeyAction) {
	switch action {
	case uiutil.KeyUndo:
		l.undoDraftChange(ctx)
	case uiutil.KeyRedo:
		l.redoDraftChange(ctx)
	}
}

// shown reports whether the Library mode is the one displayed. Root keeps every
// mode mounted and hides the others with visibility, so a keydown anywhere
// reaches this listener even while another mode is shown.
func (l *Library) shown() bool {
	el := l.JSValue()
	if el.IsNull() || el.IsUndefined() {
		return false
	}
	return app.Window().Call("getComputedStyle", el).Get("visibility").String() != "hidden"
}
