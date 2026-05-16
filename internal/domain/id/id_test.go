package id

import (
	"testing"

	"github.com/google/uuid"
)

type testEntity struct{}

func TestNewID_GeneratesUniqueIDs(t *testing.T) {
	id1, err := NewID[testEntity]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	id2, err := NewID[testEntity]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id1 == id2 {
		t.Error("expected two NewID() calls to produce different IDs")
	}
}

func TestNewID_WithExistingUUID(t *testing.T) {
	existing := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	id, err := NewID(IDWithUUID[testEntity](existing))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.UUID() != existing {
		t.Errorf("expected %v, got %v", existing, id.UUID())
	}
}

func TestIDWithString_ValidUUID(t *testing.T) {
	const raw = "550e8400-e29b-41d4-a716-446655440000"
	opt, err := IDWithString[testEntity](raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	id, err := NewID(opt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.UUID().String() != raw {
		t.Errorf("expected %s, got %s", raw, id.UUID().String())
	}
}

func TestIDWithString_InvalidUUID(t *testing.T) {
	_, err := IDWithString[testEntity]("not-a-uuid")
	if err == nil {
		t.Error("expected error for invalid UUID string, got nil")
	}
}

func TestID_UUID_Roundtrip(t *testing.T) {
	original := uuid.New()
	id, err := NewID(IDWithUUID[testEntity](original))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.UUID() != original {
		t.Errorf("UUID roundtrip failed: expected %v, got %v", original, id.UUID())
	}
}
