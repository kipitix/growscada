package restapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/migrations"
	"github.com/kipitix/growscada/internal/server/interface/restapi"
)

// The Postgres container starts on the first testDB call, so the tests that
// need no database (the Bruno collection check among them) run without Docker.
var (
	testDBOnce  sync.Once
	testDBConn  *sql.DB
	pgContainer *tcpostgres.PostgresContainer
)

func TestMain(m *testing.M) {
	code := m.Run()
	stopTestDB()
	os.Exit(code)
}

// stopTestDB closes the connection and terminates the container, whichever
// of them has been started.
func stopTestDB() {
	if testDBConn != nil {
		testDBConn.Close()
	}
	if pgContainer != nil {
		pgContainer.Terminate(context.Background())
	}
}

// failTestDB stops what startTestDB has started and panics: the panic ends
// the test binary before TestMain's own cleanup.
func failTestDB(msg string, err error) {
	stopTestDB()
	panic(msg + ": " + err.Error())
}

// testDB returns the migrated test database, starting its container on the
// first call.
func testDB() *sql.DB {
	testDBOnce.Do(startTestDB)
	return testDBConn
}

func startTestDB() {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
		),
	)
	// A container that failed to start may still be running.
	pgContainer = container
	if err != nil {
		failTestDB("failed to start postgres container", err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		failTestDB("failed to get connection string", err)
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		failTestDB("failed to open db", err)
	}
	testDBConn = db

	if err := migrations.Up(ctx, db); err != nil {
		failTestDB("failed to run migrations", err)
	}
}

func cleanTags(t *testing.T) {
	t.Helper()
	if _, err := testDB().ExecContext(context.Background(), "DELETE FROM runtime.tags; DELETE FROM engineering.tags"); err != nil {
		t.Fatalf("cleanTags: %v", err)
	}
}

func newRouter() *restapi.APIRouter {
	return newServices().router()
}

// createTagViaService creates a tag and writes its state, value and quality,
// as its supplier would; it returns the tag with that state.
func createTagViaService(t *testing.T, name, tagType, value, quality string) appdto.Tag {
	t.Helper()
	services := newServices()
	resp, err := services.tags.CreateTag(context.Background(), mainProject, appdto.CreateTagInput{Name: name, Type: tagType})
	if err != nil {
		t.Fatalf("createTagViaService(%q): %v", name, err)
	}
	written, err := services.states.SetTagValue(context.Background(), mainProject, runtimeapp.SetTagValueInput{TagID: resp.ID, Value: value, Quality: quality})
	if err != nil {
		t.Fatalf("createTagViaService(%q): write its state: %v", name, err)
	}
	resp.Value, resp.Quality, resp.Seq = written.Value, written.Quality, written.Seq
	return resp
}

// --- GET /api/v0/tags ---

func TestGetTags_EmptyDB_Returns200WithEmptyList(t *testing.T) {
	cleanTags(t)
	router := newRouter()

	req := httptest.NewRequest(http.MethodGet, mainPath+"/tags", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}

	var resp apiv0.GetTagsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(resp.Tags))
	}
}

func TestGetTags_WithTags_Returns200WithAll(t *testing.T) {
	cleanTags(t)
	createTagViaService(t, "temperature", "integer", "10", "good")
	createTagViaService(t, "pressure", "integer", "20", "good")
	router := newRouter()

	req := httptest.NewRequest(http.MethodGet, mainPath+"/tags", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}

	var resp apiv0.GetTagsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(resp.Tags))
	}
}

func TestGetTags_NameFilter_ExistingTag_Returns200WithOneTag(t *testing.T) {
	cleanTags(t)
	created := createTagViaService(t, "temperature", "integer", "10", "good")
	createTagViaService(t, "pressure", "integer", "20", "good")
	router := newRouter()

	req := httptest.NewRequest(http.MethodGet, mainPath+"/tags?name=temperature", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}

	var resp apiv0.GetTagsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(resp.Tags))
	}
	if resp.Tags[0].ID != created.ID {
		t.Errorf("id: expected %v, got %v", created.ID, resp.Tags[0].ID)
	}
}

