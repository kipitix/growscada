package repositories_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/version"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

func cleanWidgetTypes(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM widgets; DELETE FROM widget_types"); err != nil {
		t.Fatalf("cleanWidgetTypes: %v", err)
	}
}

func makeWidgetType(t *testing.T, name string, ports []library.InputPort, repo library.WidgetTypeRepository) library.WidgetType {
	t.Helper()
	newID := repo.NextID()
	wtName, err := library.NewWidgetTypeName(name)
	if err != nil {
		t.Fatalf("NewWidgetTypeName(%q): %v", name, err)
	}
	html, _ := library.NewHtmlTemplate("<div></div>")
	script, _ := library.NewScript("function update(){}")
	size := library.DefaultSize()
	wt, err := library.ReconstituteWidgetType(newID, wtName, html, script, library.ScriptLanguageJavaScript, size, ports, version.Initial[library.WidgetType]())
	if err != nil {
		t.Fatalf("NewWidgetType: %v", err)
	}
	return wt
}

// --- NextID ---

func TestWidgetTypeNextID_ReturnsUniqueIDs(t *testing.T) {
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	id1 := repo.NextID()
	id2 := repo.NextID()
	if id1 == id2 {
		t.Error("NextID should return unique IDs, got duplicates")
	}
}

// --- Save (insert) ---

func TestWidgetTypeSave_NewWidgetType_InsertsSuccessfully(t *testing.T) {
	cleanWidgetTypes(t)
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	ctx := context.Background()

	wt := makeWidgetType(t, "gauge", nil, repo)
	_, err := repo.Save(ctx, wt)
	if err != nil {
		t.Fatalf("Save returned unexpected error: %v", err)
	}
}

func TestWidgetTypeSave_NewWidgetType_ReturnsCommittedVersion(t *testing.T) {
	cleanWidgetTypes(t)
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	ctx := context.Background()

	wt := makeWidgetType(t, "gauge", nil, repo)
	saved, err := repo.Save(ctx, wt)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.Version() != version.Committed[library.WidgetType]() {
		t.Errorf("Version: expected %d, got %d", version.Committed[library.WidgetType]().Number(), saved.Version().Number())
	}
}

func TestWidgetTypeSave_DuplicateID_ReturnsError(t *testing.T) {
	cleanWidgetTypes(t)
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	ctx := context.Background()

	wt := makeWidgetType(t, "gauge", nil, repo)
	if _, err := repo.Save(ctx, wt); err != nil {
		t.Fatalf("first Save failed: %v", err)
	}
	duplicate, _ := library.ReconstituteWidgetType(
		wt.ID(), wt.Name(), wt.HtmlTemplate(), wt.Script(), wt.ScriptLanguage(), wt.DefaultSize(), nil,
		version.Initial[library.WidgetType](),
	)
	_, err := repo.Save(ctx, duplicate)
	if err == nil {
		t.Error("expected error on duplicate insert, got nil")
	}
}

// --- Save (update) ---

func TestWidgetTypeSave_ExistingWidgetType_VersionIsIncremented(t *testing.T) {
	cleanWidgetTypes(t)
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	ctx := context.Background()

	wt := makeWidgetType(t, "gauge", nil, repo)
	saved, _ := repo.Save(ctx, wt)
	found, _ := repo.FindByID(ctx, saved.ID())

	newName, _ := library.NewWidgetTypeName("gauge-v2")
	updated, err := library.ReconstituteWidgetType(found.ID(), newName, found.HtmlTemplate(), found.Script(), found.ScriptLanguage(), found.DefaultSize(), found.InputPorts(), found.Version())
	if err != nil {
		t.Fatalf("NewWidgetType: %v", err)
	}
	saved2, err := repo.Save(ctx, updated)
	if err != nil {
		t.Fatalf("update Save: %v", err)
	}
	if saved2.Version().Number() != saved.Version().Number()+1 {
		t.Errorf("Version: expected %d, got %d", saved.Version().Number()+1, saved2.Version().Number())
	}
}

// --- FindByID ---

