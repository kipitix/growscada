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
	"github.com/kipitix/growscada/internal/infrastructure/postgres/repositories"
	"github.com/kipitix/growscada/internal/interface/restapi"
	"github.com/kipitix/growscada/internal/interface/restapi/restdto"
)

func cleanIndicatorTypes(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM indicator_types"); err != nil {
		t.Fatalf("cleanIndicatorTypes: %v", err)
	}
}

func newRouterWithIndicatorTypes() *restapi.APIRouter {
	tagRepo := repositories.NewTagRepositoryPostgres(testDB)
	tagSvc := application.NewTagService(tagRepo, event.NewEventBus())
	itRepo := repositories.NewIndicatorTypeRepositoryPostgres(testDB)
	itSvc := application.NewIndicatorTypeService(itRepo, event.NewEventBus())
	wtRepo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	wtSvc := application.NewWidgetTypeService(wtRepo, event.NewEventBus())
	return restapi.NewRouter(tagSvc, itSvc, wtSvc)
}

func createIndicatorTypeViaService(t *testing.T, input appdto.CreateIndicatorTypeInput) appdto.IndicatorType {
	t.Helper()
	repo := repositories.NewIndicatorTypeRepositoryPostgres(testDB)
	svc := application.NewIndicatorTypeService(repo, event.NewEventBus())
	resp, err := svc.CreateIndicatorType(context.Background(), input)
	if err != nil {
		t.Fatalf("createIndicatorTypeViaService: %v", err)
	}
	return resp
}

var testItInput = appdto.CreateIndicatorTypeInput{
	Name:           "gauge",
	SvgTemplate:    "<svg><circle r='10'/></svg>",
	Script:         "function render(v) { return v; }",
	ScriptLanguage: "javascript",
}

// --- GET /api/v1/indicator-types ---

func TestGetIndicatorTypes_EmptyDB_Returns200WithEmptyList(t *testing.T) {
	cleanIndicatorTypes(t)
	router := newRouterWithIndicatorTypes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/indicator-types", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
	var resp restdto.GetIndicatorTypesResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.IndicatorTypes) != 0 {
		t.Errorf("expected 0 items, got %d", len(resp.IndicatorTypes))
	}
}

func TestGetIndicatorTypes_WithItems_Returns200WithAll(t *testing.T) {
	cleanIndicatorTypes(t)
	createIndicatorTypeViaService(t, testItInput)
	input2 := testItInput
	input2.Name = "thermometer"
	createIndicatorTypeViaService(t, input2)
	router := newRouterWithIndicatorTypes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/indicator-types", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
	var resp restdto.GetIndicatorTypesResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.IndicatorTypes) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.IndicatorTypes))
	}
}

// --- GET /api/v1/indicator-types/{id} ---

func TestGetIndicatorTypesByID_Existing_Returns200(t *testing.T) {
	cleanIndicatorTypes(t)
	created := createIndicatorTypeViaService(t, testItInput)
	router := newRouterWithIndicatorTypes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/indicator-types/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
	var resp restdto.IndicatorTypeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != testItInput.Name {
		t.Errorf("Name: expected %q, got %q", testItInput.Name, resp.Name)
	}
}

func TestGetIndicatorTypesByID_NotFound_Returns404(t *testing.T) {
	cleanIndicatorTypes(t)
	router := newRouterWithIndicatorTypes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/indicator-types/"+uuid.New().String(), nil)
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

func TestGetIndicatorTypesByID_InvalidUUID_Returns400(t *testing.T) {
	router := newRouterWithIndicatorTypes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/indicator-types/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

// --- POST /api/v1/indicator-types ---

func TestPostIndicatorTypes_Valid_Returns201WithID(t *testing.T) {
	cleanIndicatorTypes(t)
	router := newRouterWithIndicatorTypes()

	body, _ := json.Marshal(restdto.CreateIndicatorTypeRequest{
		Name:           "gauge",
		SvgTemplate:    "<svg/>",
		Script:         "render()",
		ScriptLanguage: "javascript",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/indicator-types", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status: expected 201, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var resp restdto.CreateIndicatorTypeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID == uuid.Nil {
		t.Error("expected non-zero ID")
	}
}

func TestPostIndicatorTypes_InvalidJSON_Returns400(t *testing.T) {
	router := newRouterWithIndicatorTypes()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/indicator-types", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

func TestPostIndicatorTypes_InvalidScriptLanguage_Returns500(t *testing.T) {
	router := newRouterWithIndicatorTypes()

	body, _ := json.Marshal(restdto.CreateIndicatorTypeRequest{
		Name:           "x",
		SvgTemplate:    "<svg/>",
		Script:         "x",
		ScriptLanguage: "ruby",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/indicator-types", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status: expected 500, got %d", rec.Code)
	}
}

// --- PUT /api/v1/indicator-types/{id} ---

func TestPutIndicatorTypesByID_Valid_Returns200WithVersion(t *testing.T) {
	cleanIndicatorTypes(t)
	created := createIndicatorTypeViaService(t, testItInput)
	router := newRouterWithIndicatorTypes()

	body, _ := json.Marshal(restdto.UpdateIndicatorTypeRequest{
		Name:           "updated-gauge",
		SvgTemplate:    "<svg><rect/></svg>",
		Script:         "print('hi')",
		ScriptLanguage: "python",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/indicator-types/"+created.ID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var resp restdto.UpdateIndicatorTypeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Version <= created.Version {
		t.Errorf("Version: expected > %d, got %d", created.Version, resp.Version)
	}
}

func TestPutIndicatorTypesByID_NotFound_Returns404(t *testing.T) {
	cleanIndicatorTypes(t)
	router := newRouterWithIndicatorTypes()

	body, _ := json.Marshal(restdto.UpdateIndicatorTypeRequest{
		Name: "x", SvgTemplate: "<svg/>", Script: "x", ScriptLanguage: "lua",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/indicator-types/"+uuid.New().String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}
}

func TestPutIndicatorTypesByID_InvalidUUID_Returns400(t *testing.T) {
	router := newRouterWithIndicatorTypes()

	body, _ := json.Marshal(restdto.UpdateIndicatorTypeRequest{
		Name: "x", SvgTemplate: "<svg/>", Script: "x", ScriptLanguage: "lua",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/indicator-types/not-a-uuid", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

// --- DELETE /api/v1/indicator-types/{id} ---

func TestDeleteIndicatorTypesByID_Existing_Returns200WithDeletedItem(t *testing.T) {
	cleanIndicatorTypes(t)
	created := createIndicatorTypeViaService(t, testItInput)
	router := newRouterWithIndicatorTypes()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/indicator-types/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var resp restdto.IndicatorTypeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID != created.ID {
		t.Errorf("ID: expected %s, got %s", created.ID, resp.ID)
	}
}

func TestDeleteIndicatorTypesByID_NotFound_Returns404(t *testing.T) {
	cleanIndicatorTypes(t)
	router := newRouterWithIndicatorTypes()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/indicator-types/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}
}

func TestDeleteIndicatorTypesByID_InvalidUUID_Returns400(t *testing.T) {
	router := newRouterWithIndicatorTypes()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/indicator-types/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}