func TestGetTags_NameFilter_NoMatch_Returns200WithEmptyList(t *testing.T) {
	cleanTags(t)
	createTagViaService(t, "temperature", "integer", "10", "good")
	router := newRouter()

	req := httptest.NewRequest(http.MethodGet, mainPath+"/tags?name=missing", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != `{"tags":[]}` {
		t.Errorf("body: expected empty list, got %s", body)
	}
}

func getTagNames(t *testing.T, router *restapi.APIRouter, query string) (int, map[string]bool) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, mainPath+"/tags?"+query, nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		return rec.Code, nil
	}
	var resp apiv0.GetTagsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	names := make(map[string]bool, len(resp.Tags))
	for _, tg := range resp.Tags {
		names[tg.Name] = true
	}
	return rec.Code, names
}

func TestGetTags_NamePattern_Returns200WithMatchingTags(t *testing.T) {
	cleanTags(t)
	createTagViaService(t, "pump1.speed", "integer", "0", "good")
	createTagViaService(t, "pump2.speed", "integer", "0", "good")
	createTagViaService(t, "pump1.state", "boolean", "false", "good")
	router := newRouter()

	code, names := getTagNames(t, router, "name_pattern="+url.QueryEscape("pump*.speed"))

	if code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d", code)
	}
	if len(names) != 2 || !names["pump1.speed"] || !names["pump2.speed"] {
		t.Errorf("got %v, want pump1.speed and pump2.speed", names)
	}
}

func TestGetTags_NameRegex_Returns200WithMatchingTags(t *testing.T) {
	cleanTags(t)
	createTagViaService(t, "pump1.speed", "integer", "0", "good")
	createTagViaService(t, "pump1.state", "boolean", "false", "good")
	router := newRouter()

	code, names := getTagNames(t, router, "name_regex="+url.QueryEscape(`\.st`))

	if code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d", code)
	}
	if len(names) != 1 || !names["pump1.state"] {
		t.Errorf("got %v, want pump1.state", names)
	}
}

func TestGetTags_NameRegex_Invalid_Returns400(t *testing.T) {
	router := newRouter()

	code, _ := getTagNames(t, router, "name_regex="+url.QueryEscape("pump("))

	if code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", code)
	}
}

func TestGetTags_SeveralNameFilters_Returns400(t *testing.T) {
	router := newRouter()

	code, _ := getTagNames(t, router, "name=a&name_pattern=a*")

	if code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", code)
	}
}

// --- GET /api/v0/tags/{id} ---

