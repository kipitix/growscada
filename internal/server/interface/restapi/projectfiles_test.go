package restapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
	"github.com/kipitix/growscada/internal/server/interface/restapi"
)

// serveRaw is serve for a body that is already the document, not a value to
// marshal.
func serveRaw(router servingRouter, method, path string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)
	return rec
}

func decodeFile(t *testing.T, rec *httptest.ResponseRecorder) projectv0.ProjectFile {
	t.Helper()
	var file projectv0.ProjectFile
	if err := json.NewDecoder(rec.Body).Decode(&file); err != nil {
		t.Fatalf("decode the project file: %v (status %d)", err, rec.Code)
	}
	return file
}

// TestProjectFiles_DownloadDraftAndDeployment: the Draft and a Deployment are
// attachments named after the Project, the document is the ProjectFile as it
// is, and the schema version stays a header (ADR 0018).
func TestProjectFiles_DownloadDraftAndDeployment(t *testing.T) {
	cleanProjectsRest(t)
	services := newServices()
	router := services.router()

	rec := serve(router, http.MethodGet, mainPath+"/projectfile", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET projectfile: %d %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type %q, want application/json", got)
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="Main-draft.json"` {
		t.Errorf("Content-Disposition %q", got)
	}
	if got := rec.Header().Get("GrowSCADA-Schema-Version"); got == "" {
		t.Error("the response states no schema version")
	}
	file := decodeFile(t, rec)
	if file.ProjectID != mainProject || !file.SchemaVersion.SameMajor(projectv0.SchemaVersion()) {
		t.Errorf("the draft file: project %s, schema %s", file.ProjectID, file.SchemaVersion)
	}

	if rec := serve(router, http.MethodPost, mainPath+"/deploy", nil); rec.Code != http.StatusCreated {
		t.Fatalf("POST deploy: %d %s", rec.Code, rec.Body)
	}
	rec = serve(router, http.MethodGet, mainPath+"/deployments/1/projectfile", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET deployment 1 projectfile: %d %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="Main-deployment-1.json"` {
		t.Errorf("Content-Disposition %q", got)
	}
	if deploymentFile := decodeFile(t, rec); deploymentFile.ProjectID != mainProject {
		t.Errorf("the deployment file names project %s", deploymentFile.ProjectID)
	}

	if rec := serve(router, http.MethodGet, mainPath+"/deployments/99/projectfile", nil); rec.Code != http.StatusNotFound {
		t.Errorf("a missing deployment's file: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodGet, mainPath+"/deployments/abc/projectfile", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("a deployment number that is not a number: %d, want 400", rec.Code)
	}
}

// TestProjectFiles_OpenReplacesTheWholeDraft: a downloaded Draft opened after
// a deletion restores it in one change, and the same file again is nothing to
// open (ADR 0018).
func TestProjectFiles_OpenReplacesTheWholeDraft(t *testing.T) {
	cleanProjectsRest(t)
	services := newServices()
	router := services.router()

	rec := serve(router, http.MethodPost, mainPath+"/tags", apiv0.CreateTagRequest{Name: "speed", Type: "integer"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST tag: %d %s", rec.Code, rec.Body)
	}
	created := decodeBody[apiv0.CreateTagResponse](t, rec)

	rec = serve(router, http.MethodGet, mainPath+"/projectfile", nil)
	downloaded := decodeFile(t, rec)

	version := services.draftVersion(t)
	if rec := serve(router, http.MethodDelete, mainPath+"/tags/"+created.ID.String()+"?version="+strconv.Itoa(version), nil); rec.Code != http.StatusOK {
		t.Fatalf("DELETE tag: %d %s", rec.Code, rec.Body)
	}
	version = services.draftVersion(t)

	rec = serve(router, http.MethodPost, mainPath+"/projectfile?version="+strconv.Itoa(version), downloaded)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST projectfile: %d %s", rec.Code, rec.Body)
	}
	opened := decodeBody[apiv0.DraftStateResponse](t, rec)
	if opened.Version == 0 {
		t.Errorf("the open states no draft version: %+v", opened)
	}
	rec = serve(router, http.MethodGet, mainPath+"/tags", nil)
	tags := decodeBody[apiv0.GetTagsResponse](t, rec)
	if !containsTag(tags.Tags, created.ID) {
		t.Errorf("the opened file did not restore the tag: %+v", tags.Tags)
	}

	// The same file is the Draft already.
	rec = serve(router, http.MethodPost, mainPath+"/projectfile?version="+strconv.Itoa(opened.Version), downloaded)
	if rec.Code != http.StatusConflict {
		t.Fatalf("opening the same file: %d %s", rec.Code, rec.Body)
	}
	problem := decodeBody[apiv0.Problem](t, rec)
	if !strings.Contains(problem.Detail, "Nothing to open") {
		t.Errorf("the refusal is %q", problem.Detail)
	}
}

func containsTag(tags []apiv0.TagResponse, id uuid.UUID) bool {
	for _, tag := range tags {
		if tag.ID == id {
			return true
		}
	}
	return false
}

// TestProjectFiles_OpenRefusals: a document of another Project, an unknown
// field and a document the validator refuses are 400 with file_issues, all of
// them listed.
func TestProjectFiles_OpenRefusals(t *testing.T) {
	cleanProjectsRest(t)
	services := newServices()
	router := services.router()
	version := services.draftVersion(t)

	foreign := projectv0.ProjectFile{Header: projectv0.Header{
		SchemaVersion: projectv0.SchemaVersion(), ProjectID: uuid.New(),
	}}
	rec := serve(router, http.MethodPost, mainPath+"/projectfile?version="+strconv.Itoa(version), foreign)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a file of another project: %d %s", rec.Code, rec.Body)
	}
	problem := decodeBody[apiv0.Problem](t, rec)
	if len(problem.FileIssues) == 0 || problem.FileIssues[0].Path != "project_id" {
		t.Errorf("the refusal's file_issues: %+v", problem.FileIssues)
	}

	unknown := []byte(`{"schema_version":"` + projectv0.SchemaVersion().String() + `","project_id":"` +
		mainProject.String() + `","surprise":1}`)
	rec = serveRaw(router, http.MethodPost, mainPath+"/projectfile?version="+strconv.Itoa(version), unknown)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a file with an unknown field: %d %s", rec.Code, rec.Body)
	}
	if problem := decodeBody[apiv0.Problem](t, rec); len(problem.FileIssues) == 0 {
		t.Errorf("the unknown field is not listed: %+v", problem)
	}

	if rec := serveRaw(router, http.MethodPost, mainPath+"/projectfile", unknown); rec.Code != http.StatusBadRequest {
		t.Errorf("a POST without a version: %d, want 400", rec.Code)
	}
}

