package repositories_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/version"
	"github.com/kipitix/growscada/internal/server/domain/widget"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

func cleanScenes(t *testing.T) {
	t.Helper()
	// widgets cascade-delete with their scene (ON DELETE CASCADE on scene_id).
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM scenes"); err != nil {
		t.Fatalf("cleanScenes: %v", err)
	}
}

func makeScene(t *testing.T, repo scene.SceneRepository, name string) scene.Scene {
	t.Helper()
	newID := repo.NextID()
	sceneName, err := scene.NewSceneName(name)
	if err != nil {
		t.Fatalf("NewSceneName(%q): %v", name, err)
	}
	size, err := scene.NewSceneSize(1920, 1080)
	if err != nil {
		t.Fatalf("NewSceneSize: %v", err)
	}
	return scene.NewScene(newID, sceneName, size, scene.NewBackgroundHTML(""), nil, version.Initial[scene.Scene]())
}

func mustSaveScene(t *testing.T, repo scene.SceneRepository, name string) scene.Scene {
	t.Helper()
	saved, err := repo.Save(context.Background(), makeScene(t, repo, name))
	if err != nil {
		t.Fatalf("Save scene %q: %v", name, err)
	}
	return saved
}

// mustSaveWidgetType stores a new WidgetType: a Widget's type must exist.
func mustSaveWidgetType(t *testing.T) id.ID[widget.WidgetType] {
	t.Helper()
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	saved, err := repo.Save(context.Background(), makeWidgetType(t, "type", nil, repo))
	if err != nil {
		t.Fatalf("Save widget type: %v", err)
	}
	return saved.ID()
}

func makeWidget(t *testing.T, repo scene.SceneRepository, name string) widget.Widget {
	t.Helper()
	newID := repo.NextWidgetID()
	newName, err := widget.NewWidgetName(name)
	if err != nil {
		t.Fatalf("NewWidgetName(%q): %v", name, err)
	}
	pos := widget.NewPosition(10.0, 20.0, 0)
	typeID := mustSaveWidgetType(t)
	return widget.NewWidget(
		newID, newName, pos, widget.DefaultSize(),
		widget.DefaultOrigin(), widget.DefaultRotation(),
		typeID, []string{"label1"}, nil,
	)
}

// ══════════════════════════════ Scene CRUD ═════════════════════════════════

func TestSceneNextID_ReturnsUniqueIDs(t *testing.T) {
	repo := repositories.NewSceneRepositoryPostgres(testDB)

	if repo.NextID() == repo.NextID() {
		t.Error("NextID should return unique IDs, got duplicates")
	}
}

func TestSceneSave_NewScene_ReturnsCommittedVersion(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)

	saved := mustSaveScene(t, repo, "scene-1")

	if saved.Version() != version.Committed[scene.Scene]() {
		t.Errorf("Version: expected %d, got %d", version.Committed[scene.Scene]().Number(), saved.Version().Number())
	}
}

func TestSceneSave_ExistingScene_VersionIsIncremented(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	saved := mustSaveScene(t, repo, "scene-1")
	newName, _ := scene.NewSceneName("scene-1-renamed")
	updated := scene.NewScene(saved.ID(), newName, saved.Size(), saved.BackgroundHTML(), nil, saved.Version())

	saved2, err := repo.Save(ctx, updated)

	if err != nil {
		t.Fatalf("Save (update): %v", err)
	}
	if saved2.Version().Number() != saved.Version().Number()+1 {
		t.Errorf("Version: expected %d, got %d", saved.Version().Number()+1, saved2.Version().Number())
	}
}

func TestSceneSave_StaleVersion_ReturnsConflict(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	saved := mustSaveScene(t, repo, "scene-1")
	badVersion, _ := version.New[scene.Scene](version.WithNumber[scene.Scene](100))
	stale := scene.NewScene(saved.ID(), saved.Name(), saved.Size(), saved.BackgroundHTML(), nil, badVersion)

	_, err := repo.Save(ctx, stale)

	if !errors.Is(err, scene.ErrSceneConflict) {
		t.Errorf("expected ErrSceneConflict, got %v", err)
	}
}

