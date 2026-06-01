package repositories_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/scene"
	"github.com/kipitix/growscada/internal/domain/tag"
	"github.com/kipitix/growscada/internal/domain/version"
	"github.com/kipitix/growscada/internal/domain/widget"
	"github.com/kipitix/growscada/internal/infrastructure/postgres/repositories"
)

// testSceneID is populated in TestMain before any test runs.
// It holds a real scene row so widget inserts satisfy the FK on scene_id.
var testSceneID uuid.UUID

// mustInsertScene inserts a minimal scene row so widget inserts satisfy the FK.
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

func cleanWidgets(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM widgets"); err != nil {
		t.Fatalf("cleanWidgets: %v", err)
	}
}

func makeWidget(t *testing.T, name string, repo widget.WidgetRepository) widget.Widget {
	t.Helper()
	newID := repo.NextID()
	newName, err := widget.NewWidgetName(name)
	if err != nil {
		t.Fatalf("NewWidgetName(%q): %v", name, err)
	}
	pos := widget.NewPosition(10.0, 20.0, 0)
	typeID := id.NewID(id.IDWithUUID[widget.WidgetType](uuid.New()))
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](testSceneID))
	return widget.NewWidget(
		newID, newName, pos, widget.DefaultSize(),
		widget.DefaultOrigin(), widget.DefaultRotation(),
		typeID, sceneID, []string{"label1"}, nil,
		version.Initial[widget.Widget](),
	)
}

func makeWidgetWithSceneID(t *testing.T, name string, sceneID id.ID[scene.Scene], repo widget.WidgetRepository) widget.Widget {
	t.Helper()
	newID := repo.NextID()
	newName, err := widget.NewWidgetName(name)
	if err != nil {
		t.Fatalf("NewWidgetName(%q): %v", name, err)
	}
	pos := widget.NewPosition(10.0, 20.0, 0)
	typeID := id.NewID(id.IDWithUUID[widget.WidgetType](uuid.New()))
	return widget.NewWidget(
		newID, newName, pos, widget.DefaultSize(),
		widget.DefaultOrigin(), widget.DefaultRotation(),
		typeID, sceneID, []string{"label1"}, nil,
		version.Initial[widget.Widget](),
	)
}

// --- NextID ---

func TestWidgetNextID_ReturnsUniqueIDs(t *testing.T) {
	repo := repositories.NewWidgetRepositoryPostgres(testDB)

	id1 := repo.NextID()
	id2 := repo.NextID()

	if id1 == id2 {
		t.Error("NextID should return unique IDs, got duplicates")
	}
}

// --- Save (insert) ---

func TestWidgetSave_NewWidget_InsertsSuccessfully(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	w := makeWidget(t, "gauge1", repo)
	_, err := repo.Save(ctx, w)

	if err != nil {
		t.Fatalf("Save returned unexpected error: %v", err)
	}
}

func TestWidgetSave_NewWidget_ReturnsCommittedVersion(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	w := makeWidget(t, "gauge1", repo)
	saved, err := repo.Save(ctx, w)

	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.Version() != version.Committed[widget.Widget]() {
		t.Errorf("Version: expected %d, got %d", version.Committed[widget.Widget]().Number(), saved.Version().Number())
	}
}

func TestWidgetSave_DuplicateID_ReturnsError(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	w := makeWidget(t, "gauge1", repo)
	if _, err := repo.Save(ctx, w); err != nil {
		t.Fatalf("first Save failed: %v", err)
	}

	duplicate := widget.NewWidget(
		w.ID(), w.Name(), w.Position(), w.Size(),
		w.Origin(), w.Rotation(),
		w.TypeID(), w.SceneID(), w.Labels(), w.PortBindings(),
		version.Initial[widget.Widget](),
	)
	_, err := repo.Save(ctx, duplicate)

	if err == nil {
		t.Error("expected error on duplicate insert, got nil")
	}
}

// --- Save (update) ---

func TestWidgetSave_ExistingWidget_UpdatesSuccessfully(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	w := makeWidget(t, "gauge1", repo)
	if _, err := repo.Save(ctx, w); err != nil {
		t.Fatalf("initial Save failed: %v", err)
	}

	found, err := repo.FindByID(ctx, w.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}

	newName, _ := widget.NewWidgetName("gauge-updated")
	updated := widget.NewWidget(
		found.ID(), newName, found.Position(), found.Size(),
		found.Origin(), found.Rotation(),
		found.TypeID(), found.SceneID(), found.Labels(), found.PortBindings(),
		found.Version(),
	)
	if _, err := repo.Save(ctx, updated); err != nil {
		t.Fatalf("update Save failed: %v", err)
	}

	refetched, err := repo.FindByID(ctx, w.ID())
	if err != nil {
		t.Fatalf("FindByID after update: %v", err)
	}
	if refetched.Name().String() != "gauge-updated" {
		t.Errorf("Name: expected 'gauge-updated', got %q", refetched.Name().String())
	}
}

