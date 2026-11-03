package runtime_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/tagstate"
	"github.com/kipitix/growscada/internal/server/domain/version"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
	runtimepg "github.com/kipitix/growscada/internal/server/infrastructure/postgres/runtime"
	"github.com/kipitix/growscada/internal/server/servertest"
)

var postgres servertest.Postgres

func TestMain(m *testing.M) {
	code := m.Run()
	postgres.Close()
	os.Exit(code)
}

// newProject cleans the Tag states and returns a new live project of the one
// organization, and that organization.
func newProject(t *testing.T) (id.ID[project.Project], id.ID[organization.Organization]) {
	t.Helper()
	db := postgres.DB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `DELETE FROM runtime.tags`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	orgID, err := repositories.FindSoleOrganization(ctx, db)
	if err != nil {
		t.Fatalf("organization: %v", err)
	}
	projectID := id.NewID[project.Project]()
	if _, err := db.ExecContext(ctx, `INSERT INTO projects (id, organization_id, name, version) VALUES ($1, $2, $3, 1)`,
		projectID.UUID(), orgID.UUID(), "p-"+projectID.String()); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return projectID, orgID
}

func definition(t *testing.T, projectID id.ID[project.Project], name string, tagType tag.TagType) tagstate.Definition {
	t.Helper()
	tagName, err := tag.NewTagName(name)
	if err != nil {
		t.Fatal(err)
	}
	return tagstate.Definition{TagID: id.NewID[tag.Tag](), ProjectID: projectID, Name: tagName, Type: tagType, Version: version.Committed[tag.Tag]()}
}

func mustFind(t *testing.T, repo tagstate.Repository, projectID id.ID[project.Project], tagID id.ID[tag.Tag]) tagstate.TagState {
	t.Helper()
	found, err := repo.FindByIDs(context.Background(), projectID, []id.ID[tag.Tag]{tagID})
	if err != nil {
		t.Fatalf("FindByIDs: %v", err)
	}
	s, ok := found[tagID]
	if !ok {
		t.Fatalf("no state of %s", tagID)
	}
	return s
}

func TestUpsertDefinitions_InsertsTheInitialStateAndKeepsAWrittenOne(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewTagStateRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	speed := definition(t, projectID, "speed", tag.TagTypeInteger)

	if err := repo.UpsertDefinitions(ctx, projectID, []tagstate.Definition{speed}); err != nil {
		t.Fatalf("UpsertDefinitions: %v", err)
	}
	initial := mustFind(t, repo, projectID, speed.TagID)
	if initial.Value().String() != "0" || initial.Quality() != tag.TagQualityBad || initial.Seq() != 0 || initial.Definition() != speed {
		t.Errorf("initial state %s of %v", initial, initial.Definition())
	}

	written, _ := initial.Write("42", tag.TagQualityGood)
	stored, err := repo.Write(ctx, written, tag.NoSupplier())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := repo.UpsertDefinitions(ctx, projectID, []tagstate.Definition{speed}); err != nil {
		t.Fatalf("UpsertDefinitions again: %v", err)
	}
	if again := mustFind(t, repo, projectID, speed.TagID); again.Value().String() != "42" || again.Seq() != stored.Seq() {
		t.Errorf("the upsert again changed the state: %s", again)
	}
}

// TestUpsertDefinitions_AfterARevision_WritesNoCopyOutsideIt: once a Project
// has a Revision, the bridge is silent — a Tag created after the Deploy commits
// is no copy of the running Revision, so a value write is 404 until the next
// Deploy (task 35, Q4).
func TestUpsertDefinitions_AfterARevision_WritesNoCopyOutsideIt(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	ctx := context.Background()
	mustCreateRevision(t, runtimepg.NewRevisionRepositoryPostgres(db), projectID.UUID(), documentOf(projectID), time.Now())

	repo := runtimepg.NewTagStateRepositoryPostgres(db)
	late := definition(t, projectID, "late", tag.TagTypeInteger)
	if err := repo.UpsertDefinitions(ctx, projectID, []tagstate.Definition{late}); err != nil {
		t.Fatalf("UpsertDefinitions: %v", err)
	}
	found, err := repo.FindByIDs(ctx, projectID, []id.ID[tag.Tag]{late.TagID})
	if err != nil {
		t.Fatalf("FindByIDs: %v", err)
	}
	if _, ok := found[late.TagID]; ok {
		t.Error("the bridge copied a tag outside the running revision")
	}
}

