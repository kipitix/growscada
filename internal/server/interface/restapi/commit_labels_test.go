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

// TestCommitLabels_AssignFilterAndChips covers the label assignment routes of a
// Project's saved state (task 85_3; ADR 0025): a label is placed and taken off
// idempotently, the history lists the chips, ?label= filters it, the label's
// assigned count follows, and a Commit or a label this server lacks is 404.
func TestCommitLabels_AssignFilterAndChips(t *testing.T) {
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
	if rec := serve(router, http.MethodPost, mainPath+"/commits", apiv0.CreateCommitRequest{
		Version: 1, Title: "First save", Author: "Ada",
	}); rec.Code != http.StatusCreated {
		t.Fatalf("POST commit: %d %s", rec.Code, rec.Body)
	}

	labelPath := mainPath + "/commits/1/labels/" + label.ID.String()
	rec := serve(router, http.MethodPut, labelPath, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT a label on a saved state: %d %s", rec.Code, rec.Body)
	}
	assigned := decodeBody[apiv0.CommitLabelsResponse](t, rec)
	if len(assigned.Labels) != 1 || assigned.Labels[0].ID != label.ID || assigned.Labels[0].Name != label.Name {
		t.Errorf("the labels after the assignment: %+v", assigned)
	}
	// A repeat is neither a conflict nor a second mark.
	if rec := serve(router, http.MethodPut, labelPath, nil); rec.Code != http.StatusOK {
		t.Errorf("a repeated PUT: %d, want 200", rec.Code)
	}
	if labels := decodeBody[apiv0.CommitLabelsResponse](t, serve(router, http.MethodPut, labelPath, nil)); len(labels.Labels) != 1 {
		t.Errorf("a repeated PUT marked the saved state twice: %+v", labels)
	}

	// The saved state carries the label, and so does the history.
	commit := decodeBody[apiv0.CommitResponse](t, serve(router, http.MethodGet, mainPath+"/commits/1", nil))
	if len(commit.Labels) != 1 || commit.Labels[0].ID != label.ID {
		t.Errorf("the commit's labels: %+v", commit.Labels)
	}
	if commits := decodeBody[apiv0.GetCommitsResponse](t, serve(router, http.MethodGet, mainPath+"/commits", nil)); len(commits.Commits) != 1 || len(commits.Commits[0].Labels) != 1 {
		t.Errorf("the history's chips: %+v", commits)
	}

	// ?label= filters the history by one label, in the query.
	filtered := decodeBody[apiv0.GetCommitsResponse](t,
		serve(router, http.MethodGet, mainPath+"/commits?"+apiv0.CommitsQueryLabel+"="+label.ID.String(), nil))
	if len(filtered.Commits) != 1 || filtered.Commits[0].Number != 1 {
		t.Errorf("the history of a label: %+v", filtered)
	}
	empty := decodeBody[apiv0.GetCommitsResponse](t,
		serve(router, http.MethodGet, mainPath+"/commits?"+apiv0.CommitsQueryLabel+"="+other.ID.String(), nil))
	if len(empty.Commits) != 0 {
		t.Errorf("a label no saved state carries lists %+v", empty.Commits)
	}
	if rec := serve(router, http.MethodGet, mainPath+"/commits?"+apiv0.CommitsQueryLabel+"=not-a-uuid", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("a label that is not a UUID: %d, want 400", rec.Code)
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
	if left := decodeBody[apiv0.CommitLabelsResponse](t, rec); len(left.Labels) != 0 {
		t.Errorf("the labels after the DELETE: %+v", left)
	}
	if rec := serve(router, http.MethodDelete, labelPath, nil); rec.Code != http.StatusOK {
		t.Errorf("a repeated DELETE: %d, want 200", rec.Code)
	}

	// A Commit or a label this server lacks is 404.
	if rec := serve(router, http.MethodPut, mainPath+"/commits/9/labels/"+label.ID.String(), nil); rec.Code != http.StatusNotFound {
		t.Errorf("a label on a missing saved state: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodPut, mainPath+"/commits/1/labels/"+uuid.NewString(), nil); rec.Code != http.StatusNotFound {
		t.Errorf("a missing label: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodPut, mainPath+"/commits/1/labels/not-a-uuid", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("a label ID that is not a UUID: %d, want 400", rec.Code)
	}
	if rec := serve(router, http.MethodPut, mainPath+"/commits/0/labels/"+label.ID.String(), nil); rec.Code != http.StatusBadRequest {
		t.Errorf("saved state 0: %d, want 400", rec.Code)
	}
}

// TestCommitLabels_LibraryScope covers the same routes under a Library: its
// saved states carry the Organization's labels too (task 85_3; ADR 0025).
func TestCommitLabels_LibraryScope(t *testing.T) {
	cleanTestLabels(t)
	services := newServices()
	router := services.router()
	ctx := context.Background()

	created, err := services.libraries.CreateLibrary(ctx, appdto.LibraryInput{Name: "commit-labels-" + uuid.NewString()[:8]})
	if err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}
	path := apiv0.PathPrefix + apiv0.LibraryPath(created.ID.String())
	label, err := services.labels.Create(ctx, "bruno-test-"+uuid.NewString()[:8], "")
	if err != nil {
		t.Fatalf("Create label: %v", err)
	}
	// A saved state another Draft holds is not this Library's: its number names
	// no Commit of it (task 85_3).
	if rec := serve(router, http.MethodPut, path+"/commits/1/labels/"+label.ID.String(), nil); rec.Code != http.StatusNotFound {
		t.Errorf("a label on a Library without Commit 1: %d, want 404", rec.Code)
	}
	if _, err := services.widgetTypes.CreateWidgetType(ctx, created.ID, 1, appdto.WidgetTypeInput{
		Name: "gauge", HtmlTemplate: "<div></div>", Script: "function update(){}",
		ScriptLanguage: "javascript", DefaultWidth: 100, DefaultHeight: 100,
	}); err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	if rec := serve(router, http.MethodPost, path+"/commits", apiv0.CreateCommitRequest{
		Version: 2, Title: "Widgets", Author: "Ada",
	}); rec.Code != http.StatusCreated {
		t.Fatalf("POST library commit: %d %s", rec.Code, rec.Body)
	}
	labelPath := path + "/commits/1/labels/" + label.ID.String()
	if rec := serve(router, http.MethodPut, labelPath, nil); rec.Code != http.StatusOK {
		t.Fatalf("PUT a label on a library's saved state: %d %s", rec.Code, rec.Body)
	}
	commits := decodeBody[apiv0.GetCommitsResponse](t,
		serve(router, http.MethodGet, path+"/commits?"+apiv0.CommitsQueryLabel+"="+label.ID.String(), nil))
	if len(commits.Commits) != 1 || commits.Commits[0].Number != 1 || len(commits.Commits[0].Labels) != 1 {
		t.Errorf("the library's history of a label: %+v", commits)
	}
	if rec := serve(router, http.MethodDelete, labelPath, nil); rec.Code != http.StatusOK {
		t.Errorf("DELETE a label off a library's saved state: %d, want 200", rec.Code)
	}
}

// TestCommitLabels_EventsOnTheServerStream: an assignment is the server's
// event, naming the Project in ID, the label in label and the Commit's number
// in commit, so History of every mode and the Labels window see it (task 85_3;
// ADR 0025).
func TestCommitLabels_EventsOnTheServerStream(t *testing.T) {
	cleanProjectsRest(t)
	cleanTestLabels(t)
	if _, err := testDB().ExecContext(context.Background(),
		"DELETE FROM engineering.drafts; DELETE FROM outbox"); err != nil {
		t.Fatalf("clean drafts and outbox: %v", err)
	}
	if rec := serve(newServicesPublishingTo(func(event.Event) {}).router(), http.MethodPost, mainPath+"/commits",
		apiv0.CreateCommitRequest{Version: 1, Title: "First save", Author: "Ada"}); rec.Code != http.StatusCreated {
		t.Fatalf("POST commit: %d %s", rec.Code, rec.Body)
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
		"type":   "label_assigned",
		"id":     mainProject.String(),
		"commit": float64(1),
		"label":  label.ID.String(),
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
