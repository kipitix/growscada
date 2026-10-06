package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

func cleanScenes(t *testing.T) {
	t.Helper()
	// widgets cascade-delete with their scene (ON DELETE CASCADE on scene_id).
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM scenes"); err != nil {
		t.Fatalf("cleanScenes: %v", err)
	}
	ensureTestWidgetType(t)
}

// testWidgetTypeID is the WidgetType of the shared widget fixtures: a
// Widget's type must exist, so ensureTestWidgetType stores it.
var testWidgetTypeID = uuid.MustParse("7e57c0de-0000-4000-8000-000000000001")

func ensureTestWidgetType(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(),
		`INSERT INTO widget_types
		    (id, name, html_template, script, script_language, input_ports, default_width, default_height, version)
		 VALUES ($1, 'test-type', '<div></div>', 'function update(){}', 'javascript', '[]', 100, 100, 1)
		 ON CONFLICT (id) DO NOTHING`,
		testWidgetTypeID,
	); err != nil {
		t.Fatalf("ensureTestWidgetType: %v", err)
	}
}

func newSceneService() application.SceneService {
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	wtRepo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	return application.NewSceneService(repo, wtRepo, event.NewEventBus())
}

func newSceneServiceWithBus() (application.SceneService, event.EventBus) {
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	wtRepo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	bus := event.NewEventBus()
	return application.NewSceneService(repo, wtRepo, bus), bus
}

var testCreateSceneInput = appdto.SceneInput{
	Name:           "test-scene",
	Width:          1920,
	Height:         1080,
	BackgroundHTML: "",
}

// mustCreateScene creates a scene through the service under test and returns
// its DTO (ID and initial committed version).
func mustCreateScene(t *testing.T, svc application.SceneService) appdto.Scene {
	t.Helper()
	created, err := svc.CreateScene(context.Background(), testCreateSceneInput)
	if err != nil {
		t.Fatalf("CreateScene: %v", err)
	}
	return created
}

var testCreateWidgetInput = appdto.WidgetInput{
	Name:            "pressure-gauge",
	X:               10.0,
	Y:               20.0,
	Z:               0,
	Width:           100,
	Height:          100,
	OriginX:         0.5,
	OriginY:         0.5,
	RotationDegrees: 0.0,
	TypeID:          testWidgetTypeID,
	Labels:          []string{"sensor", "pressure"},
	PortBindings:    nil,
}

// ══════════════════════════════ Scene CRUD ═════════════════════════════════

func TestCreateScene_Valid_ReturnsID(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()

	resp, err := svc.CreateScene(context.Background(), testCreateSceneInput)

	if err != nil {
		t.Fatalf("CreateScene returned unexpected error: %v", err)
	}
	if resp.ID == uuid.Nil {
		t.Error("expected non-zero ID in response")
	}
}

func TestCreateScene_InvalidSize_ReturnsError(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()

	input := testCreateSceneInput
	input.Width = -1
	_, err := svc.CreateScene(context.Background(), input)

	if !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for invalid width, got: %v", err)
	}
}

func TestFindAllScenes_EmptyDB_ReturnsEmptyList(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()

	resp, err := svc.FindAllScenes(context.Background())

	if err != nil {
		t.Fatalf("FindAllScenes: %v", err)
	}
	if len(resp) != 0 {
		t.Errorf("expected 0 scenes, got %d", len(resp))
	}
}

func TestFindAllScenes_Multiple_ReturnsAll(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()

	input2 := testCreateSceneInput
	input2.Name = "second-scene"
	if _, err := svc.CreateScene(ctx, testCreateSceneInput); err != nil {
		t.Fatalf("CreateScene 1: %v", err)
	}
	if _, err := svc.CreateScene(ctx, input2); err != nil {
		t.Fatalf("CreateScene 2: %v", err)
	}

	resp, err := svc.FindAllScenes(ctx)

	if err != nil {
		t.Fatalf("FindAllScenes: %v", err)
	}
	if len(resp) != 2 {
		t.Errorf("expected 2 scenes, got %d", len(resp))
	}
}