func TestSceneFindByID_Existing_ReturnsScene(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	saved := mustSaveScene(t, repo, "scene-1")

	found, err := repo.FindByID(ctx, saved.ID())

	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found.ID() != saved.ID() {
		t.Errorf("ID: expected %v, got %v", saved.ID(), found.ID())
	}
	if len(found.Widgets()) != 0 {
		t.Errorf("expected 0 widgets, got %d", len(found.Widgets()))
	}
}

func TestSceneFindByID_NotFound_ReturnsErrSceneNotFound(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)

	_, err := repo.FindByID(context.Background(), repo.NextID())

	if !errors.Is(err, scene.ErrSceneNotFound) {
		t.Errorf("expected ErrSceneNotFound, got %v", err)
	}
}

func TestSceneFindAll_MultipleScenes_ReturnsAll(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)

	mustSaveScene(t, repo, "scene-a")
	mustSaveScene(t, repo, "scene-b")

	all, err := repo.FindAll(context.Background())

	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 scenes, got %d", len(all))
	}
}

func TestSceneDeleteByID_NotFound_ReturnsErrSceneNotFound(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)

	_, err := repo.DeleteByID(context.Background(), repo.NextID())

	if !errors.Is(err, scene.ErrSceneNotFound) {
		t.Errorf("expected ErrSceneNotFound, got %v", err)
	}
}

// ═════════════════════ Widget access through SceneRepository ═══════════════

func TestSceneAddWidget_Valid_InsertsAndBumpsSceneVersion(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	sc := mustSaveScene(t, repo, "scene-1")
	w := makeWidget(t, repo, "gauge-1")

	saved, newVer, err := repo.AddWidget(ctx, sc.ID(), sc.Version(), w)

	if err != nil {
		t.Fatalf("AddWidget: %v", err)
	}
	if saved.ID() != w.ID() {
		t.Errorf("ID: expected %v, got %v", w.ID(), saved.ID())
	}
	if newVer.Number() != sc.Version().Number()+1 {
		t.Errorf("scene version: expected %d, got %d", sc.Version().Number()+1, newVer.Number())
	}
}

func TestSceneAddWidget_StaleVersion_ReturnsConflict(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	sc := mustSaveScene(t, repo, "scene-1")
	w := makeWidget(t, repo, "gauge-1")
	badVersion, _ := version.New[scene.Scene](version.WithNumber[scene.Scene](100))

	_, _, err := repo.AddWidget(ctx, sc.ID(), badVersion, w)

	if !errors.Is(err, scene.ErrSceneConflict) {
		t.Errorf("expected ErrSceneConflict, got %v", err)
	}
}

func TestSceneAddWidget_UnknownScene_ReturnsNotFound(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	w := makeWidget(t, repo, "gauge-1")
	_, _, err := repo.AddWidget(ctx, repo.NextID(), version.Initial[scene.Scene](), w)

	if !errors.Is(err, scene.ErrSceneNotFound) {
		t.Errorf("expected ErrSceneNotFound, got %v", err)
	}
}

func TestSceneFindByID_WithWidgets_ReturnsThem(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	sc := mustSaveScene(t, repo, "scene-1")
	w := makeWidget(t, repo, "gauge-1")
	if _, _, err := repo.AddWidget(ctx, sc.ID(), sc.Version(), w); err != nil {
		t.Fatalf("AddWidget: %v", err)
	}

	found, err := repo.FindByID(ctx, sc.ID())

	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if len(found.Widgets()) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(found.Widgets()))
	}
	if found.Widgets()[0].ID() != w.ID() {
		t.Errorf("widget ID: expected %v, got %v", w.ID(), found.Widgets()[0].ID())
	}
}