func TestWidgetSave_ExistingWidget_VersionIsIncremented(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	w := makeWidget(t, "gauge1", repo)
	saved, _ := repo.Save(ctx, w)
	found, _ := repo.FindByID(ctx, saved.ID())

	newName, _ := widget.NewWidgetName("gauge-v2")
	updated := widget.NewWidget(
		found.ID(), newName, found.Position(), found.Size(),
		found.Origin(), found.Rotation(),
		found.TypeID(), found.SceneID(), found.Labels(), found.PortBindings(),
		found.Version(),
	)
	saved2, err := repo.Save(ctx, updated)

	if err != nil {
		t.Fatalf("update Save: %v", err)
	}
	if saved2.Version().Number() != saved.Version().Number()+1 {
		t.Errorf("Version: expected %d, got %d", saved.Version().Number()+1, saved2.Version().Number())
	}
}

func TestWidgetSave_StaleVersion_ReturnsError(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	w := makeWidget(t, "gauge1", repo)
	if _, err := repo.Save(ctx, w); err != nil {
		t.Fatalf("initial Save failed: %v", err)
	}

	badVersion, _ := version.New[widget.Widget](version.WithNumber[widget.Widget](100))
	stale := widget.NewWidget(
		w.ID(), w.Name(), w.Position(), w.Size(),
		w.Origin(), w.Rotation(),
		w.TypeID(), w.SceneID(), w.Labels(), w.PortBindings(),
		badVersion,
	)
	_, err := repo.Save(ctx, stale)

	if err == nil {
		t.Error("expected error on stale version update, got nil")
	}
}

// --- FindByID ---

func TestWidgetFindByID_Existing_ReturnsWidget(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	w := makeWidget(t, "pressure-gauge", repo)
	saved, err := repo.Save(ctx, w)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	found, err := repo.FindByID(ctx, saved.ID())

	if err != nil {
		t.Fatalf("FindByID returned unexpected error: %v", err)
	}
	if found.ID() != saved.ID() {
		t.Errorf("ID: expected %v, got %v", saved.ID(), found.ID())
	}
	if found.Name() != saved.Name() {
		t.Errorf("Name: expected %v, got %v", saved.Name(), found.Name())
	}
	if found.Position().X() != saved.Position().X() {
		t.Errorf("Position.X: expected %v, got %v", saved.Position().X(), found.Position().X())
	}
	if found.Version() != saved.Version() {
		t.Errorf("Version: expected %d, got %d", saved.Version().Number(), found.Version().Number())
	}
}

func TestWidgetFindByID_NotFound_ReturnsErrWidgetNotFound(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	nonExistentID := repo.NextID()
	_, err := repo.FindByID(ctx, nonExistentID)

	if !errors.Is(err, widget.ErrWidgetNotFound) {
		t.Errorf("expected ErrWidgetNotFound, got %v", err)
	}
}

// --- FindAll ---

func TestWidgetFindAll_EmptyDB_ReturnsEmptySlice(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	widgets, err := repo.FindAll(ctx)

	if err != nil {
		t.Fatalf("FindAll returned unexpected error: %v", err)
	}
	if len(widgets) != 0 {
		t.Errorf("expected 0 widgets, got %d", len(widgets))
	}
}

func TestWidgetFindAll_MultipleWidgets_ReturnsAll(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	w1 := makeWidget(t, "widget-a", repo)
	w2 := makeWidget(t, "widget-b", repo)
	if _, err := repo.Save(ctx, w1); err != nil {
		t.Fatalf("Save w1: %v", err)
	}
	if _, err := repo.Save(ctx, w2); err != nil {
		t.Fatalf("Save w2: %v", err)
	}

	all, err := repo.FindAll(ctx)

	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 widgets, got %d", len(all))
	}
}

// --- PortBindings round-trip ---