// TestProjectFiles_DeployFromFileAndDiscard: a file is a Deployment of its own
// with cause file, the Draft is untouched, and a Discard of a foreign Deployment
// refuses instead of writing a pin this server cannot hold (ADR 0018).
func TestProjectFiles_DeployFromFileAndDiscard(t *testing.T) {
	cleanProjectsRest(t)
	services := newServices()
	router := services.router()

	if rec := serve(router, http.MethodPost, mainPath+"/tags", apiv0.CreateTagRequest{Name: "speed", Type: "integer"}); rec.Code != http.StatusCreated {
		t.Fatalf("POST tag: %d %s", rec.Code, rec.Body)
	}
	before := services.draftVersion(t)
	rec := serve(router, http.MethodGet, mainPath+"/projectfile", nil)
	file := decodeFile(t, rec)
	// A pin to a Library this server does not have: a file from another
	// server, whose document is still self-contained.
	file.LibraryPins = append(file.LibraryPins, projectv0.LibraryPin{LibraryID: uuid.New(), Release: 1})

	rec = serve(router, http.MethodPost, mainPath+"/projectfile/deploy", file)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST projectfile/deploy: %d %s", rec.Code, rec.Body)
	}
	deployed := decodeBody[apiv0.DeployResponse](t, rec)
	if deployed.Number != 1 {
		t.Errorf("the file became deployment %d, want 1", deployed.Number)
	}
	if got := services.draftVersion(t); got != before {
		t.Errorf("the file deploy changed the Draft: version %d, want %d", got, before)
	}

	rec = serve(router, http.MethodGet, mainPath+"/deployments", nil)
	list := decodeBody[apiv0.ListDeploymentsResponse](t, rec)
	if len(list.Deployments) != 1 || list.Deployments[0].Cause != apiv0.DeploymentCauseFile || list.Deployments[0].RolledBackFrom != nil {
		t.Errorf("the history after a file deploy: %+v", list.Deployments)
	}

	// The same document is the running Deployment already.
	if rec := serve(router, http.MethodPost, mainPath+"/projectfile/deploy", file); rec.Code != http.StatusConflict {
		t.Fatalf("deploying the running file: %d %s", rec.Code, rec.Body)
	}

	rec = serve(router, http.MethodPost, mainPath+"/discard", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("discarding a foreign deployment: %d %s", rec.Code, rec.Body)
	}
	problem := decodeBody[apiv0.Problem](t, rec)
	if len(problem.FileIssues) == 0 || !strings.HasPrefix(problem.FileIssues[0].Path, "library_pins[") {
		t.Errorf("the discard's file_issues: %+v", problem.FileIssues)
	}
	if got := services.draftVersion(t); got != before {
		t.Errorf("the refused discard changed the Draft: version %d, want %d", got, before)
	}
}

// failingProjectFiles refuses every Draft export, to exercise the route's
// answer to an inconsistent Draft without building one.
type failingProjectFiles struct {
	application.ProjectFileService
}

func (f failingProjectFiles) Draft(context.Context, uuid.UUID) (application.ProjectFileDownload, error) {
	return application.ProjectFileDownload{}, application.DraftNotDeployableError{
		Reason: "widget type 00000000-0000-4000-8000-000000000001, which no pinned release holds",
	}
}

// TestProjectFiles_DraftNotDeployable: an inconsistent Draft is 409 with the
// builder's reason in the detail, not a file (ADR 0018).
func TestProjectFiles_DraftNotDeployable(t *testing.T) {
	cleanProjectsRest(t)
	services := newServices()
	svcs := services.services()
	svcs.ProjectFiles = failingProjectFiles{svcs.ProjectFiles}
	router := testRouter{APIRouter: restapi.NewRouter(svcs, eventbus.NewEventBus(), 100)}

	rec := serve(router, http.MethodGet, mainPath+"/projectfile", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("an inconsistent Draft: %d %s", rec.Code, rec.Body)
	}
	problem := decodeBody[apiv0.Problem](t, rec)
	if !strings.Contains(problem.Detail, "no pinned release holds") {
		t.Errorf("the detail does not name the reason: %q", problem.Detail)
	}
}
