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

// createLibrary asks for a name and creates a library, then selects it.
func (l *Library) createLibrary() {
	name, ok := prompt("Name of the new library:", "", false)
	if !ok {
		return
	}
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.CreateLibraryResponse, error) {
		return l.client.CreateLibrary(c, apiv0.CreateLibraryRequest{Name: name})
	}, func(ctx app.Context, created apiv0.CreateLibraryResponse) {
		l.libraries = append(l.libraries, apiv0.LibraryResponse{
			ID: created.ID, Name: name, Version: created.Version, HasUnsavedChanges: true,
		})
		l.selectLibrary(ctx, created.ID)
		l.loadLibraries(ctx)
	}, nil)
}

// renameLibrary asks for a new name of the selected library.
func (l *Library) renameLibrary() {
	lib, ok := l.selectedLibrary()
	if !ok {
		return
	}
	name, ok := prompt("New name of the library:", lib.Name, false)
	if !ok || name == lib.Name {
		return
	}
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.UpdateLibraryResponse, error) {
		return l.client.UpdateLibrary(c, lib.ID, apiv0.UpdateLibraryRequest{Name: name, Version: lib.Version})
	}, func(ctx app.Context, _ apiv0.UpdateLibraryResponse) {
		l.loadLibraries(ctx)
	}, l.reloadOnConflict)
}

// deleteLibrary deletes the selected library after a confirmation; the
// server refuses it while a project pins it or a running Deployment names one
// of its Releases, but old Commits and Releases keep only a link to it
// (task 71).
func (l *Library) deleteLibrary() {
	lib, ok := l.selectedLibrary()
	if !ok || !app.Window().Call("confirm", "Delete the library "+lib.Name+" with its Draft and releases? A project that pins it, or a running Deployment that names one of its releases, refuses the deletion. Commits and Releases of the past keep a link to the deleted library.").Bool() {
		return
	}
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.LibraryResponse, error) {
		return l.client.DeleteLibrary(c, lib.ID)
	}, func(ctx app.Context, _ apiv0.LibraryResponse) {
		l.loadLibraries(ctx)
	}, nil)
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
		l.loadReleases(ctx)
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
		// A Commit published meanwhile (409) leaves the history the truth.
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

// editReleaseNote changes the note of a Release after asking for it; the
// content never changes (task 72_5).
func (l *Library) editReleaseNote(release apiv0.LibraryReleaseResponse) {
	note, ok := prompt(fmt.Sprintf("Note of release %d:", release.Number), release.Note, true)
	if !ok {
		return
	}
	lib, ok := l.selectedLibrary()
	if !ok {
		return
	}
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.LibraryReleaseResponse, error) {
		return l.client.Library(lib.ID).UpdateReleaseNote(c, release.Number, apiv0.UpdateLibraryReleaseNoteRequest{Note: note})
	}, func(ctx app.Context, updated apiv0.LibraryReleaseResponse) {
		for i := range l.releases {
			if l.releases[i].Number == updated.Number {
				l.releases[i] = updated
			}
		}
		ctx.Update()
	}, l.reloadOnConflict)
}

// discard resets the selected library's Draft to its last Commit, dropping the
// unsaved changes, after the browser's confirmation. Without a Commit the
// server refuses (409), which is the toast (task 72_7; ADR 0021).
func (l *Library) discard() {
	lib, ok := l.selectedLibrary()
	if !ok || !app.Window().Call("confirm",
		"Discard every change of "+lib.Name+" since commit "+itoa(lib.LatestCommit)+"? The Draft goes back to that Commit.").Bool() {
		return
	}
	l.doDiscard(l.compoCtx)
}

// doDiscard performs the reset the confirmation asked for: one transaction of
// the Draft, which the answer states.
func (l *Library) doDiscard(ctx app.Context) {
	if l.draft() == nil {
		return
	}
	callDraft(l, func(c context.Context, draft *apiclient.LibraryClient) (apiv0.DiscardLibraryResponse, error) {
		return draft.Discard(c)
	}, func(ctx app.Context, state apiv0.DiscardLibraryResponse) {
		l.draftVersions.Forget()
		l.observeDraftVersion(l.markDraft(), state.Version)
		// The Draft is its last Commit again: nothing is unsaved, even though
		// its Version rose (task 72_7).
		l.cleanDraftVersion = state.Version
		l.loadList(ctx)
		l.loadLibraries(ctx)
		l.loadReleases(ctx)
		l.loadDraftChanges(ctx) // a Discard clears the stack (ADR 0017)
		ctx.Update()
	}, func(ctx app.Context, err error) {
		uiutil.ShowError(ctx, err)
		// A 409 nothing_to_discard means the history moved: the header is the
		// truth once it is reread.
		l.loadLibraries(ctx)
		l.loadDraftChanges(ctx)
		ctx.Update()
	})
}

// revert resets the selected library's Draft to its latest Release after a
// confirmation: the published state, not the last Save (task 72_5, task 72_7).
func (l *Library) revert() {
	lib, ok := l.selectedLibrary()
	if !ok || !app.Window().Call("confirm",
		"Revert "+lib.Name+" to its latest release "+itoa(lib.LatestRelease)+"? The Draft goes back to the published state.").Bool() {
		return
	}
	l.doRevert(l.compoCtx)
}

// doRevert performs the reset the confirmation asked for.
func (l *Library) doRevert(ctx app.Context) {
	clean := l.revertIsTheLastCommit()
	callDraft(l, func(c context.Context, draft *apiclient.LibraryClient) (apiv0.RevertLibraryResponse, error) {
		return draft.RevertToRelease(c)
	}, func(ctx app.Context, state apiv0.RevertLibraryResponse) {
		l.draftVersions.Forget()
		l.observeDraftVersion(l.markDraft(), state.Version)
		// A Draft reset to the Release of its last Commit is that Commit
		// again: nothing is unsaved. A Revert to an older Release differs from
		// the last Commit, and the indicator follows the Version (task 72_7).
		if clean {
			l.cleanDraftVersion = state.Version
		} else {
			l.cleanDraftVersion = 0
		}
		l.loadList(ctx)
		l.loadLibraries(ctx)
		l.loadReleases(ctx)
		l.loadDraftChanges(ctx) // a Revert clears the stack (ADR 0017)
		ctx.Update()
	}, func(ctx app.Context, err error) {
		uiutil.ShowError(ctx, err)
		l.loadLibraries(ctx)
		l.loadDraftChanges(ctx)
		ctx.Update()
	})
}

// revertIsTheLastCommit reports whether the latest Release published the last
// Commit: a Revert then puts the Draft back to that Commit, not to some older
// published state.
func (l *Library) revertIsTheLastCommit() bool {
	lib, ok := l.selectedLibrary()
	if !ok || lib.LatestCommit == 0 || len(l.releases) == 0 {
		return false
	}
	latest := l.releases[len(l.releases)-1]
	return latest.CommitNumber != nil && *latest.CommitNumber == lib.LatestCommit
}

// reloadOnConflict shows err and, on a conflict, rereads the libraries: the
// library changed meanwhile.
func (l *Library) reloadOnConflict(ctx app.Context, err error) {
	uiutil.ShowError(ctx, err)
	if errors.Is(err, apiclient.ErrConflict) {
		l.loadLibraries(ctx)
	}
}

// selectedRelease returns the release viewed.
func (l *Library) selectedRelease() (apiv0.LibraryReleaseResponse, bool) {
	for _, r := range l.releases {
		if r.Number == l.viewing {
			return r, true
		}
	}
	return apiv0.LibraryReleaseResponse{}, false
}