func TestFindSceneByID_Existing_ReturnsCorrectFields(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()

	created := mustCreateScene(t, svc)

	found, err := svc.FindSceneByID(ctx, created.ID)

	if err != nil {
		t.Fatalf("FindSceneByID: %v", err)
	}
	if found.Name != testCreateSceneInput.Name {
		t.Errorf("Name: expected %q, got %q", testCreateSceneInput.Name, found.Name)
	}
}

func TestFindSceneByID_NotFound_ReturnsWrappedError(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()

	_, err := svc.FindSceneByID(context.Background(), uuid.New())

	if !errors.Is(err, scene.ErrSceneNotFound) {
		t.Errorf("expected wrapped ErrSceneNotFound, got: %v", err)
	}
}

func TestUpdateScene_Valid_ReturnsIncrementedVersion(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()

	created := mustCreateScene(t, svc)

	updated, err := svc.UpdateScene(ctx, created.ID, created.Version, appdto.SceneInput{
		Name: "renamed", Width: 800, Height: 600,
	})

	if err != nil {
		t.Fatalf("UpdateScene: %v", err)
	}
	if updated.Version <= created.Version {
		t.Errorf("Version: expected > %d, got %d", created.Version, updated.Version)
	}
}

func TestUpdateScene_StaleVersion_ReturnsConflict(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()

	created := mustCreateScene(t, svc)

	_, err := svc.UpdateScene(ctx, created.ID, created.Version+99, appdto.SceneInput{
		Name: "x", Width: 800, Height: 600,
	})

	if !errors.Is(err, scene.ErrSceneConflict) {
		t.Errorf("expected wrapped ErrSceneConflict, got: %v", err)
	}
}

func TestDeleteScene_Existing_ReturnsDeletedItem(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()

	created := mustCreateScene(t, svc)

	deleted, err := svc.DeleteSceneByID(ctx, created.ID)

	if err != nil {
		t.Fatalf("DeleteSceneByID: %v", err)
	}
	if deleted.ID != created.ID {
		t.Errorf("ID: expected %s, got %s", created.ID, deleted.ID)
	}
}

func TestDeleteScene_NotFound_ReturnsWrappedError(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()

	_, err := svc.DeleteSceneByID(context.Background(), uuid.New())

	if !errors.Is(err, scene.ErrSceneNotFound) {
		t.Errorf("expected wrapped ErrSceneNotFound, got: %v", err)
	}
}

// ═════════════════════════ Widget-via-Scene CRUD ═══════════════════════════

func TestCreateWidget_Valid_ReturnsNonNilID(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)

	input := testCreateWidgetInput
	resp, err := svc.CreateWidget(ctx, sc.ID, sc.Version, input)

	if err != nil {
		t.Fatalf("CreateWidget returned unexpected error: %v", err)
	}
	if resp.ID == uuid.Nil {
		t.Error("expected non-zero ID")
	}
}

func TestCreateWidget_Valid_BumpsSceneVersion(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)

	input := testCreateWidgetInput
	created, err := svc.CreateWidget(ctx, sc.ID, sc.Version, input)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	if created.SceneVersion <= sc.Version {
		t.Errorf("SceneVersion: expected > %d, got %d", sc.Version, created.SceneVersion)
	}
	refetched, err := svc.FindSceneByID(ctx, sc.ID)
	if err != nil {
		t.Fatalf("FindSceneByID: %v", err)
	}
	if refetched.Version != created.SceneVersion {
		t.Errorf("scene Version after create: expected %d, got %d", created.SceneVersion, refetched.Version)
	}
}

func TestCreateWidget_StaleSceneVersion_ReturnsConflict(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)

	input := testCreateWidgetInput
	_, err := svc.CreateWidget(ctx, sc.ID, sc.Version+99, input)

	if !errors.Is(err, scene.ErrSceneConflict) {
		t.Errorf("expected wrapped ErrSceneConflict, got: %v", err)
	}
}

