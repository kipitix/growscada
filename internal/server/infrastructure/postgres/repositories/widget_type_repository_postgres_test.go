package repositories_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/library"
)

// The WidgetTypeRepository is the read side of a Library's Draft (ADR 0016):
// its writes are the Draft's and are tested with the Draft's repository.

func makeWidgetType(t *testing.T, name string, ports []library.InputPort, repo libraryRepo) library.WidgetType {
	t.Helper()
	newID := repo.NextID()
	wtName, err := library.NewWidgetTypeName(name)
	if err != nil {
		t.Fatalf("NewWidgetTypeName(%q): %v", name, err)
	}
	html, _ := library.NewHtmlTemplate("<div></div>")
	script, _ := library.NewScript("function update(){}")
	size := library.DefaultSize()
	wt, err := library.ReconstituteWidgetType(newID, repo.libraryID, wtName, html, script, library.ScriptLanguageJavaScript, size, ports)
	if err != nil {
		t.Fatalf("ReconstituteWidgetType: %v", err)
	}
	return wt
}

// mustStoreWidgetType creates wt in the repository's Library and returns it as
// read back.
func mustStoreWidgetType(t *testing.T, wt library.WidgetType, repo libraryRepo) library.WidgetType {
	t.Helper()
	return mustCreateWidgetType(t, repo.libraryID, wt)
}

func TestWidgetTypeNextID_ReturnsUniqueIDs(t *testing.T) {
	repo := newWidgetTypeRepo(t)
	id1 := repo.NextID()
	id2 := repo.NextID()
	if id1 == id2 {
		t.Error("NextID should return unique IDs, got duplicates")
	}
}

func TestWidgetTypeFindByID_EmptyDraft_ReturnsErrWidgetTypeNotFound(t *testing.T) {
	repo := newWidgetTypeRepo(t)
	_, err := repo.FindByID(context.Background(), repo.NextID())
	if !errors.Is(err, library.ErrWidgetTypeNotFound) {
		t.Errorf("expected ErrWidgetTypeNotFound, got %v", err)
	}
}

func TestWidgetTypeFindAll_EmptyDraft(t *testing.T) {
	repo := newWidgetTypeRepo(t)
	list, err := repo.FindAll(context.Background())
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected no widget types, got %d", len(list))
	}
}

func TestWidgetType_CreateAndRead_RoundTripsTheDefinition(t *testing.T) {
	repo := newWidgetTypeRepo(t)
	portName, _ := library.NewInputPortName("value")
	built := makeWidgetType(t, "gauge", []library.InputPort{
		library.NewInputPort(portName, "the value", library.AnyTypeHint()),
	}, repo)

	stored := mustStoreWidgetType(t, built, repo)

	found, err := repo.FindByID(context.Background(), stored.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found.Name().String() != "gauge" || len(found.InputPorts()) != 1 ||
		found.InputPorts()[0].Name().String() != "value" || found.InputPorts()[0].Description() != "the value" {
		t.Errorf("the stored widget type: %s (%+v)", found, found.InputPorts())
	}
}

func TestWidgetTypeFindAll_CreationOrder(t *testing.T) {
	repo := newWidgetTypeRepo(t)
	ctx := context.Background()
	first := mustStoreWidgetType(t, makeWidgetType(t, "first", nil, repo), repo)
	second := mustStoreWidgetType(t, makeWidgetType(t, "second", nil, repo), repo)

	list, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(list) != 2 || list[0].ID() != first.ID() || list[1].ID() != second.ID() {
		t.Errorf("FindAll order: %v", list)
	}
}

func TestWidgetType_UpdateInTheDraft_IsReadBack(t *testing.T) {
	repo := newWidgetTypeRepo(t)
	ctx := context.Background()
	built := mustStoreWidgetType(t, makeWidgetType(t, "gauge", nil, repo), repo)
	portName, _ := library.NewInputPortName("value")
	changed, err := built.Update(built.Name(), built.HtmlTemplate(), built.Script(), built.ScriptLanguage(),
		built.DefaultSize(), []library.InputPort{library.NewInputPort(portName, "", library.AnyTypeHint())})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	updated := mustUpdateLibraryWidgetType(t, repo.libraryID, changed)

	found, err := repo.FindByID(ctx, built.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found.ID() != updated.ID() || len(found.InputPorts()) != 1 {
		t.Errorf("the updated widget type: %s", found)
	}
}

func TestWidgetTypeDelete_InTheDraft_RemovesIt(t *testing.T) {
	repo := newWidgetTypeRepo(t)
	ctx := context.Background()
	built := mustStoreWidgetType(t, makeWidgetType(t, "gauge", nil, repo), repo)

	mustUpdateLibrary(t, repo.libraryID, "Delete widget type gauge", func(session *draft.Session) error {
		_, err := session.DeleteWidgetType(ctx, built.ID())
		return err
	})

	if _, err := repo.FindByID(ctx, built.ID()); !errors.Is(err, library.ErrWidgetTypeNotFound) {
		t.Errorf("after the deletion: %v, want ErrWidgetTypeNotFound", err)
	}
}

func TestWidgetTypeCreate_NameTakenInTheLibrary_IsRefused(t *testing.T) {
	repo := newWidgetTypeRepo(t)
	mustStoreWidgetType(t, makeWidgetType(t, "gauge", nil, repo), repo)
	ctx := context.Background()
	again := makeWidgetType(t, "gauge", nil, repo)

	_, err := testDrafts().Update(ctx, draft.LibraryScope(repo.libraryID), libraryDraftVersionOf(t, repo.libraryID),
		"Create widget type gauge", func(session *draft.Session) error {
			session.CreateWidgetType(ctx, again)
			return nil
		})
	if !errors.Is(err, library.ErrWidgetTypeNameTaken) {
		t.Errorf("a taken name: %v, want ErrWidgetTypeNameTaken", err)
	}
}

func TestWidgetTypeReads_FollowTheLibrary(t *testing.T) {
	own := newWidgetTypeRepo(t)
	other := newWidgetTypeRepo(t)
	ctx := context.Background()
	built := mustStoreWidgetType(t, makeWidgetType(t, "gauge", nil, own), own)

	if _, err := other.FindByID(ctx, built.ID()); !errors.Is(err, library.ErrWidgetTypeNotFound) {
		t.Errorf("a widget type of another library: %v, want ErrWidgetTypeNotFound", err)
	}
	if list, err := other.FindAll(ctx); err != nil || len(list) != 0 {
		t.Errorf("another library's Draft: %v, %v", list, err)
	}
}
