package restapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/kipitix/growscada/internal/application"
	"github.com/kipitix/growscada/internal/application/appdto"
	"github.com/kipitix/growscada/internal/domain/event"
	"github.com/kipitix/growscada/internal/infrastructure/postgres/repositories"
	"github.com/kipitix/growscada/internal/interface/restapi"
	"github.com/kipitix/growscada/internal/interface/restapi/restdto"
)

var testDB *sql.DB

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
		),
	)
	if err != nil {
		panic("failed to start postgres container: " + err.Error())
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		pgContainer.Terminate(ctx)
		panic("failed to get connection string: " + err.Error())
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		pgContainer.Terminate(ctx)
		panic("failed to open db: " + err.Error())
	}

	_, currentFile, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(currentFile), "..", "..", "infrastructure", "postgres", "migrations")

	if err := goose.SetDialect("postgres"); err != nil {
		db.Close()
		pgContainer.Terminate(ctx)
		panic("failed to set goose dialect: " + err.Error())
	}
	if err := goose.Up(db, migrationsDir); err != nil {
		db.Close()
		pgContainer.Terminate(ctx)
		panic("failed to run migrations: " + err.Error())
	}

	testDB = db

	code := m.Run()

	db.Close()
	pgContainer.Terminate(ctx)
	os.Exit(code)
}

func cleanTags(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM tags"); err != nil {
		t.Fatalf("cleanTags: %v", err)
	}
}

func newRouter() *restapi.APIRouter {
	tagRepo := repositories.NewTagRepositoryPostgres(testDB)
	tagSvc := application.NewTagService(tagRepo, event.NewEventBus())
	wtRepo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	wtSvc := application.NewWidgetTypeService(wtRepo, event.NewEventBus())
	wRepo := repositories.NewWidgetRepositoryPostgres(testDB)
	wSvc := application.NewWidgetService(wRepo, event.NewEventBus())
	return restapi.NewRouter(tagSvc, wtSvc, wSvc)
}

func createTagViaService(t *testing.T, name, tagType, value, quality string) appdto.Tag {
	t.Helper()
	repo := repositories.NewTagRepositoryPostgres(testDB)
	svc := application.NewTagService(repo, event.NewEventBus())
	resp, err := svc.CreateTag(context.Background(), appdto.CreateTagInput{
		Name: name, Type: tagType, Value: value, Quality: quality,
	})
	if err != nil {
		t.Fatalf("createTagViaService(%q): %v", name, err)
	}
	return resp
}

// --- GET /api/v1/tags ---

func TestGetTags_EmptyDB_Returns200WithEmptyList(t *testing.T) {
	cleanTags(t)
	router := newRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}

	var resp restdto.GetTagsResponse
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}

	var resp restdto.GetTagsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(resp.Tags))
	}
}

// --- GET /api/v1/tags/{id} ---

func TestGetTagsByID_ExistingTag_Returns200WithTag(t *testing.T) {
	cleanTags(t)
	created := createTagViaService(t, "humidity", "integer", "55", "good")
	router := newRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}

	var resp restdto.TagResponse
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
}

func TestGetTagsByID_NotFound_Returns404WithProblemDetails(t *testing.T) {
	cleanTags(t)
	router := newRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}

	var prob restapi.ProblemDetails
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

func TestGetTagsByID_InvalidUUID_Returns400WithProblemDetails(t *testing.T) {
	router := newRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}

	var prob restapi.ProblemDetails
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

// --- POST /api/v1/tags ---

func TestPostTags_ValidBody_Returns201WithID(t *testing.T) {
	cleanTags(t)
	router := newRouter()

	body, _ := json.Marshal(restdto.CreateTagRequest{Name: "flow", Type: "integer", Value: "0", Quality: "good"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status: expected 201, got %d\nbody: %s", rec.Code, rec.Body.String())
	}

	var resp restdto.CreateTagResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID == uuid.Nil {
		t.Error("expected non-zero ID in response")
	}
}

func TestPostTags_InvalidJSON_Returns400WithProblemDetails(t *testing.T) {
	router := newRouter()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}

	var prob restapi.ProblemDetails
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

func TestPostTags_InvalidType_Returns500WithProblemDetails(t *testing.T) {
	router := newRouter()

	body, _ := json.Marshal(restdto.CreateTagRequest{Name: "sensor", Type: "unknown", Value: "0", Quality: "good"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status: expected 500, got %d", rec.Code)
	}

	var prob restapi.ProblemDetails
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob.Status != http.StatusInternalServerError {
		t.Errorf("problem status: expected 500, got %d", prob.Status)
	}
	if prob.Type != restapi.TypeInternalError {
		t.Errorf("problem type: expected %q, got %q", restapi.TypeInternalError, prob.Type)
	}
}

// --- DELETE /api/v1/tags/{id} ---

func TestDeleteTagByID_ExistingTag_Returns200WithDeletedTag(t *testing.T) {
	cleanTags(t)
	created := createTagViaService(t, "pump", "boolean", "false", "good")
	router := newRouter()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tags/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d\nbody: %s", rec.Code, rec.Body.String())
	}

	var resp restdto.TagResponse
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

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tags/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d", rec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/tags/"+created.ID.String(), nil)
	getRec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Errorf("after delete GET: expected 404, got %d", getRec.Code)
	}
}

func TestDeleteTagByID_NotFound_Returns404WithProblemDetails(t *testing.T) {
	cleanTags(t)
	router := newRouter()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tags/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}

	var prob restapi.ProblemDetails
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

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tags/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}

	var prob restapi.ProblemDetails
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

// --- PATCH /api/v1/tags/{id}/value ---

func TestPatchTagValue_ValidUpdate_Returns200WithVersion(t *testing.T) {
	cleanTags(t)
	created := createTagViaService(t, "temperature", "integer", "10", "bad")
	router := newRouter()

	body, _ := json.Marshal(map[string]string{"value": "99", "quality": "good"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/tags/"+created.ID.String()+"/value", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d\nbody: %s", rec.Code, rec.Body.String())
	}

	var resp restdto.UpdateTagResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Version != 2 {
		t.Errorf("Version: expected 2, got %d", resp.Version)
	}
}

func TestPatchTagValue_ValidUpdate_ValueAndQualityAreUpdated(t *testing.T) {
	cleanTags(t)
	created := createTagViaService(t, "humidity", "integer", "0", "bad")
	router := newRouter()

	body, _ := json.Marshal(map[string]string{"value": "75", "quality": "good"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/tags/"+created.ID.String()+"/value", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d", rec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/tags/"+created.ID.String(), nil)
	getRec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(getRec, getReq)

	var tag restdto.TagResponse
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
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/tags/"+uuid.New().String()+"/value", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}

	var prob restapi.ProblemDetails
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
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/tags/not-a-uuid/value", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}

	var prob restapi.ProblemDetails
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

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/tags/"+created.ID.String()+"/value", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}

	var prob restapi.ProblemDetails
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