// A stale write is a conflict even when the type it names is gone: the
// client must reload first, whatever else is wrong with the request.
func TestCreateWidget_StaleSceneVersionAndUnknownType_ReturnsConflict(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	sc := mustCreateScene(t, svc)

	input := testCreateWidgetInput
	input.TypeID = uuid.New()
	_, err := svc.CreateWidget(context.Background(), sc.ID, sc.Version+99, input)

	if !errors.Is(err, scene.ErrSceneConflict) {
		t.Errorf("expected wrapped ErrSceneConflict, got: %v", err)
	}
}

func TestCreateWidget_UnknownScene_ReturnsNotFound(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()

	input := testCreateWidgetInput
	_, err := svc.CreateWidget(context.Background(), uuid.New(), 1, input)

	if !errors.Is(err, scene.ErrSceneNotFound) {
		t.Errorf("expected wrapped ErrSceneNotFound, got: %v", err)
	}
}

func TestCreateWidget_EmptyName_ReturnsError(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)

	input := testCreateWidgetInput
	input.Name = ""
	_, err := svc.CreateWidget(ctx, sc.ID, sc.Version, input)

	if !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty name, got: %v", err)
	}
}

func TestCreateWidget_InvalidOrigin_ReturnsError(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)

	input := testCreateWidgetInput
	input.OriginX = 1.5 // out of [0,1]
	_, err := svc.CreateWidget(ctx, sc.ID, sc.Version, input)

	if !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for invalid origin, got: %v", err)
	}
}

func TestFindWidgetsBySceneID_EmptyScene_ReturnsEmptyList(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)

	resp, err := svc.FindWidgetsBySceneID(ctx, sc.ID)

	if err != nil {
		t.Fatalf("FindWidgetsBySceneID: %v", err)
	}
	if len(resp) != 0 {
		t.Errorf("expected 0 widgets, got %d", len(resp))
	}
}

func TestFindWidgetsBySceneID_MultipleWidgets_ReturnsOnlyThoseInScene(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	target := mustCreateScene(t, svc)
	other := mustCreateScene(t, svc)

	inTarget := testCreateWidgetInput
	inTarget.Name = "widget-in-target"
	if _, err := svc.CreateWidget(ctx, target.ID, target.Version, inTarget); err != nil {
		t.Fatalf("CreateWidget (target): %v", err)
	}

	inOther := testCreateWidgetInput
	inOther.Name = "widget-in-other"
	if _, err := svc.CreateWidget(ctx, other.ID, other.Version, inOther); err != nil {
		t.Fatalf("CreateWidget (other): %v", err)
	}

	resp, err := svc.FindWidgetsBySceneID(ctx, target.ID)

	if err != nil {
		t.Fatalf("FindWidgetsBySceneID: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(resp))
	}
	if resp[0].SceneID != target.ID {
		t.Errorf("SceneID: expected %v, got %v", target.ID, resp[0].SceneID)
	}
	if resp[0].Name != "widget-in-target" {
		t.Errorf("Name: expected 'widget-in-target', got %q", resp[0].Name)
	}
}

func TestFindWidgetByID_Existing_ReturnsCorrectFields(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)

	input := testCreateWidgetInput
	created, err := svc.CreateWidget(ctx, sc.ID, sc.Version, input)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	found, err := svc.FindWidgetByID(ctx, sc.ID, created.ID)

	if err != nil {
		t.Fatalf("FindWidgetByID: %v", err)
	}
	if found.Name != testCreateWidgetInput.Name {
		t.Errorf("Name: expected %q, got %q", testCreateWidgetInput.Name, found.Name)
	}
	if found.TransformMatrix.CSS == "" {
		t.Error("expected non-empty TransformMatrix.CSS")
	}
}

func TestFindWidgetByID_NotFound_ReturnsWrappedError(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)

	_, err := svc.FindWidgetByID(ctx, sc.ID, uuid.New())

	if !errors.Is(err, scene.ErrWidgetNotFound) {
		t.Errorf("expected wrapped ErrWidgetNotFound, got: %v", err)
	}
}

