package tag

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewTagID_GeneratesUniqueIDs(t *testing.T) {
	id1 := NewTagID()
	id2 := NewTagID()
	if id1 == id2 {
		t.Error("expected two NewTagID() calls to produce different IDs")
	}
}

func TestNewTagID_WithExistingUUID(t *testing.T) {
	existing := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	id := NewTagID(TagIDWithUUID(existing))
	if id.UUID() != existing {
		t.Errorf("expected %v, got %v", existing, id.UUID())
	}
}

func TestParseTagID_ValidUUID(t *testing.T) {
	const raw = "550e8400-e29b-41d4-a716-446655440000"
	id, err := ParseTagID(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.UUID().String() != raw {
		t.Errorf("expected %s, got %s", raw, id.UUID().String())
	}
}

func TestParseTagID_InvalidUUID(t *testing.T) {
	_, err := ParseTagID("not-a-uuid")
	if err == nil {
		t.Error("expected error for invalid UUID string, got nil")
	}
}

func TestMustParseTagID_ValidUUID(t *testing.T) {
	const raw = "550e8400-e29b-41d4-a716-446655440000"
	id := MustParseTagID(raw)
	if id.UUID().String() != raw {
		t.Errorf("expected %s, got %s", raw, id.UUID().String())
	}
}

func TestMustParseTagID_InvalidUUID_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid UUID, got none")
		}
	}()
	MustParseTagID("not-a-uuid")
}

func TestTagID_UUID_Roundtrip(t *testing.T) {
	original := uuid.New()
	id := NewTagID(TagIDWithUUID(original))
	if id.UUID() != original {
		t.Errorf("UUID roundtrip failed: expected %v, got %v", original, id.UUID())
	}
}
