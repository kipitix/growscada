package project

import (
	"errors"
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

func mustProjectName(t *testing.T, s string) ProjectName {
	t.Helper()
	name, err := NewProjectName(s)
	if err != nil {
		t.Fatalf("NewProjectName(%q): %v", s, err)
	}
	return name
}

// makeStoredProject builds a Project as read back from a repository.
func makeStoredProject(t *testing.T) Project {
	t.Helper()
	ver, _ := version.New(version.WithNumber[Project](2))
	return ReconstituteProject(id.NewID[Project](), id.NewID[organization.Organization](), mustProjectName(t, "Line 1"), false, false, ver)
}

func TestNewProjectName_RejectsEmpty(t *testing.T) {
	if _, err := NewProjectName(""); err == nil {
		t.Error("expected an error for an empty name")
	}
	if got := mustProjectName(t, "Main").String(); got != "Main" {
		t.Errorf("String() = %q, want %q", got, "Main")
	}
}

func TestCreateProject_RecordsCreatedEventAtInitialVersion(t *testing.T) {
	projectID, orgID := id.NewID[Project](), id.NewID[organization.Organization]()
	p := CreateProject(projectID, orgID, mustProjectName(t, "Main"))

	if p.ID() != projectID || p.OrganizationID() != orgID || p.Name().String() != "Main" {
		t.Errorf("unexpected project %s", p)
	}
	if p.Version() != version.Initial[Project]() || p.IsDeleted() {
		t.Errorf("expected a live project at the initial version, got %s", p)
	}
	events := p.PendingEvents()
	if len(events) != 1 || events[0].Type() != event.EventTypeProjectCreated {
		t.Fatalf("expected one project_created, got %v", events)
	}
	if created, ok := events[0].(ProjectCreatedEvent); !ok || created.ProjectID() != projectID {
		t.Errorf("expected ProjectCreatedEvent of %s, got %v", projectID, events[0])
	}
}

func TestReconstituteProject_RecordsNothing(t *testing.T) {
	if got := makeStoredProject(t).PendingEvents(); len(got) != 0 {
		t.Errorf("expected no pending events, got %v", got)
	}
}

func TestProjectRename_ChangesTheNameAndRecordsUpdatedEvent(t *testing.T) {
	p := makeStoredProject(t)

	renamed, err := p.Rename(p.Version(), mustProjectName(t, "Line 2"))

	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if renamed.Name().String() != "Line 2" || renamed.Version() != p.Version() || renamed.ID() != p.ID() {
		t.Errorf("unexpected renamed project %s", renamed)
	}
	if p.Name().String() != "Line 1" || len(p.PendingEvents()) != 0 {
		t.Error("Rename must leave the original as it was")
	}
	events := renamed.PendingEvents()
	if len(events) != 1 || events[0].Type() != event.EventTypeProjectUpdated {
		t.Errorf("expected one project_updated, got %v", events)
	}
}

func TestProjectRename_StaleVersionIsConflict(t *testing.T) {
	p := makeStoredProject(t)

	_, err := p.Rename(version.Committed[Project](), mustProjectName(t, "Line 2"))

	if !errors.Is(err, ErrProjectConflict) {
		t.Errorf("expected ErrProjectConflict, got %v", err)
	}
}

func TestProjectDelete_MarksDeletedAndRecordsDeletedEvent(t *testing.T) {
	p := makeStoredProject(t)

	deleted := p.Delete()

	if !deleted.IsDeleted() || deleted.Version() != p.Version() || deleted.Name() != p.Name() {
		t.Errorf("unexpected deleted project %s", deleted)
	}
	if p.IsDeleted() {
		t.Error("Delete must leave the original as it was")
	}
	events := deleted.PendingEvents()
	if len(events) != 1 || events[0].Type() != event.EventTypeProjectDeleted {
		t.Fatalf("expected one project_deleted, got %v", events)
	}
	if ev, ok := events[0].(ProjectEvent); !ok || ev.ProjectID() != p.ID() {
		t.Errorf("expected ProjectEvent of %s, got %v", p.ID(), events[0])
	}
}

func TestProjectStartDiscard_MarksItAndRecordsTheEvent(t *testing.T) {
	p := makeStoredProject(t)

	marked, err := p.StartDiscard()

	if err != nil {
		t.Fatalf("StartDiscard: %v", err)
	}
	if !marked.IsDiscarding() || marked.Version() != p.Version() {
		t.Errorf("unexpected marked project %s", marked)
	}
	if p.IsDiscarding() {
		t.Error("StartDiscard must leave the original as it was")
	}
	events := marked.PendingEvents()
	if len(events) != 1 || events[0].Type() != event.EventTypeProjectDiscardStarted {
		t.Fatalf("expected one project_discard_started, got %v", events)
	}
	if ev, ok := events[0].(ProjectEvent); !ok || ev.ProjectID() != p.ID() {
		t.Errorf("expected ProjectEvent of %s, got %v", p.ID(), events[0])
	}
}

func TestProjectStartDiscard_OfADiscardingProject_IsRefused(t *testing.T) {
	marked, err := makeStoredProject(t).StartDiscard()
	if err != nil {
		t.Fatalf("StartDiscard: %v", err)
	}

	if _, err := marked.StartDiscard(); !errors.Is(err, ErrProjectDiscarding) {
		t.Errorf("a second StartDiscard: %v, want ErrProjectDiscarding", err)
	}
}

func TestProjectFinishDiscard_ClearsTheMarkAndRecordsUpdated(t *testing.T) {
	marked, err := makeStoredProject(t).StartDiscard()
	if err != nil {
		t.Fatalf("StartDiscard: %v", err)
	}

	done := marked.FinishDiscard()

	if done.IsDiscarding() {
		t.Errorf("unexpected discarding project %s", done)
	}
	events := done.PendingEvents()
	if len(events) != 2 || events[1].Type() != event.EventTypeProjectUpdated {
		t.Fatalf("expected project_discard_started and project_updated, got %v", events)
	}
}

func TestProjectFinishDiscard_OfAProjectNotBeingDiscarded_IsAnOp(t *testing.T) {
	p := makeStoredProject(t)

	if done := p.FinishDiscard(); done != p {
		t.Errorf("FinishDiscard of a project not being discarded: %s, want the same", done)
	}
	if len(p.PendingEvents()) != 0 {
		t.Errorf("expected no pending events, got %v", p.PendingEvents())
	}
}
