package library

import (
	"context"
	"errors"
	"fmt"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Library operations ────────────────────────────────────────────────────────

// loadSavedState makes the saved state number the Draft (task 84), after a
// confirmation when the Draft holds unsaved changes: the load clears the undo
// stack, so the change is not one to take back.
func (l *Library) loadSavedState(number int) {
	lib, ok := l.selectedLibrary()
	if !ok || l.loadingSavedState != 0 {
		return
	}
	if l.hasUnsavedChanges() && !app.Window().Call("confirm",
		fmt.Sprintf("Replace the unsaved changes of %s with saved state %d? The Draft becomes what was saved then.",
			lib.Name, number)).Bool() {
		return
	}
	l.doLoadSavedState(l.compoCtx, number)
}

// doLoadSavedState performs the reset the confirmation asked for: one
// transaction of the Draft, which the answer states. The WidgetTypes follow
// with the list's reread.
func (l *Library) doLoadSavedState(ctx app.Context, number int) {
	if l.draft() == nil {
		return
	}
	l.loadingSavedState = number
	callDraft(l, func(c context.Context, draft *apiclient.LibraryClient) (apiv0.LoadSavedStateResponse, error) {
		return draft.LoadSavedState(c, number)
	}, func(ctx app.Context, loaded apiv0.LoadSavedStateResponse) {
		l.loadingSavedState = 0
		l.draftVersions.Forget()
		l.observeDraftVersion(l.markDraft(), loaded.Version)
		// The server's Draft state says whether the loaded saved state is the
		// last one: an older one leaves the Draft unsaved (task 74).
		l.loadDraftState(ctx)
		l.contentReplacements++ // the list fills the editor, never merges it
		l.loadList(ctx)
		l.loadLibraries(ctx)
		l.loadDraftChanges(ctx) // a load clears the stack (ADR 0017)
		l.closeHistory()
		ctx.NewActionWithValue(toast.ActionAdd, toast.Problem{Problem: apiv0.Problem{
			Title: fmt.Sprintf("Saved state %d loaded", number),
		}})
		ctx.Update()
	}, func(ctx app.Context, err error) {
		l.loadingSavedState = 0
		uiutil.ShowError(ctx, err)
		l.loadLibraries(ctx)
		l.loadDraftState(ctx)
		l.loadDraftChanges(ctx)
		ctx.Update()
	})
}

// reloadOnConflict shows err and, on a conflict, rereads the libraries: the
// library changed meanwhile.
func (l *Library) reloadOnConflict(ctx app.Context, err error) {
	uiutil.ShowError(ctx, err)
	if errors.Is(err, apiclient.ErrConflict) {
		l.loadLibraries(ctx)
	}
}
