package repositories_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/version"
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
func mustSaveWidgetType(t *testing.T) id.ID[library.WidgetType] {
	t.Helper()
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	saved, err := repo.Save(context.Background(), makeWidgetType(t, "type", nil, repo))
	if err != nil {
		t.Fatalf("Save widget type: %v", err)
	}
	return saved.ID()
}

func makeWidget(t *testing.T, repo scene.SceneRepository, name string) scene.Widget {
	t.Helper()
	newID := repo.NextWidgetID()
	newName, err := scene.NewWidgetName(name)
	if err != nil {
		t.Fatalf("NewWidgetName(%q): %v", name, err)
	}
	pos := scene.NewPosition(10.0, 20.0, 0)
	typeID := mustSaveWidgetType(t)
	return scene.NewWidget(
		newID, newName, pos, scene.DefaultWidgetSize(),
		scene.DefaultOrigin(), scene.DefaultRotation(),
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

// ═════════════════════ Scene saved and read with its widgets ════════════════

// withWidgets returns sc holding someWidgets instead of its own, at the same version.
func withWidgets(sc scene.Scene, someWidgets ...scene.Widget) scene.Scene {
	return scene.NewScene(sc.ID(), sc.Name(), sc.Size(), sc.BackgroundHTML(), someWidgets, sc.Version())
}

func mustSave(t *testing.T, repo scene.SceneRepository, sc scene.Scene) scene.Scene {
	t.Helper()
	saved, err := repo.Save(context.Background(), sc)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	return saved
}

func mustFindByID(t *testing.T, repo scene.SceneRepository, sceneID id.ID[scene.Scene]) scene.Scene {
	t.Helper()
	found, err := repo.FindByID(context.Background(), sceneID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	return found
}

func widgetIDs(ws []scene.Widget) []id.ID[scene.Widget] {
	ids := make([]id.ID[scene.Widget], len(ws))
	for i, w := range ws {
		ids[i] = w.ID()
	}
	return ids
}

func renamed(t *testing.T, w scene.Widget, name string) scene.Widget {
	t.Helper()
	return scene.NewWidget(w.ID(), mustWidgetName(t, name), w.Position(), w.Size(), w.Origin(), w.Rotation(),
		w.TypeID(), w.Labels(), w.PortBindings())
}

func TestSceneSave_NewSceneWithWidgets_InsertsThem(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)

	w1 := makeWidget(t, repo, "gauge-1")
	w2 := makeWidget(t, repo, "gauge-2")
	saved := mustSave(t, repo, withWidgets(makeScene(t, repo, "scene-1"), w1, w2))

	if saved.Version() != version.Committed[scene.Scene]() {
		t.Errorf("Version: expected %d, got %d", version.Committed[scene.Scene]().Number(), saved.Version().Number())
	}
	found := mustFindByID(t, repo, saved.ID())
	if got, want := widgetIDs(found.Widgets()), widgetIDs([]scene.Widget{w1, w2}); !slices.Equal(got, want) {
		t.Errorf("widgets: expected %v, got %v", want, got)
	}
}

func TestSceneSave_ChangedWidgets_InsertsUpdatesDeletesAndBumpsVersionOnce(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)

	w1 := makeWidget(t, repo, "gauge-1")
	w2 := makeWidget(t, repo, "gauge-2")
	sc := mustSave(t, repo, withWidgets(makeScene(t, repo, "scene-1"), w1, w2))

	w1Renamed := renamed(t, w1, "gauge-1-renamed")
	w3 := makeWidget(t, repo, "gauge-3")
	saved := mustSave(t, repo, withWidgets(sc, w1Renamed, w3))

	if saved.Version().Number() != sc.Version().Number()+1 {
		t.Errorf("scene version: expected %d, got %d", sc.Version().Number()+1, saved.Version().Number())
	}
	found := mustFindByID(t, repo, sc.ID())
	if found.Version() != saved.Version() {
		t.Errorf("stored version: expected %d, got %d", saved.Version().Number(), found.Version().Number())
	}
	if got, want := widgetIDs(found.Widgets()), widgetIDs([]scene.Widget{w1, w3}); !slices.Equal(got, want) {
		t.Fatalf("widgets: expected %v, got %v", want, got)
	}
	if found.Widgets()[0].Name().String() != "gauge-1-renamed" {
		t.Errorf("Name: expected 'gauge-1-renamed', got %q", found.Widgets()[0].Name().String())
	}
}

func TestSceneSave_UnknownScene_ReturnsNotFound(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)

	sc := makeScene(t, repo, "never-saved")
	_, err := repo.Save(context.Background(), scene.NewScene(sc.ID(), sc.Name(), sc.Size(), sc.BackgroundHTML(), nil, version.Committed[scene.Scene]()))

	if !errors.Is(err, scene.ErrSceneNotFound) {
		t.Errorf("expected ErrSceneNotFound, got %v", err)
	}
}