// TestDelete_AfterARevision_KeepsTheRunningCopy: a Draft deletion reaches the
// Runtime node with the next Deploy; while the Revision runs, its copy stays.
func TestDelete_AfterARevision_KeepsTheRunningCopy(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	ctx := context.Background()
	speed := definition(t, projectID, "speed", tag.TagTypeInteger)
	mustCreateRevision(t, runtimepg.NewRevisionRepositoryPostgres(db), projectID.UUID(), documentOf(projectID, withTags(documentTag(speed))), time.Now())

	repo := runtimepg.NewTagStateRepositoryPostgres(db)
	if err := repo.Delete(ctx, projectID, []id.ID[tag.Tag]{speed.TagID}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if stored := mustFind(t, repo, projectID, speed.TagID); stored.Definition().TagID != speed.TagID || stored.Definition().Type != speed.Type {
		t.Errorf("a Draft deletion changed the running copy: %v", stored.Definition())
	}
}

// TestUpsertDefinitions_ConcurrentWithADeploy_WritesNoCopyOutsideIt: whichever
// way a bridge write and a Deploy interleave, the bridge leaves no copy outside
// the running Revision (task 35, Q4). Both take the same advisory lock, so the
// bridge either writes before the Revision exists — and the Deploy's diff
// removes a copy the document does not name — or finds the Revision and writes
// nothing.
func TestUpsertDefinitions_ConcurrentWithADeploy_WritesNoCopyOutsideIt(t *testing.T) {
	ctx := context.Background()
	db := postgres.DB(t)
	for round := 0; round < 10; round++ {
		projectID, _ := newProject(t)
		late := definition(t, projectID, "late", tag.TagTypeInteger)
		document := documentOf(projectID)
		revisions := runtimepg.NewRevisionRepositoryPostgres(db)
		states := runtimepg.NewTagStateRepositoryPostgres(db)

		start := make(chan struct{})
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, err := revisions.CreateRevision(ctx, projectID.UUID(), document, time.Now())
			errs <- err
		}()
		go func() {
			defer wg.Done()
			<-start
			errs <- states.UpsertDefinitions(ctx, projectID, []tagstate.Definition{late})
		}()
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("round %d: %v", round, err)
			}
		}

		found, err := states.FindByIDs(ctx, projectID, []id.ID[tag.Tag]{late.TagID})
		if err != nil {
			t.Fatalf("round %d: FindByIDs: %v", round, err)
		}
		if _, ok := found[late.TagID]; ok {
			t.Fatalf("round %d: the bridge left a copy outside the running revision", round)
		}
	}
}

func TestUpsertDefinitions_OfAnotherProject_IsAnError(t *testing.T) {
	projectID, _ := newProject(t)
	other, _ := newProject(t)
	repo := runtimepg.NewTagStateRepositoryPostgres(postgres.DB(t))
	if err := repo.UpsertDefinitions(context.Background(), projectID, []tagstate.Definition{definition(t, other, "x", tag.TagTypeBoolean)}); err == nil {
		t.Error("a definition of another project was upserted")
	}
}

// TestUpsertDefinitions_OverARowOfAnotherTypeOrProject_IsAnError: the row
// stored keeps its definition, and the call fails rather than doing nothing.
func TestUpsertDefinitions_OverARowOfAnotherTypeOrProject_IsAnError(t *testing.T) {
	projectID, _ := newProject(t)
	other, _ := newProject(t)
	repo := runtimepg.NewTagStateRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	speed := definition(t, projectID, "speed", tag.TagTypeInteger)
	if err := repo.UpsertDefinitions(ctx, projectID, []tagstate.Definition{speed}); err != nil {
		t.Fatalf("UpsertDefinitions: %v", err)
	}

	retyped := speed
	retyped.Type = tag.TagTypeBoolean
	if err := repo.UpsertDefinitions(ctx, projectID, []tagstate.Definition{retyped}); err == nil {
		t.Error("an upsert of another type succeeded")
	}
	moved := speed
	moved.ProjectID = other
	if err := repo.UpsertDefinitions(ctx, other, []tagstate.Definition{moved}); err == nil {
		t.Error("an upsert into another project succeeded")
	}
	if stored := mustFind(t, repo, projectID, speed.TagID); stored.Definition() != speed {
		t.Errorf("the stored definition changed: %v", stored.Definition())
	}
}

