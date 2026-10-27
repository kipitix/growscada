package library

import (
	"context"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Data loading ──────────────────────────────────────────────────────────────

// loadLibraries (re)loads the library list and keeps a valid selection,
// falling back to the first library.
func (l *Library) loadLibraries(ctx app.Context) {
	uiutil.Load(l.compoCtx, &l.librariesReloader, l.client.ListLibraries, func(ctx app.Context, list []apiv0.LibraryResponse) {
		l.libraries = list
		if _, ok := l.selectedLibrary(); ok {
			return
		}
		next := uuid.Nil
		if len(list) > 0 {
			next = list[0].ID
		}
		l.libraryID = uuid.Nil // shown no longer: select anew
		l.selectLibrary(ctx, next)
	}, l.loadLibraries)
}

// loadList (re)loads the WidgetTypes of the selected library's Draft;
// concurrent requests are coalesced, a response of another library dropped.
func (l *Library) loadList(ctx app.Context) {
	if l.libraryID == uuid.Nil {
		l.widgetTypes = nil
		return
	}
	mark := l.versions.Mark()
	uiutil.LoadFor(l.compoCtx, &l.reloader, l.draft,
		func(c context.Context, lib *apiclient.LibraryClient) ([]apiv0.WidgetTypeResponse, error) {
			return lib.ListWidgetTypes(c)
		},
		func(ctx app.Context, _ *apiclient.LibraryClient, items []apiv0.WidgetTypeResponse) {
			l.setWidgetTypes(ctx, mark, items)
		}, l.loadList)
}

// loadReleases (re)loads the releases of the selected library.
func (l *Library) loadReleases(ctx app.Context) {
	if l.libraryID == uuid.Nil {
		l.releases = nil
		return
	}
	uiutil.LoadFor(l.compoCtx, &l.releasesReloader, l.draft,
		func(c context.Context, lib *apiclient.LibraryClient) ([]apiv0.LibraryReleaseResponse, error) {
			return lib.ListReleases(c)
		},
		func(ctx app.Context, _ *apiclient.LibraryClient, releases []apiv0.LibraryReleaseResponse) {
			l.releases = releases
		}, l.loadReleases)
}

// setWidgetTypes replaces the list and brings the editor of the selected type
// up to date: a type seen for the first time fills the editor, a known one is
// merged field by field so unsaved edits survive changes made elsewhere, and a
// deleted one drops the selection. mark is the moment the list was requested.
func (l *Library) setWidgetTypes(ctx app.Context, mark uiutil.Mark, items []apiv0.WidgetTypeResponse) {
	old, known := l.findItem(l.selectedID)
	for _, it := range items {
		l.versions.Observe(mark, it.ID, it.Version)
	}
	l.widgetTypes = items
	if l.selectedID == uuid.Nil || l.viewing != 0 {
		return
	}
	current, exists := l.findItem(l.selectedID)
	switch {
	case !exists:
		l.clearSelection()
		uiutil.StoreID(ctx, selectedIDKey, uuid.Nil)
	case !known:
		l.selectItem(l.selectedID)
	default:
		l.mergeSelected(old, current)
	}
}