func TestSceneFindWidgetsBySceneID_ReturnsOnlyMatching(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	target := mustSaveScene(t, repo, "target")
	other := mustSaveScene(t, repo, "other")

	w1 := makeWidget(t, repo, "in-target")
	if _, _, err := repo.AddWidget(ctx, target.ID(), target.Version(), w1); err != nil {
		t.Fatalf("AddWidget target: %v", err)
	}
	w2 := makeWidget(t, repo, "in-other")
	if _, _, err := repo.AddWidget(ctx, other.ID(), other.Version(), w2); err != nil {
		t.Fatalf("AddWidget other: %v", err)
	}

	result, err := repo.FindWidgetsBySceneID(ctx, target.ID())

	if err != nil {
		t.Fatalf("FindWidgetsBySceneID: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(result))
	}
	if result[0].ID() != w1.ID() {
		t.Errorf("ID: expected %v, got %v", w1.ID(), result[0].ID())
	}
}

func TestSceneFindWidgetByID_NotFound_ReturnsErrWidgetNotFound(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	sc := mustSaveScene(t, repo, "scene-1")

	_, err := repo.FindWidgetByID(ctx, sc.ID(), id.NewID[widget.Widget]())

	if !errors.Is(err, widget.ErrWidgetNotFound) {
		t.Errorf("expected ErrWidgetNotFound, got %v", err)
	}
}

func TestSceneUpdateWidget_Valid_UpdatesFieldsAndBumpsSceneVersion(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	sc := mustSaveScene(t, repo, "scene-1")
	w := makeWidget(t, repo, "gauge-1")
	_, verAfterAdd, err := repo.AddWidget(ctx, sc.ID(), sc.Version(), w)
	if err != nil {
		t.Fatalf("AddWidget: %v", err)
	}

	newName, _ := widget.NewWidgetName("gauge-renamed")
	updatedWidget := widget.NewWidget(
		w.ID(), newName, w.Position(), w.Size(), w.Origin(), w.Rotation(),
		w.TypeID(), w.Labels(), w.PortBindings(),
	)

	saved, newVer, err := repo.UpdateWidget(ctx, sc.ID(), verAfterAdd, updatedWidget)

	if err != nil {
		t.Fatalf("UpdateWidget: %v", err)
	}
	if saved.Name().String() != "gauge-renamed" {
		t.Errorf("Name: expected 'gauge-renamed', got %q", saved.Name().String())
	}
	if newVer.Number() != verAfterAdd.Number()+1 {
		t.Errorf("scene version: expected %d, got %d", verAfterAdd.Number()+1, newVer.Number())
	}
}

func TestSceneUpdateWidget_StaleVersion_ReturnsConflict(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	sc := mustSaveScene(t, repo, "scene-1")
	w := makeWidget(t, repo, "gauge-1")
	if _, _, err := repo.AddWidget(ctx, sc.ID(), sc.Version(), w); err != nil {
		t.Fatalf("AddWidget: %v", err)
	}

	badVersion, _ := version.New[scene.Scene](version.WithNumber[scene.Scene](100))
	_, _, err := repo.UpdateWidget(ctx, sc.ID(), badVersion, w)

	if !errors.Is(err, scene.ErrSceneConflict) {
		t.Errorf("expected ErrSceneConflict, got %v", err)
	}
}

func TestSceneUpdateWidget_UnknownWidget_ReturnsWidgetNotFound(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	sc := mustSaveScene(t, repo, "scene-1")
	w := makeWidget(t, repo, "ghost")

	_, _, err := repo.UpdateWidget(ctx, sc.ID(), sc.Version(), w)

	if !errors.Is(err, widget.ErrWidgetNotFound) {
		t.Errorf("expected ErrWidgetNotFound, got %v", err)
	}
}

func TestSceneDeleteWidget_Existing_RemovesAndBumpsSceneVersion(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	sc := mustSaveScene(t, repo, "scene-1")
	w := makeWidget(t, repo, "gauge-1")
	_, verAfterAdd, err := repo.AddWidget(ctx, sc.ID(), sc.Version(), w)
	if err != nil {
		t.Fatalf("AddWidget: %v", err)
	}

	deleted, newVer, err := repo.DeleteWidget(ctx, sc.ID(), w.ID())

	if err != nil {
		t.Fatalf("DeleteWidget: %v", err)
	}
	if deleted.ID() != w.ID() {
		t.Errorf("ID: expected %v, got %v", w.ID(), deleted.ID())
	}
	if newVer.Number() != verAfterAdd.Number()+1 {
		t.Errorf("scene version: expected %d, got %d", verAfterAdd.Number()+1, newVer.Number())
	}

	_, err = repo.FindWidgetByID(ctx, sc.ID(), w.ID())
	if !errors.Is(err, widget.ErrWidgetNotFound) {
		t.Errorf("expected ErrWidgetNotFound after delete, got %v", err)
	}
}

func TestSceneDeleteWidget_NotFound_ReturnsErrWidgetNotFound(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	sc := mustSaveScene(t, repo, "scene-1")

	_, _, err := repo.DeleteWidget(ctx, sc.ID(), id.NewID[widget.Widget]())

	if !errors.Is(err, widget.ErrWidgetNotFound) {
		t.Errorf("expected ErrWidgetNotFound, got %v", err)
	}
}

// ══════════════════════ Scene delete cascades widgets ═══════════════════════

func TestSceneDeleteByID_WithWidgets_ReturnsThemInResult(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	sc := mustSaveScene(t, repo, "scene-1")
	w1 := makeWidget(t, repo, "widget-1")
	_, ver, err := repo.AddWidget(ctx, sc.ID(), sc.Version(), w1)
	if err != nil {
		t.Fatalf("AddWidget 1: %v", err)
	}
	w2 := makeWidget(t, repo, "widget-2")
	if _, _, err := repo.AddWidget(ctx, sc.ID(), ver, w2); err != nil {
		t.Fatalf("AddWidget 2: %v", err)
	}

	deleted, err := repo.DeleteByID(ctx, sc.ID())

	if err != nil {
		t.Fatalf("DeleteByID: %v", err)
	}
	if len(deleted.Widgets()) != 2 {
		t.Fatalf("expected 2 widgets in deleted scene, got %d", len(deleted.Widgets()))
	}
}

func TestSceneDeleteByID_WithWidgets_CascadesWidgetRowsInDB(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	sc := mustSaveScene(t, repo, "scene-1")
	w := makeWidget(t, repo, "widget-1")
	if _, _, err := repo.AddWidget(ctx, sc.ID(), sc.Version(), w); err != nil {
		t.Fatalf("AddWidget: %v", err)
	}

	if _, err := repo.DeleteByID(ctx, sc.ID()); err != nil {
		t.Fatalf("DeleteByID: %v", err)
	}

	var count int
	if err := testDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM widgets WHERE scene_id = $1", sc.ID().UUID()).Scan(&count); err != nil {
		t.Fatalf("count widgets: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 widget rows after scene deletion, got %d", count)
	}
}

