package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

func newDraftChangesService() application.DraftChangesService {
	return application.NewDraftChangesService(contentOver(newScopes()), newLibraryContent())
}

// holdDraft makes the Draft of scope held by a client of key, whose returned
// context is the one of its requests.
func holdDraft(t *testing.T, scope editlock.Scope) context.Context {
	t.Helper()
	holder, err := editlock.NewHolder(editlock.NewKey(), "alice")
	if err != nil {
		t.Fatalf("holder: %v", err)
	}
	lock, err := editlock.CreateEditLock(scope).Acquire(holder, time.Now())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	repo := repositories.NewDraftRepositoryPostgres(testDB, repositories.NewLibraryReleaseRepositoryPostgres(testDB))
	if _, err := repo.Create(editlock.System(context.Background()), lock); err != nil {
		t.Fatalf("hold the draft: %v", err)
	}
	return editlock.WithClient(context.Background(), holder.Key())
}

// --- State ---

// TestDraftChangesState_FreshDraft: a Draft never written has no row and no
// stack: its Version is the one its first write states, with nothing to undo
// or redo.
func TestDraftChangesState_FreshDraft(t *testing.T) {
	cleanEditLocksApp(t)
	cleanTags(t)
	state, err := newDraftChangesService().State(editlock.System(context.Background()), projectScope(mainProject))
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if state.Version != 1 || state.CanUndo || state.CanRedo {
		t.Errorf("the fresh draft: %+v, want version 1 with nothing to undo or redo", state)
	}
	if state.UndoDescription != "" || state.RedoDescription != "" {
		t.Errorf("the fresh draft has descriptions: %+v", state)
	}
}

// TestDraftChangesState_NamesTheNextSteps: after one change the undo names it;
// after the undo the redo names it and the undo has nothing left.
func TestDraftChangesState_NamesTheNextSteps(t *testing.T) {
	cleanEditLocksApp(t)
	cleanTags(t)
	ctx := editlock.System(context.Background())
	svc := newDraftChangesService()
	if _, err := newService().CreateTag(ctx, mainProject,
		appdto.CreateTagInput{Name: "pressure", Type: "integer", Version: draftVersionOf(t, mainProject)}); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	state, err := svc.State(ctx, projectScope(mainProject))
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if !state.CanUndo || state.CanRedo || state.UndoDescription != "Create tag pressure" {
		t.Errorf("after the change: %+v, want an undo of 'Create tag pressure'", state)
	}

	after, err := svc.Undo(ctx, projectScope(mainProject), state.Version)
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if after.CanUndo || !after.CanRedo || after.RedoDescription != "Create tag pressure" {
		t.Errorf("after the undo: %+v, want a redo of 'Create tag pressure'", after)
	}
	if after.Version != state.Version+1 {
		t.Errorf("the undo answered version %d, want %d", after.Version, state.Version+1)
	}

	redone, err := svc.Redo(ctx, projectScope(mainProject), after.Version)
	if err != nil {
		t.Fatalf("Redo: %v", err)
	}
	if redone.CanRedo || !redone.CanUndo {
		t.Errorf("after the redo: %+v, want nothing to redo", redone)
	}
}

// --- Undo and redo ---

