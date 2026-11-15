package label

import (
	"fmt"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// Assignment - the mark of one Label on one saved state: an entity of the Label
// aggregate, not an aggregate of its own (task 85_3; ADR 0025). It names the
// saved state and the Draft's scope, so a label's assignment can be read
// without loading the saved state, and it carries no Version: assigning marks
// no content and is never a conflict with an Engineer editing a Draft. Value
// Object.
type Assignment interface {
	SavedStateID() id.ID[draft.SavedState]
	Scope() draft.Scope
	SavedStateNumber() int
	AssignedAt() time.Time

	fmt.Stringer
}

type assignmentImpl struct {
	savedStateID     id.ID[draft.SavedState]
	scope            draft.Scope
	savedStateNumber int
	assignedAt       time.Time
}

var _ Assignment = (*assignmentImpl)(nil)

// NewAssignment creates an assignment of a Label to the saved state
// savedStateID of scope, numbered savedStateNumber within its Draft. Returns
// ErrInvalidAssignment if the number is not positive or the scope names neither
// a Project nor a Library.
func NewAssignment(savedStateID id.ID[draft.SavedState], scope draft.Scope, savedStateNumber int, at time.Time) (Assignment, error) {
	if savedStateNumber < 1 {
		return nil, fmt.Errorf("%w: saved state number must be positive, got %d", ErrInvalidAssignment, savedStateNumber)
	}
	switch {
	case savedStateID.IsZero():
		return nil, fmt.Errorf("%w: a saved state is required", ErrInvalidAssignment)
	case scope.IsProject() == scope.IsLibrary():
		return nil, fmt.Errorf("%w: one project or library is required, got %s", ErrInvalidAssignment, scope)
	}
	return &assignmentImpl{
		savedStateID:     savedStateID,
		scope:            scope,
		savedStateNumber: savedStateNumber,
		assignedAt:       at.UTC(),
	}, nil
}

func (a *assignmentImpl) SavedStateID() id.ID[draft.SavedState] { return a.savedStateID }
func (a *assignmentImpl) Scope() draft.Scope                    { return a.scope }
func (a *assignmentImpl) SavedStateNumber() int                 { return a.savedStateNumber }
func (a *assignmentImpl) AssignedAt() time.Time                 { return a.assignedAt }

// String implements [fmt.Stringer].
func (a *assignmentImpl) String() string {
	return fmt.Sprintf("Assignment: %d of %s at %s", a.savedStateNumber, a.scope, a.assignedAt.Format(time.RFC3339))
}
