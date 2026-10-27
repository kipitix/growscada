package repositories

import (
	"fmt"
	"testing"

	"github.com/lib/pq"
)

// The integration tests run PostgreSQL 16; the dev database runs 18, which
// reports a delete blocked by ON DELETE RESTRICT with another SQLSTATE.
func TestIsForeignKeyViolation_AcceptsBothPostgresCodes(t *testing.T) {
	for _, code := range []pq.ErrorCode{foreignKeyViolationCode, restrictViolationCode} {
		err := fmt.Errorf("wrapped: %w", &pq.Error{Code: code, Constraint: constraintLibraryPinsLibraryID})
		if !isForeignKeyViolation(err, constraintLibraryPinsLibraryID) {
			t.Errorf("code %s: expected a foreign key violation", code)
		}
	}
	if isForeignKeyViolation(&pq.Error{Code: foreignKeyViolationCode, Constraint: "other"}, constraintLibraryPinsLibraryID) {
		t.Error("another constraint: expected false")
	}
	if isForeignKeyViolation(&pq.Error{Code: uniqueViolationCode, Constraint: constraintLibraryPinsLibraryID}, constraintLibraryPinsLibraryID) {
		t.Error("unique violation: expected false")
	}
}
