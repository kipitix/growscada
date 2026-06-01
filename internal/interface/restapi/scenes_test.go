package restapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/application"
	"github.com/kipitix/growscada/internal/application/appdto"
	"github.com/kipitix/growscada/internal/domain/event"
	"github.com/kipitix/growscada/internal/domain/scene"
	"github.com/kipitix/growscada/internal/infrastructure/postgres/repositories"
	"github.com/kipitix/growscada/internal/interface/restapi"
	"github.com/kipitix/growscada/internal/interface/restapi/restdto"
)

func cleanScenesRest(t *testing.T) {
	t.Helper()
	// ON DELETE CASCADE removes widgets too; that's fine for scene tests.
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM scenes"); err != nil {
		t.Fatalf("cleanScenesRest: %v", err)
	}
}

func newRouterWithScenes() *restapi.APIRouter {
	tagRepo := repositories.NewTagRepositoryPostgres(testDB)
	tagSvc := application.NewTagService(tagRepo, event.NewEventBus())
	wtRepo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	wRepo := repositories.NewWidgetRepositoryPostgres(testDB)
	wtSvc := application.NewWidgetTypeService(wtRepo, wRepo, event.NewEventBus())
	wSvc := application.NewWidgetService(wRepo, wtRepo, event.NewEventBus())
	sceneRepo := repositories.NewSceneRepositoryPostgres(testDB)
	sceneSvc := application.NewSceneService(sceneRepo, event.NewEventBus())
	return restapi.NewRouter(tagSvc, wtSvc, wSvc, sceneSvc)
}

func createSceneViaService(t *testing.T, input appdto.CreateSceneInput) appdto.Scene {
	t.Helper()
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	svc := application.NewSceneService(repo, event.NewEventBus())
	resp, err := svc.CreateScene(context.Background(), input)
	if err != nil {
		t.Fatalf("createSceneViaService: %v", err)
	}
	return resp
}

var testSceneInput = appdto.CreateSceneInput{
	Name:           "main-dashboard",
	Width:          1920,
	Height:         1080,
	BackgroundHTML: `<div class="bg"></div>`,
}

// --- GET /api/v1/scenes ---

func TestGetScenes_EmptyDB_Returns200WithEmptyList(t *testing.T) {
	cleanScenesRest(t)
	router := newRouterWithScenes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scenes", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
	var resp restdto.GetScenesResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Scenes) != 0 {
		t.Errorf("expected 0 items, got %d", len(resp.Scenes))
	}
}

func TestGetScenes_WithItems_Returns200WithAll(t *testing.T) {
	cleanScenesRest(t)
	createSceneViaService(t, testSceneInput)
	input2 := testSceneInput
	input2.Name = "overview-panel"
	createSceneViaService(t, input2)
	router := newRouterWithScenes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scenes", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
	var resp restdto.GetScenesResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Scenes) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Scenes))
	}
}

// --- GET /api/v1/scenes/{id} ---

func TestGetScenesByID_Existing_Returns200WithCorrectFields(t *testing.T) {
	cleanScenesRest(t)
	created := createSceneViaService(t, testSceneInput)
	router := newRouterWithScenes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scenes/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
	var resp restdto.SceneResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != testSceneInput.Name {
		t.Errorf("Name: expected %q, got %q", testSceneInput.Name, resp.Name)
	}
	if resp.Width != testSceneInput.Width {
		t.Errorf("Width: expected %d, got %d", testSceneInput.Width, resp.Width)
	}
	if resp.Height != testSceneInput.Height {
		t.Errorf("Height: expected %d, got %d", testSceneInput.Height, resp.Height)
	}
	if resp.BackgroundHTML != testSceneInput.BackgroundHTML {
		t.Errorf("BackgroundHTML: expected %q, got %q", testSceneInput.BackgroundHTML, resp.BackgroundHTML)
	}
}

func TestGetScenesByID_NotFound_Returns404(t *testing.T) {
	cleanScenesRest(t)
	router := newRouterWithScenes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scenes/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}
	var prob restapi.ProblemDetails
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob.Type != restapi.TypeNotFound {
		t.Errorf("problem type: expected %q, got %q", restapi.TypeNotFound, prob.Type)
	}
}

