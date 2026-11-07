package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/tagstate"
	runtimepg "github.com/kipitix/growscada/internal/server/infrastructure/postgres/runtime"
)

// documentTag is the Tag definition of a document from a state definition.
func documentTag(def tagstate.Definition) projectv0.TagDefinition {
	return projectv0.TagDefinition{ID: def.TagID.UUID(), Name: def.Name.String(), Type: def.Type.String()}
}

// TestDeploymentService_Deploy_KeepsTheTagStatesOfTheRunningDeployment drives the
// service over the real repository: two Deploys in a row, the state of a Tag
// that stays, a new Tag's initial state, a TagType change and a Tag the next
// Deployment no longer has (task 35, Q4).
func TestDeploymentService_Deploy_KeepsTheTagStatesOfTheRunningDeployment(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	deployments := runtimepg.NewDeploymentRepositoryPostgres(db)
	states := runtimepg.NewTagStateRepositoryPostgres(db)
	ctx := context.Background()

	var published []event.Event
	service := runtimeapp.NewDeploymentService(deployments, func(e event.Event) {
		published = append(published, e)
		ev, ok := e.(runtimeapp.DeploymentCreatedEvent)
		if !ok {
			t.Errorf("published a %T, not a DeploymentCreatedEvent", e)
			return
		}
		// The row is readable when the event is published: the commit happened.
		var number int
		if err := db.QueryRowContext(ctx,
			`SELECT number FROM runtime.deployments WHERE project_id = $1 AND number = $2`,
			projectID.UUID(), ev.Number(),
		).Scan(&number); err != nil {
			t.Errorf("deployment %d published before its commit: %v", ev.Number(), err)
		}
	})

	speed := definition(t, projectID, "speed", tag.TagTypeInteger)
	running := definition(t, projectID, "running", tag.TagTypeBoolean)

	first := documentOf(projectID, withTags(documentTag(speed)))
	rev1, err := service.Deploy(ctx, projectID.UUID(), first)
	if err != nil {
		t.Fatalf("Deploy 1: %v", err)
	}
	if rev1.Number != 1 {
		t.Errorf("first deployment number %d, want 1", rev1.Number)
	}
	initialSpeed := mustFind(t, states, projectID, speed.TagID)
	if initialSpeed.Value().String() != "0" || initialSpeed.Quality() != tag.TagQualityBad {
		t.Errorf("the new tag's initial state: %s", initialSpeed)
	}
	written, err := initialSpeed.Write("42", tag.TagQualityGood)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, _, err := states.Write(ctx, written, tag.NoSupplier()); err != nil {
		t.Fatalf("Write: %v", err)
	}

	second := documentOf(projectID, withTags(documentTag(speed), documentTag(running)))
	rev2, err := service.Deploy(ctx, projectID.UUID(), second)
	if err != nil {
		t.Fatalf("Deploy 2: %v", err)
	}
	if rev2.Number != 2 {
		t.Errorf("second deployment number %d, want 2", rev2.Number)
	}
	if rev2.DeployedAt.Before(rev1.DeployedAt) {
		t.Errorf("deployed_at went back: %s then %s", rev1.DeployedAt, rev2.DeployedAt)
	}
	if kept := mustFind(t, states, projectID, speed.TagID); kept.Value().String() != "42" || kept.Quality() != tag.TagQualityGood {
		t.Errorf("the remaining tag lost its state: %s", kept)
	}
	if fresh := mustFind(t, states, projectID, running.TagID); fresh.Value().String() != "false" || fresh.Quality() != tag.TagQualityBad {
		t.Errorf("the new tag's state: %s", fresh)
	}

	// The same Tag with another TagType starts again; the one the document no
	// longer names goes with its state.
	retyped := speed
	retyped.Type = tag.TagTypeString
	third := documentOf(projectID, withTags(documentTag(retyped)))
	if _, err := service.Deploy(ctx, projectID.UUID(), third); err != nil {
		t.Fatalf("Deploy 3: %v", err)
	}
	if reset := mustFind(t, states, projectID, speed.TagID); reset.Value().String() != "" || reset.Quality() != tag.TagQualityBad {
		t.Errorf("a TagType change did not reset the state: %s", reset)
	}
	if gone := findStates(t, states, projectID, running.TagID); len(gone) != 0 {
		t.Errorf("the state of the removed tag survived: %v", gone)
	}

	// A Tag created by the bridge after the Deploy commits stays outside the
	// running Deployment: its value is 404 until the next Deploy.
	late := definition(t, projectID, "late", tag.TagTypeInteger)
	if err := states.UpsertDefinitions(ctx, projectID, []tagstate.Definition{late}); err != nil {
		t.Fatalf("UpsertDefinitions: %v", err)
	}
	if found := findStates(t, states, projectID, late.TagID); len(found) != 0 {
		t.Errorf("the bridge copied a tag outside the deployment: %v", found)
	}
	unwritten, err := tagstate.Initial(late)
	if err != nil {
		t.Fatalf("initial state: %v", err)
	}
	if _, _, err := states.Write(ctx, unwritten, tag.NoSupplier()); !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("writing a tag outside the deployment: %v, want ErrTagNotFound", err)
	}

	fourth := documentOf(projectID, withTags(documentTag(retyped), documentTag(late)))
	if _, err := service.Deploy(ctx, projectID.UUID(), fourth); err != nil {
		t.Fatalf("Deploy 4: %v", err)
	}
	fresh := mustFind(t, states, projectID, late.TagID)
	if fresh.Value().String() != "0" || fresh.Quality() != tag.TagQualityBad {
		t.Errorf("the tag of the next deployment starts at %s", fresh)
	}
	writtenLate, err := fresh.Write("7", tag.TagQualityGood)
	if err != nil {
		t.Fatalf("write the late tag: %v", err)
	}
	if _, _, err := states.Write(ctx, writtenLate, tag.NoSupplier()); err != nil {
		t.Errorf("the deployed tag refuses a write: %v", err)
	}

	if len(published) != 4 {
		t.Fatalf("published %d events, want 4", len(published))
	}
	for i, e := range published {
		ev, ok := e.(runtimeapp.DeploymentCreatedEvent)
		if !ok {
			t.Fatalf("published a %T, not a DeploymentCreatedEvent", e)
		}
		if ev.Number() != i+1 {
			t.Errorf("event %d names deployment %d, want %d", i, ev.Number(), i+1)
		}
		if ev.ProjectID().UUID() != projectID.UUID() {
			t.Errorf("event %d is of project %s", i, ev.ProjectID())
		}
		if ev.DeployedAt().IsZero() || ev.DeployedAt().After(time.Now()) {
			t.Errorf("event %d deployed at %s", i, ev.DeployedAt())
		}
	}
}

// findStates returns the states of the Tags that have one.
func findStates(t *testing.T, repo tagstate.Repository, projectID id.ID[project.Project], tagIDs ...id.ID[tag.Tag]) map[id.ID[tag.Tag]]tagstate.TagState {
	t.Helper()
	found, err := repo.FindByIDs(context.Background(), projectID, tagIDs)
	if err != nil {
		t.Fatalf("FindByIDs: %v", err)
	}
	return found
}