func TestUpdateWidget_Valid_BumpsSceneVersion(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)

	input := testCreateWidgetInput
	created, err := svc.CreateWidget(ctx, sc.ID, sc.Version, input)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	updateInput := appdto.WidgetInput{
		Name: "updated-gauge", X: 5, Y: 15, Width: 100, Height: 100,
		OriginX: 0.5, OriginY: 0.5, TypeID: created.TypeID,
	}
	updated, err := svc.UpdateWidget(ctx, sc.ID, created.ID, created.SceneVersion, updateInput)

	if err != nil {
		t.Fatalf("UpdateWidget: %v", err)
	}
	if updated.SceneVersion <= created.SceneVersion {
		t.Errorf("SceneVersion: expected > %d, got %d", created.SceneVersion, updated.SceneVersion)
	}
	if updated.Name != "updated-gauge" {
		t.Errorf("Name: expected 'updated-gauge', got %q", updated.Name)
	}
}

func TestUpdateWidget_StaleSceneVersion_ReturnsConflict(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)

	input := testCreateWidgetInput
	created, err := svc.CreateWidget(ctx, sc.ID, sc.Version, input)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	_, err = svc.UpdateWidget(ctx, sc.ID, created.ID, created.SceneVersion+99, appdto.WidgetInput{
		Name: "stale", TypeID: created.TypeID, OriginX: 0.5, OriginY: 0.5,
		Width: 100, Height: 100,
	})

	if !errors.Is(err, scene.ErrSceneConflict) {
		t.Errorf("expected wrapped ErrSceneConflict, got: %v", err)
	}
}

func TestUpdateWidget_StaleSceneVersionAndUnknownType_ReturnsConflict(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)
	created, err := svc.CreateWidget(ctx, sc.ID, sc.Version, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	input := testCreateWidgetInput
	input.TypeID = uuid.New()
	_, err = svc.UpdateWidget(ctx, sc.ID, created.ID, created.SceneVersion+99, input)

	if !errors.Is(err, scene.ErrSceneConflict) {
		t.Errorf("expected wrapped ErrSceneConflict, got: %v", err)
	}
}

func TestUpdateWidget_NotFound_ReturnsWrappedError(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)

	_, err := svc.UpdateWidget(ctx, sc.ID, uuid.New(), sc.Version, appdto.WidgetInput{
		Name: "x", TypeID: testWidgetTypeID, OriginX: 0.5, OriginY: 0.5,
		Width: 100, Height: 100,
	})

	if !errors.Is(err, scene.ErrWidgetNotFound) {
		t.Errorf("expected wrapped ErrWidgetNotFound, got: %v", err)
	}
}

func TestDeleteWidget_Existing_RemovedFromDB(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)

	input := testCreateWidgetInput
	created, err := svc.CreateWidget(ctx, sc.ID, sc.Version, input)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	if _, err = svc.DeleteWidgetByID(ctx, sc.ID, created.ID); err != nil {
		t.Fatalf("DeleteWidgetByID: %v", err)
	}

	_, err = svc.FindWidgetByID(ctx, sc.ID, created.ID)
	if !errors.Is(err, scene.ErrWidgetNotFound) {
		t.Errorf("expected ErrWidgetNotFound after delete, got: %v", err)
	}
}

func TestDeleteWidget_NotFound_ReturnsWrappedError(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)

	_, err := svc.DeleteWidgetByID(ctx, sc.ID, uuid.New())

	if !errors.Is(err, scene.ErrWidgetNotFound) {
		t.Errorf("expected wrapped ErrWidgetNotFound, got: %v", err)
	}
}

// ══════════════════════════ Cascade delete + events ════════════════════════

func TestDeleteSceneByID_WithWidgets_CascadesInDB(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)

	input := testCreateWidgetInput
	created, err := svc.CreateWidget(ctx, sc.ID, sc.Version, input)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	if _, err := svc.DeleteSceneByID(ctx, sc.ID); err != nil {
		t.Fatalf("DeleteSceneByID: %v", err)
	}

	_, err = svc.FindWidgetByID(ctx, sc.ID, created.ID)
	if !errors.Is(err, scene.ErrSceneNotFound) {
		t.Errorf("expected ErrSceneNotFound for widget lookup after scene delete, got: %v", err)
	}
}

