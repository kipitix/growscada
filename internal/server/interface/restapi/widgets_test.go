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
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
	"github.com/kipitix/growscada/internal/server/interface/restapi"
)

func cleanWidgetsRest(t *testing.T) {
	t.Helper()
	// ON DELETE CASCADE removes widgets with their scene, which is fine here —
	// each test creates the scene(s) it needs.
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM scenes"); err != nil {
		t.Fatalf("cleanWidgetsRest: %v", err)
	}
}

func newRouterWithWidgets() *restapi.APIRouter {
	tagRepo := repositories.NewTagRepositoryPostgres(testDB)
	tagSvc := application.NewTagService(tagRepo, event.NewEventBus())
	wtRepo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	sceneRepo := repositories.NewSceneRepositoryPostgres(testDB)
	wtSvc := application.NewWidgetTypeService(wtRepo, sceneRepo, event.NewEventBus())
	sceneSvc := application.NewSceneService(sceneRepo, wtRepo, event.NewEventBus())
	return restapi.NewRouter(tagSvc, wtSvc, sceneSvc, event.NewEventBus(), 100)
}

func createSceneViaSceneService(t *testing.T) appdto.Scene {
	t.Helper()
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	wtRepo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	svc := application.NewSceneService(repo, wtRepo, event.NewEventBus())
	resp, err := svc.CreateScene(context.Background(), appdto.CreateSceneInput{
		Name: "test-scene-" + uuid.New().String(), Width: 1920, Height: 1080,
	})
	if err != nil {
		t.Fatalf("createSceneViaSceneService: %v", err)
	}
	return resp
}

func createWidgetViaService(t *testing.T, sceneID uuid.UUID, input appdto.CreateWidgetInput) appdto.Widget {
	t.Helper()
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	wtRepo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	svc := application.NewSceneService(repo, wtRepo, event.NewEventBus())
	resp, err := svc.CreateWidget(context.Background(), sceneID, input)
	if err != nil {
		t.Fatalf("createWidgetViaService: %v", err)
	}
	return resp
}

var testWidgetInput = appdto.CreateWidgetInput{
	Name:            "pressure-gauge",
	X:               10.0,
	Y:               20.0,
	Z:               0,
	Width:           100,
	Height:          100,
	OriginX:         0.5,
	OriginY:         0.5,
	RotationDegrees: 0.0,
	TypeID:          uuid.New(),
	Labels:          []string{"sensor"},
	PortBindings:    nil,
}

// --- GET /api/v0/scenes/{id}/widgets ---

func TestGetWidgetsBySceneID_EmptyScene_Returns200WithEmptyList(t *testing.T) {
	cleanWidgetsRest(t)
	sc := createSceneViaSceneService(t)
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodGet, "/api/v0/scenes/"+sc.ID.String()+"/widgets", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
	var resp apiv0.GetWidgetsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Widgets) != 0 {
		t.Errorf("expected 0 items, got %d", len(resp.Widgets))
	}
}

func TestGetWidgetsBySceneID_UnknownScene_Returns404(t *testing.T) {
	cleanWidgetsRest(t)
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodGet, "/api/v0/scenes/"+uuid.New().String()+"/widgets", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}
}

func TestGetWidgetsBySceneID_WithItems_Returns200WithAll(t *testing.T) {
	cleanWidgetsRest(t)
	sc := createSceneViaSceneService(t)
	input := testWidgetInput
	input.SceneVersion = sc.Version
	created := createWidgetViaService(t, sc.ID, input)
	input2 := testWidgetInput
	input2.Name = "thermometer"
	input2.SceneVersion = created.SceneVersion
	createWidgetViaService(t, sc.ID, input2)
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodGet, "/api/v0/scenes/"+sc.ID.String()+"/widgets", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
	var resp apiv0.GetWidgetsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Widgets) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Widgets))
	}
}

