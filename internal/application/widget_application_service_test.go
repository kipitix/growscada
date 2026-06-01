package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/application"
	"github.com/kipitix/growscada/internal/application/appdto"
	"github.com/kipitix/growscada/internal/domain/event"
	"github.com/kipitix/growscada/internal/domain/widget"
	"github.com/kipitix/growscada/internal/infrastructure/postgres/repositories"
)

// testSceneID is populated in TestMain before any test runs.
// It holds the UUID of a scene row inserted into the DB so that widget
// inserts satisfy the FK constraint on widgets.scene_id.
var testSceneID uuid.UUID

func cleanWidgets(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM widgets"); err != nil {
		t.Fatalf("cleanWidgets: %v", err)
	}
}

// mustInsertScene inserts a minimal scene row directly into the DB so that
// widget inserts satisfy the FK constraint on widgets.scene_id.
func mustInsertScene(t *testing.T, sceneID uuid.UUID) {
	t.Helper()
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO scenes (id, name, width, height, background_html, version) VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (id) DO NOTHING`,
		sceneID, "test-scene-"+sceneID.String(), 1920, 1080, "", 1,
	)
	if err != nil {
		t.Fatalf("mustInsertScene: %v", err)
	}
	t.Cleanup(func() {
		testDB.ExecContext(context.Background(), "DELETE FROM scenes WHERE id = $1", sceneID)
	})
}

func newWidgetService() application.WidgetService {
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	return application.NewWidgetService(repo, event.NewEventBus())
}

func newWidgetServiceWithBus() (application.WidgetService, event.EventBus) {
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	bus := event.NewEventBus()
	return application.NewWidgetService(repo, bus), bus
}

// testCreateWidgetInput is a template used by widget tests.
// Its SceneID field is set to testSceneID in TestMain after the shared scene
// row has been inserted into the DB.
var testCreateWidgetInput = appdto.CreateWidgetInput{
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
	Labels:          []string{"sensor", "pressure"},
	PortBindings:    nil,
	// SceneID is assigned in TestMain once testSceneID is available.
}

// --- CreateWidget ---

func TestCreateWidget_Valid_ReturnsNonNilID(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()

	resp, err := svc.CreateWidget(context.Background(), testCreateWidgetInput)

	if err != nil {
		t.Fatalf("CreateWidget returned unexpected error: %v", err)
	}
	if resp.ID == uuid.Nil {
		t.Error("expected non-zero ID")
	}
}

func TestCreateWidget_Valid_FieldsAreStored(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()
	ctx := context.Background()

	created, err := svc.CreateWidget(ctx, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	found, err := svc.FindWidgetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindWidgetByID: %v", err)
	}

	if found.Name != testCreateWidgetInput.Name {
		t.Errorf("Name: expected %q, got %q", testCreateWidgetInput.Name, found.Name)
	}
	if found.X != testCreateWidgetInput.X {
		t.Errorf("X: expected %v, got %v", testCreateWidgetInput.X, found.X)
	}
	if found.TypeID != testCreateWidgetInput.TypeID {
		t.Errorf("TypeID: expected %v, got %v", testCreateWidgetInput.TypeID, found.TypeID)
	}
	if found.OriginX != testCreateWidgetInput.OriginX {
		t.Errorf("OriginX: expected %v, got %v", testCreateWidgetInput.OriginX, found.OriginX)
	}
	if found.RotationDegrees != testCreateWidgetInput.RotationDegrees {
		t.Errorf("RotationDegrees: expected %v, got %v", testCreateWidgetInput.RotationDegrees, found.RotationDegrees)
	}
}

func TestCreateWidget_EmptyName_ReturnsError(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()

	input := testCreateWidgetInput
	input.Name = ""
	_, err := svc.CreateWidget(context.Background(), input)

	if err == nil {
		t.Error("expected error for empty name, got nil")
	}
}

func TestCreateWidget_InvalidOrigin_ReturnsError(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()

	input := testCreateWidgetInput
	input.OriginX = 1.5 // out of [0,1]
	_, err := svc.CreateWidget(context.Background(), input)

	if err == nil {
		t.Error("expected error for invalid origin, got nil")
	}
}

// --- FindAllWidgets ---

func TestFindAllWidgets_EmptyDB_ReturnsEmptyList(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()

	resp, err := svc.FindAllWidgets(context.Background())

	if err != nil {
		t.Fatalf("FindAllWidgets: %v", err)
	}
	if len(resp) != 0 {
		t.Errorf("expected 0 widgets, got %d", len(resp))
	}
}

func TestFindAllWidgets_Multiple_ReturnsAll(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()
	ctx := context.Background()

	input2 := testCreateWidgetInput
	input2.Name = "thermometer"
	if _, err := svc.CreateWidget(ctx, testCreateWidgetInput); err != nil {
		t.Fatalf("CreateWidget 1: %v", err)
	}
	if _, err := svc.CreateWidget(ctx, input2); err != nil {
		t.Fatalf("CreateWidget 2: %v", err)
	}

	resp, err := svc.FindAllWidgets(ctx)

	if err != nil {
		t.Fatalf("FindAllWidgets: %v", err)
	}
	if len(resp) != 2 {
		t.Errorf("expected 2 widgets, got %d", len(resp))
	}
}

// --- FindWidgetByID ---

func TestFindWidgetByID_Existing_ReturnsCorrectFields(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()
	ctx := context.Background()

	created, err := svc.CreateWidget(ctx, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	found, err := svc.FindWidgetByID(ctx, created.ID)

	if err != nil {
		t.Fatalf("FindWidgetByID: %v", err)
	}
	if found.Name != testCreateWidgetInput.Name {
		t.Errorf("Name: expected %q, got %q", testCreateWidgetInput.Name, found.Name)
	}
	if found.Y != testCreateWidgetInput.Y {
		t.Errorf("Y: expected %v, got %v", testCreateWidgetInput.Y, found.Y)
	}
}

func TestFindWidgetByID_Existing_TransformMatrixIsPresent(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()
	ctx := context.Background()

	created, err := svc.CreateWidget(ctx, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	found, err := svc.FindWidgetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindWidgetByID: %v", err)
	}

	if found.TransformMatrix.CSS == "" {
		t.Error("expected non-empty TransformMatrix.CSS")
	}
	// Identity-like matrix for zero rotation (a=1, b=0, c=0, d=1).
	if found.TransformMatrix.A != 1.0 {
		t.Errorf("TransformMatrix.A: expected 1.0 for 0° rotation, got %v", found.TransformMatrix.A)
	}
}

func TestFindWidgetByID_NotFound_ReturnsWrappedError(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()

	_, err := svc.FindWidgetByID(context.Background(), uuid.New())

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, widget.ErrWidgetNotFound) {
		t.Errorf("expected wrapped ErrWidgetNotFound, got: %v", err)
	}
}

// --- UpdateWidget ---

func TestUpdateWidget_Valid_ReturnsIncrementedVersion(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()
	ctx := context.Background()

	created, err := svc.CreateWidget(ctx, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	updated, err := svc.UpdateWidget(ctx, appdto.UpdateWidgetInput{
		ID:              created.ID,
		Name:            "updated-gauge",
		X:               5.0,
		Y:               15.0,
		Z:               1,
		Width:           100,
		Height:          100,
		OriginX:         0.5,
		OriginY:         0.5,
		RotationDegrees: 0.0,
		TypeID:          created.TypeID,
		SceneID:         created.SceneID,
		Labels:          []string{"updated"},
		Version:         created.Version,
	})

	if err != nil {
		t.Fatalf("UpdateWidget: %v", err)
	}
	if updated.Version <= created.Version {
		t.Errorf("Version: expected > %d, got %d", created.Version, updated.Version)
	}
}

func TestUpdateWidget_Valid_FieldsAreUpdated(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()
	ctx := context.Background()

	created, err := svc.CreateWidget(ctx, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	newTypeID := uuid.New()
	_, err = svc.UpdateWidget(ctx, appdto.UpdateWidgetInput{
		ID:              created.ID,
		Name:            "renamed-widget",
		X:               99.0,
		Y:               88.0,
		Z:               7,
		Width:           200,
		Height:          150,
		OriginX:         0.0,
		OriginY:         1.0,
		RotationDegrees: 45.0,
		TypeID:          newTypeID,
		SceneID:         created.SceneID,
		Labels:          []string{"x"},
		Version:         created.Version,
	})
	if err != nil {
		t.Fatalf("UpdateWidget: %v", err)
	}

	found, err := svc.FindWidgetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindWidgetByID: %v", err)
	}

	if found.Name != "renamed-widget" {
		t.Errorf("Name: expected 'renamed-widget', got %q", found.Name)
	}
	if found.X != 99.0 {
		t.Errorf("X: expected 99.0, got %v", found.X)
	}
	if found.RotationDegrees != 45.0 {
		t.Errorf("RotationDegrees: expected 45.0, got %v", found.RotationDegrees)
	}
	if found.TypeID != newTypeID {
		t.Errorf("TypeID: expected %v, got %v", newTypeID, found.TypeID)
	}
}

func TestUpdateWidget_StaleVersion_ReturnsConflict(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()
	ctx := context.Background()

	created, err := svc.CreateWidget(ctx, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	_, err = svc.UpdateWidget(ctx, appdto.UpdateWidgetInput{
		ID:      created.ID,
		Name:    "stale-attempt",
		TypeID:  created.TypeID,
		SceneID: created.SceneID,
		OriginX: 0.5, OriginY: 0.5,
		Width: 100, Height: 100,
		Version: created.Version + 99, // wrong version
	})

	if err == nil {
		t.Fatal("expected error on stale version, got nil")
	}
	if !errors.Is(err, widget.ErrWidgetConflict) {
		t.Errorf("expected wrapped ErrWidgetConflict, got: %v", err)
	}
}

func TestUpdateWidget_ZeroVersion_ReturnsError(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()
	ctx := context.Background()

	created, err := svc.CreateWidget(ctx, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	_, err = svc.UpdateWidget(ctx, appdto.UpdateWidgetInput{
		ID:      created.ID,
		Name:    "bad",
		TypeID:  created.TypeID,
		Version: 0, // invalid: version 0 is the initial (unsaved) state
	})

	if err == nil {
		t.Error("expected error for version 0, got nil")
	}
}

func TestUpdateWidget_NotFound_ReturnsWrappedError(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()

	_, err := svc.UpdateWidget(context.Background(), appdto.UpdateWidgetInput{
		ID:      uuid.New(),
		Name:    "x",
		TypeID:  uuid.New(),
		SceneID: testSceneID,
		OriginX: 0.5, OriginY: 0.5,
		Width: 100, Height: 100,
		Version: 1,
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, widget.ErrWidgetNotFound) {
		t.Errorf("expected wrapped ErrWidgetNotFound, got: %v", err)
	}
}

func TestUpdateWidget_EmptyName_ReturnsError(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()
	ctx := context.Background()

	created, err := svc.CreateWidget(ctx, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	_, err = svc.UpdateWidget(ctx, appdto.UpdateWidgetInput{
		ID:      created.ID,
		Name:    "",
		TypeID:  created.TypeID,
		OriginX: 0.5, OriginY: 0.5,
		Width: 100, Height: 100,
		Version: created.Version,
	})

	if err == nil {
		t.Error("expected error for empty name, got nil")
	}
}

// --- DeleteWidgetByID ---

func TestDeleteWidget_Existing_ReturnsDeletedItem(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()
	ctx := context.Background()

	created, err := svc.CreateWidget(ctx, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	deleted, err := svc.DeleteWidgetByID(ctx, created.ID)

	if err != nil {
		t.Fatalf("DeleteWidgetByID: %v", err)
	}
	if deleted.ID != created.ID {
		t.Errorf("ID: expected %s, got %s", created.ID, deleted.ID)
	}
}

func TestDeleteWidget_Existing_RemovedFromDB(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()
	ctx := context.Background()

	created, err := svc.CreateWidget(ctx, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	if _, err = svc.DeleteWidgetByID(ctx, created.ID); err != nil {
		t.Fatalf("DeleteWidgetByID: %v", err)
	}

	_, err = svc.FindWidgetByID(ctx, created.ID)
	if !errors.Is(err, widget.ErrWidgetNotFound) {
		t.Errorf("expected ErrWidgetNotFound after delete, got: %v", err)
	}
}

func TestDeleteWidget_NotFound_ReturnsWrappedError(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()

	_, err := svc.DeleteWidgetByID(context.Background(), uuid.New())

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, widget.ErrWidgetNotFound) {
		t.Errorf("expected wrapped ErrWidgetNotFound, got: %v", err)
	}
}

// --- Events ---

func TestCreateWidget_Success_PublishesCreatedEvent(t *testing.T) {
	cleanWidgets(t)
	svc, bus := newWidgetServiceWithBus()
	ctx := context.Background()

	var received []event.Event
	bus.Subscribe(event.EventTypeWidgetCreated, func(e event.Event) {
		received = append(received, e)
	})

	resp, err := svc.CreateWidget(ctx, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	wEvent, ok := received[0].(event.WidgetEvent)
	if !ok {
		t.Fatal("expected event to implement WidgetEvent")
	}
	if wEvent.WidgetID().UUID() != resp.ID {
		t.Errorf("WidgetID: expected %s, got %s", resp.ID, wEvent.WidgetID().UUID())
	}
}

func TestDeleteWidget_Success_PublishesDeletedEvent(t *testing.T) {
	cleanWidgets(t)
	svc, bus := newWidgetServiceWithBus()
	ctx := context.Background()

	created, err := svc.CreateWidget(ctx, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	var received []event.Event
	bus.Subscribe(event.EventTypeWidgetDeleted, func(e event.Event) {
		received = append(received, e)
	})

	if _, err = svc.DeleteWidgetByID(ctx, created.ID); err != nil {
		t.Fatalf("DeleteWidgetByID: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	wEvent, ok := received[0].(event.WidgetEvent)
	if !ok {
		t.Fatal("expected event to implement WidgetEvent")
	}
	if wEvent.WidgetID().UUID() != created.ID {
		t.Errorf("WidgetID: expected %s, got %s", created.ID, wEvent.WidgetID().UUID())
	}
}

// --- FindWidgetsBySceneID ---

func TestFindWidgetsBySceneID_EmptyDB_ReturnsEmptyList(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()

	resp, err := svc.FindWidgetsBySceneID(context.Background(), uuid.New())

	if err != nil {
		t.Fatalf("FindWidgetsBySceneID: %v", err)
	}
	if len(resp) != 0 {
		t.Errorf("expected 0 widgets, got %d", len(resp))
	}
}

func TestFindWidgetsBySceneID_WidgetsInScene_ReturnsOnlyThoseWidgets(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()
	ctx := context.Background()

	targetSceneID := uuid.New()
	otherSceneID := uuid.New()
	mustInsertScene(t, targetSceneID)
	mustInsertScene(t, otherSceneID)

	inTarget := testCreateWidgetInput
	inTarget.Name = "widget-in-target"
	inTarget.SceneID = targetSceneID

	inOther := testCreateWidgetInput
	inOther.Name = "widget-in-other"
	inOther.SceneID = otherSceneID

	if _, err := svc.CreateWidget(ctx, inTarget); err != nil {
		t.Fatalf("CreateWidget (target): %v", err)
	}
	if _, err := svc.CreateWidget(ctx, inOther); err != nil {
		t.Fatalf("CreateWidget (other): %v", err)
	}

	resp, err := svc.FindWidgetsBySceneID(ctx, targetSceneID)

	if err != nil {
		t.Fatalf("FindWidgetsBySceneID: %v", err)
	}
	if len(resp) != 1 {
		t.Errorf("expected 1 widget, got %d", len(resp))
	}
	if len(resp) == 1 && resp[0].SceneID != targetSceneID {
		t.Errorf("SceneID: expected %v, got %v", targetSceneID, resp[0].SceneID)
	}
}

func TestFindWidgetsBySceneID_MultipleWidgetsInSameScene_ReturnsAll(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()
	ctx := context.Background()

	sceneID := uuid.New()
	mustInsertScene(t, sceneID)

	for _, name := range []string{"widget-a", "widget-b", "widget-c"} {
		input := testCreateWidgetInput
		input.Name = name
		input.SceneID = sceneID
		if _, err := svc.CreateWidget(ctx, input); err != nil {
			t.Fatalf("CreateWidget %q: %v", name, err)
		}
	}

	resp, err := svc.FindWidgetsBySceneID(ctx, sceneID)

	if err != nil {
		t.Fatalf("FindWidgetsBySceneID: %v", err)
	}
	if len(resp) != 3 {
		t.Errorf("expected 3 widgets, got %d", len(resp))
	}
}

func TestFindWidgetsBySceneID_UnknownScene_ReturnsEmptyList(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()
	ctx := context.Background()

	// Create a widget in a known scene.
	knownSceneID := uuid.New()
	mustInsertScene(t, knownSceneID)
	input := testCreateWidgetInput
	input.SceneID = knownSceneID
	if _, err := svc.CreateWidget(ctx, input); err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	// Query a completely different scene.
	resp, err := svc.FindWidgetsBySceneID(ctx, uuid.New())

	if err != nil {
		t.Fatalf("FindWidgetsBySceneID: %v", err)
	}
	if len(resp) != 0 {
		t.Errorf("expected 0 widgets for unknown scene, got %d", len(resp))
	}
}

func TestFindWidgetsBySceneID_TransformMatrixIsPresent(t *testing.T) {
	cleanWidgets(t)
	svc := newWidgetService()
	ctx := context.Background()

	sceneID := uuid.New()
	mustInsertScene(t, sceneID)
	input := testCreateWidgetInput
	input.SceneID = sceneID
	if _, err := svc.CreateWidget(ctx, input); err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	resp, err := svc.FindWidgetsBySceneID(ctx, sceneID)

	if err != nil {
		t.Fatalf("FindWidgetsBySceneID: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(resp))
	}
	if resp[0].TransformMatrix.CSS == "" {
		t.Error("expected non-empty TransformMatrix.CSS")
	}
	if resp[0].TransformMatrix.A != 1.0 {
		t.Errorf("TransformMatrix.A: expected 1.0 for 0° rotation, got %v", resp[0].TransformMatrix.A)
	}
}

func TestUpdateWidget_Success_PublishesUpdatedEvent(t *testing.T) {
	cleanWidgets(t)
	svc, bus := newWidgetServiceWithBus()
	ctx := context.Background()

	created, err := svc.CreateWidget(ctx, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	var received []event.Event
	bus.Subscribe(event.EventTypeWidgetUpdated, func(e event.Event) {
		received = append(received, e)
	})

	_, err = svc.UpdateWidget(ctx, appdto.UpdateWidgetInput{
		ID:      created.ID,
		Name:    "updated",
		TypeID:  created.TypeID,
		SceneID: created.SceneID,
		OriginX: 0.5, OriginY: 0.5,
		Width: 100, Height: 100,
		Version: created.Version,
	})
	if err != nil {
		t.Fatalf("UpdateWidget: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	wEvent, ok := received[0].(event.WidgetEvent)
	if !ok {
		t.Fatal("expected event to implement WidgetEvent")
	}
	if wEvent.WidgetID().UUID() != created.ID {
		t.Errorf("WidgetID: expected %s, got %s", created.ID, wEvent.WidgetID().UUID())
	}
}
