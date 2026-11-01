package id

import (
	"testing"

	"github.com/google/uuid"
)

type testEntity struct{}

func TestNewID_GeneratesUniqueIDs(t *testing.T) {
	id1 := NewID[testEntity]()
	id2 := NewID[testEntity]()
	if id1 == id2 {
		t.Error("expected two NewID() calls to produce different IDs")
	}
}

func TestNewID_WithExistingUUID(t *testing.T) {
	existing := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	id := NewID(IDWithUUID[testEntity](existing))
	if id.UUID() != existing {
		t.Errorf("expected %v, got %v", existing, id.UUID())
	}
}

func TestIDWithUUID_ValidUUIDString_RoundTrips(t *testing.T) {
	const raw = "550e8400-e29b-41d4-a716-446655440000"
	parsed, err := uuid.Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error parsing UUID: %v", err)
	}
	id := NewID(IDWithUUID[testEntity](parsed))
	if id.UUID().String() != raw {
		t.Errorf("expected %s, got %s", raw, id.UUID().String())
	}
}

func TestID_UUID_Roundtrip(t *testing.T) {
	original := uuid.New()
	id := NewID(IDWithUUID[testEntity](original))
	if id.UUID() != original {
		t.Errorf("UUID roundtrip failed: expected %v, got %v", original, id.UUID())
	}
}

func TestID_IsZero(t *testing.T) {
	var absent ID[testEntity]
	if !absent.IsZero() {
		t.Error("the zero ID must report itself zero: it means no aggregate")
	}
	if NewID[testEntity]().IsZero() {
		t.Error("a generated ID must not report itself zero")
	}
}
