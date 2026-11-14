package label_test

import (
	"errors"
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/label"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// projectScope and libraryScope are the two kinds of saved state a label
// marks (task 85_3; ADR 0025).
func projectScope() draft.Scope { return draft.ProjectScope(id.NewID[project.Project]()) }
func libraryScope() draft.Scope { return draft.LibraryScope(id.NewID[library.Library]()) }

func TestNewAssignment_ValidatesScopeAndNumber(t *testing.T) {
	at := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	commitID := id.NewID[draft.Commit]()
	assignment, err := label.NewAssignment(commitID, projectScope(), 3, at)
	if err != nil {
		t.Fatalf("a valid assignment: %v", err)
	}
	if assignment.CommitID() != commitID || !assignment.Scope().IsProject() ||
		assignment.CommitNumber() != 3 || !assignment.AssignedAt().Equal(at) {
		t.Errorf("the assignment: %s", assignment)
	}
	if _, err := label.NewAssignment(commitID, libraryScope(), 1, at); err != nil {
		t.Errorf("a library's assignment: %v", err)
	}

	for _, bad := range []struct {
		commitID id.ID[draft.Commit]
		scope    draft.Scope
		number   int
	}{
		{commitID, projectScope(), 0},
		{commitID, projectScope(), -1},
		{commitID, draft.Scope{}, 1},
		{id.ID[draft.Commit]{}, projectScope(), 1},
	} {
		if _, err := label.NewAssignment(bad.commitID, bad.scope, bad.number, at); !errors.Is(err, label.ErrInvalidAssignment) {
			t.Errorf("assignment %v/%d: %v, want ErrInvalidAssignment", bad.scope, bad.number, err)
		}
	}
}

func TestLabel_AssignIsIdempotent(t *testing.T) {
	labelID := id.NewID[label.Label]()
	scope := projectScope()
	commitID := id.NewID[draft.Commit]()
	at := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	created := label.ReconstituteLabel(labelID, id.NewID[organization.Organization](),
		mustName(t, "Ready"), mustColor(t, "#22c55e"), at, nil)

	assigned, err := created.Assign(commitID, scope, 3, at)
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if len(assigned.Assignments()) != 1 || assigned.Assignments()[0].CommitID() != commitID {
		t.Fatalf("the assignment: %v", assigned.Assignments())
	}
	if len(created.Assignments()) != 0 {
		t.Errorf("Assign changed the aggregate it was called on: %v", created.Assignments())
	}
	pending := assigned.PendingEvents()
	if countEvents(pending, event.EventTypeLabelAssigned) != 1 {
		t.Fatalf("the assignment's events: %v", pending)
	}
	recorded, ok := pending[0].(label.LabelAssignedEvent)
	if !ok || recorded.LabelID() != labelID || recorded.Scope() != scope || recorded.CommitNumber() != 3 {
		t.Errorf("the recorded assignment event: %v", pending[0])
	}

	// The same mark again changes nothing and records nothing.
	repeated, err := assigned.Assign(commitID, scope, 3, at.Add(time.Hour))
	if err != nil {
		t.Fatalf("a repeated Assign: %v", err)
	}
	if len(repeated.Assignments()) != 1 || countEvents(repeated.PendingEvents(), event.EventTypeLabelAssigned) != 1 {
		t.Errorf("a repeated Assign: %v, events %v", repeated.Assignments(), repeated.PendingEvents())
	}

	// Another saved state is another mark; the set is ordered as it was made.
	otherCommit := id.NewID[draft.Commit]()
	second, err := repeated.Assign(otherCommit, libraryScope(), 1, at)
	if err != nil {
		t.Fatalf("a second Assign: %v", err)
	}
	if len(second.Assignments()) != 2 || second.Assignments()[0].CommitID() != commitID {
		t.Errorf("the assignments: %v", second.Assignments())
	}
}

func TestLabel_UnassignIsIdempotent(t *testing.T) {
	labelID := id.NewID[label.Label]()
	scope := libraryScope()
	commitID := id.NewID[draft.Commit]()
	at := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	created := label.ReconstituteLabel(labelID, id.NewID[organization.Organization](),
		mustName(t, "Ready"), label.Color{}, at, nil)
	assigned, err := created.Assign(commitID, scope, 2, at)
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}

	unassigned, err := assigned.Unassign(commitID)
	if err != nil {
		t.Fatalf("Unassign: %v", err)
	}
	if len(unassigned.Assignments()) != 0 || len(assigned.Assignments()) != 1 {
		t.Errorf("the unassignment: %v / %v", unassigned.Assignments(), assigned.Assignments())
	}
	pending := unassigned.PendingEvents()
	if countEvents(pending, event.EventTypeLabelUnassigned) != 1 {
		t.Fatalf("the unassignment's events: %v", pending)
	}
	recorded, ok := pending[len(pending)-1].(label.LabelUnassignedEvent)
	if !ok || recorded.Scope() != scope || recorded.CommitNumber() != 2 || recorded.LabelID() != labelID {
		t.Errorf("the recorded unassignment event: %v", pending[0])
	}

	// Taking off a mark that is not there changes nothing and records nothing.
	repeated, err := unassigned.Unassign(commitID)
	if err != nil {
		t.Fatalf("a repeated Unassign: %v", err)
	}
	if len(repeated.Assignments()) != 0 || countEvents(repeated.PendingEvents(), event.EventTypeLabelUnassigned) != 1 {
		t.Errorf("a repeated Unassign: %v, events %v", repeated.Assignments(), repeated.PendingEvents())
	}
}

func TestLabel_AssignmentsAreCopiedOut(t *testing.T) {
	at := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	assigned, err := label.ReconstituteLabel(id.NewID[label.Label](), id.NewID[organization.Organization](),
		mustName(t, "Ready"), label.Color{}, at, nil).
		Assign(id.NewID[draft.Commit](), projectScope(), 1, at)
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	out := assigned.Assignments()
	out[0] = nil
	if got := assigned.Assignments(); len(got) != 1 || got[0] == nil {
		t.Errorf("the caller changed the label's assignments: %v", got)
	}
}

// countEvents is how many of the events are of the given type: a chained call
// keeps the events recorded before it, so an idempotent repeat is checked by
// the absence of a new one.
func countEvents(events []event.Event, eventType event.EventType) int {
	count := 0
	for _, e := range events {
		if e.Type() == eventType {
			count++
		}
	}
	return count
}