// TestDraftChangesUndoRedo_Project: an undo removes the Tag the change
// created, a redo brings it back, and the Draft's Version follows the stack.
func TestDraftChangesUndoRedo_Project(t *testing.T) {
	cleanEditLocksApp(t)
	cleanTags(t)
	ctx := editlock.System(context.Background())
	svc := newDraftChangesService()
	created, err := newService().CreateTag(ctx, mainProject,
		appdto.CreateTagInput{Name: "pressure", Type: "integer", Version: draftVersionOf(t, mainProject)})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	version := created.Version

	if _, err := svc.Undo(ctx, projectScope(mainProject), version); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	tags, err := newService().FindAllTags(ctx, mainProject)
	if err != nil {
		t.Fatalf("FindAllTags: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("the undone Tag is still there: %+v", tags)
	}

	if _, err := svc.Redo(ctx, projectScope(mainProject), version+1); err != nil {
		t.Fatalf("Redo: %v", err)
	}
	tags, err = newService().FindAllTags(ctx, mainProject)
	if err != nil {
		t.Fatalf("FindAllTags: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "pressure" {
		t.Errorf("the redone Tag: %+v, want pressure", tags)
	}
}

// TestDraftChangesUndoRedo_Library: a Library's Draft undoes and redoes the
// creation of a WidgetType the same way.
func TestDraftChangesUndoRedo_Library(t *testing.T) {
	cleanEditLocksApp(t)
	cleanWidgetTypes(t)
	ctx := editlock.System(context.Background())
	svc := newDraftChangesService()
	created, err := newWidgetTypeService().CreateWidgetType(ctx, wtLibrary,
		libraryDraftVersionOf(t, wtLibrary), testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	scope := libraryScope(wtLibrary)

	state, err := svc.State(ctx, scope)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if !state.CanUndo || state.UndoDescription != "Create widget type gauge" {
		t.Fatalf("the library's stack: %+v", state)
	}
	if _, err := svc.Undo(ctx, scope, created.Version); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	list, err := newWidgetTypeService().FindAllWidgetTypes(ctx, wtLibrary)
	if err != nil {
		t.Fatalf("FindAllWidgetTypes: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("the undone WidgetType is still there: %+v", list)
	}
	if _, err := svc.Redo(ctx, scope, created.Version+1); err != nil {
		t.Fatalf("Redo: %v", err)
	}
	list, err = newWidgetTypeService().FindAllWidgetTypes(ctx, wtLibrary)
	if err != nil {
		t.Fatalf("FindAllWidgetTypes: %v", err)
	}
	if len(list) != 1 || list[0].Name != "gauge" {
		t.Errorf("the redone WidgetType: %+v, want gauge", list)
	}
}

// TestDraftChanges_NotLiveScope: a Project or Library that does not exist is
// not found, and so is one marked deleted.
func TestDraftChanges_NotLiveScope(t *testing.T) {
	cleanEditLocksApp(t)
	ctx := editlock.System(context.Background())
	svc := newDraftChangesService()
	if _, err := svc.State(ctx, projectScope(uuid.New())); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("State of a missing project: %v, want ErrProjectNotFound", err)
	}
	if _, err := svc.State(ctx, libraryScope(uuid.New())); !errors.Is(err, library.ErrLibraryNotFound) {
		t.Errorf("State of a missing library: %v, want ErrLibraryNotFound", err)
	}
	if _, err := svc.Undo(ctx, projectScope(uuid.New()), 1); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("Undo of a missing project: %v, want ErrProjectNotFound", err)
	}
}

// TestDraftChangesUndo_GateAndStaleVersion: undo and redo are writes of the
// Draft: without the EditLock they are refused (no client is 400, another
// client's lock is 409), a stale Version is a conflict, and an empty stack is
// nothing to undo.
func TestDraftChangesUndo_GateAndStaleVersion(t *testing.T) {
	cleanEditLocksApp(t)
	cleanTags(t)
	system := editlock.System(context.Background())
	svc := newDraftChangesService()
	stale := draftVersionOf(t, mainProject)
	created, err := newService().CreateTag(system, mainProject,
		appdto.CreateTagInput{Name: "pressure", Type: "integer", Version: draftVersionOf(t, mainProject)})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	if _, err := svc.Undo(context.Background(), projectScope(mainProject), created.Version); !errors.Is(err, editlock.ErrNoClient) {
		t.Errorf("Undo with no client: %v, want ErrNoClient", err)
	}
	holder := holdDraft(t, projectScope(mainProject))
	other := editlock.WithClient(context.Background(), editlock.NewKey())
	if _, err := svc.Undo(other, projectScope(mainProject), created.Version); !errors.Is(err, editlock.ErrEditLockNotHeld) {
		t.Errorf("Undo of another client: %v, want ErrEditLockNotHeld", err)
	}
	if _, err := svc.Undo(holder, projectScope(mainProject), stale); !errors.Is(err, draft.ErrDraftConflict) {
		t.Errorf("Undo with a stale version: %v, want ErrDraftConflict", err)
	}
	after, err := svc.Undo(holder, projectScope(mainProject), created.Version)
	if err != nil {
		t.Fatalf("Undo of the holder: %v", err)
	}
	if _, err := svc.Undo(holder, projectScope(mainProject), after.Version); !errors.Is(err, draft.ErrNothingToUndo) {
		t.Errorf("Undo of an empty stack: %v, want ErrNothingToUndo", err)
	}
	if _, err := svc.Redo(holder, projectScope(mainProject), after.Version); err != nil {
		t.Fatalf("Redo of the holder: %v", err)
	}
}

// TestDraftChangesUndo_InvalidVersion: a negative Version is the client's
// input, not a conflict.
func TestDraftChangesUndo_InvalidVersion(t *testing.T) {
	cleanEditLocksApp(t)
	_, err := newDraftChangesService().Undo(editlock.System(context.Background()), projectScope(mainProject), -1)
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("Undo with a negative version: %v, want ErrInvalidInput", err)
	}
}
