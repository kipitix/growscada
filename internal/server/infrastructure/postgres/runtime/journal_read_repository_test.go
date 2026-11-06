package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/tagstate"
	runtimepg "github.com/kipitix/growscada/internal/server/infrastructure/postgres/runtime"
)

// journalOf writes changes to a fresh Project and returns its Tag state
// repository and the Project's ID, for a test that reads the Journal back.
// The values are written to one Tag, one entry per change.
func journalOf(t *testing.T, values ...string) (tagstate.Repository, uuid.UUID) {
	t.Helper()
	projectID, _ := newProject(t)
	states := runtimepg.NewTagStateRepositoryPostgres(postgres.DB(t))
	speed := definition(t, projectID, "speed", tag.TagTypeInteger)
	if err := states.UpsertDefinitions(context.Background(), projectID, []tagstate.Definition{speed}); err != nil {
		t.Fatalf("UpsertDefinitions: %v", err)
	}
	for _, value := range values {
		written, _ := mustFind(t, states, projectID, speed.TagID).Write(value, tag.TagQualityGood)
		if _, _, err := states.Write(context.Background(), written, tag.NoSupplier()); err != nil {
			t.Fatalf("write %q: %v", value, err)
		}
	}
	return states, projectID.UUID()
}

// TestJournalRepository_ReadsARangeInTheOrderOfPositions: the page is the
// Project's entries in the order of their positions, from is exclusive, and
// limit cuts the page (task 42).
func TestJournalRepository_ReadsARangeInTheOrderOfPositions(t *testing.T) {
	_, projectID := journalOf(t, "1", "2", "3")
	journal := runtimepg.NewJournalRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()

	all, err := journal.FindAfter(ctx, projectID, 0, 100)
	if err != nil {
		t.Fatalf("FindAfter: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("%d entries, want 3", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].Seq <= all[i-1].Seq {
			t.Errorf("entry %d at %d, not after %d", i, all[i].Seq, all[i-1].Seq)
		}
	}

	afterFirst, err := journal.FindAfter(ctx, projectID, all[0].Seq, 100)
	if err != nil {
		t.Fatalf("FindAfter from the first position: %v", err)
	}
	if len(afterFirst) != 2 || afterFirst[0].Seq != all[1].Seq {
		t.Errorf("after %d: %+v", all[0].Seq, afterFirst)
	}

	page, err := journal.FindAfter(ctx, projectID, 0, 2)
	if err != nil {
		t.Fatalf("FindAfter with a limit: %v", err)
	}
	if len(page) != 2 || page[1].Seq != all[1].Seq {
		t.Errorf("a page of 2: %+v", page)
	}

	position, err := journal.Position(ctx, projectID)
	if err != nil {
		t.Fatalf("Position: %v", err)
	}
	if position != all[len(all)-1].Seq {
		t.Errorf("position %d, the latest entry at %d", position, all[len(all)-1].Seq)
	}

	empty, err := journal.FindAfter(ctx, projectID, position, 100)
	if err != nil {
		t.Fatalf("FindAfter after the latest: %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("after the latest: %+v, want an empty page", empty)
	}
}

// TestJournalRepository_ReadsOnlyItsProject: a page of one Project's Journal
// never carries another Project's entry, whatever its position.
func TestJournalRepository_ReadsOnlyItsProject(t *testing.T) {
	_, first := journalOf(t, "1")
	_, second := journalOf(t, "2")
	journal := runtimepg.NewJournalRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()

	entries, err := journal.FindAfter(ctx, first, 0, 100)
	if err != nil {
		t.Fatalf("FindAfter: %v", err)
	}
	if len(entries) != 1 || entries[0].ProjectID != first {
		t.Errorf("the Journal of %s: %+v", first, entries)
	}
	position, err := journal.Position(ctx, first)
	if err != nil {
		t.Fatalf("Position: %v", err)
	}
	if position != entries[0].Seq {
		t.Errorf("position %d, its only entry at %d", position, entries[0].Seq)
	}
	if second == first {
		t.Fatal("both projects are the same")
	}
}

// TestJournalRepository_EmptyForAProjectWithoutEntries: a live Project that
// wrote nothing has an empty page and position 0.
func TestJournalRepository_EmptyForAProjectWithoutEntries(t *testing.T) {
	projectID, _ := newProject(t)
	journal := runtimepg.NewJournalRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()

	entries, err := journal.FindAfter(ctx, projectID.UUID(), 0, 100)
	if err != nil {
		t.Fatalf("FindAfter: %v", err)
	}
	if entries == nil || len(entries) != 0 {
		t.Errorf("the page of a silent project: %+v", entries)
	}
	position, err := journal.Position(ctx, projectID.UUID())
	if err != nil {
		t.Fatalf("Position: %v", err)
	}
	if position != 0 {
		t.Errorf("position %d of a project without entries", position)
	}
}

// TestJournalRepository_DeletedProjectIsNotFound: a deleted Project's rows are
// not its Journal to serve any more: both reads answer project.ErrProjectNotFound,
// not an empty page (task 42).
func TestJournalRepository_DeletedProjectIsNotFound(t *testing.T) {
	_, projectID := journalOf(t, "1")
	journal := runtimepg.NewJournalRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	if _, err := postgres.DB(t).ExecContext(ctx, `UPDATE projects SET deleted_at = now() WHERE id = $1`, projectID); err != nil {
		t.Fatalf("delete the project: %v", err)
	}

	if _, err := journal.FindAfter(ctx, projectID, 0, 100); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("FindAfter of a deleted project: %v, want project.ErrProjectNotFound", err)
	}
	if _, err := journal.Position(ctx, projectID); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("Position of a deleted project: %v, want project.ErrProjectNotFound", err)
	}
}
