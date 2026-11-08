package library

import (
	"context"
	"errors"
	"fmt"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
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

// releaseCommit publishes a Commit of the selected library as the next Library
// Release (task 72_5; ADR 0021): a note is asked for, and the Author is the
// stored one, asked for when none is.
func (l *Library) releaseCommit(commit apiv0.CommitResponse) {
	lib, ok := l.selectedLibrary()
	if !ok || commit.Released {
		return
	}
	note, ok := prompt(fmt.Sprintf("Publish commit #%d of %s as release %d. Note (optional):",
		commit.Number, lib.Name, lib.LatestRelease+1), "", true)
	if !ok {
		return
	}
	author := l.saveAuthor
	if author == "" {
		if author, ok = prompt("Author:", "", false); !ok || author == "" {
			return
		}
		l.saveAuthor = author
		l.compoCtx.LocalStorage().Set(uiutil.AuthorKey, author)
	}
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.LibraryReleaseResponse, error) {
		return l.client.Library(lib.ID).Release(c, apiv0.CreateReleaseRequest{
			Commit: commit.Number, Note: note, Author: author,
		})
	}, func(ctx app.Context, _ apiv0.LibraryReleaseResponse) {
		l.loadReleases(ctx)
		l.loadCommits(ctx)
		l.loadLibraries(ctx)
	}, l.reloadOnConflict)
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

// discard resets the selected library's Draft to its last Commit after a
// confirmation; the server does it in one transaction and answers its state.
func (l *Library) discard() {
	lib, ok := l.selectedLibrary()
	if !ok || !app.Window().Call("confirm",
		"Discard every change of "+lib.Name+" since commit "+itoa(lib.LatestCommit)+"? The Draft goes back to that Commit.").Bool() {
		return
	}
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.DiscardLibraryResponse, error) {
		return l.client.Library(lib.ID).Discard(c)
	}, func(ctx app.Context, state apiv0.DiscardLibraryResponse) {
		l.draftVersions.Forget()
		l.observeDraftVersion(l.markDraft(), state.Version)
		l.loadList(ctx)
		l.loadLibraries(ctx)
		l.loadReleases(ctx)
		l.loadDraftChanges(ctx) // a Discard clears the stack (ADR 0017)
	}, l.reloadOnConflict)
}

// revert resets the selected library's Draft to its latest Release after a
// confirmation: the published state (task 72_5).
func (l *Library) revert() {
	lib, ok := l.selectedLibrary()
	if !ok || !app.Window().Call("confirm",
		"Revert "+lib.Name+" to its latest release "+itoa(lib.LatestRelease)+"? The Draft goes back to the published state.").Bool() {
		return
	}
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.RevertLibraryResponse, error) {
		return l.client.Library(lib.ID).RevertToRelease(c)
	}, func(ctx app.Context, state apiv0.RevertLibraryResponse) {
		l.draftVersions.Forget()
		l.observeDraftVersion(l.markDraft(), state.Version)
		l.loadList(ctx)
		l.loadLibraries(ctx)
		l.loadReleases(ctx)
		l.loadDraftChanges(ctx) // a Revert clears the stack (ADR 0017)
	}, l.reloadOnConflict)
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
