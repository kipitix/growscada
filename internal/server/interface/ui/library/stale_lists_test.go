package library

import (
	"slices"
	"testing"

	"github.com/google/uuid"
)

// A list requested before a WidgetType was created is older than what the
// mode knows: it does not delete the new type and does not drop its selection
// (task 61).
func TestCreateWidgetType_AListSentBefore_KeepsTheSelection(t *testing.T) {
	url, api := newServer(t)
	createWidgetType(t, api, "Gauge")
	f := mount(t, url, api)
	stale := slices.Clone(f.l.widgetTypes) // a list in flight, before the new type
	mark := f.l.markDraft()

	f.l.createItem(f.l.compoCtx)
	f.run()
	created := f.l.selectedID
	f.l.setWidgetTypes(f.l.compoCtx, mark, f.l.contentReplacements, stale)

	if created == uuid.Nil || f.l.selectedID != created {
		t.Fatalf("selected %v, want the new WidgetType %v", f.l.selectedID, created)
	}
	if f.l.itemIdx(created) < 0 {
		t.Errorf("the new WidgetType is gone from the list: %+v", f.l.widgetTypes)
	}
}

// A list requested before a rename states the old name: it does not roll the
// name back in the list or in the editor, so the next Apply does not silently
// send the old one (task 61).
func TestWidgetTypeList_AnsweredAfterARename_DoesNotRollTheNameBack(t *testing.T) {
	f, id := selected(t)
	stale := slices.Clone(f.l.widgetTypes) // a list in flight, before the rename
	mark := f.l.markDraft()

	f.l.startEditing(id, "Gauge")
	f.l.editingName = "Dial"
	f.l.commitEdit(f.l.compoCtx)
	f.run()
	f.l.setWidgetTypes(f.l.compoCtx, mark, f.l.contentReplacements, stale)

	it, ok := f.l.findItem(id)
	if !ok || it.Name != "Dial" {
		t.Errorf("listed WidgetType %+v: the stale list rolled the name back", it)
	}
	if f.l.edit.name != "Dial" {
		t.Errorf("editor name %q: the stale list rolled it back", f.l.edit.name)
	}
}