// TestSceneSave_WriteRace_ReturnsConflictAndKeepsOtherWrite: two writers read
// the same version; the second Save must neither succeed nor undo the first.
func TestSceneSave_WriteRace_ReturnsConflictAndKeepsOtherWrite(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	w1 := makeWidget(t, repo, "gauge-1")
	sc := mustSave(t, repo, withWidgets(makeScene(t, repo, "scene-1"), w1))
	first := mustFindByID(t, repo, sc.ID())
	second := mustFindByID(t, repo, sc.ID())

	w2 := makeWidget(t, repo, "gauge-2")
	mustSave(t, repo, withWidgets(first, w1, w2))

	_, err := repo.Save(ctx, withWidgets(second))

	if !errors.Is(err, scene.ErrSceneConflict) {
		t.Fatalf("expected ErrSceneConflict, got %v", err)
	}
	found := mustFindByID(t, repo, sc.ID())
	if got, want := widgetIDs(found.Widgets()), widgetIDs([]scene.Widget{w1, w2}); !slices.Equal(got, want) {
		t.Errorf("widgets: expected the first write %v, got %v", want, got)
	}
}

// TestSceneSave_ConcurrentSaves_OneWins runs the write race for real: of
// several Saves of the same version exactly one succeeds.
func TestSceneSave_ConcurrentSaves_OneWins(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	sc := mustSaveScene(t, repo, "scene-1")
	const writers = 8
	ws := make([]scene.Widget, writers)
	for i := range ws {
		ws[i] = makeWidget(t, repo, fmt.Sprintf("gauge-%d", i))
	}

	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for _, w := range ws {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.Save(ctx, withWidgets(sc, w))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	succeeded := 0
	for err := range errs {
		switch {
		case err == nil:
			succeeded++
		case !errors.Is(err, scene.ErrSceneConflict):
			t.Errorf("expected ErrSceneConflict, got %v", err)
		}
	}
	if succeeded != 1 {
		t.Errorf("expected exactly 1 successful Save, got %d", succeeded)
	}
	found := mustFindByID(t, repo, sc.ID())
	if len(found.Widgets()) != 1 || found.Version().Number() != sc.Version().Number()+1 {
		t.Errorf("expected 1 widget at version %d, got %d at %d", sc.Version().Number()+1, len(found.Widgets()), found.Version().Number())
	}
}

func TestSceneSave_UnknownWidgetType_ReturnsErrWidgetTypeNotFound(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	w := makeWidget(t, repo, "w1")
	sc := mustSave(t, repo, withWidgets(makeScene(t, repo, "scene-1"), w))
	unknownType := scene.NewWidget(w.ID(), w.Name(), w.Position(), w.Size(), w.Origin(), w.Rotation(),
		id.NewID[library.WidgetType](), w.Labels(), w.PortBindings())

	tests := map[string]scene.Scene{
		"insert": withWidgets(makeScene(t, repo, "scene-2"), scene.NewWidget(repo.NextWidgetID(), w.Name(), w.Position(), w.Size(), w.Origin(), w.Rotation(), unknownType.TypeID(), nil, nil)),
		"update": withWidgets(sc, unknownType),
	}
	for name, toSave := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := repo.Save(ctx, toSave)
			if !errors.Is(err, library.ErrWidgetTypeNotFound) {
				t.Errorf("expected ErrWidgetTypeNotFound, got %v", err)
			}
		})
	}
	if found := mustFindByID(t, repo, sc.ID()); found.Version() != sc.Version() {
		t.Errorf("a failed Save must change nothing: version %d, got %d", sc.Version().Number(), found.Version().Number())
	}
}

func TestSceneFindAll_ReturnsWidgetsOfEachScene(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)

	w1 := makeWidget(t, repo, "in-first")
	w2 := makeWidget(t, repo, "in-second")
	mustSave(t, repo, withWidgets(makeScene(t, repo, "first"), w1))
	mustSave(t, repo, withWidgets(makeScene(t, repo, "second"), w2))
	mustSaveScene(t, repo, "empty")

	all, err := repo.FindAll(context.Background())

	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 scenes, got %d", len(all))
	}
	for i, want := range [][]scene.Widget{{w1}, {w2}, nil} {
		if got := widgetIDs(all[i].Widgets()); !slices.Equal(got, widgetIDs(want)) {
			t.Errorf("scene %d widgets: expected %v, got %v", i, widgetIDs(want), got)
		}
	}
}

// ══════════════════════ Scene delete cascades widgets ═══════════════════════