// TestWrite_NumbersEveryWriteHigher: several writes in a row take
// increasing seqs, each different (the acceptance of task 32).
func TestWrite_NumbersEveryWriteHigher(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewTagStateRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	speed := definition(t, projectID, "speed", tag.TagTypeInteger)
	if err := repo.UpsertDefinitions(ctx, projectID, []tagstate.Definition{speed}); err != nil {
		t.Fatalf("UpsertDefinitions: %v", err)
	}

	current := mustFind(t, repo, projectID, speed.TagID)
	var last tagstate.Seq
	for i, value := range []string{"1", "2", "2", "3"} {
		written, _ := current.Write(value, tag.TagQualityGood)
		stored, err := repo.Write(ctx, written, tag.NoSupplier())
		if err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
		if stored.Seq() <= last {
			t.Errorf("write %d took seq %d, not above %d", i, stored.Seq(), last)
		}
		last = stored.Seq()
	}
	if got := mustFind(t, repo, projectID, speed.TagID); got.Value().String() != "3" || got.Seq() != last {
		t.Errorf("stored %s, want the last write 3 at seq %d", got, last)
	}
}

// TestWrite_Concurrent_TheStoredStateHasTheGreatestSeq: writes racing for one
// Tag each take their own seq, and the state stored is the one with the
// greatest.
func TestWrite_Concurrent_TheStoredStateHasTheGreatestSeq(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewTagStateRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	speed := definition(t, projectID, "speed", tag.TagTypeInteger)
	if err := repo.UpsertDefinitions(ctx, projectID, []tagstate.Definition{speed}); err != nil {
		t.Fatalf("UpsertDefinitions: %v", err)
	}
	initial := mustFind(t, repo, projectID, speed.TagID)

	const writers = 8
	results := make([]tagstate.TagState, writers)
	var wg sync.WaitGroup
	for i := range writers {
		wg.Go(func() {
			written, _ := initial.Write(int64(i), tag.TagQualityGood)
			stored, err := repo.Write(ctx, written, tag.NoSupplier())
			if err != nil {
				t.Errorf("Write %d: %v", i, err)
			}
			results[i] = stored
		})
	}
	wg.Wait()

	seen := map[tagstate.Seq]bool{}
	var greatest tagstate.TagState
	for _, r := range results {
		if seen[r.Seq()] {
			t.Errorf("seq %d taken twice", r.Seq())
		}
		seen[r.Seq()] = true
		if r.Seq() > greatest.Seq() {
			greatest = r
		}
	}
	if got := mustFind(t, repo, projectID, speed.TagID); got.Seq() != greatest.Seq() || got.Value() != greatest.Value() {
		t.Errorf("stored %s, want the write with the greatest seq %s", got, greatest)
	}
}

func TestWrite_WithoutAState_IsTagNotFound(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewTagStateRepositoryPostgres(postgres.DB(t))
	never, _ := tagstate.Initial(definition(t, projectID, "never", tag.TagTypeBoolean))
	written, _ := never.Write("true", tag.TagQualityGood)
	if _, err := repo.Write(context.Background(), written, tag.NoSupplier()); !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("a write of a tag without a state: %v, want ErrTagNotFound", err)
	}
}

