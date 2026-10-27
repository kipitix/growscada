package restapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/interface/restapi"
)

// wtLibrary is the library whose Draft the widget type tests work on, a new
// and empty one after each cleanWidgetTypes; wtPath is the path of its
// widget types.
var (
	wtLibrary uuid.UUID
	wtPath    string
)

func cleanWidgetTypes(t *testing.T) {
	t.Helper()
	created, err := newServices().libraries.CreateLibrary(context.Background(), appdto.LibraryInput{Name: "lib-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("cleanWidgetTypes: %v", err)
	}
	wtLibrary = created.ID
	wtPath = apiv0.PathPrefix + apiv0.LibraryPath(wtLibrary.String()) + "/widget-types"
}

func newRouterWithWidgetTypes() *restapi.APIRouter {
	return newServices().router()
}

func createWidgetTypeViaService(t *testing.T, input appdto.WidgetTypeInput) appdto.WidgetType {
	t.Helper()
	resp, err := newServices().widgetTypes.CreateWidgetType(context.Background(), wtLibrary, input)
	if err != nil {
		t.Fatalf("createWidgetTypeViaService: %v", err)
	}
	return resp
}

var testWtInput = appdto.WidgetTypeInput{
	Name:           "gauge",
	HtmlTemplate:   "<div class='gauge'><span class='value'></span></div>",
	Script:         "function render(v) { return v; }",
	ScriptLanguage: "javascript",
	DefaultWidth:   200,
	DefaultHeight:  150,
}

// --- GET /api/v0/libraries/{id}/widget-types ---

func TestGetWidgetTypes_EmptyDB_Returns200WithEmptyList(t *testing.T) {
	cleanWidgetTypes(t)
	router := newRouterWithWidgetTypes()

	req := httptest.NewRequest(http.MethodGet, wtPath, nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
	var resp apiv0.GetWidgetTypesResponse
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

	req := httptest.NewRequest(http.MethodGet, wtPath, nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
	var resp apiv0.GetWidgetTypesResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.WidgetTypes) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.WidgetTypes))
	}
}

// --- GET /api/v0/libraries/{id}/widget-types/{id} ---

func TestGetWidgetTypesByID_Existing_Returns200(t *testing.T) {
	cleanWidgetTypes(t)
	created := createWidgetTypeViaService(t, testWtInput)
	router := newRouterWithWidgetTypes()

	req := httptest.NewRequest(http.MethodGet, wtPath+"/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
	var resp apiv0.WidgetTypeResponse
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

	missingID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, wtPath+"/"+missingID, nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}
	var prob apiv0.Problem
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob.Type != restapi.TypeNotFound {
		t.Errorf("problem type: expected %q, got %q", restapi.TypeNotFound, prob.Type)
	}
	if want := "WidgetType with ID '" + missingID + "' not found"; prob.Detail != want {
		t.Errorf("detail: expected %q, got %q", want, prob.Detail)
	}
}