func TestGetWidgetsBySceneID_InvalidUUID_Returns400(t *testing.T) {
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodGet, "/api/v0/scenes/not-a-uuid/widgets", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

func TestGetWidgetsBySceneID_TwoScenes_ReturnsOnlyMatchingScene(t *testing.T) {
	cleanWidgetsRest(t)
	target := createSceneViaSceneService(t)
	other := createSceneViaSceneService(t)

	inTarget := testWidgetInput
	inTarget.Name = "widget-in-target"
	inTarget.SceneVersion = target.Version
	createWidgetViaService(t, target.ID, inTarget)

	inOther := testWidgetInput
	inOther.Name = "widget-in-other"
	inOther.SceneVersion = other.Version
	createWidgetViaService(t, other.ID, inOther)

	router := newRouterWithWidgets()
	req := httptest.NewRequest(http.MethodGet, "/api/v0/scenes/"+target.ID.String()+"/widgets", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var resp apiv0.GetWidgetsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Widgets) != 1 {
		t.Errorf("expected 1 widget for target scene, got %d", len(resp.Widgets))
	}
	if len(resp.Widgets) == 1 && resp.Widgets[0].SceneID != target.ID {
		t.Errorf("SceneID: expected %v, got %v", target.ID, resp.Widgets[0].SceneID)
	}
}

// --- GET /api/v0/scenes/{sceneId}/widgets/{widgetId} ---

func TestGetWidgetsByID_Existing_Returns200(t *testing.T) {
	cleanWidgetsRest(t)
	sc := createSceneViaSceneService(t)
	input := testWidgetInput
	input.SceneVersion = sc.Version
	created := createWidgetViaService(t, sc.ID, input)
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodGet, "/api/v0/scenes/"+sc.ID.String()+"/widgets/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
	var resp apiv0.WidgetResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != testWidgetInput.Name {
		t.Errorf("Name: expected %q, got %q", testWidgetInput.Name, resp.Name)
	}
	if resp.Position.X != testWidgetInput.X {
		t.Errorf("Position.X: expected %v, got %v", testWidgetInput.X, resp.Position.X)
	}
	if resp.TransformMatrix.CSS == "" {
		t.Error("expected non-empty TransformMatrix.CSS")
	}
}

func TestGetWidgetsByID_NotFound_Returns404(t *testing.T) {
	cleanWidgetsRest(t)
	sc := createSceneViaSceneService(t)
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodGet, "/api/v0/scenes/"+sc.ID.String()+"/widgets/"+uuid.New().String(), nil)
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

	req := httptest.NewRequest(http.MethodGet, "/api/v0/scenes/"+uuid.New().String()+"/widgets/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

// --- POST /api/v0/scenes/{id}/widgets ---

func TestPostWidgets_Valid_Returns201WithID(t *testing.T) {
	cleanWidgetsRest(t)
	sc := createSceneViaSceneService(t)
	router := newRouterWithWidgets()

	body, _ := json.Marshal(apiv0.CreateWidgetRequest{
		Name:         "flow-meter",
		Position:     apiv0.PositionRequest{X: 5, Y: 10, Z: 0},
		Size:         apiv0.SizeRequest{Width: 100, Height: 100},
		Origin:       apiv0.OriginRequest{X: 0.5, Y: 0.5},
		Rotation:     apiv0.RotationRequest{Degrees: 0},
		TypeID:       uuid.New(),
		SceneVersion: sc.Version,
		Labels:       []string{"flow"},
		PortBindings: []apiv0.PortBinding{},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v0/scenes/"+sc.ID.String()+"/widgets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status: expected 201, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var resp apiv0.CreateWidgetResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID == uuid.Nil {
		t.Error("expected non-zero ID")
	}
	if resp.SceneVersion <= sc.Version {
		t.Errorf("SceneVersion: expected > %d, got %d", sc.Version, resp.SceneVersion)
	}
}

func TestPostWidgets_InvalidJSON_Returns400(t *testing.T) {
	sc := createSceneViaSceneService(t)
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodPost, "/api/v0/scenes/"+sc.ID.String()+"/widgets", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

func TestPostWidgets_EmptyName_Returns500(t *testing.T) {
	sc := createSceneViaSceneService(t)
	router := newRouterWithWidgets()

	body, _ := json.Marshal(apiv0.CreateWidgetRequest{
		Name:         "",
		Position:     apiv0.PositionRequest{X: 0, Y: 0, Z: 0},
		Size:         apiv0.SizeRequest{Width: 100, Height: 100},
		Origin:       apiv0.OriginRequest{X: 0.5, Y: 0.5},
		TypeID:       uuid.New(),
		SceneVersion: sc.Version,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v0/scenes/"+sc.ID.String()+"/widgets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status: expected 500, got %d", rec.Code)
	}
}

func TestPostWidgets_StaleSceneVersion_Returns409(t *testing.T) {
	cleanWidgetsRest(t)
	sc := createSceneViaSceneService(t)
	router := newRouterWithWidgets()

	body, _ := json.Marshal(apiv0.CreateWidgetRequest{
		Name:         "flow-meter",
		Position:     apiv0.PositionRequest{X: 5, Y: 10, Z: 0},
		Size:         apiv0.SizeRequest{Width: 100, Height: 100},
		Origin:       apiv0.OriginRequest{X: 0.5, Y: 0.5},
		TypeID:       uuid.New(),
		SceneVersion: sc.Version + 99,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v0/scenes/"+sc.ID.String()+"/widgets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status: expected 409, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
}

// --- PUT /api/v0/scenes/{sceneId}/widgets/{widgetId} ---

func TestPutWidgetsByID_Valid_Returns200WithIncrementedSceneVersion(t *testing.T) {
	cleanWidgetsRest(t)
	sc := createSceneViaSceneService(t)
	input := testWidgetInput
	input.SceneVersion = sc.Version
	created := createWidgetViaService(t, sc.ID, input)
	router := newRouterWithWidgets()

	body, _ := json.Marshal(apiv0.UpdateWidgetRequest{
		Name:         "updated-gauge",
		Position:     apiv0.PositionRequest{X: 1, Y: 2, Z: 3},
		Size:         apiv0.SizeRequest{Width: 100, Height: 100},
		Origin:       apiv0.OriginRequest{X: 0.5, Y: 0.5},
		Rotation:     apiv0.RotationRequest{Degrees: 0},
		TypeID:       uuid.New(),
		Labels:       []string{"updated"},
		PortBindings: []apiv0.PortBinding{},
		SceneVersion: created.SceneVersion,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v0/scenes/"+sc.ID.String()+"/widgets/"+created.ID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var resp apiv0.UpdateWidgetResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.SceneVersion <= created.SceneVersion {
		t.Errorf("SceneVersion: expected > %d, got %d", created.SceneVersion, resp.SceneVersion)
	}
}

func TestPutWidgetsByID_NotFound_Returns404(t *testing.T) {
	cleanWidgetsRest(t)
	sc := createSceneViaSceneService(t)
	router := newRouterWithWidgets()

	body, _ := json.Marshal(apiv0.UpdateWidgetRequest{
		Name:         "x",
		TypeID:       uuid.New(),
		Origin:       apiv0.OriginRequest{X: 0.5, Y: 0.5},
		Size:         apiv0.SizeRequest{Width: 100, Height: 100},
		SceneVersion: sc.Version,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v0/scenes/"+sc.ID.String()+"/widgets/"+uuid.New().String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}
}

func TestPutWidgetsByID_InvalidUUID_Returns400(t *testing.T) {
	router := newRouterWithWidgets()

	body, _ := json.Marshal(apiv0.UpdateWidgetRequest{
		Name:   "x",
		TypeID: uuid.New(),
		Origin: apiv0.OriginRequest{X: 0.5, Y: 0.5},
		Size:   apiv0.SizeRequest{Width: 100, Height: 100},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v0/scenes/"+uuid.New().String()+"/widgets/not-a-uuid", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

func TestPutWidgetsByID_Conflict_Returns409(t *testing.T) {
	svc := &stubSceneServiceForWidgets{updateErr: scene.ErrSceneConflict}
	tagRepo := repositories.NewTagRepositoryPostgres(testDB)
	tagSvc := application.NewTagService(tagRepo, event.NewEventBus())
	wtRepo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	sceneRepo := repositories.NewSceneRepositoryPostgres(testDB)
	wtSvc := application.NewWidgetTypeService(wtRepo, sceneRepo, event.NewEventBus())
	router := restapi.NewRouter(tagSvc, wtSvc, svc, event.NewEventBus(), 100)

	body, _ := json.Marshal(apiv0.UpdateWidgetRequest{
		Name:   "x",
		TypeID: uuid.New(),
		Origin: apiv0.OriginRequest{X: 0.5, Y: 0.5},
		Size:   apiv0.SizeRequest{Width: 100, Height: 100},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v0/scenes/"+uuid.New().String()+"/widgets/"+uuid.New().String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status: expected 409, got %d", rec.Code)
	}
}

// stubSceneServiceForWidgets is a minimal SceneService stub for widget
// handler-level tests.
type stubSceneServiceForWidgets struct {
	application.SceneService
	updateErr error
}

func (s *stubSceneServiceForWidgets) UpdateWidget(_ context.Context, _, _ uuid.UUID, _ appdto.UpdateWidgetInput) (appdto.Widget, error) {
	return appdto.Widget{}, s.updateErr
}

// --- DELETE /api/v0/scenes/{sceneId}/widgets/{widgetId} ---

func TestDeleteWidgetsByID_Existing_Returns200WithDeletedItem(t *testing.T) {
	cleanWidgetsRest(t)
	sc := createSceneViaSceneService(t)
	input := testWidgetInput
	input.SceneVersion = sc.Version
	created := createWidgetViaService(t, sc.ID, input)
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodDelete, "/api/v0/scenes/"+sc.ID.String()+"/widgets/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var resp apiv0.WidgetResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID != created.ID {
		t.Errorf("ID: expected %s, got %s", created.ID, resp.ID)
	}
}

func TestDeleteWidgetsByID_NotFound_Returns404(t *testing.T) {
	cleanWidgetsRest(t)
	sc := createSceneViaSceneService(t)
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodDelete, "/api/v0/scenes/"+sc.ID.String()+"/widgets/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}
}

func TestDeleteWidgetsByID_InvalidUUID_Returns400(t *testing.T) {
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodDelete, "/api/v0/scenes/"+uuid.New().String()+"/widgets/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

func TestDeleteWidgetsByID_Existing_RemovedFromDB(t *testing.T) {
	cleanWidgetsRest(t)
	sc := createSceneViaSceneService(t)
	input := testWidgetInput
	input.SceneVersion = sc.Version
	created := createWidgetViaService(t, sc.ID, input)
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodDelete, "/api/v0/scenes/"+sc.ID.String()+"/widgets/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("delete status: expected 200, got %d", rec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v0/scenes/"+sc.ID.String()+"/widgets/"+created.ID.String(), nil)
	getRec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Errorf("after delete GET: expected 404, got %d", getRec.Code)
	}
}

// --- Scene delete cascades widgets (audit-trail regression) ---

func TestDeleteScenesByID_WithWidgets_WidgetsAreGoneAfter(t *testing.T) {
	cleanWidgetsRest(t)
	sc := createSceneViaSceneService(t)
	input := testWidgetInput
	input.SceneVersion = sc.Version
	created := createWidgetViaService(t, sc.ID, input)
	router := newRouterWithWidgets()

	delReq := httptest.NewRequest(http.MethodDelete, "/api/v0/scenes/"+sc.ID.String(), nil)
	delRec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(delRec, delReq)

	if delRec.Code != http.StatusOK {
		t.Fatalf("delete scene status: expected 200, got %d", delRec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v0/scenes/"+sc.ID.String()+"/widgets/"+created.ID.String(), nil)
	getRec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Errorf("after scene delete, widget GET: expected 404, got %d", getRec.Code)
	}
}