// TestDeleteSceneByID_WithWidgets_PublishesWidgetDeletedPerWidget is the
// regression test for the original architecture problem: deleting a scene
// must not silently drop its widgets from the event/audit trail just because
// the DB cascade handles their removal.
func TestDeleteSceneByID_WithWidgets_PublishesWidgetDeletedPerWidget(t *testing.T) {
	cleanScenes(t)
	svc, bus := newSceneServiceWithBus()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)

	firstInput := testCreateWidgetInput
	firstInput.Name = "widget-1"
	first, err := svc.CreateWidget(ctx, sc.ID, sc.Version, firstInput)
	if err != nil {
		t.Fatalf("CreateWidget 1: %v", err)
	}

	secondInput := testCreateWidgetInput
	secondInput.Name = "widget-2"
	second, err := svc.CreateWidget(ctx, sc.ID, first.SceneVersion, secondInput)
	if err != nil {
		t.Fatalf("CreateWidget 2: %v", err)
	}

	var widgetDeleted []event.Event
	bus.Subscribe(event.EventTypeWidgetDeleted, func(e event.Event) {
		widgetDeleted = append(widgetDeleted, e)
	})
	var sceneDeleted []event.Event
	bus.Subscribe(event.EventTypeSceneDeleted, func(e event.Event) {
		sceneDeleted = append(sceneDeleted, e)
	})

	if _, err := svc.DeleteSceneByID(ctx, sc.ID); err != nil {
		t.Fatalf("DeleteSceneByID: %v", err)
	}

	if len(widgetDeleted) != 2 {
		t.Fatalf("expected 2 WidgetDeletedEvent, got %d", len(widgetDeleted))
	}
	gotIDs := map[uuid.UUID]bool{}
	for _, e := range widgetDeleted {
		wEvent, ok := e.(event.WidgetEvent)
		if !ok {
			t.Fatal("expected event to implement WidgetEvent")
		}
		gotIDs[wEvent.WidgetID().UUID()] = true
	}
	if !gotIDs[first.ID] || !gotIDs[second.ID] {
		t.Errorf("expected WidgetDeletedEvent for both %s and %s, got %v", first.ID, second.ID, gotIDs)
	}

	if len(sceneDeleted) != 1 {
		t.Fatalf("expected 1 SceneDeletedEvent, got %d", len(sceneDeleted))
	}
}

func TestDeleteSceneByID_EmptyScene_PublishesOnlySceneDeletedEvent(t *testing.T) {
	cleanScenes(t)
	svc, bus := newSceneServiceWithBus()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)

	var widgetDeleted []event.Event
	bus.Subscribe(event.EventTypeWidgetDeleted, func(e event.Event) {
		widgetDeleted = append(widgetDeleted, e)
	})

	if _, err := svc.DeleteSceneByID(ctx, sc.ID); err != nil {
		t.Fatalf("DeleteSceneByID: %v", err)
	}

	if len(widgetDeleted) != 0 {
		t.Errorf("expected no WidgetDeletedEvent for an empty scene, got %d", len(widgetDeleted))
	}
}

func TestCreateWidget_UnknownType_ReturnsInvalidInput(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	sc := mustCreateScene(t, svc)

	input := testCreateWidgetInput
	input.TypeID = uuid.New()
	_, err := svc.CreateWidget(context.Background(), sc.ID, sc.Version, input)

	if !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got: %v", err)
	}
}

func TestUpdateWidget_UnknownType_ReturnsInvalidInput(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)
	input := testCreateWidgetInput
	created, err := svc.CreateWidget(ctx, sc.ID, sc.Version, input)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	_, err = svc.UpdateWidget(ctx, sc.ID, created.ID, created.SceneVersion, appdto.WidgetInput{
		Name: "x", TypeID: uuid.New(), OriginX: 0.5, OriginY: 0.5,
		Width: 100, Height: 100,
	})

	if !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got: %v", err)
	}
}