func TestWidgetTypeFindByID_NotFound_ReturnsErrWidgetTypeNotFound(t *testing.T) {
	cleanWidgetTypes(t)
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	ctx := context.Background()

	nonExistentID := id.NewID(id.IDWithUUID[library.WidgetType](uuid.New()))
	_, err := repo.FindByID(ctx, nonExistentID)
	if !errors.Is(err, library.ErrWidgetTypeNotFound) {
		t.Errorf("expected ErrWidgetTypeNotFound, got %v", err)
	}
}

// --- InputPorts round-trip ---

func TestWidgetTypeSave_WithInputPorts_RoundTripsCorrectly(t *testing.T) {
	cleanWidgetTypes(t)
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	ctx := context.Background()

	p1Name, _ := library.NewInputPortName("temperature")
	p2Name, _ := library.NewInputPortName("pressure")
	integerHint, err := library.TypeHintFor(tag.TagTypeInteger)
	if err != nil {
		t.Fatalf("TypeHintFor: %v", err)
	}
	ports := []library.InputPort{
		library.NewInputPort(p1Name, "Process temperature", integerHint),
		library.NewInputPort(p2Name, "Line pressure", library.AnyTypeHint()),
	}

	wt := makeWidgetType(t, "dual-gauge", ports, repo)
	saved, err := repo.Save(ctx, wt)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	found, err := repo.FindByID(ctx, saved.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}

	if len(found.InputPorts()) != 2 {
		t.Fatalf("InputPorts: expected 2, got %d", len(found.InputPorts()))
	}
	if found.InputPorts()[0].Name().String() != "temperature" {
		t.Errorf("InputPorts[0].Name: expected 'temperature', got %q", found.InputPorts()[0].Name().String())
	}
	if found.InputPorts()[0].Description() != "Process temperature" {
		t.Errorf("InputPorts[0].Description: expected 'Process temperature', got %q", found.InputPorts()[0].Description())
	}
	if found.InputPorts()[0].TypeHint() != integerHint {
		t.Errorf("InputPorts[0].TypeHint: expected integer, got %v", found.InputPorts()[0].TypeHint())
	}
	if found.InputPorts()[1].Name().String() != "pressure" {
		t.Errorf("InputPorts[1].Name: expected 'pressure', got %q", found.InputPorts()[1].Name().String())
	}
	if !found.InputPorts()[1].TypeHint().IsAny() {
		t.Errorf("InputPorts[1].TypeHint: expected any, got %v", found.InputPorts()[1].TypeHint())
	}
}