func TestWidgetFindAll_WithPortBindings_RoundTripsCorrectly(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	tagID1 := id.NewID[tag.Tag]()
	tagID2 := id.NewID[tag.Tag]()
	portName1, _ := widget.NewInputPortName("temperature")
	portName2, _ := widget.NewInputPortName("pressure")

	newID := repo.NextID()
	newName, _ := widget.NewWidgetName("with-bindings")
	pos := widget.NewPosition(1, 2, 3)
	typeID := id.NewID(id.IDWithUUID[widget.WidgetType](uuid.New()))
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](testSceneID))
	bindings := []widget.PortBinding{
		widget.NewPortBinding(portName1, tagID1),
		widget.NewPortBinding(portName2, tagID2),
	}
	w := widget.NewWidget(
		newID, newName, pos, widget.DefaultSize(),
		widget.DefaultOrigin(), widget.DefaultRotation(),
		typeID, sceneID, []string{"a", "b"}, bindings,
		version.Initial[widget.Widget](),
	)

	if _, err := repo.Save(ctx, w); err != nil {
		t.Fatalf("Save: %v", err)
	}

	found, err := repo.FindByID(ctx, newID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}

	if len(found.PortBindings()) != 2 {
		t.Fatalf("PortBindings: expected 2, got %d", len(found.PortBindings()))
	}
	if found.PortBindings()[0].PortName().String() != "temperature" {
		t.Errorf("PortBindings[0].PortName: expected 'temperature', got %q", found.PortBindings()[0].PortName().String())
	}
	if found.PortBindings()[1].PortName().String() != "pressure" {
		t.Errorf("PortBindings[1].PortName: expected 'pressure', got %q", found.PortBindings()[1].PortName().String())
	}
	if len(found.Labels()) != 2 {
		t.Errorf("Labels: expected 2, got %d", len(found.Labels()))
	}
}

func TestWidgetFindAll_NoPortBindings_RoundTripsCorrectly(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	w := makeWidget(t, "empty-bindings", repo)
	if _, err := repo.Save(ctx, w); err != nil {
		t.Fatalf("Save: %v", err)
	}
	found, err := repo.FindByID(ctx, w.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if len(found.PortBindings()) != 0 {
		t.Errorf("expected 0 port bindings, got %d", len(found.PortBindings()))
	}
}

// --- Origin and Rotation round-trip ---

func TestWidgetFindByID_OriginAndRotation_RoundTripsCorrectly(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	newID := repo.NextID()
	newName, _ := widget.NewWidgetName("rotated-widget")
	pos := widget.NewPosition(50, 100, 2)
	size, _ := widget.NewSize(200, 150)
	origin, _ := widget.NewOrigin(0.25, 0.75)
	rotation := widget.NewRotation(90.0)
	typeID := id.NewID(id.IDWithUUID[widget.WidgetType](uuid.New()))
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](testSceneID))

	w := widget.NewWidget(
		newID, newName, pos, size, origin, rotation,
		typeID, sceneID, nil, nil,
		version.Initial[widget.Widget](),
	)

	if _, err := repo.Save(ctx, w); err != nil {
		t.Fatalf("Save: %v", err)
	}

	found, err := repo.FindByID(ctx, newID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}

	if found.Origin().X() != origin.X() || found.Origin().Y() != origin.Y() {
		t.Errorf("Origin: expected (%.4f, %.4f), got (%.4f, %.4f)",
			origin.X(), origin.Y(), found.Origin().X(), found.Origin().Y())
	}
	if found.Rotation().Degrees() != rotation.Degrees() {
		t.Errorf("Rotation: expected %.4f°, got %.4f°", rotation.Degrees(), found.Rotation().Degrees())
	}
	if found.Position().Z() != pos.Z() {
		t.Errorf("Position.Z: expected %d, got %d", pos.Z(), found.Position().Z())
	}
}

// --- DeleteByID ---

func TestWidgetDeleteByID_Existing_ReturnsDeletedWidget(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	w := makeWidget(t, "valve-widget", repo)
	if _, err := repo.Save(ctx, w); err != nil {
		t.Fatalf("Save: %v", err)
	}

	deleted, err := repo.DeleteByID(ctx, w.ID())

	if err != nil {
		t.Fatalf("DeleteByID returned unexpected error: %v", err)
	}
	if deleted.ID() != w.ID() {
		t.Errorf("ID: expected %v, got %v", w.ID(), deleted.ID())
	}
	if deleted.Name() != w.Name() {
		t.Errorf("Name: expected %v, got %v", w.Name(), deleted.Name())
	}
}

func TestWidgetDeleteByID_Existing_WidgetIsRemovedFromDB(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	w := makeWidget(t, "pump-widget", repo)
	if _, err := repo.Save(ctx, w); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := repo.DeleteByID(ctx, w.ID()); err != nil {
		t.Fatalf("DeleteByID: %v", err)
	}

	_, err := repo.FindByID(ctx, w.ID())

	if !errors.Is(err, widget.ErrWidgetNotFound) {
		t.Errorf("expected ErrWidgetNotFound after delete, got %v", err)
	}
}