func TestCreateWidget_UnknownSceneAndType_ReturnsSceneNotFound(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()

	input := testCreateWidgetInput
	input.TypeID = uuid.New()
	input.PortBindings = []appdto.PortBinding{{PortName: "value", TagID: uuid.New()}}
	_, err := svc.CreateWidget(context.Background(), uuid.New(), 1, input)

	if !errors.Is(err, scene.ErrSceneNotFound) {
		t.Errorf("expected wrapped ErrSceneNotFound, got: %v", err)
	}
	if errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("expected the missing scene to win over the unknown type, got: %v", err)
	}
}

func TestUpdateWidget_UnknownWidgetAndType_ReturnsWidgetNotFound(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	sc := mustCreateScene(t, svc)

	_, err := svc.UpdateWidget(context.Background(), sc.ID, uuid.New(), sc.Version, appdto.WidgetInput{
		Name: "x", TypeID: uuid.New(), OriginX: 0.5, OriginY: 0.5,
		Width: 100, Height: 100,
	})

	if !errors.Is(err, scene.ErrWidgetNotFound) {
		t.Errorf("expected wrapped ErrWidgetNotFound, got: %v", err)
	}
	if errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("expected the missing widget to win over the unknown type, got: %v", err)
	}
}

func TestCreateWidget_UnknownTypeWithBindings_ReturnsInvalidInput(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	sc := mustCreateScene(t, svc)

	input := testCreateWidgetInput
	input.TypeID = uuid.New()
	input.PortBindings = []appdto.PortBinding{{PortName: "value", TagID: uuid.New()}}
	_, err := svc.CreateWidget(context.Background(), sc.ID, sc.Version, input)

	if !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got: %v", err)
	}
}

func TestCreateWidget_UndeclaredPort_ReturnsInvalidInput(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	sc := mustCreateScene(t, svc)

	input := testCreateWidgetInput
	input.PortBindings = []appdto.PortBinding{{PortName: "missing", TagID: uuid.New()}}
	_, err := svc.CreateWidget(context.Background(), sc.ID, sc.Version, input)

	if !errors.Is(err, application.ErrInvalidInput) || !errors.Is(err, scene.ErrPortNotDeclared) {
		t.Errorf("expected ErrInvalidInput wrapping ErrPortNotDeclared, got: %v", err)
	}
}

func TestDeleteWidget_Existing_BumpsSceneVersionOnce(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)
	created, err := svc.CreateWidget(ctx, sc.ID, sc.Version, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	deleted, err := svc.DeleteWidgetByID(ctx, sc.ID, created.ID)

	if err != nil {
		t.Fatalf("DeleteWidgetByID: %v", err)
	}
	if deleted.SceneVersion != created.SceneVersion+1 {
		t.Errorf("SceneVersion: expected %d, got %d", created.SceneVersion+1, deleted.SceneVersion)
	}
	found, err := svc.FindSceneByID(ctx, sc.ID)
	if err != nil {
		t.Fatalf("FindSceneByID: %v", err)
	}
	if found.Version != deleted.SceneVersion {
		t.Errorf("stored version: expected %d, got %d", deleted.SceneVersion, found.Version)
	}
}

// racingSceneRepository runs race once, right after the first FindByID: a
// write by someone else lands between the service's read and its Save.
type racingSceneRepository struct {
	scene.SceneRepository
	race func()
}

func (r *racingSceneRepository) FindByID(ctx context.Context, sceneID id.ID[scene.Scene]) (scene.Scene, error) {
	found, err := r.SceneRepository.FindByID(ctx, sceneID)
	if race := r.race; race != nil {
		r.race = nil
		race()
	}
	return found, err
}

