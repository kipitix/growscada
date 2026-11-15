package restapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/outbox"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
	"github.com/kipitix/growscada/internal/server/interface/restapi"
)

// TestSavedStateLabels_AssignFilterAndChips covers the label assignment routes
// of a Project's saved state (task 85_3; ADR 0025): a label is placed and taken
// off idempotently, the history lists the chips, ?label= filters it, the
// label's assigned count follows, and a saved state or a label this server
// lacks is 404.
func TestSavedStateLabels_AssignFilterAndChips(t *testing.T) {
	cleanProjectsRest(t)
	cleanTestLabels(t)
	if _, err := testDB().ExecContext(context.Background(), "DELETE FROM engineering.drafts"); err != nil {
		t.Fatalf("clean drafts: %v", err)
	}
	services := newServices()
	router := services.router()
	ctx := context.Background()

	label, err := services.labels.Create(ctx, "bruno-test-"+uuid.NewString()[:8], "#22c55e")
	if err != nil {
		t.Fatalf("Create label: %v", err)
	}
	other, err := services.labels.Create(ctx, "bruno-test-"+uuid.NewString()[:8], "")
	if err != nil {
		t.Fatalf("Create another label: %v", err)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/saved-states", apiv0.CreateSavedStateRequest{
		Version: 1, Title: "First save", Author: "Ada",
	}); rec.Code != http.StatusCreated {
		t.Fatalf("POST saved state: %d %s", rec.Code, rec.Body)
	}

	labelPath := mainPath + "/saved-states/1/labels/" + label.ID.String()
	rec := serve(router, http.MethodPut, labelPath, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT a label on a saved state: %d %s", rec.Code, rec.Body)
	}
	assigned := decodeBody[apiv0.SavedStateLabelsResponse](t, rec)
	if len(assigned.Labels) != 1 || assigned.Labels[0].ID != label.ID || assigned.Labels[0].Name != label.Name {
		t.Errorf("the labels after the assignment: %+v", assigned)
	}
	// A repeat is neither a conflict nor a second mark.
	if rec := serve(router, http.MethodPut, labelPath, nil); rec.Code != http.StatusOK {
		t.Errorf("a repeated PUT: %d, want 200", rec.Code)
	}
	if labels := decodeBody[apiv0.SavedStateLabelsResponse](t, serve(router, http.MethodPut, labelPath, nil)); len(labels.Labels) != 1 {
		t.Errorf("a repeated PUT marked the saved state twice: %+v", labels)
	}

	// The saved state carries the label, and so does the history.
	saved := decodeBody[apiv0.SavedStateResponse](t, serve(router, http.MethodGet, mainPath+"/saved-states/1", nil))
	if len(saved.Labels) != 1 || saved.Labels[0].ID != label.ID {
		t.Errorf("the saved state's labels: %+v", saved.Labels)
	}
	if savedStates := decodeBody[apiv0.GetSavedStatesResponse](t, serve(router, http.MethodGet, mainPath+"/saved-states", nil)); len(savedStates.SavedStates) != 1 || len(savedStates.SavedStates[0].Labels) != 1 {
		t.Errorf("the history's chips: %+v", savedStates)
	}

	// ?label= filters the history by one label, in the query.
	filtered := decodeBody[apiv0.GetSavedStatesResponse](t,
		serve(router, http.MethodGet, mainPath+"/saved-states?"+apiv0.SavedStatesQueryLabel+"="+label.ID.String(), nil))
	if len(filtered.SavedStates) != 1 || filtered.SavedStates[0].Number != 1 {
		t.Errorf("the history of a label: %+v", filtered)
	}
	empty := decodeBody[apiv0.GetSavedStatesResponse](t,
		serve(router, http.MethodGet, mainPath+"/saved-states?"+apiv0.SavedStatesQueryLabel+"="+other.ID.String(), nil))
	if len(empty.SavedStates) != 0 {
		t.Errorf("a label no saved state carries lists %+v", empty.SavedStates)
	}
	if rec := serve(router, http.MethodGet, mainPath+"/saved-states?"+apiv0.SavedStatesQueryLabel+"=not-a-uuid", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("a label that is not a UUID: %d, want 400", rec.Code)
	}
	// The zero UUID names no label; it is not the filter's "every state".
	if rec := serve(router, http.MethodGet, mainPath+"/saved-states?"+apiv0.SavedStatesQueryLabel+"="+uuid.Nil.String(), nil); rec.Code != http.StatusBadRequest {
		t.Errorf("the zero UUID as a label: %d, want 400", rec.Code)
	}

	// The label's assigned count follows its marks.
	labels := decodeBody[apiv0.GetLabelsResponse](t, serve(router, http.MethodGet, apiv0.PathPrefix+apiv0.LabelPath, nil))
	for _, l := range labels.Labels {
		if l.ID == label.ID && l.AssignedCount != 1 {
			t.Errorf("the label's assigned count: %+v, want 1", l)
		}
		if l.ID == other.ID && l.AssignedCount != 0 {
			t.Errorf("an unassigned label's count: %+v, want 0", l)
		}
	}

	// DELETE takes it off, and a repeat is a 200 with no label, never a 409.
	rec = serve(router, http.MethodDelete, labelPath, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE a label off a saved state: %d %s", rec.Code, rec.Body)
	}
	if left := decodeBody[apiv0.SavedStateLabelsResponse](t, rec); len(left.Labels) != 0 {
		t.Errorf("the labels after the DELETE: %+v", left)
	}
	if rec := serve(router, http.MethodDelete, labelPath, nil); rec.Code != http.StatusOK {
		t.Errorf("a repeated DELETE: %d, want 200", rec.Code)
	}

	// A saved state or a label this server lacks is 404.
	if rec := serve(router, http.MethodPut, mainPath+"/saved-states/9/labels/"+label.ID.String(), nil); rec.Code != http.StatusNotFound {
		t.Errorf("a label on a missing saved state: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodPut, mainPath+"/saved-states/1/labels/"+uuid.NewString(), nil); rec.Code != http.StatusNotFound {
		t.Errorf("a missing label: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodPut, mainPath+"/saved-states/1/labels/not-a-uuid", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("a label ID that is not a UUID: %d, want 400", rec.Code)
	}
	if rec := serve(router, http.MethodPut, mainPath+"/saved-states/0/labels/"+label.ID.String(), nil); rec.Code != http.StatusBadRequest {
		t.Errorf("saved state 0: %d, want 400", rec.Code)
	}
}

// TestSavedStateLabels_LibraryScope covers the same routes under a Library: its
// saved states carry the Organization's labels too (task 85_3; ADR 0025).
func TestSavedStateLabels_LibraryScope(t *testing.T) {
	cleanTestLabels(t)
	services := newServices()
	router := services.router()
	ctx := context.Background()

	created, err := services.libraries.CreateLibrary(ctx, appdto.LibraryInput{Name: "saved-state-labels-" + uuid.NewString()[:8]})
	if err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}
	path := apiv0.PathPrefix + apiv0.LibraryPath(created.ID.String())
	label, err := services.labels.Create(ctx, "bruno-test-"+uuid.NewString()[:8], "")
	if err != nil {
		t.Fatalf("Create label: %v", err)
	}
	// A saved state another Draft holds is not this Library's: its number names
	// no saved state of it (task 85_3).
	if rec := serve(router, http.MethodPut, path+"/saved-states/1/labels/"+label.ID.String(), nil); rec.Code != http.StatusNotFound {
		t.Errorf("a label on a Library without saved state 1: %d, want 404", rec.Code)
	}
	if _, err := services.widgetTypes.CreateWidgetType(ctx, created.ID, 1, appdto.WidgetTypeInput{
		Name: "gauge", HtmlTemplate: "<div></div>", Script: "function update(){}",
		ScriptLanguage: "javascript", DefaultWidth: 100, DefaultHeight: 100,
	}); err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	if rec := serve(router, http.MethodPost, path+"/saved-states", apiv0.CreateSavedStateRequest{
		Version: 2, Title: "Widgets", Author: "Ada",
	}); rec.Code != http.StatusCreated {
		t.Fatalf("POST library saved state: %d %s", rec.Code, rec.Body)
	}
	labelPath := path + "/saved-states/1/labels/" + label.ID.String()
	if rec := serve(router, http.MethodPut, labelPath, nil); rec.Code != http.StatusOK {
		t.Fatalf("PUT a label on a library's saved state: %d %s", rec.Code, rec.Body)
	}
	savedStates := decodeBody[apiv0.GetSavedStatesResponse](t,
		serve(router, http.MethodGet, path+"/saved-states?"+apiv0.SavedStatesQueryLabel+"="+label.ID.String(), nil))
	if len(savedStates.SavedStates) != 1 || savedStates.SavedStates[0].Number != 1 || len(savedStates.SavedStates[0].Labels) != 1 {
		t.Errorf("the library's history of a label: %+v", savedStates)
	}
	if rec := serve(router, http.MethodDelete, labelPath, nil); rec.Code != http.StatusOK {
		t.Errorf("DELETE a label off a library's saved state: %d, want 200", rec.Code)
	}
}

