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
	"github.com/kipitix/growscada/internal/domain/widget"
	"github.com/kipitix/growscada/internal/infrastructure/postgres/repositories"
	"github.com/kipitix/growscada/internal/interface/restapi"
	"github.com/kipitix/growscada/internal/interface/restapi/restdto"
)

func cleanWidgetsRest(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM widgets"); err != nil {
		t.Fatalf("cleanWidgetsRest: %v", err)
	}
}

func newRouterWithWidgets() *restapi.APIRouter {
	tagRepo := repositories.NewTagRepositoryPostgres(testDB)
	tagSvc := application.NewTagService(tagRepo, event.NewEventBus())
	wtRepo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	wtSvc := application.NewWidgetTypeService(wtRepo, event.NewEventBus())
	wRepo := repositories.NewWidgetRepositoryPostgres(testDB)
	wSvc := application.NewWidgetService(wRepo, event.NewEventBus())
	sceneRepo := repositories.NewSceneRepositoryPostgres(testDB)
	sceneSvc := application.NewSceneService(sceneRepo, event.NewEventBus())
	return restapi.NewRouter(tagSvc, wtSvc, wSvc, sceneSvc)
}

func createWidgetViaService(t *testing.T, input appdto.CreateWidgetInput) appdto.Widget {
	t.Helper()
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	svc := application.NewWidgetService(repo, event.NewEventBus())
	resp, err := svc.CreateWidget(context.Background(), input)
	if err != nil {
		t.Fatalf("createWidgetViaService: %v", err)
	}
	return resp
}

var testWidgetInput = appdto.CreateWidgetInput{
	Name:   "pressure-gauge",
	X:      10.0,
	Y:      20.0,
	Z:      0.0,
	TypeID: uuid.New(),
	Labels: []string{"sensor"},
	TagIDs: nil,
}

// --- GET /api/v1/widgets ---

func TestGetWidgets_EmptyDB_Returns200WithEmptyList(t *testing.T) {
	cleanWidgetsRest(t)
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
	var resp restdto.GetWidgetsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Widgets) != 0 {
		t.Errorf("expected 0 items, got %d", len(resp.Widgets))
	}
}

func TestGetWidgets_WithItems_Returns200WithAll(t *testing.T) {
	cleanWidgetsRest(t)
	createWidgetViaService(t, testWidgetInput)
	input2 := testWidgetInput
	input2.Name = "thermometer"
	createWidgetViaService(t, input2)
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
	var resp restdto.GetWidgetsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Widgets) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Widgets))
	}
}

// --- GET /api/v1/widgets/{id} ---

func TestGetWidgetsByID_Existing_Returns200(t *testing.T) {
	cleanWidgetsRest(t)
	created := createWidgetViaService(t, testWidgetInput)
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
	var resp restdto.WidgetResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != testWidgetInput.Name {
		t.Errorf("Name: expected %q, got %q", testWidgetInput.Name, resp.Name)
	}
	if resp.Coordinates.X != testWidgetInput.X {
		t.Errorf("Coordinates.X: expected %v, got %v", testWidgetInput.X, resp.Coordinates.X)
	}
}