func TestDeleteWidget_SceneChangedMeanwhile_ReturnsConflictKeepsChange(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)
	created, err := svc.CreateWidget(ctx, sc.ID, sc.Version, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	repo := &racingSceneRepository{SceneRepository: repositories.NewSceneRepositoryPostgres(testDB)}
	repo.race = func() {
		if _, err := svc.UpdateScene(ctx, sc.ID, created.SceneVersion, appdto.SceneInput{Name: "renamed", Width: 800, Height: 600}); err != nil {
			t.Fatalf("concurrent UpdateScene: %v", err)
		}
	}
	racingSvc := application.NewSceneService(repo, repositories.NewWidgetTypeRepositoryPostgres(testDB), event.NewEventBus())

	_, err = racingSvc.DeleteWidgetByID(ctx, sc.ID, created.ID)

	if !errors.Is(err, scene.ErrSceneConflict) {
		t.Fatalf("expected ErrSceneConflict, got: %v", err)
	}
	found, err := svc.FindSceneByID(ctx, sc.ID)
	if err != nil {
		t.Fatalf("FindSceneByID: %v", err)
	}
	if found.Name != "renamed" || found.Version != created.SceneVersion+1 {
		t.Errorf("expected the concurrent change kept (renamed, version %d), got %q, version %d", created.SceneVersion+1, found.Name, found.Version)
	}
	if _, err := svc.FindWidgetByID(ctx, sc.ID, created.ID); err != nil {
		t.Errorf("widget must survive the refused delete: %v", err)
	}
}

// racingWidgetTypeRepository runs race once, right after the first FindByID:
// the type is changed or deleted between the service's check and its Save.
type racingWidgetTypeRepository struct {
	library.WidgetTypeRepository
	race func()
}

func (r *racingWidgetTypeRepository) FindByID(ctx context.Context, typeID id.ID[library.WidgetType]) (library.WidgetType, error) {
	found, err := r.WidgetTypeRepository.FindByID(ctx, typeID)
	if race := r.race; race != nil {
		r.race = nil
		race()
	}
	return found, err
}

// racingTypeID is a WidgetType the race deletes; the shared fixture type stays.
var racingTypeID = uuid.MustParse("7e57c0de-0000-4000-8000-000000000002")

func insertRacingWidgetType(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(),
		`INSERT INTO widget_types
		    (id, name, html_template, script, script_language, input_ports, default_width, default_height, version)
		 VALUES ($1, 'racing-type', '<div></div>', 'function update(){}', 'javascript', '[]', 100, 100, 1)
		 ON CONFLICT (id) DO NOTHING`,
		racingTypeID,
	); err != nil {
		t.Fatalf("insertRacingWidgetType: %v", err)
	}
}

func newServiceDeletingRacingType(t *testing.T) application.SceneService {
	t.Helper()
	wtRepo := &racingWidgetTypeRepository{WidgetTypeRepository: repositories.NewWidgetTypeRepositoryPostgres(testDB)}
	wtRepo.race = func() {
		if _, err := testDB.ExecContext(context.Background(), "DELETE FROM widget_types WHERE id = $1", racingTypeID); err != nil {
			t.Fatalf("concurrent delete of widget type: %v", err)
		}
	}
	return application.NewSceneService(repositories.NewSceneRepositoryPostgres(testDB), wtRepo, event.NewEventBus())
}

func TestCreateWidget_TypeDeletedMeanwhile_ReturnsInvalidInput(t *testing.T) {
	cleanScenes(t)
	insertRacingWidgetType(t)
	sc := mustCreateScene(t, newSceneService())
	svc := newServiceDeletingRacingType(t)

	input := testCreateWidgetInput
	input.TypeID = racingTypeID
	_, err := svc.CreateWidget(context.Background(), sc.ID, sc.Version, input)

	if !errors.Is(err, application.ErrInvalidInput) || !errors.Is(err, library.ErrWidgetTypeNotFound) {
		t.Errorf("expected ErrInvalidInput wrapping ErrWidgetTypeNotFound, got: %v", err)
	}
}

func TestUpdateWidget_TypeDeletedMeanwhile_ReturnsInvalidInput(t *testing.T) {
	cleanScenes(t)
	insertRacingWidgetType(t)
	plain := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, plain)
	created, err := plain.CreateWidget(ctx, sc.ID, sc.Version, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}
	svc := newServiceDeletingRacingType(t)

	input := testCreateWidgetInput
	input.TypeID = racingTypeID
	_, err = svc.UpdateWidget(ctx, sc.ID, created.ID, created.SceneVersion, input)

	if !errors.Is(err, application.ErrInvalidInput) || !errors.Is(err, library.ErrWidgetTypeNotFound) {
		t.Errorf("expected ErrInvalidInput wrapping ErrWidgetTypeNotFound, got: %v", err)
	}
}
