package library

import (
	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v10/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Data loading ──────────────────────────────────────────────────────────────

// loadList (re)loads the widget type list; concurrent requests are coalesced.
func (l *Library) loadList(ctx app.Context) {
	mark := l.versions.Mark()
	uiutil.Load(l.compoCtx, &l.reloader, l.client.ListWidgetTypes, func(ctx app.Context, items []apiv0.WidgetTypeResponse) {
		l.setWidgetTypes(ctx, mark, items)
	}, l.loadList)
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
	if l.selectedID == uuid.Nil {
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