// ══════════════════════════ FindWidgetsByTypeID ═════════════════════════════

func TestSceneFindWidgetsByTypeID_ReturnsMatchingAcrossScenes(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	typeID := mustSaveWidgetType(t)
	otherTypeID := mustSaveWidgetType(t)

	sc1 := mustSaveScene(t, repo, "scene-1")
	sc2 := mustSaveScene(t, repo, "scene-2")

	matching1 := widget.NewWidget(repo.NextWidgetID(), mustWidgetName(t, "m1"), widget.NewPosition(0, 0, 0), widget.DefaultSize(), widget.DefaultOrigin(), widget.DefaultRotation(), typeID, nil, nil)
	matching2 := widget.NewWidget(repo.NextWidgetID(), mustWidgetName(t, "m2"), widget.NewPosition(0, 0, 0), widget.DefaultSize(), widget.DefaultOrigin(), widget.DefaultRotation(), typeID, nil, nil)
	nonMatching := widget.NewWidget(repo.NextWidgetID(), mustWidgetName(t, "n1"), widget.NewPosition(0, 0, 0), widget.DefaultSize(), widget.DefaultOrigin(), widget.DefaultRotation(), otherTypeID, nil, nil)

	_, sc1VerAfterMatching1, err := repo.AddWidget(ctx, sc1.ID(), sc1.Version(), matching1)
	if err != nil {
		t.Fatalf("AddWidget matching1: %v", err)
	}
	if _, _, err := repo.AddWidget(ctx, sc2.ID(), sc2.Version(), matching2); err != nil {
		t.Fatalf("AddWidget matching2: %v", err)
	}
	if _, _, err := repo.AddWidget(ctx, sc1.ID(), sc1VerAfterMatching1, nonMatching); err != nil {
		t.Fatalf("AddWidget nonMatching: %v", err)
	}

	result, err := repo.FindWidgetsByTypeID(ctx, typeID)

	if err != nil {
		t.Fatalf("FindWidgetsByTypeID: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 widgets, got %d", len(result))
	}
	gotScenes := map[uuid.UUID]bool{}
	for _, item := range result {
		if item.Widget.TypeID() != typeID {
			t.Errorf("TypeID: expected %v, got %v", typeID, item.Widget.TypeID())
		}
		gotScenes[item.SceneID.UUID()] = true
		if item.SceneVersion.Number() == 0 {
			t.Error("expected non-zero SceneVersion")
		}
	}
	if !gotScenes[sc1.ID().UUID()] || !gotScenes[sc2.ID().UUID()] {
		t.Errorf("expected widgets from both scenes, got %v", gotScenes)
	}
}

