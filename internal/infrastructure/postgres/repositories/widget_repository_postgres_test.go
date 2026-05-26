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
	w, err := widget.NewWidget(
		newID, newName, pos, widget.DefaultSize(),
		widget.DefaultOrigin(), widget.DefaultRotation(),
		typeID, id.ID[scene.Scene]{}, []string{"label1"}, nil,
		version.Initial[widget.Widget](),
	)
	if err != nil {
		t.Fatalf("NewWidget: %v", err)
	}
	return w
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

	duplicate, _ := widget.NewWidget(
		w.ID(), w.Name(), w.Position(), w.Size(),
		w.Origin(), w.Rotation(),
		w.TypeID(), w.SceneID(), w.Labels(), w.TagIDs(),
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
	updated, _ := widget.NewWidget(
		found.ID(), newName, found.Position(), found.Size(),
		found.Origin(), found.Rotation(),
		found.TypeID(), found.SceneID(), found.Labels(), found.TagIDs(),
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
	updated, _ := widget.NewWidget(
		found.ID(), newName, found.Position(), found.Size(),
		found.Origin(), found.Rotation(),
		found.TypeID(), found.SceneID(), found.Labels(), found.TagIDs(),
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
	stale, _ := widget.NewWidget(
		w.ID(), w.Name(), w.Position(), w.Size(),
		w.Origin(), w.Rotation(),
		w.TypeID(), w.SceneID(), w.Labels(), w.TagIDs(),
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

// --- FindAll with tag IDs ---

func TestWidgetFindAll_WithTagIDs_RoundTripsCorrectly(t *testing.T) {
	cleanWidgets(t)
	repo := repositories.NewWidgetRepositoryPostgres(testDB)
	ctx := context.Background()

	tagID1 := id.NewID[tag.Tag]()
	tagID2 := id.NewID[tag.Tag]()

	newID := repo.NextID()
	newName, _ := widget.NewWidgetName("with-tags")
	pos := widget.NewPosition(1, 2, 3)
	typeID := id.NewID(id.IDWithUUID[widget.WidgetType](uuid.New()))
	w, _ := widget.NewWidget(
		newID, newName, pos, widget.DefaultSize(),
		widget.DefaultOrigin(), widget.DefaultRotation(),
		typeID, id.ID[scene.Scene]{}, []string{"a", "b"},
		[]id.ID[tag.Tag]{tagID1, tagID2},
		version.Initial[widget.Widget](),
	)

	if _, err := repo.Save(ctx, w); err != nil {
		t.Fatalf("Save: %v", err)
	}

	found, err := repo.FindByID(ctx, newID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}

	if len(found.TagIDs()) != 2 {
		t.Errorf("TagIDs: expected 2, got %d", len(found.TagIDs()))
	}
	if len(found.Labels()) != 2 {
		t.Errorf("Labels: expected 2, got %d", len(found.Labels()))
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

	w, _ := widget.NewWidget(
		newID, newName, pos, size, origin, rotation,
		typeID, id.ID[scene.Scene]{}, nil, nil,
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
