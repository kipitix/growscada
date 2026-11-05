package draft

import (
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

func makeDraft(t *testing.T) Draft {
	t.Helper()
	scope := editlock.ProjectScope(id.NewID[project.Project]())
	return NewDraft(id.NewID[Draft](), scope, version.Committed[Draft](), 3, editlock.CreateEditLock(scope))
}

func TestDraft_Metadata(t *testing.T) {
	d := makeDraft(t)
	if d.Version() != version.Committed[Draft]() || d.StackPosition() != 3 {
		t.Errorf("draft %s at %d", d.Version(), d.StackPosition())
	}
	if d.Scope().IsProject() == d.Scope().IsLibrary() {
		t.Errorf("the scope names exactly one Draft: %s", d.Scope())
	}
	if d.Lock() == nil || d.Lock().Held() {
		t.Errorf("a new draft has a free EditLock, got %v", d.Lock())
	}
}

func TestDraft_WithVersionAndStackPosition(t *testing.T) {
	d := makeDraft(t)
	next := d.WithVersion(version.Committed[Draft]().Next()).WithStackPosition(4)
	if next.Version().Number() != 2 || next.StackPosition() != 4 {
		t.Errorf("the copy is at %d/%d", next.Version().Number(), next.StackPosition())
	}
	if d.Version().Number() != 1 || d.StackPosition() != 3 {
		t.Errorf("the original changed: %d/%d", d.Version().Number(), d.StackPosition())
	}
}

func TestDraft_LockChangesKeepTheContentVersion(t *testing.T) {
	d := makeDraft(t)
	holder, err := editlock.NewHolder(editlock.NewKey(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := d.Lock().Acquire(holder, time.Now())
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	held := d.WithLock(lock)
	if !held.Lock().Held() || held.Version() != d.Version() {
		t.Errorf("after a Check out: held %t at %s", held.Lock().Held(), held.Version())
	}
	if len(held.PendingEvents()) != 1 {
		t.Errorf("the lock recorded %d events, want the Check out", len(held.PendingEvents()))
	}
}