func mustWidgetName(t *testing.T, s string) widget.WidgetName {
	t.Helper()
	n, err := widget.NewWidgetName(s)
	if err != nil {
		t.Fatalf("NewWidgetName(%q): %v", s, err)
	}
	return n
}

// ══════════════════════════ PortBindings round-trip ═════════════════════════

func TestSceneAddWidget_WithPortBindings_RoundTripsCorrectly(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	sc := mustSaveScene(t, repo, "scene-1")

	tagID1 := id.NewID[tag.Tag]()
	tagID2 := id.NewID[tag.Tag]()
	portName1, _ := widget.NewInputPortName("temperature")
	portName2, _ := widget.NewInputPortName("pressure")
	bindings := []widget.PortBinding{
		widget.NewPortBinding(portName1, tagID1),
		widget.NewPortBinding(portName2, tagID2),
	}

	w := widget.NewWidget(
		repo.NextWidgetID(), mustWidgetName(t, "with-bindings"), widget.NewPosition(1, 2, 3),
		widget.DefaultSize(), widget.DefaultOrigin(), widget.DefaultRotation(),
		mustSaveWidgetType(t), []string{"a", "b"}, bindings,
	)

	if _, _, err := repo.AddWidget(ctx, sc.ID(), sc.Version(), w); err != nil {
		t.Fatalf("AddWidget: %v", err)
	}

	found, err := repo.FindWidgetByID(ctx, sc.ID(), w.ID())
	if err != nil {
		t.Fatalf("FindWidgetByID: %v", err)
	}

	if len(found.PortBindings()) != 2 {
		t.Fatalf("PortBindings: expected 2, got %d", len(found.PortBindings()))
	}
	if found.PortBindings()[0].PortName().String() != "temperature" {
		t.Errorf("PortBindings[0].PortName: expected 'temperature', got %q", found.PortBindings()[0].PortName().String())
	}
	if len(found.Labels()) != 2 {
		t.Errorf("Labels: expected 2, got %d", len(found.Labels()))
	}
}

// ══════════════════════════════ Ordering ═══════════════════════════════════
//
// Postgres returns unordered rows in storage order, and an UPDATE moves a
// row; lists must keep the creation order however often their items change.