func TestGetScenesByID_InvalidUUID_Returns400(t *testing.T) {
	router := newRouterWithScenes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scenes/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

// --- POST /api/v1/scenes ---

func TestPostScenes_Valid_Returns201WithID(t *testing.T) {
	cleanScenesRest(t)
	router := newRouterWithScenes()

	body, _ := json.Marshal(restdto.CreateSceneRequest{
		Name:           "pump-room",
		Width:          1280,
		Height:         720,
		BackgroundHTML: `<div class="pump-room-bg"></div>`,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scenes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status: expected 201, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var resp restdto.CreateSceneResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID == uuid.Nil {
		t.Error("expected non-zero ID")
	}
}

func TestPostScenes_InvalidJSON_Returns400(t *testing.T) {
	router := newRouterWithScenes()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/scenes", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

func TestPostScenes_EmptyName_Returns422(t *testing.T) {
	router := newRouterWithScenes()

	body, _ := json.Marshal(restdto.CreateSceneRequest{
		Name:   "",
		Width:  1920,
		Height: 1080,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scenes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status: expected 422, got %d", rec.Code)
	}
}

func TestPostScenes_ZeroWidth_Returns422(t *testing.T) {
	router := newRouterWithScenes()

	body, _ := json.Marshal(restdto.CreateSceneRequest{
		Name:   "bad-scene",
		Width:  0,
		Height: 1080,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scenes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status: expected 422, got %d", rec.Code)
	}
}

// --- PUT /api/v1/scenes/{id} ---

func TestPutScenesByID_Valid_Returns200WithIncrementedVersion(t *testing.T) {
	cleanScenesRest(t)
	created := createSceneViaService(t, testSceneInput)
	router := newRouterWithScenes()

	body, _ := json.Marshal(restdto.UpdateSceneRequest{
		Name:           "main-dashboard-updated",
		Width:          2560,
		Height:         1440,
		BackgroundHTML: `<div class="updated-bg"></div>`,
		Version:        created.Version,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/scenes/"+created.ID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var resp restdto.UpdateSceneResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Version <= created.Version {
		t.Errorf("Version: expected > %d, got %d", created.Version, resp.Version)
	}
}

func TestPutScenesByID_NotFound_Returns404(t *testing.T) {
	cleanScenesRest(t)
	router := newRouterWithScenes()

	body, _ := json.Marshal(restdto.UpdateSceneRequest{
		Name:   "x",
		Width:  800,
		Height: 600,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/scenes/"+uuid.New().String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}
}

func TestPutScenesByID_InvalidUUID_Returns400(t *testing.T) {
	router := newRouterWithScenes()

	body, _ := json.Marshal(restdto.UpdateSceneRequest{Name: "x", Width: 800, Height: 600})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/scenes/not-a-uuid", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

func TestPutScenesByID_Conflict_Returns409(t *testing.T) {
	svc := &stubSceneService{updateErr: scene.ErrSceneConflict}
	tagRepo := repositories.NewTagRepositoryPostgres(testDB)
	tagSvc := application.NewTagService(tagRepo, event.NewEventBus())
	wtRepo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	wRepo := repositories.NewWidgetRepositoryPostgres(testDB)
	wtSvc := application.NewWidgetTypeService(wtRepo, wRepo, event.NewEventBus())
	wSvc := application.NewWidgetService(wRepo, wtRepo, event.NewEventBus())
	router := restapi.NewRouter(tagSvc, wtSvc, wSvc, svc)

	body, _ := json.Marshal(restdto.UpdateSceneRequest{Name: "x", Width: 800, Height: 600})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/scenes/"+uuid.New().String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status: expected 409, got %d", rec.Code)
	}
}

// stubSceneService is a minimal SceneService stub for handler-level tests.
type stubSceneService struct {
	application.SceneService
	updateErr error
}

func (s *stubSceneService) UpdateScene(_ context.Context, _ appdto.UpdateSceneInput) (appdto.Scene, error) {
	return appdto.Scene{}, s.updateErr
}

// --- DELETE /api/v1/scenes/{id} ---

func TestDeleteScenesByID_Existing_Returns200WithDeletedItem(t *testing.T) {
	cleanScenesRest(t)
	created := createSceneViaService(t, testSceneInput)
	router := newRouterWithScenes()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/scenes/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var resp restdto.SceneResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID != created.ID {
		t.Errorf("ID: expected %s, got %s", created.ID, resp.ID)
	}
	if resp.Name != testSceneInput.Name {
		t.Errorf("Name: expected %q, got %q", testSceneInput.Name, resp.Name)
	}
}

func TestDeleteScenesByID_NotFound_Returns404(t *testing.T) {
	cleanScenesRest(t)
	router := newRouterWithScenes()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/scenes/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}
}

func TestDeleteScenesByID_InvalidUUID_Returns400(t *testing.T) {
	router := newRouterWithScenes()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/scenes/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

func TestDeleteScenesByID_Existing_RemovedFromDB(t *testing.T) {
	cleanScenesRest(t)
	created := createSceneViaService(t, testSceneInput)
	router := newRouterWithScenes()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/scenes/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("delete status: expected 200, got %d", rec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/scenes/"+created.ID.String(), nil)
	getRec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Errorf("after delete GET: expected 404, got %d", getRec.Code)
	}
}
