package restapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/outbox"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
	"github.com/kipitix/growscada/internal/server/interface/restapi"
)

// cleanTestLabels removes the labels the tests create, leaving the
// migrations' seed labels alone.
func cleanTestLabels(t *testing.T) {
	t.Helper()
	if _, err := testDB().ExecContext(context.Background(),
		`DELETE FROM engineering.labels WHERE name LIKE 'bruno-test-%'`); err != nil {
		t.Fatalf("cleanTestLabels: %v", err)
	}
}

// TestLabels_CRUD walks a label through the API: create, the taken name, the
// rename and recolor, the update that keeps what it does not state, and the
// delete (task 85_2; ADR 0025).
func TestLabels_CRUD(t *testing.T) {
	cleanTestLabels(t)
	router := newServices().router()
	const p = apiv0.PathPrefix
	name := "bruno-test-" + uuid.NewString()[:8]

	rec := serve(router, http.MethodPost, p+apiv0.LabelPath, apiv0.CreateLabelRequest{Name: "  " + name + "  ", Color: "#A1B2C3"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /labels: %d %s", rec.Code, rec.Body)
	}
	created := decodeBody[apiv0.LabelResponse](t, rec)
	if created.Name != name || created.Color != "#a1b2c3" || created.AssignedCount != 0 || created.ID == uuid.Nil {
		t.Errorf("the created label: %+v", created)
	}

	rec = serve(router, http.MethodPost, p+apiv0.LabelPath, apiv0.CreateLabelRequest{Name: strings.ToUpper(name)})
	if rec.Code != http.StatusConflict {
		t.Fatalf("a taken name: %d, want 409", rec.Code)
	}
	if problem := decodeBody[apiv0.Problem](t, rec); problem.Type != restapi.TypeConflict || !strings.Contains(problem.Title, "Label") {
		t.Errorf("the conflict problem: %+v", problem)
	}
	if rec := serve(router, http.MethodPost, p+apiv0.LabelPath, apiv0.CreateLabelRequest{Name: ""}); rec.Code != http.StatusBadRequest {
		t.Errorf("an empty name: %d, want 400", rec.Code)
	}
	if rec := serve(router, http.MethodPost, p+apiv0.LabelPath, apiv0.CreateLabelRequest{Name: name + "-x", Color: "#12345"}); rec.Code != http.StatusBadRequest {
		t.Errorf("a bad color: %d, want 400", rec.Code)
	}

	// PATCH states only the fields that change.
	labelPath := p + apiv0.LabelPathOf(created.ID.String())
	renamed := name + "-renamed"
	rec = serve(router, http.MethodPatch, labelPath, apiv0.UpdateLabelRequest{Name: &renamed})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH the name: %d %s", rec.Code, rec.Body)
	}
	if got := decodeBody[apiv0.LabelResponse](t, rec); got.Name != renamed || got.Color != "#a1b2c3" {
		t.Errorf("the renamed label: %+v", got)
	}
	none := ""
	rec = serve(router, http.MethodPatch, labelPath, apiv0.UpdateLabelRequest{Color: &none})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH the color: %d %s", rec.Code, rec.Body)
	}
	if got := decodeBody[apiv0.LabelResponse](t, rec); got.Name != renamed || got.Color != "" {
		t.Errorf("the cleared color: %+v", got)
	}
	if rec := serve(router, http.MethodPatch, labelPath, apiv0.UpdateLabelRequest{Name: ptr(" ")}); rec.Code != http.StatusBadRequest {
		t.Errorf("an empty rename: %d, want 400", rec.Code)
	}
	if rec := serve(router, http.MethodPatch, p+apiv0.LabelPathOf(uuid.NewString()), apiv0.UpdateLabelRequest{Name: &renamed}); rec.Code != http.StatusNotFound {
		t.Errorf("a missing label: %d, want 404", rec.Code)
	}

	labels := decodeBody[apiv0.GetLabelsResponse](t, serve(router, http.MethodGet, p+apiv0.LabelPath, nil))
	var found bool
	for _, l := range labels.Labels {
		if l.ID == created.ID {
			found = true
			if l.Name != renamed || l.Color != "" {
				t.Errorf("the listed label: %+v", l)
			}
		}
	}
	if !found {
		t.Errorf("GET /labels misses the created label: %+v", labels)
	}

	if rec := serve(router, http.MethodDelete, labelPath, nil); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE: %d, want 204", rec.Code)
	}
	if rec := serve(router, http.MethodDelete, labelPath, nil); rec.Code != http.StatusNotFound {
		t.Errorf("a second DELETE: %d, want 404", rec.Code)
	}
}

// TestLabels_SeedsOfTheCleanDatabase: a migrated database carries the two
// starter labels in its one Organization, and the API lists them by name
// (task 85_2; ADR 0025).
func TestLabels_SeedsOfTheCleanDatabase(t *testing.T) {
	cleanTestLabels(t)
	router := newServices().router()
	labels := decodeBody[apiv0.GetLabelsResponse](t,
		serve(router, http.MethodGet, apiv0.PathPrefix+apiv0.LabelPath, nil))

	want := []string{"ready_for_deploy", "ready_for_import"}
	var got []string
	for _, l := range labels.Labels {
		if l.Color != "" || l.AssignedCount != 0 || l.CreatedAt.IsZero() {
			t.Errorf("the seed label %q: %+v, want no color, no assignment and a time", l.Name, l)
		}
		got = append(got, l.Name)
	}
	for _, name := range want {
		found := false
		for _, l := range labels.Labels {
			found = found || l.Name == name
		}
		if !found {
			t.Errorf("the seed label %q is missing: %v", name, got)
		}
	}
}

// TestLabels_EventsOnTheServerStream: a label change is a server event, so
// every tab sees it, and a Project's stream carries its own content alone
// (task 85_2; ADR 0025).
func TestLabels_EventsOnTheServerStream(t *testing.T) {
	cleanTestLabels(t)
	if _, err := testDB().ExecContext(context.Background(), "DELETE FROM outbox"); err != nil {
		t.Fatalf("clean outbox: %v", err)
	}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	bus := eventbus.NewEventBus()
	dispatcher := outbox.NewDispatcher(testDB(), bus.Publish)
	go dispatcher.Run(ctx)
	services := newServicesPublishingTo(bus.Publish, repositories.NotifyOnCommit(dispatcher.Notify))

	h := restapi.NewEventsHandler(bus, services.projects, 100)
	rec := newSyncRecorder()
	cancel, done := runEventsHandler(t, h, rec)
	waitFor(t, time.Second, func() bool { return strings.Contains(rec.body(), ": connected") })

	created, err := services.labels.Create(ctx, "bruno-test-"+uuid.NewString()[:8], "#22c55e")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool { return strings.Contains(rec.body(), "label_created") })
	cancel()
	<-done

	want := map[string]any{"type": "label_created", "id": created.ID.String()}
	found := false
	for _, m := range sseEventMessages(t, rec.body()) {
		if m["type"] != "label_created" {
			continue
		}
		found = true
		for key, value := range want {
			if m[key] != value {
				t.Errorf("label_created %s = %v, want %v", key, m[key], value)
			}
		}
	}
	if !found {
		t.Fatalf("the server's stream carried no label_created: %q", rec.body())
	}
}
