package restapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
	"github.com/kipitix/growscada/internal/server/interface/restapi"
)

// cleanProjectsRest removes every project but mainProject, with the content of
// all of them.
func cleanProjectsRest(t *testing.T) {
	t.Helper()
	if _, err := testDB().ExecContext(context.Background(),
		"DELETE FROM runtime.tags; DELETE FROM engineering.tags; DELETE FROM engineering.scenes; DELETE FROM projects WHERE id <> '"+mainProject.String()+"'; "+
			"UPDATE projects SET name = 'Main', version = 1, deleted_at = NULL"); err != nil {
		t.Fatalf("cleanProjectsRest: %v", err)
	}
}

// serve sends a request with the JSON body (nil: none) to the router.
func serve(router *restapi.APIRouter, method, path string, body any) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)
	return rec
}

func decodeBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decode %T: %v (status %d)", v, err, rec.Code)
	}
	return v
}

func TestProjects_CreateListRenameDelete(t *testing.T) {
	cleanProjectsRest(t)
	router := newServices().router()

	rec := serve(router, http.MethodPost, apiv0.PathPrefix+"/projects", apiv0.CreateProjectRequest{Name: "Line 1"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /projects: %d %s", rec.Code, rec.Body)
	}
	created := decodeBody[apiv0.CreateProjectResponse](t, rec)
	path := apiv0.PathPrefix + apiv0.ProjectPath(created.ID.String())

	if rec := serve(router, http.MethodPost, apiv0.PathPrefix+"/projects", apiv0.CreateProjectRequest{Name: "Line 1"}); rec.Code != http.StatusConflict {
		t.Errorf("a taken name: %d, want 409", rec.Code)
	}

	rec = serve(router, http.MethodGet, apiv0.PathPrefix+"/projects", nil)
	list := decodeBody[apiv0.GetProjectsResponse](t, rec)
	if len(list.Projects) != 2 || list.Projects[0].ID != mainProject || list.Projects[1].Name != "Line 1" {
		t.Errorf("GET /projects: %+v", list)
	}
	rec = serve(router, http.MethodGet, apiv0.PathPrefix+"/projects?name=Line%201", nil)
	if byName := decodeBody[apiv0.GetProjectsResponse](t, rec); len(byName.Projects) != 1 || byName.Projects[0].ID != created.ID {
		t.Errorf("GET /projects?name=: %+v", byName)
	}
	rec = serve(router, http.MethodGet, apiv0.PathPrefix+"/projects?name=missing", nil)
	if none := decodeBody[apiv0.GetProjectsResponse](t, rec); rec.Code != http.StatusOK || len(none.Projects) != 0 {
		t.Errorf("GET /projects?name=missing: %d %+v", rec.Code, none)
	}

	rec = serve(router, http.MethodPut, path, apiv0.UpdateProjectRequest{Name: "Line 2", Version: created.Version})
	if rec.Code != http.StatusOK || decodeBody[apiv0.UpdateProjectResponse](t, rec).Version != created.Version+1 {
		t.Errorf("PUT: %d", rec.Code)
	}
	if rec := serve(router, http.MethodPut, path, apiv0.UpdateProjectRequest{Name: "Line 3", Version: created.Version}); rec.Code != http.StatusConflict {
		t.Errorf("a stale PUT: %d, want 409", rec.Code)
	}
	rec = serve(router, http.MethodGet, path, nil)
	if got := decodeBody[apiv0.ProjectResponse](t, rec); got.Name != "Line 2" {
		t.Errorf("GET: %+v", got)
	}

	rec = serve(router, http.MethodDelete, path, nil)
	if rec.Code != http.StatusOK || decodeBody[apiv0.ProjectResponse](t, rec).ID != created.ID {
		t.Errorf("DELETE: %d", rec.Code)
	}
	if rec := serve(router, http.MethodGet, path, nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET of a deleted project: %d, want 404", rec.Code)
	}
	rec = serve(router, http.MethodGet, path+"/tags", nil)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "Project with ID") {
		t.Errorf("the tags of a deleted project: %d %s, want a 404 of the project", rec.Code, rec.Body)
	}
}

