package project

import (
	"slices"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Widget selection (task 40) ────────────────────────────────────────────────
//
// The selection is an ordered set of Widget IDs of the shown Scene; the last
// element is primary — the one the properties panel edits and the one
// LocalStorage keeps. A click replaces the set with one ID, a modified click
// adds or removes one without moving anything, a marquee replaces it (or adds
// to it with Shift), Ctrl+A selects every Widget of the Scene, Esc clears it.

// isWidgetSelected reports whether id is in the selection.
func (p *Project) isWidgetSelected(id uuid.UUID) bool {
	return slices.Contains(p.selectedWidgetIDs, id)
}

// primaryWidgetID returns the last selected Widget, uuid.Nil for an empty
// selection.
func (p *Project) primaryWidgetID() uuid.UUID {
	if len(p.selectedWidgetIDs) == 0 {
		return uuid.Nil
	}
	return p.selectedWidgetIDs[len(p.selectedWidgetIDs)-1]
}

// singleSelected returns the only selected Widget and whether exactly one is
// selected: the properties panel and the canvas handles act on that one alone.
func (p *Project) singleSelected() (uuid.UUID, bool) {
	if len(p.selectedWidgetIDs) != 1 {
		return uuid.Nil, false
	}
	return p.selectedWidgetIDs[0], true
}

// selectOnly replaces the selection with id (uuid.Nil clears it).
func (p *Project) selectOnly(ctx app.Context, id uuid.UUID) {
	if id == uuid.Nil {
		p.selectWidgets(ctx, nil)
		return
	}
	p.selectWidgets(ctx, []uuid.UUID{id})
}

// selectWidgets replaces the selection with ids, in their order. A Widget no
// longer in the Scene is dropped.
func (p *Project) selectWidgets(ctx app.Context, ids []uuid.UUID) {
	p.endArrowSeries(ctx)
	p.selectedWidgetIDs = p.existingWidgets(ids)
	p.afterSelectionChange(ctx)
}

// addToSelection adds ids not selected yet, keeping the current order and the
// order of ids for the new ones.
func (p *Project) addToSelection(ctx app.Context, ids []uuid.UUID) {
	p.endArrowSeries(ctx)
	p.selectedWidgetIDs = p.existingWidgets(append(slices.Clone(p.selectedWidgetIDs), ids...))
	p.afterSelectionChange(ctx)
}

// toggleWidgetSelection adds id, or removes it when it is selected. A removed
// primary hands the lead to the last remaining Widget.
func (p *Project) toggleWidgetSelection(ctx app.Context, id uuid.UUID) {
	p.endArrowSeries(ctx)
	if i := slices.Index(p.selectedWidgetIDs, id); i >= 0 {
		p.selectedWidgetIDs = slices.Delete(p.selectedWidgetIDs, i, i+1)
	} else if _, ok := p.findWidget(id); ok {
		p.selectedWidgetIDs = append(p.selectedWidgetIDs, id)
	}
	p.afterSelectionChange(ctx)
}

// clearWidgetSelection empties the selection.
func (p *Project) clearWidgetSelection(ctx app.Context) {
	p.selectWidgets(ctx, nil)
}

// removeWidgetFromSelection drops id from the selection, if present.
func (p *Project) removeWidgetFromSelection(ctx app.Context, id uuid.UUID) {
	if i := slices.Index(p.selectedWidgetIDs, id); i >= 0 {
		p.selectedWidgetIDs = slices.Delete(p.selectedWidgetIDs, i, i+1)
		p.afterSelectionChange(ctx)
	}
}

// pruneSelection drops the selected IDs the Scene no longer holds, keeping
// the order and moving the lead to the last remaining. A reload of the
// Widgets calls it, so a Widget deleted by anyone leaves the selection. The
// properties editor is left to the caller: it still holds the user's unsaved
// fields until the matching Widget is known.
func (p *Project) pruneSelection(ctx app.Context) {
	kept := p.existingWidgets(p.selectedWidgetIDs)
	if len(kept) == len(p.selectedWidgetIDs) {
		return
	}
	p.selectedWidgetIDs = kept
	uiutil.StoreID(ctx, p.selectionKey(widgetIDKey), p.primaryWidgetID())
}

// selectAllWidgets selects every Widget of the Scene, in the Scene's order.
func (p *Project) selectAllWidgets(ctx app.Context) {
	ids := make([]uuid.UUID, 0, len(p.widgets))
	for _, w := range p.widgets {
		ids = append(ids, w.ID)
	}
	p.selectWidgets(ctx, ids)
}

// existingWidgets returns ids that are Widgets of the shown Scene, deduplicated
// in their first-seen order.
func (p *Project) existingWidgets(ids []uuid.UUID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if slices.Contains(out, id) {
			continue
		}
		if _, ok := p.findWidget(id); ok {
			out = append(out, id)
		}
	}
	return out
}

// afterSelectionChange stores the primary for the next reload and points the
// properties editor at the single selected Widget (no fields for none or
// several).
func (p *Project) afterSelectionChange(ctx app.Context) {
	uiutil.StoreID(ctx, p.selectionKey(widgetIDKey), p.primaryWidgetID())
	if id, ok := p.singleSelected(); ok {
		if w, found := p.findWidget(id); found {
			p.syncEditingFields(w)
			return
		}
	}
	p.widgetEdit = widgetEditFields{}
}

// syncMovedEditingFields keeps the properties panel in step with a move of the
// single selected Widget: the panel shows the position it now has.
func (p *Project) syncMovedEditingFields() {
	if idx := p.selectedWidgetIdx(); idx >= 0 {
		f := widgetEditFieldsOf(p.widgets[idx])
		p.widgetEdit.posX, p.widgetEdit.posY = f.posX, f.posY
	}
}