func TestGetWidgetsByID_NotFound_Returns404(t *testing.T) {
	cleanWidgetsRest(t)
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/"+uuid.New().String(), nil)
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

func TestGetWidgetsByID_InvalidUUID_Returns400(t *testing.T) {
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

// --- POST /api/v1/widgets ---

func TestPostWidgets_Valid_Returns201WithID(t *testing.T) {
	cleanWidgetsRest(t)
	router := newRouterWithWidgets()

	body, _ := json.Marshal(restdto.CreateWidgetRequest{
		Name:        "flow-meter",
		Coordinates: restdto.CoordinatesRequest{X: 5, Y: 10, Z: 0},
		TypeID:      uuid.New(),
		Labels:      []string{"flow"},
		TagIDs:      []uuid.UUID{},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/widgets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status: expected 201, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var resp restdto.CreateWidgetResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID == uuid.Nil {
		t.Error("expected non-zero ID")
	}
}

func TestPostWidgets_InvalidJSON_Returns400(t *testing.T) {
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/widgets", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

func TestPostWidgets_EmptyName_Returns500(t *testing.T) {
	router := newRouterWithWidgets()

	body, _ := json.Marshal(restdto.CreateWidgetRequest{
		Name:        "",
		Coordinates: restdto.CoordinatesRequest{X: 0, Y: 0, Z: 0},
		TypeID:      uuid.New(),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/widgets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status: expected 500, got %d", rec.Code)
	}
}

// --- PUT /api/v1/widgets/{id} ---

func TestPutWidgetsByID_Valid_Returns200WithIncrementedVersion(t *testing.T) {
	cleanWidgetsRest(t)
	created := createWidgetViaService(t, testWidgetInput)
	router := newRouterWithWidgets()

	body, _ := json.Marshal(restdto.UpdateWidgetRequest{
		Name:        "updated-gauge",
		Coordinates: restdto.CoordinatesRequest{X: 1, Y: 2, Z: 3},
		TypeID:      uuid.New(),
		Labels:      []string{"updated"},
		TagIDs:      []uuid.UUID{},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/widgets/"+created.ID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var resp restdto.UpdateWidgetResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Version <= created.Version {
		t.Errorf("Version: expected > %d, got %d", created.Version, resp.Version)
	}
}

func TestPutWidgetsByID_NotFound_Returns404(t *testing.T) {
	cleanWidgetsRest(t)
	router := newRouterWithWidgets()

	body, _ := json.Marshal(restdto.UpdateWidgetRequest{
		Name:   "x",
		TypeID: uuid.New(),
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/widgets/"+uuid.New().String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}
}

func TestPutWidgetsByID_InvalidUUID_Returns400(t *testing.T) {
	router := newRouterWithWidgets()

	body, _ := json.Marshal(restdto.UpdateWidgetRequest{Name: "x", TypeID: uuid.New()})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/widgets/not-a-uuid", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

func TestPutWidgetsByID_Conflict_Returns409(t *testing.T) {
	svc := &stubWidgetService{updateErr: widget.ErrWidgetConflict}
	tagRepo := repositories.NewTagRepositoryPostgres(testDB)
	tagSvc := application.NewTagService(tagRepo, event.NewEventBus())
	wtRepo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	wtSvc := application.NewWidgetTypeService(wtRepo, event.NewEventBus())
	sceneRepo := repositories.NewSceneRepositoryPostgres(testDB)
	sceneSvc := application.NewSceneService(sceneRepo, event.NewEventBus())
	router := restapi.NewRouter(tagSvc, wtSvc, svc, sceneSvc)

	body, _ := json.Marshal(restdto.UpdateWidgetRequest{Name: "x", TypeID: uuid.New()})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/widgets/"+uuid.New().String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status: expected 409, got %d", rec.Code)
	}
}

// stubWidgetService is a minimal WidgetService stub for handler-level tests.
type stubWidgetService struct {
	application.WidgetService
	updateErr error
}

func (s *stubWidgetService) UpdateWidget(_ context.Context, _ appdto.UpdateWidgetInput) (appdto.Widget, error) {
	return appdto.Widget{}, s.updateErr
}

// --- DELETE /api/v1/widgets/{id} ---

func TestDeleteWidgetsByID_Existing_Returns200WithDeletedItem(t *testing.T) {
	cleanWidgetsRest(t)
	created := createWidgetViaService(t, testWidgetInput)
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/widgets/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var resp restdto.WidgetResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID != created.ID {
		t.Errorf("ID: expected %s, got %s", created.ID, resp.ID)
	}
}

func TestDeleteWidgetsByID_NotFound_Returns404(t *testing.T) {
	cleanWidgetsRest(t)
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/widgets/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}
}

func TestDeleteWidgetsByID_InvalidUUID_Returns400(t *testing.T) {
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/widgets/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

func TestDeleteWidgetsByID_Existing_RemovedFromDB(t *testing.T) {
	cleanWidgetsRest(t)
	created := createWidgetViaService(t, testWidgetInput)
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/widgets/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("delete status: expected 200, got %d", rec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/"+created.ID.String(), nil)
	getRec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Errorf("after delete GET: expected 404, got %d", getRec.Code)
	}
}
