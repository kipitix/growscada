package device_test

import (
	"errors"
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

func TestCreateEnvironment_FieldsAndEvent(t *testing.T) {
	anID := id.NewID[device.Environment]()
	env, err := device.CreateEnvironment(anID, testProject, environmentName(t, "test bench"), "the lab")
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}

	if env.ID() != anID || env.ProjectID() != testProject || env.Name().String() != "test bench" || env.Comment() != "the lab" {
		t.Fatalf("unexpected environment %s", env)
	}
	if env.Version() != version.Initial[device.Environment]() {
		t.Errorf("Version = %s, want the initial one", env.Version())
	}

	events := env.PendingEvents()
	if len(events) != 1 {
		t.Fatalf("PendingEvents = %v, want [environment_created]", events)
	}
	created, ok := events[0].(device.EnvironmentCreatedEvent)
	if !ok || created.Type() != event.EventTypeEnvironmentCreated || created.EnvironmentID() != anID || created.ProjectID() != testProject {
		t.Errorf("unexpected event %v", events[0])
	}
	if _, ok := any(events[0]).(project.ContentEvent); !ok {
		t.Error("an Environment event is content of its Project")
	}
}

// TestEnvironment_RejectsInvalidName: the zero name is invalid, and no
// constructor or change method takes it.
func TestEnvironment_RejectsInvalidName(t *testing.T) {
	anID := id.NewID[device.Environment]()

	if _, err := device.CreateEnvironment(anID, testProject, device.EnvironmentName{}, ""); err == nil {
		t.Error("CreateEnvironment took an invalid name")
	}
	if _, err := device.ReconstituteEnvironment(anID, testProject, device.EnvironmentName{}, "",
		version.Committed[device.Environment]()); err == nil {
		t.Error("ReconstituteEnvironment took an invalid name")
	}
	stored, err := device.ReconstituteEnvironment(anID, testProject, environmentName(t, "test bench"), "",
		version.Committed[device.Environment]())
	if err != nil {
		t.Fatalf("ReconstituteEnvironment: %v", err)
	}
	if _, err := stored.Update(stored.Version(), device.EnvironmentName{}, ""); err == nil {
		t.Error("Update took an invalid name")
	}
	if got := stored.PendingEvents(); len(got) != 0 {
		t.Errorf("a rejected Update recorded %v", got)
	}
}

func TestEnvironment_Update(t *testing.T) {
	stored, err := device.ReconstituteEnvironment(id.NewID[device.Environment](), testProject,
		environmentName(t, "test bench"), "the lab", version.Committed[device.Environment]())
	if err != nil {
		t.Fatalf("ReconstituteEnvironment: %v", err)
	}

	if _, err := stored.Update(version.Initial[device.Environment](), environmentName(t, "plant"), "the bench"); !errors.Is(err, device.ErrEnvironmentConflict) {
		t.Errorf("stale update: %v, want ErrEnvironmentConflict", err)
	}

	updated, err := stored.Update(stored.Version(), environmentName(t, "plant"), "the bench")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name().String() != "plant" || updated.Comment() != "the bench" {
		t.Errorf("unexpected environment %s", updated)
	}
	if stored.Name().String() != "test bench" || stored.Comment() != "the lab" {
		t.Errorf("Update changed the environment it was called on: %s", stored)
	}
	if got := updated.PendingEvents(); len(got) != 1 || got[0].Type() != event.EventTypeEnvironmentUpdated {
		t.Errorf("PendingEvents = %v, want [environment_updated]", got)
	}
	if got := stored.PendingEvents(); len(got) != 0 {
		t.Errorf("the stored environment recorded %v", got)
	}
}

func TestEnvironment_Delete(t *testing.T) {
	stored, err := device.ReconstituteEnvironment(id.NewID[device.Environment](), testProject,
		environmentName(t, "test bench"), "", version.Committed[device.Environment]())
	if err != nil {
		t.Fatalf("ReconstituteEnvironment: %v", err)
	}

	deleted := stored.Delete()
	if got := deleted.PendingEvents(); len(got) != 1 || got[0].Type() != event.EventTypeEnvironmentDeleted {
		t.Errorf("PendingEvents = %v, want [environment_deleted]", got)
	}
	if got := stored.PendingEvents(); len(got) != 0 {
		t.Errorf("Delete changed the environment it was called on: %v", got)
	}
}
