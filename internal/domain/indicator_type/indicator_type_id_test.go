package indicator_type

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewIndicatorTypeID_GeneratesUniqueIDs(t *testing.T) {
	id1 := NewIndicatorTypeID()
	id2 := NewIndicatorTypeID()
	if id1 == id2 {
		t.Error("expected two NewIndicatorTypeID() calls to produce different IDs")
	}
}

func TestNewIndicatorTypeID_WithExistingUUID(t *testing.T) {
	existing := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	id := NewIndicatorTypeID(IndicatorTypeIDWithUUID(existing))
	if id.UUID() != existing {
		t.Errorf("expected %v, got %v", existing, id.UUID())
	}
}

func TestParseIndicatorTypeID_ValidUUID(t *testing.T) {
	const raw = "550e8400-e29b-41d4-a716-446655440000"
	id, err := ParseIndicatorTypeID(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.UUID().String() != raw {
		t.Errorf("expected %s, got %s", raw, id.UUID().String())
	}
}

func TestParseIndicatorTypeID_InvalidUUID(t *testing.T) {
	_, err := ParseIndicatorTypeID("not-a-uuid")
	if err == nil {
		t.Error("expected error for invalid UUID string, got nil")
	}
}

func TestMustParseIndicatorTypeID_ValidUUID(t *testing.T) {
	const raw = "550e8400-e29b-41d4-a716-446655440000"
	id := MustParseIndicatorTypeID(raw)
	if id.UUID().String() != raw {
		t.Errorf("expected %s, got %s", raw, id.UUID().String())
	}
}

func TestMustParseIndicatorTypeID_InvalidUUID_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid UUID, got none")
		}
	}()
	MustParseIndicatorTypeID("not-a-uuid")
}

func TestIndicatorTypeID_UUID_Roundtrip(t *testing.T) {
	original := uuid.New()
	id := NewIndicatorTypeID(IndicatorTypeIDWithUUID(original))
	if id.UUID() != original {
		t.Errorf("UUID roundtrip failed: expected %v, got %v", original, id.UUID())
	}
}