func TestGetTagsByID_ExistingTag_Returns200WithTag(t *testing.T) {
	cleanTags(t)
	created := createTagViaService(t, "humidity", "integer", "55", "good")
	router := newRouter()

	req := httptest.NewRequest(http.MethodGet, mainPath+"/tags/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}

	var resp apiv0.TagResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Name != "humidity" {
		t.Errorf("Name: expected 'humidity', got %q", resp.Name)
	}
	if resp.Value != "55" {
		t.Errorf("Value: expected '55', got %q", resp.Value)
	}
	if resp.Type != "integer" {
		t.Errorf("Type:expected 'integer', got %q", resp.Type)
	}
	if resp.Quality != "good" {
		t.Errorf("Quality: expected 'good', got %q", resp.Quality)
	}
	if resp.Seq != created.Seq || resp.Seq == 0 || resp.Version != created.Version {
		t.Errorf("Seq %d, Version %d: want the write's seq %d and the definition's version %d", resp.Seq, resp.Version, created.Seq, created.Version)
	}
}

// TestGetTagsByID_NotWrittenYet_IsTheInitialStateWithSeqZero: a tag no
// supplier has written shows its type's zero value with quality bad, and
// seq 0 in the body.
func TestGetTagsByID_NotWrittenYet_IsTheInitialStateWithSeqZero(t *testing.T) {
	cleanTags(t)
	created, err := newServices().tags.CreateTag(context.Background(), mainProject, appdto.CreateTagInput{Name: "fresh", Type: "boolean"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, mainPath+"/tags/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	newRouter().ServeMux().ServeHTTP(rec, req)

	var raw map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if raw["value"] != "false" || raw["quality"] != "bad" {
		t.Errorf("state %v, want false, bad", raw)
	}
	if seq, ok := raw["seq"]; !ok || seq != float64(0) {
		t.Errorf("seq %v (present %v) in the body of a state not written yet, want 0", seq, ok)
	}
}

func TestGetTagsByID_NotFound_Returns404WithProblemDetails(t *testing.T) {
	cleanTags(t)
	router := newRouter()

	missingID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, mainPath+"/tags/"+missingID, nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}

	var prob apiv0.Problem
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob.Status != http.StatusNotFound {
		t.Errorf("problem status: expected 404, got %d", prob.Status)
	}
	if prob.Type != restapi.TypeNotFound {
		t.Errorf("problem type: expected %q, got %q", restapi.TypeNotFound, prob.Type)
	}
	if want := "Tag with ID '" + missingID + "' not found"; prob.Detail != want {
		t.Errorf("detail: expected %q, got %q", want, prob.Detail)
	}
}

func TestGetTagsByID_InvalidUUID_Returns400WithProblemDetails(t *testing.T) {
	router := newRouter()

	req := httptest.NewRequest(http.MethodGet, mainPath+"/tags/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}

	var prob apiv0.Problem
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob.Status != http.StatusBadRequest {
		t.Errorf("problem status: expected 400, got %d", prob.Status)
	}
	if prob.Type != restapi.TypeBadRequest {
		t.Errorf("problem type: expected %q, got %q", restapi.TypeBadRequest, prob.Type)
	}
}

// --- POST /api/v0/tags ---

func TestPostTags_ValidBody_Returns201WithID(t *testing.T) {
	cleanTags(t)
	router := newRouter()

	body, _ := json.Marshal(apiv0.CreateTagRequest{Name: "flow", Type: "integer"})
	req := httptest.NewRequest(http.MethodPost, mainPath+"/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status: expected 201, got %d\nbody: %s", rec.Code, rec.Body.String())
	}

	var resp apiv0.CreateTagResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID == uuid.Nil {
		t.Error("expected non-zero ID in response")
	}
}

func TestPostTags_DuplicateName_Returns409WithProblemDetails(t *testing.T) {
	cleanTags(t)
	createTagViaService(t, "flow", "integer", "0", "good")
	router := newRouter()

	body, _ := json.Marshal(apiv0.CreateTagRequest{Name: "flow", Type: "boolean"})
	req := httptest.NewRequest(http.MethodPost, mainPath+"/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status: expected 409, got %d\nbody: %s", rec.Code, rec.Body.String())
	}

	var prob apiv0.Problem
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob.Type != restapi.TypeConflict {
		t.Errorf("problem type: expected %q, got %q", restapi.TypeConflict, prob.Type)
	}
}

func TestPostTags_InvalidJSON_Returns400WithProblemDetails(t *testing.T) {
	router := newRouter()

	req := httptest.NewRequest(http.MethodPost, mainPath+"/tags", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}

	var prob apiv0.Problem
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob.Status != http.StatusBadRequest {
		t.Errorf("problem status: expected 400, got %d", prob.Status)
	}
	if prob.Type != restapi.TypeBadRequest {
		t.Errorf("problem type: expected %q, got %q", restapi.TypeBadRequest, prob.Type)
	}
}

func TestPostTags_InvalidType_Returns400WithProblemDetails(t *testing.T) {
	router := newRouter()

	body, _ := json.Marshal(apiv0.CreateTagRequest{Name: "sensor", Type: "unknown"})
	req := httptest.NewRequest(http.MethodPost, mainPath+"/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}

	var prob apiv0.Problem
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob.Status != http.StatusBadRequest {
		t.Errorf("problem status: expected 400, got %d", prob.Status)
	}
	if prob.Type != restapi.TypeBadRequest {
		t.Errorf("problem type: expected %q, got %q", restapi.TypeBadRequest, prob.Type)
	}
}

// --- DELETE /api/v0/tags/{id} ---

func TestDeleteTagByID_ExistingTag_Returns200WithDeletedTag(t *testing.T) {
	cleanTags(t)
	created := createTagViaService(t, "pump", "boolean", "false", "good")
	router := newRouter()

	req := httptest.NewRequest(http.MethodDelete, mainPath+"/tags/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d\nbody: %s", rec.Code, rec.Body.String())
	}

	var resp apiv0.TagResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != created.ID {
		t.Errorf("deleted tag ID: expected %s, got %s", created.ID, resp.ID)
	}
	if resp.Name != "pump" {
		t.Errorf("deleted tag Name: expected 'pump', got %q", resp.Name)
	}
}

func TestDeleteTagByID_ExistingTag_TagIsRemovedFromDB(t *testing.T) {
	cleanTags(t)
	created := createTagViaService(t, "valve", "boolean", "true", "good")
	router := newRouter()

	req := httptest.NewRequest(http.MethodDelete, mainPath+"/tags/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d", rec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, mainPath+"/tags/"+created.ID.String(), nil)
	getRec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Errorf("after delete GET: expected 404, got %d", getRec.Code)
	}
}