func TestProjectContent_UnknownProject_Is404OfTheProject(t *testing.T) {
	cleanProjectsRest(t)
	router := newServices().router()
	unknown := apiv0.PathPrefix + apiv0.ProjectPath(uuid.NewString())

	for _, req := range []struct{ method, path string }{
		{http.MethodGet, unknown + "/tags"},
		{http.MethodGet, unknown + "/tags/" + uuid.NewString()},
		{http.MethodGet, unknown + "/scenes"},
		{http.MethodGet, unknown + "/scenes/" + uuid.NewString() + "/widgets"},
	} {
		rec := serve(router, req.method, req.path, nil)
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "Project with ID") {
			t.Errorf("%s %s: %d %s, want a 404 of the project", req.method, req.path, rec.Code, rec.Body)
		}
	}
	if rec := serve(router, http.MethodGet, apiv0.PathPrefix+"/projects/not-a-uuid/tags", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("a project ID that is no UUID: %d, want 400", rec.Code)
	}
}

func TestTags_SameNameInTwoProjects(t *testing.T) {
	cleanProjectsRest(t)
	router := newServices().router()
	rec := serve(router, http.MethodPost, apiv0.PathPrefix+"/projects", apiv0.CreateProjectRequest{Name: "Line 1"})
	other := decodeBody[apiv0.CreateProjectResponse](t, rec)
	otherPath := apiv0.PathPrefix + apiv0.ProjectPath(other.ID.String())
	tagReq := apiv0.CreateTagRequest{Name: "speed", Type: "integer"}

	if rec := serve(router, http.MethodPost, mainPath+"/tags", tagReq); rec.Code != http.StatusCreated {
		t.Fatalf("POST tag in Main: %d %s", rec.Code, rec.Body)
	}
	rec = serve(router, http.MethodPost, otherPath+"/tags", tagReq)
	if rec.Code != http.StatusCreated {
		t.Fatalf("the same name in another project: %d %s", rec.Code, rec.Body)
	}
	inOther := decodeBody[apiv0.CreateTagResponse](t, rec)

	if rec := serve(router, http.MethodGet, mainPath+"/tags/"+inOther.ID.String(), nil); rec.Code != http.StatusNotFound {
		t.Errorf("another project's tag through Main: %d, want 404", rec.Code)
	}
	rec = serve(router, http.MethodGet, otherPath+"/tags?name=speed", nil)
	if list := decodeBody[apiv0.GetTagsResponse](t, rec); len(list.Tags) != 1 || list.Tags[0].ID != inOther.ID {
		t.Errorf("GET ?name= in the other project: %+v", list)
	}
}

// ── Event streams ────────────────────────────────────────────────────────────

func TestEventStreams_RouteEventsByProject(t *testing.T) {
	bus := eventbus.NewEventBus()
	h := restapi.NewEventsHandler(bus, anyProjects{}, 100)
	other := uuid.New()
	otherID := id.NewID(id.IDWithUUID[project.Project](other))

	server := newSyncRecorder()
	cancelServer, doneServer := runEventsHandler(t, h, server)
	ofMain := newSyncRecorder()
	cancelMain, doneMain := runProjectEventsHandler(t, h, ofMain, mainProject)
	waitFor(t, time.Second, func() bool {
		return strings.Contains(server.body(), ": connected") && strings.Contains(ofMain.body(), ": connected")
	})

	otherTag, mainTag := id.NewID[tag.Tag](), id.NewID[tag.Tag]()
	bus.Publish(tag.NewTagCreatedEvent(otherTag, otherID))
	bus.Publish(tag.NewTagCreatedEvent(mainTag, mainProjectID))
	bus.Publish(project.NewProjectCreatedEvent(otherID))
	bus.Publish(library.NewWidgetTypeCreatedEvent(id.NewID[library.WidgetType]()))

	waitFor(t, time.Second, func() bool {
		return strings.Contains(server.body(), "widget_type_created") && strings.Contains(ofMain.body(), "tag_created")
	})
	cancelServer()
	cancelMain()
	<-doneServer
	<-doneMain

	typesOf := func(body string) []string {
		var types []string
		for _, m := range sseEventMessages(t, body) {
			if typ := m["type"].(string); !strings.HasPrefix(typ, "client_") {
				types = append(types, typ+" "+m["id"].(string))
			}
		}
		return types
	}
	if got := typesOf(server.body()); len(got) != 2 || !strings.HasPrefix(got[0], "project_created "+other.String()) || !strings.HasPrefix(got[1], "widget_type_created") {
		t.Errorf("the server's stream: %v, want project_created and widget_type_created only", got)
	}
	if got := typesOf(ofMain.body()); len(got) != 1 || got[0] != "tag_created "+mainTag.String() {
		t.Errorf("Main's stream: %v, want Main's tag_created only", got)
	}
	if strings.Contains(ofMain.body(), "client_connected") {
		t.Error("a project's stream carries the connection events of the server's")
	}
}