func TestGetWidgetTypesByID_InvalidUUID_Returns400(t *testing.T) {
	cleanWidgetTypes(t)
	router := newRouterWithWidgetTypes()

	req := httptest.NewRequest(http.MethodGet, wtPath+"/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

// --- POST /api/v0/widget-types ---

func TestPostWidgetTypes_Valid_Returns201WithID(t *testing.T) {
	cleanWidgetTypes(t)
	router := newRouterWithWidgetTypes()

	body, _ := json.Marshal(apiv0.CreateWidgetTypeRequest{
		Name:           "gauge",
		HtmlTemplate:   "<div class='gauge'></div>",
		Script:         "render()",
		ScriptLanguage: "javascript",
		DefaultWidth:   200,
		DefaultHeight:  150,
	})
	req := httptest.NewRequest(http.MethodPost, wtPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status: expected 201, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var resp apiv0.CreateWidgetTypeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID == uuid.Nil {
		t.Error("expected non-zero ID")
	}
	if resp.Version < 1 {
		t.Errorf("version: expected the new version, got %d", resp.Version)
	}
}

func TestPostWidgetTypes_InvalidJSON_Returns400(t *testing.T) {
	router := newRouterWithWidgetTypes()

	req := httptest.NewRequest(http.MethodPost, wtPath, bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

func TestPostWidgetTypes_InvalidScriptLanguage_Returns400(t *testing.T) {
	router := newRouterWithWidgetTypes()

	body, _ := json.Marshal(apiv0.CreateWidgetTypeRequest{
		Name:           "x",
		HtmlTemplate:   "<div/>",
		Script:         "x",
		ScriptLanguage: "ruby",
	})
	req := httptest.NewRequest(http.MethodPost, wtPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

// --- PUT /api/v0/libraries/{id}/widget-types/{id} ---

func TestPutWidgetTypesByID_Valid_Returns200WithVersion(t *testing.T) {
	cleanWidgetTypes(t)
	created := createWidgetTypeViaService(t, testWtInput)
	router := newRouterWithWidgetTypes()

	body, _ := json.Marshal(apiv0.UpdateWidgetTypeRequest{
		Name:           "updated-gauge",
		HtmlTemplate:   "<div class='updated'></div>",
		Script:         "print('hi')",
		ScriptLanguage: "python",
		DefaultWidth:   300,
		DefaultHeight:  200,
		Version:        created.Version,
	})
	req := httptest.NewRequest(http.MethodPut, wtPath+"/"+created.ID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var resp apiv0.UpdateWidgetTypeResponse
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

	body, _ := json.Marshal(apiv0.UpdateWidgetTypeRequest{
		Name: "x", HtmlTemplate: "<div/>", Script: "x", ScriptLanguage: "lua",
	})
	req := httptest.NewRequest(http.MethodPut, wtPath+"/"+uuid.New().String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}
}

func TestPutWidgetTypesByID_InvalidUUID_Returns400(t *testing.T) {
	cleanWidgetTypes(t)
	router := newRouterWithWidgetTypes()

	body, _ := json.Marshal(apiv0.UpdateWidgetTypeRequest{
		Name: "x", HtmlTemplate: "<div/>", Script: "x", ScriptLanguage: "lua",
	})
	req := httptest.NewRequest(http.MethodPut, wtPath+"/not-a-uuid", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

func TestPutWidgetTypesByID_Conflict_Returns409(t *testing.T) {
	cleanWidgetTypes(t)
	svc := &stubWidgetTypeService{updateErr: library.ErrWidgetTypeConflict}
	services := newServices()
	services.widgetTypes = svc
	router := services.router()

	body, _ := json.Marshal(apiv0.UpdateWidgetTypeRequest{
		Name: "x", HtmlTemplate: "<div/>", Script: "x", ScriptLanguage: "lua",
	})
	req := httptest.NewRequest(http.MethodPut, wtPath+"/"+uuid.New().String(), bytes.NewReader(body))
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

func (s *stubWidgetTypeService) UpdateWidgetType(_ context.Context, _, _ uuid.UUID, _ int, _ appdto.WidgetTypeInput) (appdto.WidgetType, error) {
	return appdto.WidgetType{}, s.updateErr
}

// --- DELETE /api/v0/libraries/{id}/widget-types/{id} ---

func TestDeleteWidgetTypesByID_Existing_Returns200WithDeletedItem(t *testing.T) {
	cleanWidgetTypes(t)
	created := createWidgetTypeViaService(t, testWtInput)
	router := newRouterWithWidgetTypes()

	req := httptest.NewRequest(http.MethodDelete, wtPath+"/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var resp apiv0.WidgetTypeResponse
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

	req := httptest.NewRequest(http.MethodDelete, wtPath+"/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}
}

func TestDeleteWidgetTypesByID_InvalidUUID_Returns400(t *testing.T) {
	cleanWidgetTypes(t)
	router := newRouterWithWidgetTypes()

	req := httptest.NewRequest(http.MethodDelete, wtPath+"/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

func TestPostWidgetTypes_InvalidInput_Returns400(t *testing.T) {
	valid := func() apiv0.CreateWidgetTypeRequest {
		return apiv0.CreateWidgetTypeRequest{
			Name: "gauge", HtmlTemplate: "<div/>", Script: "x", ScriptLanguage: "javascript",
			DefaultWidth: 100, DefaultHeight: 100,
		}
	}
	cases := map[string]func(*apiv0.CreateWidgetTypeRequest){
		"empty name":        func(r *apiv0.CreateWidgetTypeRequest) { r.Name = "" },
		"zero default size": func(r *apiv0.CreateWidgetTypeRequest) { r.DefaultWidth = 0 },
		"unknown type hint": func(r *apiv0.CreateWidgetTypeRequest) {
			r.InputPorts = []apiv0.InputPort{{Name: "value", TypeHint: "unknown"}}
		},
		"port name not JS id": func(r *apiv0.CreateWidgetTypeRequest) {
			r.InputPorts = []apiv0.InputPort{{Name: "1value", TypeHint: "integer"}}
		},
		"duplicate port name": func(r *apiv0.CreateWidgetTypeRequest) {
			r.InputPorts = []apiv0.InputPort{{Name: "value", TypeHint: "integer"}, {Name: "value", TypeHint: "string"}}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			request := valid()
			mutate(&request)
			body, _ := json.Marshal(request)
			req := httptest.NewRequest(http.MethodPost, wtPath, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			newRouterWithWidgetTypes().ServeMux().ServeHTTP(rec, req)

			assertBadRequest(t, rec)
		})
	}
}

func TestWidgetTypes_OfAMissingLibrary_Return404OfTheLibrary(t *testing.T) {
	router := newRouterWithWidgetTypes()
	missing := uuid.New().String()

	req := httptest.NewRequest(http.MethodGet, apiv0.PathPrefix+apiv0.LibraryPath(missing)+"/widget-types", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: expected 404, got %d", rec.Code)
	}
	var problem apiv0.Problem
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil || problem.Detail != "Library with ID '"+missing+"' not found" {
		t.Errorf("problem %+v, %v", problem, err)
	}
}