func TestDeleteTagByID_NotFound_Returns404WithProblemDetails(t *testing.T) {
	cleanTags(t)
	router := newRouter()

	req := httptest.NewRequest(http.MethodDelete, mainPath+"/tags/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}

	var prob apiv0.Problem
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob.Status != http.StatusNotFound {
		t.Errorf("problem status: expected 404, got %d", prob.Status)
	}
	if prob.Type != restapi.TypeNotFound {
		t.Errorf("problem type: expected %q, got %q", restapi.TypeNotFound, prob.Type)
	}
}

func TestDeleteTagByID_InvalidUUID_Returns400WithProblemDetails(t *testing.T) {
	router := newRouter()

	req := httptest.NewRequest(http.MethodDelete, mainPath+"/tags/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}

	var prob apiv0.Problem
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob.Status != http.StatusBadRequest {
		t.Errorf("problem status: expected 400, got %d", prob.Status)
	}
	if prob.Type != restapi.TypeBadRequest {
		t.Errorf("problem type: expected %q, got %q", restapi.TypeBadRequest, prob.Type)
	}
}

// --- PATCH /api/v0/tags/{id}/value ---

// patchValue sends PATCH /tags/{id}/value with body.
func patchValue(router *restapi.APIRouter, tagID uuid.UUID, body any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, mainPath+"/tags/"+tagID.String()+"/value", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)
	return rec
}

// TestPatchTagValue_WritesInARow_Return200WithIncreasingSeqs: no version is
// stated, none is checked; each write takes a seq above the one before.
func TestPatchTagValue_WritesInARow_Return200WithIncreasingSeqs(t *testing.T) {
	cleanTags(t)
	created := createTagViaService(t, "temperature", "integer", "10", "bad")
	router := newRouter()

	last := created.Seq
	for _, value := range []string{"99", "99", "100"} {
		rec := patchValue(router, created.ID, apiv0.UpdateTagRequest{Value: value, Quality: "good"})
		if rec.Code != http.StatusOK {
			t.Fatalf("status: expected 200, got %d\nbody: %s", rec.Code, rec.Body.String())
		}
		var resp apiv0.UpdateTagResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Seq <= last {
			t.Errorf("seq %d, want above %d", resp.Seq, last)
		}
		last = resp.Seq
	}
}

// TestPatchTagValue_WithAVersion_Returns400: the body has no version any
// more; strictly decoded, a client still sending one is told so.
func TestPatchTagValue_WithAVersion_Returns400(t *testing.T) {
	cleanTags(t)
	created := createTagViaService(t, "temperature", "integer", "10", "bad")

	rec := patchValue(newRouter(), created.ID, map[string]any{"value": "1", "quality": "good", "version": created.Version})

	assertBadRequest(t, rec)
}

func TestPatchTagValue_ValidUpdate_ValueAndQualityAreUpdated(t *testing.T) {
	cleanTags(t)
	created := createTagViaService(t, "humidity", "integer", "0", "bad")
	router := newRouter()

	body, _ := json.Marshal(apiv0.UpdateTagRequest{Value: "75", Quality: "good"})
	req := httptest.NewRequest(http.MethodPatch, mainPath+"/tags/"+created.ID.String()+"/value", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d", rec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, mainPath+"/tags/"+created.ID.String(), nil)
	getRec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(getRec, getReq)

	var tag apiv0.TagResponse
	if err := json.NewDecoder(getRec.Body).Decode(&tag); err != nil {
		t.Fatalf("decode tag: %v", err)
	}
	if tag.Value != "75" {
		t.Errorf("Value: expected '75', got %q", tag.Value)
	}
	if tag.Quality != "good" {
		t.Errorf("Quality: expected 'good', got %q", tag.Quality)
	}
}

func TestPatchTagValue_NotFound_Returns404WithProblemDetails(t *testing.T) {
	cleanTags(t)
	router := newRouter()

	body, _ := json.Marshal(map[string]string{"value": "1", "quality": "good"})
	req := httptest.NewRequest(http.MethodPatch, mainPath+"/tags/"+uuid.New().String()+"/value", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}

	var prob apiv0.Problem
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob.Status != http.StatusNotFound {
		t.Errorf("problem status: expected 404, got %d", prob.Status)
	}
	if prob.Type != restapi.TypeNotFound {
		t.Errorf("problem type: expected %q, got %q", restapi.TypeNotFound, prob.Type)
	}
}

func TestPatchTagValue_InvalidUUID_Returns400WithProblemDetails(t *testing.T) {
	router := newRouter()

	body, _ := json.Marshal(map[string]string{"value": "1", "quality": "good"})
	req := httptest.NewRequest(http.MethodPatch, mainPath+"/tags/not-a-uuid/value", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}

	var prob apiv0.Problem
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob.Status != http.StatusBadRequest {
		t.Errorf("problem status: expected 400, got %d", prob.Status)
	}
	if prob.Type != restapi.TypeBadRequest {
		t.Errorf("problem type: expected %q, got %q", restapi.TypeBadRequest, prob.Type)
	}
}

func TestPatchTagValue_InvalidJSON_Returns400WithProblemDetails(t *testing.T) {
	cleanTags(t)
	created := createTagViaService(t, "sensor", "integer", "0", "good")
	router := newRouter()

	req := httptest.NewRequest(http.MethodPatch, mainPath+"/tags/"+created.ID.String()+"/value", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}

	var prob apiv0.Problem
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob.Status != http.StatusBadRequest {
		t.Errorf("problem status: expected 400, got %d", prob.Status)
	}
	if prob.Type != restapi.TypeBadRequest {
		t.Errorf("problem type: expected %q, got %q", restapi.TypeBadRequest, prob.Type)
	}
}

// --- invalid input → 400 ---

// assertBadRequest fails unless rec is a 400 Problem Details response.
func assertBadRequest(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: expected 400, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var prob apiv0.Problem
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob.Type != restapi.TypeBadRequest {
		t.Errorf("problem type: expected %q, got %q", restapi.TypeBadRequest, prob.Type)
	}
	if prob.Detail == "" {
		t.Error("problem detail: expected the cause, got empty")
	}
}

func TestPostTags_InvalidInput_Returns400(t *testing.T) {
	// A tag is created with its definition alone: a value or a quality in
	// the body is an unknown field.
	cases := map[string]map[string]string{
		"type":    {"name": "sensor", "type": ""},
		"value":   {"name": "sensor", "type": "integer", "value": "0"},
		"quality": {"name": "sensor", "type": "integer", "quality": "good"},
	}
	for name, request := range cases {
		t.Run(name, func(t *testing.T) {
			cleanTags(t)
			body, _ := json.Marshal(request)
			req := httptest.NewRequest(http.MethodPost, mainPath+"/tags", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			newRouter().ServeMux().ServeHTTP(rec, req)

			assertBadRequest(t, rec)
		})
	}
}

func TestPatchTagValue_InvalidInput_Returns400(t *testing.T) {
	cases := map[string]apiv0.UpdateTagRequest{
		"quality":        {Value: "1", Quality: "simulated"},
		"value for type": {Value: "abc", Quality: "good"},
	}
	for name, request := range cases {
		t.Run(name, func(t *testing.T) {
			cleanTags(t)
			created := createTagViaService(t, "level", "integer", "0", "good")
			body, _ := json.Marshal(request)
			req := httptest.NewRequest(http.MethodPatch, mainPath+"/tags/"+created.ID.String()+"/value", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			newRouter().ServeMux().ServeHTTP(rec, req)

			assertBadRequest(t, rec)
		})
	}
}