func TestSceneFindAll_CreationOrderEvenAfterUpdates(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	var created []scene.Scene
	for _, name := range []string{"charlie", "alpha", "bravo"} {
		created = append(created, mustSaveScene(t, repo, name))
	}
	for round := range 3 {
		for i := range 2 {
			newName, _ := scene.NewSceneName(fmt.Sprintf("renamed-%d-%d", i, round))
			sc := created[i]
			saved, err := repo.Save(ctx, scene.NewScene(sc.ID(), newName, sc.Size(), sc.BackgroundHTML(), nil, sc.Version()))
			if err != nil {
				t.Fatalf("Save (update): %v", err)
			}
			created[i] = saved
		}
	}

	all, err := repo.FindAll(ctx)

	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(all) != len(created) {
		t.Fatalf("expected %d scenes, got %d", len(created), len(all))
	}
	for i := range created {
		if all[i].ID() != created[i].ID() {
			t.Errorf("position %d: got %q, want %q (creation order)", i, all[i].Name().String(), created[i].Name().String())
		}
	}
}

func TestSceneFindWidgetsBySceneID_CreationOrderEvenAfterUpdates(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	sc := mustSaveScene(t, repo, "scene-1")
	ver := sc.Version()
	var widgets []widget.Widget
	for _, name := range []string{"w-1", "w-2", "w-3"} {
		w := makeWidget(t, repo, name)
		saved, newVer, err := repo.AddWidget(ctx, sc.ID(), ver, w)
		if err != nil {
			t.Fatalf("AddWidget: %v", err)
		}
		widgets, ver = append(widgets, saved), newVer
	}
	for round := range 3 {
		for _, w := range widgets[:2] {
			moved := widget.NewWidget(
				w.ID(), w.Name(), widget.NewPosition(float64(round), 0, 0), w.Size(), w.Origin(), w.Rotation(),
				w.TypeID(), w.Labels(), w.PortBindings(),
			)
			_, newVer, err := repo.UpdateWidget(ctx, sc.ID(), ver, moved)
			if err != nil {
				t.Fatalf("UpdateWidget: %v", err)
			}
			ver = newVer
		}
	}

	found, err := repo.FindWidgetsBySceneID(ctx, sc.ID())

	if err != nil {
		t.Fatalf("FindWidgetsBySceneID: %v", err)
	}
	if len(found) != len(widgets) {
		t.Fatalf("expected %d widgets, got %d", len(widgets), len(found))
	}
	for i := range widgets {
		if found[i].ID() != widgets[i].ID() {
			t.Errorf("position %d: got %q, want %q (creation order)", i, found[i].Name().String(), widgets[i].Name().String())
		}
	}
}

func TestSceneAddWidget_UnknownType_ReturnsErrWidgetTypeNotFound(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()
	sc := mustSaveScene(t, repo, "scene-1")
	w := makeWidget(t, repo, "w1")
	w = widget.NewWidget(w.ID(), w.Name(), w.Position(), w.Size(), w.Origin(), w.Rotation(),
		id.NewID[widget.WidgetType](), w.Labels(), w.PortBindings())

	_, _, err := repo.AddWidget(ctx, sc.ID(), sc.Version(), w)
	if !errors.Is(err, widget.ErrWidgetTypeNotFound) {
		t.Errorf("expected ErrWidgetTypeNotFound, got %v", err)
	}
}

func TestSceneUpdateWidget_UnknownType_ReturnsErrWidgetTypeNotFound(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()
	sc := mustSaveScene(t, repo, "scene-1")
	w := makeWidget(t, repo, "w1")
	_, ver, err := repo.AddWidget(ctx, sc.ID(), sc.Version(), w)
	if err != nil {
		t.Fatalf("AddWidget: %v", err)
	}
	w = widget.NewWidget(w.ID(), w.Name(), w.Position(), w.Size(), w.Origin(), w.Rotation(),
		id.NewID[widget.WidgetType](), w.Labels(), w.PortBindings())

	_, _, err = repo.UpdateWidget(ctx, sc.ID(), ver, w)
	if !errors.Is(err, widget.ErrWidgetTypeNotFound) {
		t.Errorf("expected ErrWidgetTypeNotFound, got %v", err)
	}
}
