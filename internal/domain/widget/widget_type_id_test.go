package widget

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewWidgetTypeID_GeneratesUniqueIDs(t *testing.T) {
	id1 := NewWidgetTypeID()
	id2 := NewWidgetTypeID()
	if id1 == id2 {
		t.Error("expected two NewWidgetTypeID() calls to produce different IDs")
	}
}

func TestNewWidgetTypeID_WithExistingUUID(t *testing.T) {
	existing := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	id := NewWidgetTypeID(WidgetTypeIDWithUUID(existing))
	if id.UUID() != existing {
		t.Errorf("expected %v, got %v", existing, id.UUID())
	}
}

func TestParseWidgetTypeID_ValidUUID(t *testing.T) {
	const raw = "550e8400-e29b-41d4-a716-446655440000"
	id, err := ParseWidgetTypeID(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.UUID().String() != raw {
		t.Errorf("expected %s, got %s", raw, id.UUID().String())
	}
}

func TestParseWidgetTypeID_InvalidUUID(t *testing.T) {
	_, err := ParseWidgetTypeID("not-a-uuid")
	if err == nil {
		t.Error("expected error for invalid UUID string, got nil")
	}
}

func TestMustParseWidgetTypeID_ValidUUID(t *testing.T) {
	const raw = "550e8400-e29b-41d4-a716-446655440000"
	id := MustParseWidgetTypeID(raw)
	if id.UUID().String() != raw {
		t.Errorf("expected %s, got %s", raw, id.UUID().String())
	}
}

func TestMustParseWidgetTypeID_InvalidUUID_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid UUID, got none")
		}
	}()
	MustParseWidgetTypeID("not-a-uuid")
}

func TestWidgetTypeID_UUID_Roundtrip(t *testing.T) {
	original := uuid.New()
	id := NewWidgetTypeID(WidgetTypeIDWithUUID(original))
	if id.UUID() != original {
		t.Errorf("UUID roundtrip failed: expected %v, got %v", original, id.UUID())
	}
}
