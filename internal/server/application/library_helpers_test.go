package application_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// testLibrary is the Library of the shared widget fixtures; its saved state 1
// holds testWidgetTypeID and is imported in mainProject.
var testLibrary = uuid.MustParse("7e57c0de-1000-4000-8000-000000000001")

// testWidgetTypeID is the WidgetType of the shared widget fixtures: a
// Widget's type must be pinned, so ensureTestWidgetType stores and pins it.
var testWidgetTypeID = uuid.MustParse("7e57c0de-0000-4000-8000-000000000001")

// ensureTestWidgetType stores testLibrary with testWidgetTypeID, its saved
// state 1 and its import in mainProject, unless they are there.
func ensureTestWidgetType(t *testing.T) {
	t.Helper()
	ensurePinnedType(t, testLibrary, "test-library", testWidgetTypeID, "test-type", mainProject)
}

// ensurePinnedType stores, unless they are there, the library libraryID
// named libraryName with the WidgetType typeID named typeName and no ports,
// its saved state 1 holding it, and the import of that saved state in
// projectID (task 85_6).
func ensurePinnedType(t *testing.T, libraryID uuid.UUID, libraryName string, typeID uuid.UUID, typeName string, projectID uuid.UUID) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(),
		`INSERT INTO engineering.libraries (id, organization_id, name, version)
		     SELECT $1, id, $2, 1 FROM organizations ON CONFLICT (id) DO NOTHING;`,
		libraryID, libraryName,
	); err != nil {
		t.Fatalf("ensurePinnedType: library: %v", err)
	}
	for _, q := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO engineering.widget_types
		    (id, library_id, name, html_template, script, script_language, input_ports, default_width, default_height)
		 VALUES ($2, $1, $3, '<div></div>', 'function update(){}', 'javascript', '[]', 100, 100)
		 ON CONFLICT (id) DO NOTHING`, []any{libraryID, typeID, typeName}},
	} {
		if _, err := testDB.ExecContext(context.Background(), q.query, q.args...); err != nil {
			t.Fatalf("ensurePinnedType: %v", err)
		}
	}
	savedState := mustSaveLibraryState(t, libraryID)
	if _, _, err := newLibraryImportService(t).PutLibraryImport(context.Background(),
		projectID, libraryID, savedState.Number, draftVersionOf(t, projectID)); err != nil {
		t.Fatalf("ensurePinnedType: import: %v", err)
	}
}

// newLibraryContent opens the Libraries over the test database, their writes
// through the Drafts.
func newLibraryContent(opts ...repositories.Option) application.LibraryContent {
	return application.NewLibraryContent(repositories.NewLibraryRepositoryPostgres(testDB, opts...),
		repositories.NewDraftRepositoryPostgres(testDB, opts...),
		soleOrganization())
}

func newLibraryService() application.LibraryService {
	return application.NewLibraryService(newLibraryContent(), repositories.NewLibraryRepositoryPostgres(testDB),
		repositories.NewWidgetTypeRepositories(testDB), newRuntimeNode())
}

// libraryDraftVersionOf returns the Version of a Library's Draft, 1 for one
// never written.
func libraryDraftVersionOf(t *testing.T, libraryID uuid.UUID) int {
	t.Helper()
	drafts := repositories.NewDraftRepositoryPostgres(testDB)
	found, ok, err := drafts.Read(context.Background(),
		draft.LibraryScope(id.NewID(id.IDWithUUID[library.Library](libraryID))))
	if err != nil {
		t.Fatalf("read the draft of library %s: %v", libraryID, err)
	}
	if !ok {
		return 1
	}
	return found.Version().Number()
}

func newLibraryImportService(t *testing.T) application.LibraryImportService {
	t.Helper()
	return application.NewLibraryImportService(contentOver(newScopes()),
		repositories.NewLibraryRepositoryPostgres(testDB), newLabelService(t))
}

// mustCreateLibrary creates a library of a name of its own.
func mustCreateLibrary(t *testing.T) appdto.Library {
	t.Helper()
	lib, err := newLibraryService().CreateLibrary(context.Background(), appdto.LibraryInput{Name: "lib-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}
	return lib
}

// mustSaveLibraryState saves the library's Draft as a SavedState as it is.
func mustSaveLibraryState(t *testing.T, libraryID uuid.UUID) appdto.SavedState {
	t.Helper()
	return mustSaveLibraryStateTitled(t, libraryID, "Save", "Ada")
}

// mustSaveLibraryStateTitled saves the library's Draft as a SavedState titled
// title.
func mustSaveLibraryStateTitled(t *testing.T, libraryID uuid.UUID, title, author string) appdto.SavedState {
	t.Helper()
	savedState, err := newSavedStateService().Save(context.Background(),
		draft.LibraryScope(id.NewID(id.IDWithUUID[library.Library](libraryID))),
		libraryDraftVersionOf(t, libraryID), title, author)
	if err != nil {
		t.Fatalf("Save savedState: %v", err)
	}
	return savedState
}

// importNewWidgetType creates, in a new library, a WidgetType of input (its
// name made unique), releases it and imports its saved state in projectID; it
// returns the type and its library.
func importNewWidgetType(t *testing.T, projectID uuid.UUID, input appdto.WidgetTypeInput) (appdto.WidgetType, appdto.Library) {
	t.Helper()
	ctx := context.Background()
	lib := mustCreateLibrary(t)
	if input.Name == "" {
		input.Name = "type"
	}
	if input.ScriptLanguage == "" {
		input.ScriptLanguage = "javascript"
	}
	if input.DefaultWidth == 0 {
		input.DefaultWidth, input.DefaultHeight = 100, 100
	}
	wt, err := newWidgetTypeService().CreateWidgetType(ctx, lib.ID, libraryDraftVersionOf(t, lib.ID), input)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	savedState := mustSaveLibraryState(t, lib.ID)
	if _, _, err := newLibraryImportService(t).PutLibraryImport(ctx, projectID, lib.ID, savedState.Number, draftVersionOf(t, projectID)); err != nil {
		t.Fatalf("PutLibraryImport: %v", err)
	}
	return wt, lib
}
