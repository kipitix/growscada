package label

import (
	"fmt"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// Assignment - the mark of one Label on one saved state: an entity of the
// Label aggregate, not an aggregate of its own (task 85_3; ADR 0025). It names
// the Commit and the Draft's scope, so a label's assignment can be read
// without loading the Commit, and it carries no Version: assigning marks no
// content and is never a conflict with an Engineer editing a Draft.
// Value Object.
type Assignment interface {
	CommitID() id.ID[draft.Commit]
	Scope() draft.Scope
	CommitNumber() int
	AssignedAt() time.Time

	fmt.Stringer
}

type assignmentImpl struct {
	commitID     id.ID[draft.Commit]
	scope        draft.Scope
	commitNumber int
	assignedAt   time.Time
}

var _ Assignment = (*assignmentImpl)(nil)

// NewAssignment creates an assignment of a Label to the Commit commitID of
// scope, numbered commitNumber within its Draft. Returns ErrInvalidAssignment
// if the number is not positive or the scope names neither a Project nor a
// Library.
func NewAssignment(commitID id.ID[draft.Commit], scope draft.Scope, commitNumber int, at time.Time) (Assignment, error) {
	if commitNumber < 1 {
		return nil, fmt.Errorf("%w: commit number must be positive, got %d", ErrInvalidAssignment, commitNumber)
	}
	switch {
	case commitID.IsZero():
		return nil, fmt.Errorf("%w: a commit is required", ErrInvalidAssignment)
	case scope.IsProject() == scope.IsLibrary():
		return nil, fmt.Errorf("%w: one project or library is required, got %s", ErrInvalidAssignment, scope)
	}
	return &assignmentImpl{
		commitID:     commitID,
		scope:        scope,
		commitNumber: commitNumber,
		assignedAt:   at.UTC(),
	}, nil
}

func (a *assignmentImpl) CommitID() id.ID[draft.Commit] { return a.commitID }
func (a *assignmentImpl) Scope() draft.Scope            { return a.scope }
func (a *assignmentImpl) CommitNumber() int             { return a.commitNumber }
func (a *assignmentImpl) AssignedAt() time.Time         { return a.assignedAt }

// String implements [fmt.Stringer].
func (a *assignmentImpl) String() string {
	return fmt.Sprintf("Assignment: %d of %s at %s", a.commitNumber, a.scope, a.assignedAt.Format(time.RFC3339))
}
