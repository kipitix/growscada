package id

import (
	"errors"
	"fmt"
	"testing"
)

func TestNotFound_MatchesSentinelAndCarriesID(t *testing.T) {
	sentinel := errors.New("thing not found")
	anID := NewID[int]()

	err := fmt.Errorf("wrapped: %w", NotFound(sentinel, anID))

	if !errors.Is(err, sentinel) {
		t.Errorf("expected errors.Is to match the sentinel, got: %v", err)
	}
	var nf NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected a NotFoundError, got: %v", err)
	}
	if nf.ID != anID.UUID() {
		t.Errorf("ID: expected %s, got %s", anID, nf.ID)
	}
}
