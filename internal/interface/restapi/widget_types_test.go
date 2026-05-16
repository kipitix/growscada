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

func cleanWidgetTypes(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM widget_types"); err != nil {
		t.Fatalf("cleanWidgetTypes: %v", err)
	}
}

func newRouterWithWidgetTypes() *restapi.APIRouter {
	tagRepo := repositories.NewTagRepositoryPostgres(testDB)
	tagSvc := application.NewTagService(tagRepo, event.NewEventBus())
	wtRepo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	wtSvc := application.NewWidgetTypeService(wtRepo, event.NewEventBus())
	wRepo := repositories.NewWidgetRepositoryPostgres(testDB)
	wSvc := application.NewWidgetService(wRepo, event.NewEventBus())
	return restapi.NewRouter(tagSvc, wtSvc, wSvc)
}

func createWidgetTypeViaService(t *testing.T, input appdto.CreateWidgetTypeInput) appdto.WidgetType {
	t.Helper()
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	svc := application.NewWidgetTypeService(repo, event.NewEventBus())
	resp, err := svc.CreateWidgetType(context.Background(), input)
	if err != nil {
		t.Fatalf("createWidgetTypeViaService: %v", err)
	}
	return resp
}

var testWtInput = appdto.CreateWidgetTypeInput{
	Name:           "gauge",
	HtmlTemplate:   "<div class='gauge'><span class='value'></span></div>",
	Script:         "function render(v) { return v; }",
	ScriptLanguage: "javascript",
}

// --- GET /api/v1/widget-types ---

func TestGetWidgetTypes_EmptyDB_Returns200WithEmptyList(t *testing.T) {
	cleanWidgetTypes(t)
	router := newRouterWithWidgetTypes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widget-types", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
	var resp restdto.GetWidgetTypesResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.WidgetTypes) != 0 {
		t.Errorf("expected 0 items, got %d", len(resp.WidgetTypes))
	}
}

func TestGetWidgetTypes_WithItems_Returns200WithAll(t *testing.T) {
	cleanWidgetTypes(t)
	createWidgetTypeViaService(t, testWtInput)
	input2 := testWtInput
	input2.Name = "thermometer"
	createWidgetTypeViaService(t, input2)
	router := newRouterWithWidgetTypes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widget-types", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
	var resp restdto.GetWidgetTypesResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.WidgetTypes) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.WidgetTypes))
	}
}

// --- GET /api/v1/widget-types/{id} ---

func TestGetWidgetTypesByID_Existing_Returns200(t *testing.T) {
	cleanWidgetTypes(t)
	created := createWidgetTypeViaService(t, testWtInput)
	router := newRouterWithWidgetTypes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widget-types/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
	var resp restdto.WidgetTypeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != testWtInput.Name {
		t.Errorf("Name: expected %q, got %q", testWtInput.Name, resp.Name)
	}
}

