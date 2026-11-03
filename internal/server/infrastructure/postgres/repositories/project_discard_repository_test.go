package repositories_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// makeTagOf builds an unsaved Tag of the Project projectID, named name, for
// repo.
func makeTagOf(t *testing.T, name string, projectID id.ID[project.Project], repo tag.TagRepository) tag.Tag {
	t.Helper()
	tagName, err := tag.NewTagName(name)
	if err != nil {
		t.Fatalf("NewTagName(%q): %v", name, err)
	}
	created, err := tag.CreateTag(repo.NextID(), projectID, tagName, tag.TagTypeInteger, tag.NoSupplier())
	if err != nil {
		t.Fatalf("CreateTag(%q): %v", name, err)
	}
	return created
}

// TestProjectSave_DiscardMark_IsStoredAndCleared: the mark a Discard sets
// travels with the Project's row, and clearing it does the same.
func TestProjectSave_DiscardMark_IsStoredAndCleared(t *testing.T) {
	cleanProjects(t)
	ctx := context.Background()
	repo := repositories.NewProjectRepositoryPostgres(testDB)
	line := mustCreateProject(t, repo, "Line 1")

	marked, err := line.StartDiscard()
	if err != nil {
		t.Fatalf("StartDiscard: %v", err)
	}
	saved, err := repo.Save(ctx, marked)
	if err != nil {
		t.Fatalf("Save the marked project: %v", err)
	}
	if !saved.IsDiscarding() {
		t.Errorf("the saved project %s lost its mark", saved)
	}
	found, err := repo.FindByID(ctx, line.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if !found.IsDiscarding() {
		t.Errorf("the stored project %s lost its mark", found)
	}

	saved, err = repo.Save(ctx, found.FinishDiscard())
	if err != nil {
		t.Fatalf("Save the finished project: %v", err)
	}
	found, err = repo.FindByID(ctx, line.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if saved.IsDiscarding() || found.IsDiscarding() {
		t.Errorf("the mark was not cleared: saved %s, stored %s", saved, found)
	}
}

// TestDraftWrites_WhileADiscardRuns_AreRefused: a write of the Project's Draft
// takes the Project's row and refuses while its Discard is under way; the
// repositories a Discard opens write then, and only then (task 35, Q10).
func TestDraftWrites_WhileADiscardRuns_AreRefused(t *testing.T) {
	cleanProjects(t)
	cleanTags(t)
	ctx := context.Background()
	repo := repositories.NewProjectRepositoryPostgres(testDB)
	line := mustCreateProject(t, repo, "Line 1")
	tags := repositories.NewTagRepositoryPostgres(testDB, line.ID())

	stored, err := tags.Save(ctx, makeTagOf(t, "speed", line.ID(), tags))
	if err != nil {
		t.Fatalf("Save the tag: %v", err)
	}
	marked, err := line.StartDiscard()
	if err != nil {
		t.Fatalf("StartDiscard: %v", err)
	}
	if _, err := repo.Save(ctx, marked); err != nil {
		t.Fatalf("Save the marked project: %v", err)
	}

	if _, err := tags.Save(ctx, makeTagOf(t, "flow", line.ID(), tags)); !errors.Is(err, project.ErrProjectDiscarding) {
		t.Errorf("a new tag while the Discard runs: %v, want ErrProjectDiscarding", err)
	}
	if err := tags.Delete(ctx, stored.Delete()); !errors.Is(err, project.ErrProjectDiscarding) {
		t.Errorf("a removed tag while the Discard runs: %v, want ErrProjectDiscarding", err)
	}

	discardTags := repositories.NewProjectScopes(testDB).ForDiscard(line.ID()).Tags()
	if err := discardTags.Delete(ctx, stored.Delete()); err != nil {
		t.Errorf("the Discard's own removal of a tag: %v", err)
	}
}
