package restapi_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/outbox"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
	"github.com/kipitix/growscada/internal/server/interface/restapi"
)

// TestEventsHandlers_GetProjectEvents_DeliversCommittedChangesInCommitOrder runs the
// whole path of a domain event: a service saves an aggregate with its events
// into the outbox, the dispatcher delivers them after commit to the EventBus,
// and the SSE stream sends them to the client, in commit order.
func TestEventsHandlers_GetProjectEvents_DeliversCommittedChangesInCommitOrder(t *testing.T) {
	cleanTags(t)
	if _, err := testDB().ExecContext(context.Background(), "DELETE FROM outbox"); err != nil {
		t.Fatalf("clean outbox: %v", err)
	}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	bus := eventbus.NewEventBus()
	dispatcher := outbox.NewDispatcher(testDB(), bus.Publish)
	go dispatcher.Run(ctx)
	onCommit := repositories.NotifyOnCommit(dispatcher.Notify)
	services := newServices(onCommit)
	tagSvc, sceneSvc := services.tags, services.scenes

	h := restapi.NewEventsHandler(bus, services.projects, 100)
	rec := newSyncRecorder()
	cancel, done := runProjectEventsHandler(t, h, rec, mainProject)
	waitFor(t, time.Second, func() bool { return strings.Contains(rec.body(), ": connected") })

	created, err := tagSvc.CreateTag(ctx, mainProject, appdto.CreateTagInput{Name: "level", Type: "integer", Value: "0", Quality: "good"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	sc, err := sceneSvc.CreateScene(ctx, mainProject, appdto.SceneInput{Name: "ordered", Width: 800, Height: 600})
	if err != nil {
		t.Fatalf("CreateScene: %v", err)
	}
	ver := created.Version
	for i := range 3 {
		updated, err := tagSvc.SetTagValueByID(ctx, mainProject, appdto.UpdateTagInput{ID: created.ID, Value: string(rune('1' + i)), Quality: "good", Version: ver})
		if err != nil {
			t.Fatalf("SetTagValueByID: %v", err)
		}
		ver = updated.Version
	}
	if _, err := sceneSvc.DeleteSceneByID(ctx, mainProject, sc.ID); err != nil {
		t.Fatalf("DeleteSceneByID: %v", err)
	}
	if _, err := tagSvc.DeleteTagByID(ctx, mainProject, created.ID); err != nil {
		t.Fatalf("DeleteTagByID: %v", err)
	}

	waitFor(t, 5*time.Second, func() bool { return strings.Contains(rec.body(), "tag_deleted") })
	cancel()
	<-done

	var types []string
	var values []string
	for _, m := range sseEventMessages(t, rec.body()) {
		typ := m["type"].(string)
		if strings.HasPrefix(typ, "client_") {
			continue
		}
		types = append(types, typ)
		if typ == "tag_updated" {
			values = append(values, m["tag"].(map[string]any)["value"].(string))
		}
	}
	want := []string{"tag_created", "scene_created", "tag_updated", "tag_updated", "tag_updated", "scene_deleted", "tag_deleted"}
	if !slices.Equal(types, want) {
		t.Errorf("events %v, want in commit order %v", types, want)
	}
	if !slices.Equal(values, []string{"1", "2", "3"}) {
		t.Errorf("tag_updated values %v, want [1 2 3]", values)
	}
}
