package repositories_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/label"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// TestLabelAssignments_WriteReadFilterAndCascade covers the assignments of
// labels to saved states (task 85_3; ADR 0025): the one row a mark is, the
// event it records once, the read of a history's chips, the SQL filter of a
// SavedState list and the cascades of a deleted label and a deleted Draft.
func TestLabelAssignments_WriteReadFilterAndCascade(t *testing.T) {
	cleanTestLabels(t)
	cleanDrafts(t)
	ctx := context.Background()
	repo := repositories.NewLabelRepositoryPostgres(testDB)
	org := soleOrganization(t)
	scope := draft.ProjectScope(mainProject)

	created := label.CreateLabel(repo.NextID(), org, mustLabelName(t, "test-"+uuid.NewString()[:8]),
		mustLabelColor(t, "#22c55e"), time.Now())
	saved, err := repo.Create(ctx, created)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	other, err := repo.Create(ctx, label.CreateLabel(repo.NextID(), org,
		mustLabelName(t, "test-"+uuid.NewString()[:8]), label.Color{}, time.Now()))
	if err != nil {
		t.Fatalf("Create another: %v", err)
	}

	savedState, err := testDrafts().SaveSavedState(ctx, scope, draftVersionOf(t, mainProject),
		commitTitle(t, "First"), commitAuthor(t), projectSavedStateContent())
	if err != nil {
		t.Fatalf("SaveSavedState: %v", err)
	}

	// Mark the saved state; the label read back carries the mark.
	found, err := repo.FindByID(ctx, org, saved.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	assigned, err := found.Assign(savedState.ID(), scope, savedState.Number(), time.Now())
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if _, err := repo.Assign(ctx, assigned, savedState.ID()); err != nil {
		t.Fatalf("store the assignment: %v", err)
	}
	stored, err := repo.FindByID(ctx, org, saved.ID())
	if err != nil {
		t.Fatalf("FindByID after the assignment: %v", err)
	}
	if len(stored.Assignments()) != 1 || stored.Assignments()[0].SavedStateNumber() != 1 ||
		stored.Assignments()[0].Scope() != scope || stored.Assignments()[0].SavedStateID() != savedState.ID() {
		t.Errorf("the stored assignment: %v", stored.Assignments())
	}
	if all, err := repo.FindAll(ctx, org); err != nil {
		t.Errorf("FindAll: %v", err)
	} else {
		for _, l := range all {
			if l.ID() == saved.ID() && len(l.Assignments()) != 1 {
				t.Errorf("FindAll of the marked label: %v", l.Assignments())
			}
		}
	}

	// A repeat stores no second row and records no second event.
	repeated, err := stored.Assign(savedState.ID(), scope, savedState.Number(), time.Now())
	if err != nil {
		t.Fatalf("a repeated Assign: %v", err)
	}
	if _, err := repo.Assign(ctx, repeated, savedState.ID()); err != nil {
		t.Fatalf("store a repeated assignment: %v", err)
	}
	var rows, events int
	if err := testDB.QueryRowContext(ctx,
		`SELECT count(*) FROM engineering.saved_state_labels WHERE label_id = $1`, saved.ID().UUID()).Scan(&rows); err != nil {
		t.Fatalf("count the assignments: %v", err)
	}
	if rows != 1 {
		t.Errorf("the stored assignments: %d, want 1 (the pair is the key)", rows)
	}
	if err := testDB.QueryRowContext(ctx,
		`SELECT count(*) FROM outbox WHERE type = 'label_assigned' AND payload->>'label_id' = $1`,
		saved.ID().UUID().String()).Scan(&events); err != nil {
		t.Fatalf("count the assignment events: %v", err)
	}
	if events != 1 {
		t.Errorf("label_assigned events: %d, want 1", events)
	}

	// The history's chips and the filter of the SavedState list read the mark.
	byNumber, err := repo.LabelsOfScope(ctx, org, scope)
	if err != nil {
		t.Fatalf("LabelsOfScope: %v", err)
	}
	if got := byNumber[savedState.Number()]; len(got) != 1 || got[0].ID() != saved.ID() {
		t.Errorf("the labels of the saved state: %v", got)
	}
	list, err := testDrafts().FindSavedStates(ctx, scope, draft.SavedStateFilter{LabelID: saved.ID().UUID()})
	if err != nil {
		t.Fatalf("FindSavedStates by label: %v", err)
	}
	if len(list) != 1 || list[0].Number() != 1 {
		t.Errorf("the saved states of the label: %v", list)
	}
	if list, err := testDrafts().FindSavedStates(ctx, scope, draft.SavedStateFilter{LabelID: other.ID().UUID()}); err != nil || len(list) != 0 {
		t.Errorf("the saved states of a label nothing carries: %v, %v", list, err)
	}

	// A repeat of the DELETE is not an error and records nothing.
	unassigned, err := stored.Unassign(savedState.ID())
	if err != nil {
		t.Fatalf("Unassign: %v", err)
	}
	if _, err := repo.Unassign(ctx, unassigned, savedState.ID()); err != nil {
		t.Fatalf("delete the assignment: %v", err)
	}
	if _, err := repo.Unassign(ctx, unassigned, savedState.ID()); err != nil {
		t.Errorf("a repeated Unassign: %v", err)
	}

	// Deleting the label takes its assignments with it, and so does deleting
	// the SavedState's Draft (task 85_3; ADR 0025).
	if _, err := repo.Assign(ctx, assigned, savedState.ID()); err != nil {
		t.Fatalf("store the assignment again: %v", err)
	}
	if err := repo.Delete(ctx, stored.Delete()); err != nil {
		t.Fatalf("Delete the label: %v", err)
	}
	if err := testDB.QueryRowContext(ctx,
		`SELECT count(*) FROM engineering.saved_state_labels WHERE label_id = $1`, saved.ID().UUID()).Scan(&rows); err != nil {
		t.Fatalf("count the assignments after the delete: %v", err)
	}
	if rows != 0 {
		t.Errorf("the assignments of the deleted label: %d, want 0", rows)
	}

	// A label that is gone is not found, and so is a SavedState this server lacks.
	if _, err := repo.Assign(ctx, assigned, savedState.ID()); !errors.Is(err, label.ErrLabelNotFound) {
		t.Errorf("Assign of a deleted label: %v, want ErrLabelNotFound", err)
	}
	live, err := repo.FindByID(ctx, org, other.ID())
	if err != nil {
		t.Fatalf("FindByID another: %v", err)
	}
	missingSavedState := id.NewID[draft.SavedState]()
	orphaned, err := live.Assign(missingSavedState, scope, 9, time.Now())
	if err != nil {
		t.Fatalf("Assign a missing SavedState: %v", err)
	}
	if _, err := repo.Assign(ctx, orphaned, missingSavedState); !errors.Is(err, draft.ErrSavedStateNotFound) {
		t.Errorf("Assign of a missing SavedState: %v, want ErrSavedStateNotFound", err)
	}
}

// TestLabelAssignments_SavedStateCascade: deleting a Draft deletes its saved
// states, and their marks go with them — the exception of ADR 0025 for a
// deleted Project or Library (task 85_3).
func TestLabelAssignments_SavedStateCascade(t *testing.T) {
	cleanTestLabels(t)
	cleanDrafts(t)
	ctx := context.Background()
	repo := repositories.NewLabelRepositoryPostgres(testDB)
	org := soleOrganization(t)
	scope := draft.ProjectScope(mainProject)

	saved, err := repo.Create(ctx, label.CreateLabel(repo.NextID(), org,
		mustLabelName(t, "test-"+uuid.NewString()[:8]), label.Color{}, time.Now()))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	savedState, err := testDrafts().SaveSavedState(ctx, scope, draftVersionOf(t, mainProject),
		commitTitle(t, "First"), commitAuthor(t), projectSavedStateContent())
	if err != nil {
		t.Fatalf("SaveSavedState: %v", err)
	}
	assigned, err := saved.Assign(savedState.ID(), scope, savedState.Number(), time.Now())
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if _, err := repo.Assign(ctx, assigned, savedState.ID()); err != nil {
		t.Fatalf("store the assignment: %v", err)
	}

	cleanDrafts(t)
	var rows int
	if err := testDB.QueryRowContext(ctx,
		`SELECT count(*) FROM engineering.saved_state_labels WHERE label_id = $1`, saved.ID().UUID()).Scan(&rows); err != nil {
		t.Fatalf("count the assignments: %v", err)
	}
	if rows != 0 {
		t.Errorf("the assignments after the Draft's removal: %d, want 0", rows)
	}
}

// TestLabelAssignments_DeleteRacesAssign: a delete of the label and a mark of a
// saved state serialize on the label's row, so the mark is stored and then
// cascade-removed or refused with ErrLabelNotFound — never a dangling row (task
// 85_3; ADR 0025).
func TestLabelAssignments_DeleteRacesAssign(t *testing.T) {
	cleanTestLabels(t)
	cleanDrafts(t)
	ctx := context.Background()
	repo := repositories.NewLabelRepositoryPostgres(testDB)
	org := soleOrganization(t)
	scope := draft.ProjectScope(mainProject)

	saved, err := repo.Create(ctx, label.CreateLabel(repo.NextID(), org,
		mustLabelName(t, "test-"+uuid.NewString()[:8]), label.Color{}, time.Now()))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	savedState, err := testDrafts().SaveSavedState(ctx, scope, draftVersionOf(t, mainProject),
		commitTitle(t, "First"), commitAuthor(t), projectSavedStateContent())
	if err != nil {
		t.Fatalf("SaveSavedState: %v", err)
	}
	assigned, err := saved.Assign(savedState.ID(), scope, savedState.Number(), time.Now())
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, err := repo.Assign(ctx, assigned, savedState.ID())
		results <- err
	}()
	go func() {
		<-start
		results <- repo.Delete(ctx, saved.Delete())
	}()
	close(start)

	for i := 0; i < 2; i++ {
		if err := <-results; err != nil && !errors.Is(err, label.ErrLabelNotFound) {
			t.Errorf("the racing write: %v, want nil or ErrLabelNotFound", err)
		}
	}
	var rows int
	if err := testDB.QueryRowContext(ctx,
		`SELECT count(*) FROM engineering.saved_state_labels WHERE label_id = $1`, saved.ID().UUID()).Scan(&rows); err != nil {
		t.Fatalf("count the assignments: %v", err)
	}
	if rows != 0 {
		t.Errorf("the assignments of the deleted label: %d, want 0", rows)
	}
}
