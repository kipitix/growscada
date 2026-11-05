package tag

import (
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// testProject is the Project of the tags the tests build.
var testProject = id.NewID[project.Project]()

func makeTestTag(t *testing.T) Tag {
	t.Helper()
	name, _ := NewTagName("temperature")
	tag, err := ReconstituteTag(id.NewID[Tag](), testProject, name, TagTypeInteger, NoSupplier())
	if err != nil {
		t.Fatalf("ReconstituteTag returned unexpected error: %v", err)
	}
	return tag
}

func TestReconstituteTag_FieldsAreSet(t *testing.T) {
	tagID := id.NewID[Tag]()
	name, _ := NewTagName("pressure")
	tag, err := ReconstituteTag(tagID, testProject, name, TagTypeString, NoSupplier())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tag.ID() != tagID {
		t.Errorf("ID mismatch: expected %v, got %v", tagID, tag.ID())
	}
	if tag.ProjectID() != testProject {
		t.Errorf("ProjectID mismatch: expected %v, got %v", testProject, tag.ProjectID())
	}
	if tag.Name() != name {
		t.Errorf("Name mismatch: expected %v, got %v", name, tag.Name())
	}
	if tag.Type() != TagTypeString {
		t.Errorf("Type mismatch: expected %v, got %v", TagTypeString, tag.Type())
	}
	if tag.Supplier().HasDevice() {
		t.Errorf("Supplier mismatch: expected no supplier, got %s", tag.Supplier())
	}
}

func TestTag_KeepsItsSupplier(t *testing.T) {
	deviceID := id.NewID[device.Device]()
	supplier := SuppliedByDevice(deviceID)
	name, _ := NewTagName("pressure")

	created, err := CreateTag(id.NewID[Tag](), testProject, name, TagTypeInteger, supplier)
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if created.Supplier() != supplier || created.Supplier().DeviceID() != deviceID {
		t.Errorf("Supplier = %s, want the supplier %s", created.Supplier(), supplier)
	}

	stored, err := ReconstituteTag(id.NewID[Tag](), testProject, name, TagTypeInteger, supplier)
	if err != nil {
		t.Fatalf("ReconstituteTag: %v", err)
	}
	if stored.Supplier() != supplier {
		t.Errorf("Supplier = %s, want the supplier %s", stored.Supplier(), supplier)
	}
}

func TestReconstituteTag_InvalidType_ReturnsError(t *testing.T) {
	name, _ := NewTagName("temperature")
	if _, err := ReconstituteTag(id.NewID[Tag](), testProject, name, TagType{}, NoSupplier()); err == nil {
		t.Error("expected error, got nil")
	}
}

func TestCreateTag_InvalidType_ReturnsError(t *testing.T) {
	name, _ := NewTagName("temperature")
	if _, err := CreateTag(id.NewID[Tag](), testProject, name, TagType{}, NoSupplier()); err == nil {
		t.Error("expected error, got nil")
	}
}
