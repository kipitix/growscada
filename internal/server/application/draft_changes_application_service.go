package application

import (
	"context"
	"fmt"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// DraftChangesService is the Engineering node's read and undo/redo of a
// Draft's DraftChange stack (ADR 0017). It works on the Draft of a Project or
// of a Library, named by its Scope: both are edited the same way, and both
// are opened through their content, so a Project or Library that is not live
// (or is of another Organization) is not found. Undo and redo are gated by the
// EditLock and state the Version of the Draft the client saw.
type DraftChangesService interface {
	// State returns the Draft's Version, whether an undo and a redo are
	// possible and the description of the change each would apply.
	State(ctx context.Context, scope editlock.Scope) (appdto.DraftChanges, error)
	// Undo applies the before states of the DraftChange at the stack's
	// position and returns the state after it. A stack with no change there is
	// draft.ErrNothingToUndo; a stale version is draft.ErrDraftConflict.
	Undo(ctx context.Context, scope editlock.Scope, version int) (appdto.DraftChanges, error)
	// Redo applies the after states of the DraftChange after the stack's
	// position; nothing to redo is draft.ErrNothingToRedo.
	Redo(ctx context.Context, scope editlock.Scope, version int) (appdto.DraftChanges, error)
}

type draftChangesServiceImpl struct {
	projects  ProjectContent
	libraries LibraryContent
}

var _ DraftChangesService = (*draftChangesServiceImpl)(nil)

// NewDraftChangesService returns the service over the Drafts of the live
// Projects and Libraries the contents serve. Its writes go through the Draft's
// repository, which records their snapshots and events (ADR 0016, ADR 0017).
func NewDraftChangesService(projectContent ProjectContent, libraryContent LibraryContent) DraftChangesService {
	return &draftChangesServiceImpl{projects: projectContent, libraries: libraryContent}
}

func (s draftChangesServiceImpl) State(ctx context.Context, scope editlock.Scope) (appdto.DraftChanges, error) {
	if err := s.open(ctx, scope); err != nil {
		return appdto.DraftChanges{}, err
	}
	return s.state(ctx, scope)
}

func (s draftChangesServiceImpl) Undo(ctx context.Context, scope editlock.Scope, rawVersion int) (appdto.DraftChanges, error) {
	if err := s.open(ctx, scope); err != nil {
		return appdto.DraftChanges{}, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.DraftChanges{}, err
	}
	if _, err := s.drafts(scope).Undo(ctx, scope, expected); err != nil {
		return appdto.DraftChanges{}, fmt.Errorf("cannot undo the draft of %s: %w", scope, err)
	}
	return s.state(ctx, scope)
}

func (s draftChangesServiceImpl) Redo(ctx context.Context, scope editlock.Scope, rawVersion int) (appdto.DraftChanges, error) {
	if err := s.open(ctx, scope); err != nil {
		return appdto.DraftChanges{}, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.DraftChanges{}, err
	}
	if _, err := s.drafts(scope).Redo(ctx, scope, expected); err != nil {
		return appdto.DraftChanges{}, fmt.Errorf("cannot redo the draft of %s: %w", scope, err)
	}
	return s.state(ctx, scope)
}

// open checks that the Draft's Project or Library is live and of the server's
// Organization, as every access to its content does.
func (s draftChangesServiceImpl) open(ctx context.Context, scope editlock.Scope) error {
	switch {
	case scope.IsProject():
		_, _, err := s.projects.open(ctx, scope.ProjectID().UUID())
		return err
	case scope.IsLibrary():
		_, err := s.libraries.open(ctx, scope.LibraryID().UUID())
		return err
	default:
		return fmt.Errorf("%w: the scope names no draft", draft.ErrDraftNotFound)
	}
}

// drafts is the repository of the Drafts of the scope's own kind: a Library's
// Draft is read and written through the Library content it was opened with, a
// Project's through its own (ADR 0016).
func (s draftChangesServiceImpl) drafts(scope editlock.Scope) draft.Repository {
	if scope.IsLibrary() {
		return s.libraries.Drafts()
	}
	return s.projects.Drafts()
}

// state reads the stack of a Draft already checked to be live. A Draft never
// written has no row and no changes: it is the fresh Draft at the Version its
// first write states.
func (s draftChangesServiceImpl) state(ctx context.Context, scope editlock.Scope) (appdto.DraftChanges, error) {
	stack, ok, err := s.drafts(scope).Stack(ctx, scope)
	if err != nil {
		return appdto.DraftChanges{}, fmt.Errorf("cannot read the draft changes of %s: %w", scope, err)
	}
	if !ok {
		return appdto.DraftChanges{Version: version.Committed[draft.Draft]().Number()}, nil
	}
	state := appdto.DraftChanges{Version: stack.Draft.Version().Number()}
	if stack.Undo != nil {
		state.CanUndo = true
		state.UndoDescription = stack.Undo.Description
	}
	if stack.Redo != nil {
		state.CanRedo = true
		state.RedoDescription = stack.Redo.Description
	}
	return state, nil
}