func TestFindInLiveProject_ChecksTheProject(t *testing.T) {
	projectID, orgID := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewTagStateRepositoryPostgres(db)
	ctx := context.Background()
	speed := definition(t, projectID, "speed", tag.TagTypeInteger)
	if err := repo.UpsertDefinitions(ctx, projectID, []tagstate.Definition{speed}); err != nil {
		t.Fatalf("UpsertDefinitions: %v", err)
	}

	if found, err := repo.FindInLiveProject(ctx, projectID, speed.TagID, orgID); err != nil || found.TagID() != speed.TagID {
		t.Errorf("a state of a live project: %v, %v", found, err)
	}
	if _, err := repo.FindInLiveProject(ctx, projectID, speed.TagID, id.NewID[organization.Organization]()); !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("through another organization: %v, want ErrTagNotFound", err)
	}
	other, _ := newProject(t) // cleans the states: upsert again
	if err := repo.UpsertDefinitions(ctx, projectID, []tagstate.Definition{speed}); err != nil {
		t.Fatalf("UpsertDefinitions: %v", err)
	}
	if _, err := repo.FindInLiveProject(ctx, other, speed.TagID, orgID); !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("through another project: %v, want ErrTagNotFound", err)
	}
	if live, err := repo.IsProjectLive(ctx, projectID, orgID); err != nil || !live {
		t.Errorf("IsProjectLive of a live project: %v, %v", live, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE projects SET deleted_at = now() WHERE id = $1`, projectID.UUID()); err != nil {
		t.Fatalf("mark the project deleted: %v", err)
	}
	if _, err := repo.FindInLiveProject(ctx, projectID, speed.TagID, orgID); !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("a state of a deleted project: %v, want ErrTagNotFound", err)
	}
	if live, err := repo.IsProjectLive(ctx, projectID, orgID); err != nil || live {
		t.Errorf("IsProjectLive of a deleted project: %v, %v", live, err)
	}
	if _, err := repo.FindByIDs(ctx, projectID, []id.ID[tag.Tag]{speed.TagID}); err != nil {
		t.Errorf("FindByIDs still reads it, for the cleanup: %v", err)
	}
}

func TestDelete_RemovesTheStatesOfTheProjectOnly(t *testing.T) {
	projectID, _ := newProject(t)
	other, _ := newProject(t)
	repo := runtimepg.NewTagStateRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	a, b := definition(t, projectID, "a", tag.TagTypeString), definition(t, projectID, "b", tag.TagTypeString)
	c := definition(t, other, "c", tag.TagTypeString)
	if err := repo.UpsertDefinitions(ctx, projectID, []tagstate.Definition{a, b}); err != nil {
		t.Fatalf("UpsertDefinitions: %v", err)
	}
	if err := repo.UpsertDefinitions(ctx, other, []tagstate.Definition{c}); err != nil {
		t.Fatalf("UpsertDefinitions: %v", err)
	}

	if err := repo.Delete(ctx, other, []id.ID[tag.Tag]{a.TagID}); err != nil {
		t.Fatalf("Delete through another project: %v", err)
	}
	if err := repo.Delete(ctx, projectID, []id.ID[tag.Tag]{a.TagID, id.NewID[tag.Tag]()}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	found, err := repo.FindByIDs(ctx, projectID, []id.ID[tag.Tag]{a.TagID, b.TagID})
	if _, gone := found[a.TagID]; err != nil || gone || len(found) != 1 {
		t.Errorf("after Delete: %v, %v; want b alone", found, err)
	}

	if err := repo.DeleteAllOfProject(ctx, projectID); err != nil {
		t.Fatalf("DeleteAllOfProject: %v", err)
	}
	if found, _ := repo.FindByIDs(ctx, projectID, []id.ID[tag.Tag]{b.TagID}); len(found) != 0 {
		t.Errorf("a state left: %v", found)
	}
	if found, _ := repo.FindByIDs(ctx, other, []id.ID[tag.Tag]{c.TagID}); len(found) != 1 {
		t.Error("the other project's state was removed")
	}
}

// TestWrite_OnlyByTheSupplier: a Tag a Device supplies is written by that
// Device alone; a Tag nobody supplies is written by people alone. The
// definition's copy carries the supplier, and a write of another writer is
// refused, not taken for a missing Tag.
func TestWrite_OnlyByTheSupplier(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewTagStateRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	supplier := tag.SuppliedByDevice(id.NewID[device.Device]())
	speed := definition(t, projectID, "speed", tag.TagTypeInteger)
	speed.Supplier = supplier
	if err := repo.UpsertDefinitions(ctx, projectID, []tagstate.Definition{speed}); err != nil {
		t.Fatalf("UpsertDefinitions: %v", err)
	}
	current := mustFind(t, repo, projectID, speed.TagID)
	if current.Definition().Supplier != supplier {
		t.Errorf("stored supplier %s, want %s", current.Definition().Supplier, supplier)
	}
	written, _ := current.Write("1", tag.TagQualityGood)

	for name, writer := range map[string]tag.Supplier{
		"another device": tag.SuppliedByDevice(id.NewID[device.Device]()),
		"a person":       tag.NoSupplier(),
	} {
		if _, err := repo.Write(ctx, written, writer); !errors.Is(err, tag.ErrTagNotSuppliedByDevice) {
			t.Errorf("%s writing a supplied tag: %v, want ErrTagNotSuppliedByDevice", name, err)
		}
	}
	if stored, err := repo.Write(ctx, written, supplier); err != nil || stored.Value().String() != "1" {
		t.Errorf("the supplier's write: %v, %v", stored, err)
	}

	open := definition(t, projectID, "open", tag.TagTypeInteger)
	if err := repo.UpsertDefinitions(ctx, projectID, []tagstate.Definition{open}); err != nil {
		t.Fatalf("UpsertDefinitions(open): %v", err)
	}
	openWritten, _ := mustFind(t, repo, projectID, open.TagID).Write("2", tag.TagQualityGood)
	if _, err := repo.Write(ctx, openWritten, tag.SuppliedByDevice(id.NewID[device.Device]())); !errors.Is(err, tag.ErrTagNotSuppliedByDevice) {
		t.Errorf("a device writing a tag nobody supplies: %v, want ErrTagNotSuppliedByDevice", err)
	}
	if _, err := repo.Write(ctx, openWritten, tag.NoSupplier()); err != nil {
		t.Errorf("a person writing a tag nobody supplies: %v", err)
	}
}