// TestSavedStateLabels_EventsOnTheServerStream: an assignment is the server's
// event, naming the Project in ID, the label in label and the saved state's
// number in saved_state, so History of every mode and the Labels window see it
// (task 85_3; ADR 0025).
func TestSavedStateLabels_EventsOnTheServerStream(t *testing.T) {
	cleanProjectsRest(t)
	cleanTestLabels(t)
	if _, err := testDB().ExecContext(context.Background(),
		"DELETE FROM engineering.drafts; DELETE FROM outbox"); err != nil {
		t.Fatalf("clean drafts and outbox: %v", err)
	}
	if rec := serve(newServicesPublishingTo(func(event.Event) {}).router(), http.MethodPost, mainPath+"/saved-states",
		apiv0.CreateSavedStateRequest{Version: 1, Title: "First save", Author: "Ada"}); rec.Code != http.StatusCreated {
		t.Fatalf("POST saved state: %d %s", rec.Code, rec.Body)
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

	label, err := services.labels.Create(ctx, "bruno-test-"+uuid.NewString()[:8], "")
	if err != nil {
		t.Fatalf("Create label: %v", err)
	}
	if _, err := services.labels.Assign(ctx, draft.ProjectScope(id.NewID(id.IDWithUUID[project.Project](mainProject))), 1, label.ID); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool { return strings.Contains(rec.body(), "label_assigned") })
	cancel()
	<-done

	want := map[string]any{
		"type":        "label_assigned",
		"id":          mainProject.String(),
		"saved_state": float64(1),
		"label":       label.ID.String(),
	}
	found := false
	for _, m := range sseEventMessages(t, rec.body()) {
		if m["type"] != "label_assigned" {
			continue
		}
		found = true
		for key, value := range want {
			if m[key] != value {
				t.Errorf("label_assigned %s = %v, want %v", key, m[key], value)
			}
		}
	}
	if !found {
		t.Fatalf("the server's stream carried no label_assigned: %q", rec.body())
	}
}
