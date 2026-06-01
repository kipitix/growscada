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

// testSceneID is populated in TestMain before any test runs.
// It holds a real scene row so widget inserts satisfy the FK on scene_id.
var testSceneID uuid.UUID

// mustInsertScene inserts a minimal scene row so that widget inserts satisfy
// the FK constraint on scene_id. The row is cleaned up after the test.
func mustInsertScene(t *testing.T, sceneID uuid.UUID) {
	t.Helper()
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO scenes (id, name, width, height, background_html, version)
		 VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (id) DO NOTHING`,
		sceneID, "test-scene-"+sceneID.String(), 1920, 1080, "", 1,
	)
	if err != nil {
		t.Fatalf("mustInsertScene: %v", err)
	}
	t.Cleanup(func() {
		testDB.ExecContext(context.Background(), "DELETE FROM scenes WHERE id = $1", sceneID)
	})
}

func cleanWidgetsRest(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM widgets"); err != nil {
		t.Fatalf("cleanWidgetsRest: %v", err)
	}
	// cleanScenesRest (used by scene tests) deletes all scenes including testSceneID.
	// Re-insert it here so widget tests always have a valid scene_id to reference.
	if _, err := testDB.ExecContext(context.Background(),
		`INSERT INTO scenes (id, name, width, height, background_html, version)
		 VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (id) DO NOTHING`,
		testSceneID, "test-scene", 1920, 1080, "", 1,
	); err != nil {
		t.Fatalf("cleanWidgetsRest re-insert testSceneID: %v", err)
	}
}

func newRouterWithWidgets() *restapi.APIRouter {
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

func createWidgetViaService(t *testing.T, input appdto.CreateWidgetInput) appdto.Widget {
	t.Helper()
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	wtRepo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	svc := application.NewWidgetService(repo, wtRepo, event.NewEventBus())
	resp, err := svc.CreateWidget(context.Background(), input)
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
	Labels:       []string{"sensor"},
	PortBindings: nil,
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
	if resp.Position.X != testWidgetInput.X {
		t.Errorf("Position.X: expected %v, got %v", testWidgetInput.X, resp.Position.X)
	}
	if resp.TransformMatrix.CSS == "" {
		t.Error("expected non-empty TransformMatrix.CSS")
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
		Name:     "flow-meter",
		Position: restdto.PositionRequest{X: 5, Y: 10, Z: 0},
		Size:     restdto.SizeRequest{Width: 100, Height: 100},
		Origin:   restdto.OriginRequest{X: 0.5, Y: 0.5},
		Rotation: restdto.RotationRequest{Degrees: 0},
		TypeID:   uuid.New(),
		SceneID:  testSceneID,
		Labels:       []string{"flow"},
		PortBindings: []restdto.PortBindingDTO{},
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
		Name:     "",
		Position: restdto.PositionRequest{X: 0, Y: 0, Z: 0},
		Size:     restdto.SizeRequest{Width: 100, Height: 100},
		Origin:   restdto.OriginRequest{X: 0.5, Y: 0.5},
		TypeID:   uuid.New(),
		SceneID:  testSceneID,
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
		Name:     "updated-gauge",
		Position: restdto.PositionRequest{X: 1, Y: 2, Z: 3},
		Size:     restdto.SizeRequest{Width: 100, Height: 100},
		Origin:   restdto.OriginRequest{X: 0.5, Y: 0.5},
		Rotation: restdto.RotationRequest{Degrees: 0},
		TypeID:   uuid.New(),
		SceneID:  created.SceneID,
		Labels:       []string{"updated"},
		PortBindings: []restdto.PortBindingDTO{},
		Version:      created.Version,
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
		Name:    "x",
		TypeID:  uuid.New(),
		SceneID: testSceneID,
		Origin:  restdto.OriginRequest{X: 0.5, Y: 0.5},
		Size:    restdto.SizeRequest{Width: 100, Height: 100},
		Version: 1,
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

	body, _ := json.Marshal(restdto.UpdateWidgetRequest{
		Name:    "x",
		TypeID:  uuid.New(),
		Origin:  restdto.OriginRequest{X: 0.5, Y: 0.5},
		Size:    restdto.SizeRequest{Width: 100, Height: 100},
		Version: 1,
	})
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
	wRepo := repositories.NewWidgetRepositoryPostgres(testDB)
	wtSvc := application.NewWidgetTypeService(wtRepo, wRepo, event.NewEventBus())
	sceneRepo := repositories.NewSceneRepositoryPostgres(testDB)
	sceneSvc := application.NewSceneService(sceneRepo, event.NewEventBus())
	router := restapi.NewRouter(tagSvc, wtSvc, svc, sceneSvc)

	body, _ := json.Marshal(restdto.UpdateWidgetRequest{
		Name:    "x",
		TypeID:  uuid.New(),
		Origin:  restdto.OriginRequest{X: 0.5, Y: 0.5},
		Size:    restdto.SizeRequest{Width: 100, Height: 100},
		Version: 1,
	})
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

// --- GET /api/v1/scenes/{id}/widgets ---

func TestGetWidgetsBySceneID_EmptyScene_Returns200WithEmptyList(t *testing.T) {
	cleanWidgetsRest(t)
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scenes/"+uuid.New().String()+"/widgets", nil)
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

func TestGetWidgetsBySceneID_WithWidgets_ReturnsOnlyMatchingScene(t *testing.T) {
	cleanWidgetsRest(t)

	targetSceneID := uuid.New()
	otherSceneID := uuid.New()
	mustInsertScene(t, targetSceneID)
	mustInsertScene(t, otherSceneID)

	inTarget := testWidgetInput
	inTarget.Name = "widget-in-target"
	inTarget.SceneID = targetSceneID
	createWidgetViaService(t, inTarget)

	inOther := testWidgetInput
	inOther.Name = "widget-in-other"
	inOther.SceneID = otherSceneID
	createWidgetViaService(t, inOther)

	router := newRouterWithWidgets()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/scenes/"+targetSceneID.String()+"/widgets", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	var resp restdto.GetWidgetsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Widgets) != 1 {
		t.Errorf("expected 1 widget for target scene, got %d", len(resp.Widgets))
	}
	if len(resp.Widgets) == 1 && resp.Widgets[0].SceneID != targetSceneID {
		t.Errorf("SceneID: expected %v, got %v", targetSceneID, resp.Widgets[0].SceneID)
	}
}

func TestGetWidgetsBySceneID_InvalidUUID_Returns400(t *testing.T) {
	router := newRouterWithWidgets()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scenes/not-a-uuid/widgets", nil)
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
}

func TestGetWidgetsBySceneID_MultipleWidgets_ReturnsAll(t *testing.T) {
	cleanWidgetsRest(t)

	sceneID := uuid.New()
	otherSceneID := uuid.New()
	mustInsertScene(t, sceneID)
	mustInsertScene(t, otherSceneID)

	for _, name := range []string{"widget-a", "widget-b"} {
		input := testWidgetInput
		input.Name = name
		input.SceneID = sceneID
		createWidgetViaService(t, input)
	}
	// Also create a widget in another scene — should not appear.
	other := testWidgetInput
	other.Name = "widget-other"
	other.SceneID = otherSceneID
	createWidgetViaService(t, other)

	router := newRouterWithWidgets()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/scenes/"+sceneID.String()+"/widgets", nil)
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
		t.Errorf("expected 2 widgets, got %d", len(resp.Widgets))
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
