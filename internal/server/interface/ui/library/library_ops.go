package library

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Library operations ────────────────────────────────────────────────────────

// prompt asks the user for a text in the browser's dialog; false if they
// cancel. empty tells whether an empty answer is one.
func prompt(message, value string, empty bool) (string, bool) {
	answer := app.Window().Call("prompt", message, value)
	if answer.IsNull() || answer.IsUndefined() {
		return "", false
	}
	return answer.String(), empty || answer.String() != ""
}

// openReleaseDialog opens the publication of a Commit of the selected library
// as the next Library Release: a note (optional) and the stored Author
// (task 72_7; ADR 0021).
func (l *Library) openReleaseDialog(commit apiv0.CommitResponse) {
	if l.draft() == nil || commit.Released {
		return
	}
	l.releaseCommit = commit.Number
	l.releaseNote = ""
	l.releaseAuthor = l.saveAuthor
	l.releaseDialog = true
	l.compoCtx.Update()
}

// publishRelease confirms the release dialog: the Commit becomes the next
// immutable Release. It changes no Draft, so no Version is stated (task 72_7).
func (l *Library) publishRelease() {
	draft := l.draft()
	number := l.releaseCommit
	if draft == nil || l.releasing || number == 0 {
		return
	}
	author := strings.TrimSpace(l.releaseAuthor)
	if author == "" {
		uiutil.ShowError(l.compoCtx, fmt.Errorf("an author is required"))
		return
	}
	l.releasing = true
	l.saveAuthor = author
	l.compoCtx.LocalStorage().Set(uiutil.AuthorKey, author)
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.LibraryReleaseResponse, error) {
		return draft.Release(c, apiv0.CreateReleaseRequest{Commit: number, Note: l.releaseNote, Author: author})
	}, func(ctx app.Context, created apiv0.LibraryReleaseResponse) {
		if draft != l.draft() {
			l.releasing = false
			return
		}
		l.releasing, l.releaseDialog, l.releaseCommit = false, false, 0
		ctx.NewActionWithValue(toast.ActionAdd, toast.Problem{Problem: apiv0.Problem{
			Title:  fmt.Sprintf("Release %d published", created.Number),
			Detail: releaseTitle(created),
		}})
		l.loadReleases(ctx) // the row names the Release that published it
		l.loadCommits(ctx)
		l.loadLibraries(ctx)
		ctx.Update()
	}, func(ctx app.Context, err error) {
		if draft != l.draft() {
			l.releasing = false
			return
		}
		l.releasing = false
		uiutil.ShowError(ctx, err)
		// A saved state published meanwhile (409) leaves the history the truth.
		l.loadReleases(ctx)
		l.loadCommits(ctx)
		l.loadLibraries(ctx)
		ctx.Update()
	})
}

// releaseTitle is the Commit title a Release names as its detail.
func releaseTitle(released apiv0.LibraryReleaseResponse) string {
	if released.Title == nil {
		return ""
	}
	return *released.Title
}

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
	callDraft(l, func(c context.Context, draft *apiclient.LibraryClient) (apiv0.LoadCommitResponse, error) {
		return draft.LoadCommit(c, number)
	}, func(ctx app.Context, loaded apiv0.LoadCommitResponse) {
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
