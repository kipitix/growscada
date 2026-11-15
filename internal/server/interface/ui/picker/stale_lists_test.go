package picker

import (
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/servertest"
)

// A list requested before a rename states the old name: it does not roll it
// back on screen, so the next edit does not state the old Version (task 61).
func TestPicker_AListSentBeforeARename_DoesNotRollTheNameBack(t *testing.T) {
	f := startProjects(t, nil)
	f.run()
	item := f.selectByName(servertest.MainProjectName)
	stale := slices.Clone(f.p.items) // a list in flight, before the rename
	mark := f.p.versions.Mark()

	f.p.startEdit(f.p.compoCtx, fieldName)
	f.p.value = "Omega"
	f.p.commitEdit(f.p.compoCtx)
	f.run()
	f.p.applyList(f.p.compoCtx, mark, stale)

	got, ok := f.p.find(item.ID)
	if !ok || got.Name != "Omega" {
		t.Errorf("listed %+v: the stale list rolled the name back", got)
	}
	if f.p.selectedName != "Omega" {
		t.Errorf("selected name %q: the stale list rolled it back", f.p.selectedName)
	}
}

// A list requested before a creation misses the new entity: it neither clears
// the selection nor takes the created row away (task 61).
func TestPicker_AListSentBeforeACreate_KeepsTheSelection(t *testing.T) {
	f := startProjects(t, nil)
	f.run()
	stale := slices.Clone(f.p.items) // a list in flight, before the creation
	mark := f.p.versions.Mark()

	f.p.startCreate(f.p.compoCtx)
	f.p.createName = "New"
	f.p.commitCreate(f.p.compoCtx)
	f.run()
	created := f.p.selected
	f.p.applyList(f.p.compoCtx, mark, stale)

	if created == uuid.Nil || f.p.selected != created {
		t.Fatalf("selected %v, want the new entity %v", f.p.selected, created)
	}
	if _, ok := f.p.find(created); !ok {
		t.Errorf("the new entity is gone from the list: %v", f.names())
	}
}
