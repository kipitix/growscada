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
	env := device.CreateEnvironment(anID, testProject, environmentName(t, "test bench"), "the lab")

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

func TestEnvironment_Rename(t *testing.T) {
	stored := device.ReconstituteEnvironment(id.NewID[device.Environment](), testProject,
		environmentName(t, "test bench"), "the lab", version.Committed[device.Environment]())

	if _, err := stored.Rename(version.Initial[device.Environment](), environmentName(t, "plant")); !errors.Is(err, device.ErrEnvironmentConflict) {
		t.Errorf("stale rename: %v, want ErrEnvironmentConflict", err)
	}

	renamed, err := stored.Rename(stored.Version(), environmentName(t, "plant"))
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if renamed.Name().String() != "plant" || renamed.Comment() != "the lab" {
		t.Errorf("unexpected environment %s", renamed)
	}
	if stored.Name().String() != "test bench" {
		t.Errorf("Rename changed the environment it was called on: %s", stored)
	}
	if got := renamed.PendingEvents(); len(got) != 1 || got[0].Type() != event.EventTypeEnvironmentUpdated {
		t.Errorf("PendingEvents = %v, want [environment_updated]", got)
	}
	if got := stored.PendingEvents(); len(got) != 0 {
		t.Errorf("the stored environment recorded %v", got)
	}
}

func TestEnvironment_Delete(t *testing.T) {
	stored := device.ReconstituteEnvironment(id.NewID[device.Environment](), testProject,
		environmentName(t, "test bench"), "", version.Committed[device.Environment]())

	deleted := stored.Delete()
	if got := deleted.PendingEvents(); len(got) != 1 || got[0].Type() != event.EventTypeEnvironmentDeleted {
		t.Errorf("PendingEvents = %v, want [environment_deleted]", got)
	}
	if got := stored.PendingEvents(); len(got) != 0 {
		t.Errorf("Delete changed the environment it was called on: %v", got)
	}
}
