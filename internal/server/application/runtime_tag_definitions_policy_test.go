package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

func TestRuntimeTagDefinitionsPolicy_JobKey(t *testing.T) {
	policy := application.NewRuntimeTagDefinitionsPolicy(nil, nil)
	tagID := id.NewID[tag.Tag]()
	want := "project:" + mainProject.String() + "/tag:" + tagID.String()
	for _, e := range []event.Event{
		tag.NewTagCreatedEvent(tagID, mainProjectID),
		tag.NewTagDeletedEvent(tagID, mainProjectID),
	} {
		if key, ok := policy.JobKey(e); !ok || key != want {
			t.Errorf("%s: key %q (%t), want %q", e.Type(), key, ok, want)
		}
	}
	for _, e := range []event.Event{
		project.NewProjectDeletedEvent(mainProjectID),
		scene.NewSceneCreatedEvent(id.NewID[scene.Scene](), mainProjectID),
	} {
		if key, ok := policy.JobKey(e); ok {
			t.Errorf("%s: unexpected job %q", e.Type(), key)
		}
	}
}

// tagStateOf reports whether the Runtime node holds a state of the Tag.
func tagStateOf(t *testing.T, tagID uuid.UUID) bool {
	t.Helper()
	var n int
	if err := testDB.QueryRowContext(context.Background(), `SELECT count(*) FROM runtime.tags WHERE id = $1`, tagID).Scan(&n); err != nil {
		t.Fatalf("count tag states: %v", err)
	}
	return n == 1
}

// TestRuntimeTagDefinitions_ACreationThatFailedToUpsert_IsMadeUpForOnTagCreated:
// the creation's call to the Runtime node failed; the job of its
// tag_created gives the definition, and the tag accepts values.
func TestRuntimeTagDefinitions_ACreationThatFailedToUpsert_IsMadeUpForOnTagCreated(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	svc := application.NewTagService(contentOver(newScopes()), failingRuntimeNode{RuntimeNode: newRuntimeNode()})

	created, err := svc.CreateTag(ctx, mainProject, appdto.CreateTagInput{Name: "speed", Type: "integer"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if tagStateOf(t, created.ID) {
		t.Fatal("the failed upsert gave a state")
	}
	if _, err := r.states.SetTagValue(ctx, mainProject, runtimeapp.SetTagValueInput{TagID: created.ID, Value: "1", Quality: "good"}); !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("a value before the policy ran: %v, want ErrTagNotFound", err)
	}

	r.settle(t)

	if _, err := r.states.SetTagValue(ctx, mainProject, runtimeapp.SetTagValueInput{TagID: created.ID, Value: "1", Quality: "good"}); err != nil {
		t.Errorf("a value after the policy ran: %v", err)
	}
	// A job run again (merged, or late) keeps the state written.
	if err := application.NewRuntimeTagDefinitionsPolicy(newScopes(), newRuntimeNode()).Run(ctx, "project:"+mainProject.String()+"/tag:"+created.ID.String()); err != nil {
		t.Fatalf("Run again: %v", err)
	}
	found, err := r.tagSvc.FindTagByID(ctx, mainProject, created.ID)
	if err != nil || found.Value != "1" || found.Quality != "good" {
		t.Errorf("after the job ran again: %+v, %v; want the written 1, good", found, err)
	}
}

// TestRuntimeTagDefinitions_ADeletionThatFailedToRemove_IsMadeUpForOnTagDeleted:
// the deletion's call to the Runtime node failed; the job of its
// tag_deleted removes the state.
func TestRuntimeTagDefinitions_ADeletionThatFailedToRemove_IsMadeUpForOnTagDeleted(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	tagID := r.createTag(t, "boolean")
	r.settle(t)
	svc := application.NewTagService(contentOver(newScopes()), failingRuntimeNode{RuntimeNode: newRuntimeNode()})

	if _, err := svc.DeleteTagByID(ctx, mainProject, tagID); err != nil {
		t.Fatalf("DeleteTagByID: %v", err)
	}
	if !tagStateOf(t, tagID) {
		t.Fatal("the failed removal removed the state")
	}

	r.settle(t)

	if tagStateOf(t, tagID) {
		t.Error("the state of the deleted tag is left after the policy ran")
	}
}

// TestRuntimeTagDefinitions_ALateJobOfACreation_DoesNotBringADeletedTagBack:
// the job of a tag_created run after the tag was deleted finds no
// definition in the Draft and leaves no state.
func TestRuntimeTagDefinitions_ALateJobOfACreation_DoesNotBringADeletedTagBack(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	tagID := r.createTag(t, "integer")
	if _, err := r.tagSvc.DeleteTagByID(ctx, mainProject, tagID); err != nil {
		t.Fatalf("DeleteTagByID: %v", err)
	}

	r.settle(t) // tag_created's job runs after the deletion

	if tagStateOf(t, tagID) {
		t.Error("a late job of the creation brought the state back")
	}
}