func TestWidgetDeleteByID_Existing_OtherWidgetsAreUnaffected(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	w1 := makeWidget(t, "widget-1", repo)
	w2 := makeWidget(t, "widget-2", repo)
	if _, err := repo.Save(ctx, w1); err != nil {
		t.Fatalf("Save w1: %v", err)
	}
	if _, err := repo.Save(ctx, w2); err != nil {
		t.Fatalf("Save w2: %v", err)
	}

	if _, err := repo.DeleteByID(ctx, w1.ID()); err != nil {
		t.Fatalf("DeleteByID: %v", err)
	}

	_, err := repo.FindByID(ctx, w2.ID())
	if err != nil {
		t.Errorf("w2 should still exist, got error: %v", err)
	}
}

func TestWidgetDeleteByID_NotFound_ReturnsErrWidgetNotFound(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	nonExistentID := repo.NextID()
	_, err := repo.DeleteByID(ctx, nonExistentID)

	if !errors.Is(err, widget.ErrWidgetNotFound) {
		t.Errorf("expected ErrWidgetNotFound, got %v", err)
	}
}

// --- FindBySceneID ---

func TestWidgetFindBySceneID_EmptyDB_ReturnsEmptySlice(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	sceneID := id.NewID[scene.Scene]()
	result, err := repo.FindBySceneID(ctx, sceneID)

	if err != nil {
		t.Fatalf("FindBySceneID returned unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 widgets, got %d", len(result))
	}
}

func TestWidgetFindBySceneID_WidgetsInScene_ReturnsOnlyMatchingWidgets(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	targetSceneID := id.NewID[scene.Scene]()
	otherSceneID := id.NewID[scene.Scene]()
	mustInsertScene(t, targetSceneID.UUID())
	mustInsertScene(t, otherSceneID.UUID())

	w1 := makeWidgetWithSceneID(t, "widget-in-target-1", targetSceneID, repo)
	w2 := makeWidgetWithSceneID(t, "widget-in-target-2", targetSceneID, repo)
	w3 := makeWidgetWithSceneID(t, "widget-in-other", otherSceneID, repo)

	for _, w := range []widget.Widget{w1, w2, w3} {
		if _, err := repo.Save(ctx, w); err != nil {
			t.Fatalf("Save %q: %v", w.Name().String(), err)
		}
	}

	result, err := repo.FindBySceneID(ctx, targetSceneID)

	if err != nil {
		t.Fatalf("FindBySceneID: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 widgets, got %d", len(result))
	}
	for _, w := range result {
		if w.SceneID() != targetSceneID {
			t.Errorf("widget %v has wrong SceneID: expected %v, got %v", w.ID(), targetSceneID, w.SceneID())
		}
	}
}

func TestWidgetFindBySceneID_NoMatch_ReturnsEmptySlice(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	existingSceneID := id.NewID[scene.Scene]()
	mustInsertScene(t, existingSceneID.UUID())
	w := makeWidgetWithSceneID(t, "some-widget", existingSceneID, repo)
	if _, err := repo.Save(ctx, w); err != nil {
		t.Fatalf("Save: %v", err)
	}

	nonExistentSceneID := id.NewID[scene.Scene]()
	result, err := repo.FindBySceneID(ctx, nonExistentSceneID)

	if err != nil {
		t.Fatalf("FindBySceneID: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 widgets for unknown scene, got %d", len(result))
	}
}

func TestWidgetFindBySceneID_PreservesWidgetFields(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	sceneID := id.NewID[scene.Scene]()
	mustInsertScene(t, sceneID.UUID())
	w := makeWidgetWithSceneID(t, "pressure-gauge", sceneID, repo)
	saved, err := repo.Save(ctx, w)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	result, err := repo.FindBySceneID(ctx, sceneID)

	if err != nil {
		t.Fatalf("FindBySceneID: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(result))
	}
	got := result[0]
	if got.ID() != saved.ID() {
		t.Errorf("ID: expected %v, got %v", saved.ID(), got.ID())
	}
	if got.Name() != saved.Name() {
		t.Errorf("Name: expected %v, got %v", saved.Name(), got.Name())
	}
	if got.SceneID() != sceneID {
		t.Errorf("SceneID: expected %v, got %v", sceneID, got.SceneID())
	}
}