func TestWidgetTypeSave_EmptyInputPorts_RoundTripsCorrectly(t *testing.T) {
	cleanWidgetTypes(t)
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	ctx := context.Background()

	wt := makeWidgetType(t, "simple", nil, repo)
	saved, err := repo.Save(ctx, wt)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	found, err := repo.FindByID(ctx, saved.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if len(found.InputPorts()) != 0 {
		t.Errorf("expected 0 input ports, got %d", len(found.InputPorts()))
	}
}

func TestWidgetTypeSave_InputPortsAreUpdated(t *testing.T) {
	cleanWidgetTypes(t)
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	ctx := context.Background()

	wt := makeWidgetType(t, "updatable", nil, repo)
	saved, _ := repo.Save(ctx, wt)
	found, _ := repo.FindByID(ctx, saved.ID())

	pName, _ := library.NewInputPortName("val")
	booleanHint, err := library.TypeHintFor(tag.TagTypeBoolean)
	if err != nil {
		t.Fatalf("TypeHintFor: %v", err)
	}
	ports := []library.InputPort{library.NewInputPort(pName, "new port", booleanHint)}
	updated, err := library.ReconstituteWidgetType(found.ID(), found.Name(), found.HtmlTemplate(), found.Script(), found.ScriptLanguage(), found.DefaultSize(), ports, found.Version())
	if err != nil {
		t.Fatalf("NewWidgetType: %v", err)
	}
	saved2, err := repo.Save(ctx, updated)
	if err != nil {
		t.Fatalf("update Save: %v", err)
	}

	refetched, err := repo.FindByID(ctx, saved2.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if len(refetched.InputPorts()) != 1 {
		t.Fatalf("expected 1 input port after update, got %d", len(refetched.InputPorts()))
	}
	if refetched.InputPorts()[0].Name().String() != "val" {
		t.Errorf("port name: expected 'val', got %q", refetched.InputPorts()[0].Name().String())
	}
}

// --- Delete ---

func TestWidgetTypeDelete_NotFound_ReturnsErrWidgetTypeNotFound(t *testing.T) {
	cleanWidgetTypes(t)
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	ctx := context.Background()

	wt := makeWidgetType(t, "never-saved", nil, repo)
	committed, _ := library.ReconstituteWidgetType(wt.ID(), wt.Name(), wt.HtmlTemplate(), wt.Script(), wt.ScriptLanguage(), wt.DefaultSize(), nil, version.Committed[library.WidgetType]())
	err := repo.Delete(ctx, committed.Delete())
	if !errors.Is(err, library.ErrWidgetTypeNotFound) {
		t.Errorf("expected ErrWidgetTypeNotFound, got %v", err)
	}
}

func TestWidgetTypeDelete_Existing_RemovesIt(t *testing.T) {
	cleanWidgetTypes(t)
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	ctx := context.Background()

	saved, err := repo.Save(ctx, makeWidgetType(t, "temp-gauge", nil, repo))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := repo.Delete(ctx, saved.Delete()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.FindByID(ctx, saved.ID()); !errors.Is(err, library.ErrWidgetTypeNotFound) {
		t.Errorf("expected ErrWidgetTypeNotFound after delete, got %v", err)
	}
}

func TestWidgetTypeDelete_StaleVersion_ReturnsConflict(t *testing.T) {
	cleanWidgetTypes(t)
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	ctx := context.Background()

	stale, err := repo.Save(ctx, makeWidgetType(t, "temp-gauge", nil, repo))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := repo.Save(ctx, stale); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	if err := repo.Delete(ctx, stale.Delete()); !errors.Is(err, library.ErrWidgetTypeConflict) {
		t.Errorf("expected ErrWidgetTypeConflict, got %v", err)
	}
}

func TestWidgetTypeFindAll_CreationOrderEvenAfterUpdates(t *testing.T) {
	// Postgres returns unordered rows in storage order, and an UPDATE moves a
	// row; the Library must keep the creation order however often types change.
	cleanWidgetTypes(t)
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	ctx := context.Background()

	var created []library.WidgetType
	for _, name := range []string{"charlie", "alpha", "bravo"} {
		saved, err := repo.Save(ctx, makeWidgetType(t, name, nil, repo))
		if err != nil {
			t.Fatalf("Save %s: %v", name, err)
		}
		created = append(created, saved)
	}
	for round := range 3 {
		for i := range 2 {
			wt := created[i]
			newName, _ := library.NewWidgetTypeName(fmt.Sprintf("renamed-%d-%d", i, round))
			updated, err := library.ReconstituteWidgetType(wt.ID(), newName, wt.HtmlTemplate(), wt.Script(), wt.ScriptLanguage(), wt.DefaultSize(), wt.InputPorts(), wt.Version())
			if err != nil {
				t.Fatalf("NewWidgetType: %v", err)
			}
			saved, err := repo.Save(ctx, updated)
			if err != nil {
				t.Fatalf("update Save: %v", err)
			}
			created[i] = saved
		}
	}

	all, err := repo.FindAll(ctx)

	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(all) != len(created) {
		t.Fatalf("expected %d widget types, got %d", len(created), len(all))
	}
	for i := range created {
		if all[i].ID() != created[i].ID() {
			t.Errorf("position %d: got %q, want %q (creation order)", i, all[i].Name().String(), created[i].Name().String())
		}
	}
}

func TestWidgetTypeDelete_UsedByWidget_ReturnsErrWidgetTypeInUse(t *testing.T) {
	cleanScenes(t)
	cleanWidgetTypes(t)
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	sceneRepo := repositories.NewSceneRepositoryPostgres(testDB)
	ctx := context.Background()

	w := makeWidget(t, sceneRepo, "w1")
	mustSave(t, sceneRepo, withWidgets(makeScene(t, sceneRepo, "scene-1"), w))

	wt, err := repo.FindByID(ctx, w.TypeID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	err = repo.Delete(ctx, wt.Delete())
	if !errors.Is(err, library.ErrWidgetTypeInUse) {
		t.Errorf("expected ErrWidgetTypeInUse, got %v", err)
	}
	if _, err := repo.FindByID(ctx, w.TypeID()); err != nil {
		t.Errorf("widget type after refused delete: %v", err)
	}
}