func TestSceneDeleteByID_WithWidgets_ReturnsThemInResult(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	sc := mustSave(t, repo, withWidgets(makeScene(t, repo, "scene-1"), makeWidget(t, repo, "widget-1"), makeWidget(t, repo, "widget-2")))

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

	sc := mustSave(t, repo, withWidgets(makeScene(t, repo, "scene-1"), makeWidget(t, repo, "widget-1")))

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

// ══════════════════════════ FindByWidgetTypeID ══════════════════════════════

func TestSceneFindByWidgetTypeID_ReturnsScenesWithAllTheirWidgets(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	typeID := mustSaveWidgetType(t)
	otherTypeID := mustSaveWidgetType(t)
	newWidget := func(name string, typeID id.ID[library.WidgetType]) scene.Widget {
		return scene.NewWidget(repo.NextWidgetID(), mustWidgetName(t, name), scene.NewPosition(0, 0, 0), scene.DefaultWidgetSize(), scene.DefaultOrigin(), scene.DefaultRotation(), typeID, nil, nil)
	}

	matching1 := newWidget("m1", typeID)
	nonMatching := newWidget("n1", otherTypeID)
	matching2 := newWidget("m2", typeID)
	sc1 := mustSave(t, repo, withWidgets(makeScene(t, repo, "scene-1"), matching1, nonMatching))
	sc2 := mustSave(t, repo, withWidgets(makeScene(t, repo, "scene-2"), matching2))
	mustSave(t, repo, withWidgets(makeScene(t, repo, "scene-3"), newWidget("n2", otherTypeID)))

	result, err := repo.FindByWidgetTypeID(ctx, typeID)

	if err != nil {
		t.Fatalf("FindByWidgetTypeID: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 scenes, got %d", len(result))
	}
	if result[0].ID() != sc1.ID() || result[1].ID() != sc2.ID() {
		t.Errorf("expected scenes %v, %v in creation order, got %v, %v", sc1.ID(), sc2.ID(), result[0].ID(), result[1].ID())
	}
	if got, want := widgetIDs(result[0].Widgets()), widgetIDs([]scene.Widget{matching1, nonMatching}); !slices.Equal(got, want) {
		t.Errorf("scene-1 widgets: expected all of them %v, got %v", want, got)
	}
	if result[0].Version() != sc1.Version() {
		t.Errorf("scene-1 version: expected %d, got %d", sc1.Version().Number(), result[0].Version().Number())
	}
}

func mustWidgetName(t *testing.T, s string) scene.WidgetName {
	t.Helper()
	n, err := scene.NewWidgetName(s)
	if err != nil {
		t.Fatalf("NewWidgetName(%q): %v", s, err)
	}
	return n
}

// ══════════════════════════ PortBindings round-trip ═════════════════════════

func TestSceneSave_WidgetWithPortBindings_RoundTripsCorrectly(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)

	tagID1 := id.NewID[tag.Tag]()
	tagID2 := id.NewID[tag.Tag]()
	portName1, _ := library.NewInputPortName("temperature")
	portName2, _ := library.NewInputPortName("pressure")
	bindings := []scene.PortBinding{
		scene.NewPortBinding(portName1, tagID1),
		scene.NewPortBinding(portName2, tagID2),
	}

	w := scene.NewWidget(
		repo.NextWidgetID(), mustWidgetName(t, "with-bindings"), scene.NewPosition(1, 2, 3),
		scene.DefaultWidgetSize(), scene.DefaultOrigin(), scene.DefaultRotation(),
		mustSaveWidgetType(t), []string{"a", "b"}, bindings,
	)
	sc := mustSave(t, repo, withWidgets(makeScene(t, repo, "scene-1"), w))

	found, err := mustFindByID(t, repo, sc.ID()).FindWidget(w.ID())
	if err != nil {
		t.Fatalf("FindWidget: %v", err)
	}

	if !slices.Equal(found.PortBindings(), bindings) {
		t.Errorf("PortBindings: expected %v, got %v", bindings, found.PortBindings())
	}
	if !slices.Equal(found.Labels(), []string{"a", "b"}) {
		t.Errorf("Labels: expected [a b], got %v", found.Labels())
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

func TestSceneFindByID_WidgetsInCreationOrderEvenAfterUpdates(t *testing.T) {
	cleanScenes(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB)

	widgets := []scene.Widget{makeWidget(t, repo, "w-1"), makeWidget(t, repo, "w-2"), makeWidget(t, repo, "w-3")}
	sc := mustSave(t, repo, withWidgets(makeScene(t, repo, "scene-1"), widgets...))
	for round := range 3 {
		moved := sc.Widgets()
		for i, w := range moved[:2] {
			moved[i] = scene.NewWidget(
				w.ID(), w.Name(), scene.NewPosition(float64(round), 0, 0), w.Size(), w.Origin(), w.Rotation(),
				w.TypeID(), w.Labels(), w.PortBindings(),
			)
		}
		sc = mustSave(t, repo, withWidgets(sc, moved...))
	}

	found := mustFindByID(t, repo, sc.ID())

	if got, want := widgetIDs(found.Widgets()), widgetIDs(widgets); !slices.Equal(got, want) {
		t.Errorf("widgets: got %v, want %v (creation order)", got, want)
	}
}
