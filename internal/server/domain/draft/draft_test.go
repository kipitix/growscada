package draft

import (
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

func makeDraft(t *testing.T) Draft {
	t.Helper()
	scope := ProjectScope(id.NewID[project.Project]())
	return NewDraft(id.NewID[Draft](), scope, version.Committed[Draft](), 3)
}

func TestDraft_Metadata(t *testing.T) {
	d := makeDraft(t)
	if d.Version() != version.Committed[Draft]() || d.StackPosition() != 3 {
		t.Errorf("draft %s at %d", d.Version(), d.StackPosition())
	}
	if d.Scope().IsProject() == d.Scope().IsLibrary() {
		t.Errorf("the scope names exactly one Draft: %s", d.Scope())
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