func TestGetEvents_WithProject_CarriesServerAndProjectEvents(t *testing.T) {
	bus := eventbus.NewEventBus()
	h := restapi.NewEventsHandler(bus, anyProjects{}, 100)
	other := uuid.New()
	otherID := id.NewID(id.IDWithUUID[project.Project](other))

	rec := newSyncRecorder()
	cancel, done := runEventsOfProjectHandler(t, h, rec, mainProject.String())
	waitFor(t, time.Second, func() bool { return strings.Contains(rec.body(), ": connected") })

	mainTag := id.NewID[tag.Tag]()
	bus.Publish(tag.NewTagCreatedEvent(id.NewID[tag.Tag](), otherID))
	bus.Publish(tag.NewTagCreatedEvent(mainTag, mainProjectID))
	bus.Publish(project.NewProjectCreatedEvent(otherID))

	waitFor(t, time.Second, func() bool { return strings.Contains(rec.body(), "project_created") })
	cancel()
	<-done

	var got []string
	for _, m := range sseEventMessages(t, rec.body()) {
		if typ := m["type"].(string); !strings.HasPrefix(typ, "client_") {
			got = append(got, typ+" "+m["id"].(string))
		}
	}
	if len(got) != 2 || got[0] != "tag_created "+mainTag.String() || got[1] != "project_created "+other.String() {
		t.Errorf("the stream with ?project=Main: %v, want Main's tag_created and project_created only", got)
	}
}

func TestGetEvents_WithUnknownProject_CarriesServerEventsOnly(t *testing.T) {
	cleanProjectsRest(t)
	services := newServices()
	bus := eventbus.NewEventBus()
	h := restapi.NewEventsHandler(bus, services.projects, 100)
	gone := uuid.New()
	goneID := id.NewID(id.IDWithUUID[project.Project](gone))

	rec := newSyncRecorder()
	cancel, done := runEventsOfProjectHandler(t, h, rec, gone.String())
	waitFor(t, time.Second, func() bool { return strings.Contains(rec.body(), ": connected") })

	bus.Publish(tag.NewTagCreatedEvent(id.NewID[tag.Tag](), goneID))
	bus.Publish(project.NewProjectDeletedEvent(goneID))

	waitFor(t, time.Second, func() bool { return strings.Contains(rec.body(), "project_deleted") })
	cancel()
	<-done

	if strings.Contains(rec.body(), "tag_created") {
		t.Errorf("the stream of a project not found carries its content: %q", rec.body())
	}
}

func TestGetEvents_InvalidProject_Is400(t *testing.T) {
	h := restapi.NewEventsHandler(eventbus.NewEventBus(), anyProjects{}, 100)

	rec := newSyncRecorder()
	cancel, done := runEventsOfProjectHandler(t, h, rec, "not-a-uuid")
	defer cancel()
	<-done

	if rec.code() != http.StatusBadRequest {
		t.Errorf("expected 400, got %d %q", rec.code(), rec.body())
	}
}

func TestGetProjectEvents_UnknownProject_Is404(t *testing.T) {
	cleanProjectsRest(t)
	services := newServices()
	h := restapi.NewEventsHandler(eventbus.NewEventBus(), services.projects, 100)

	rec := newSyncRecorder()
	cancel, done := runProjectEventsHandler(t, h, rec, uuid.New())
	defer cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("GetProjectEvents of an unknown project did not return")
	}

	if rec.code() != http.StatusNotFound || !strings.Contains(rec.body(), "Project with ID") {
		t.Errorf("expected a 404 of the project, got %d %q", rec.code(), rec.body())
	}
}