func TestGetWidgetTypesByID_NotFound_Returns404(t *testing.T) {
	cleanWidgetTypes(t)
	router := newRouterWithWidgetTypes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widget-types/"+uuid.New().String(), nil)
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

func TestGetWidgetTypesByID_InvalidUUID_Returns400(t *testing.T) {
	router := newRouterWithWidgetTypes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widget-types/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

// --- POST /api/v1/widget-types ---

func TestPostWidgetTypes_Valid_Returns201WithID(t *testing.T) {
	cleanWidgetTypes(t)
	router := newRouterWithWidgetTypes()

	body, _ := json.Marshal(restdto.CreateWidgetTypeRequest{
		Name:           "gauge",
		HtmlTemplate:   "<div class='gauge'></div>",
		Script:         "render()",
		ScriptLanguage: "javascript",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/widget-types", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status: expected 201, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var resp restdto.CreateWidgetTypeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID == uuid.Nil {
		t.Error("expected non-zero ID")
	}
}

func TestPostWidgetTypes_InvalidJSON_Returns400(t *testing.T) {
	router := newRouterWithWidgetTypes()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/widget-types", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

func TestPostWidgetTypes_InvalidScriptLanguage_Returns500(t *testing.T) {
	router := newRouterWithWidgetTypes()

	body, _ := json.Marshal(restdto.CreateWidgetTypeRequest{
		Name:           "x",
		HtmlTemplate:   "<div/>",
		Script:         "x",
		ScriptLanguage: "ruby",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/widget-types", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status: expected 500, got %d", rec.Code)
	}
}

// --- PUT /api/v1/widget-types/{id} ---

func TestPutWidgetTypesByID_Valid_Returns200WithVersion(t *testing.T) {
	cleanWidgetTypes(t)
	created := createWidgetTypeViaService(t, testWtInput)
	router := newRouterWithWidgetTypes()

	body, _ := json.Marshal(restdto.UpdateWidgetTypeRequest{
		Name:           "updated-gauge",
		HtmlTemplate:   "<div class='updated'></div>",
		Script:         "print('hi')",
		ScriptLanguage: "python",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/widget-types/"+created.ID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var resp restdto.UpdateWidgetTypeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Version <= created.Version {
		t.Errorf("Version: expected > %d, got %d", created.Version, resp.Version)
	}
}

func TestPutWidgetTypesByID_NotFound_Returns404(t *testing.T) {
	cleanWidgetTypes(t)
	router := newRouterWithWidgetTypes()

	body, _ := json.Marshal(restdto.UpdateWidgetTypeRequest{
		Name: "x", HtmlTemplate: "<div/>", Script: "x", ScriptLanguage: "lua",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/widget-types/"+uuid.New().String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}
}

func TestPutWidgetTypesByID_InvalidUUID_Returns400(t *testing.T) {
	router := newRouterWithWidgetTypes()

	body, _ := json.Marshal(restdto.UpdateWidgetTypeRequest{
		Name: "x", HtmlTemplate: "<div/>", Script: "x", ScriptLanguage: "lua",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/widget-types/not-a-uuid", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

func TestPutWidgetTypesByID_Conflict_Returns409(t *testing.T) {
	svc := &stubWidgetTypeService{updateErr: widget.ErrWidgetTypeConflict}
	tagRepo := repositories.NewTagRepositoryPostgres(testDB)
	tagSvc := application.NewTagService(tagRepo, event.NewEventBus())
	wRepo := repositories.NewWidgetRepositoryPostgres(testDB)
	wSvc := application.NewWidgetService(wRepo, event.NewEventBus())
	router := restapi.NewRouter(tagSvc, svc, wSvc)

	body, _ := json.Marshal(restdto.UpdateWidgetTypeRequest{
		Name: "x", HtmlTemplate: "<div/>", Script: "x", ScriptLanguage: "lua",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/widget-types/"+uuid.New().String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status: expected 409, got %d", rec.Code)
	}
}

// stubWidgetTypeService is a minimal WidgetTypeService stub for handler-level tests.
type stubWidgetTypeService struct {
	application.WidgetTypeService
	updateErr error
}

func (s *stubWidgetTypeService) UpdateWidgetType(_ context.Context, _ appdto.UpdateWidgetTypeInput) (appdto.WidgetType, error) {
	return appdto.WidgetType{}, s.updateErr
}

// --- DELETE /api/v1/widget-types/{id} ---

func TestDeleteWidgetTypesByID_Existing_Returns200WithDeletedItem(t *testing.T) {
	cleanWidgetTypes(t)
	created := createWidgetTypeViaService(t, testWtInput)
	router := newRouterWithWidgetTypes()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/widget-types/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var resp restdto.WidgetTypeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID != created.ID {
		t.Errorf("ID: expected %s, got %s", created.ID, resp.ID)
	}
}

func TestDeleteWidgetTypesByID_NotFound_Returns404(t *testing.T) {
	cleanWidgetTypes(t)
	router := newRouterWithWidgetTypes()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/widget-types/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}
}

func TestDeleteWidgetTypesByID_InvalidUUID_Returns400(t *testing.T) {
	router := newRouterWithWidgetTypes()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/widget-types/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}
