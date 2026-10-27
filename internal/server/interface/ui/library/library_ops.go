package library

import (
	"context"
	"errors"

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
		l.libraries = append(l.libraries, apiv0.LibraryResponse{ID: created.ID, Name: name, Version: created.Version, HasUnreleasedChanges: true})
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
// server refuses it while a project pins it.
func (l *Library) deleteLibrary() {
	lib, ok := l.selectedLibrary()
	if !ok || !app.Window().Call("confirm", "Delete the library "+lib.Name+" with its Draft and releases?").Bool() {
		return
	}
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.LibraryResponse, error) {
		return l.client.DeleteLibrary(c, lib.ID)
	}, func(ctx app.Context, _ apiv0.LibraryResponse) {
		l.loadLibraries(ctx)
	}, nil)
}

// release asks for an optional note and releases the selected library's
// Draft.
func (l *Library) release() {
	lib, ok := l.selectedLibrary()
	if !ok {
		return
	}
	note, ok := prompt("Release "+lib.Name+" as release "+itoa(lib.LatestRelease+1)+". Note (optional):", "", true)
	if !ok {
		return
	}
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.LibraryReleaseResponse, error) {
		return l.client.Library(lib.ID).Release(c, apiv0.CreateReleaseRequest{Note: note, Version: lib.Version})
	}, func(ctx app.Context, _ apiv0.LibraryReleaseResponse) {
		l.loadLibraries(ctx)
		l.loadReleases(ctx)
	}, l.reloadOnConflict)
}

// discard resets the selected library's Draft to its latest release after a
// confirmation; the server does it after the call, the Draft read-only
// meanwhile.
func (l *Library) discard() {
	lib, ok := l.selectedLibrary()
	if !ok || !app.Window().Call("confirm",
		"Discard every change of "+lib.Name+" since release "+itoa(lib.LatestRelease)+"? The Draft goes back to that release.").Bool() {
		return
	}
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.LibraryResponse, error) {
		return l.client.Library(lib.ID).Discard(c)
	}, func(ctx app.Context, marked apiv0.LibraryResponse) {
		if i := uiutil.IdxByID(l.libraries, marked.ID, uiutil.LibraryID); i >= 0 {
			l.libraries[i] = marked
		}
		l.loadLibraries(ctx)
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
