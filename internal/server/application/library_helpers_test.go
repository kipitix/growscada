package application_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// testLibrary is the Library of the shared widget fixtures; its release 1
// holds testWidgetTypeID and is pinned in mainProject.
var testLibrary = uuid.MustParse("7e57c0de-1000-4000-8000-000000000001")

// testWidgetTypeID is the WidgetType of the shared widget fixtures: a
// Widget's type must be pinned, so ensureTestWidgetType stores and pins it.
var testWidgetTypeID = uuid.MustParse("7e57c0de-0000-4000-8000-000000000001")

// ensureTestWidgetType stores testLibrary with testWidgetTypeID, its release
// 1 and its pin in mainProject, unless they are there.
func ensureTestWidgetType(t *testing.T) {
	t.Helper()
	ensurePinnedType(t, testLibrary, "test-library", testWidgetTypeID, "test-type", mainProject)
}

// ensurePinnedType stores, unless they are there, the library libraryID
// named libraryName with the WidgetType typeID named typeName and no ports,
// its release 1 holding it, and the pin of the release in projectID.
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
		    (id, library_id, name, html_template, script, script_language, input_ports, default_width, default_height, version)
		 VALUES ($2, $1, $3, '<div></div>', 'function update(){}', 'javascript', '[]', 100, 100, 1)
		 ON CONFLICT (id) DO NOTHING`, []any{libraryID, typeID, typeName}},
		{`INSERT INTO engineering.library_releases (library_id, number) VALUES ($1, 1) ON CONFLICT DO NOTHING`, []any{libraryID}},
		{`INSERT INTO engineering.released_widget_types
		    (library_id, release_number, id, name, html_template, script, script_language, input_ports, default_width, default_height)
		 VALUES ($1, 1, $2, $3, '<div></div>', 'function update(){}', 'javascript', '[]', 100, 100)
		 ON CONFLICT DO NOTHING`, []any{libraryID, typeID, typeName}},
	} {
		if _, err := testDB.ExecContext(context.Background(), q.query, q.args...); err != nil {
			t.Fatalf("ensurePinnedType: %v", err)
		}
	}
	if _, err := testDB.ExecContext(context.Background(),
		`INSERT INTO engineering.library_pins (project_id, library_id, release_number, version)
		 VALUES ($1, $2, 1, 1) ON CONFLICT DO NOTHING`,
		projectID, libraryID,
	); err != nil {
		t.Fatalf("ensurePinnedType: pin: %v", err)
	}
}

func newLibraryService() application.LibraryService {
	return application.NewLibraryService(repositories.NewLibraryRepositoryPostgres(testDB), repositories.NewLibraryReleaseRepositoryPostgres(testDB),
		repositories.NewWidgetTypeRepositories(testDB), soleOrganization())
}

func newLibraryPinService() application.LibraryPinService {
	return application.NewLibraryPinService(contentOver(newScopes()), repositories.NewLibraryRepositoryPostgres(testDB), repositories.NewLibraryReleaseRepositoryPostgres(testDB))
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

// mustReleaseLibrary releases the library's Draft as it is.
func mustReleaseLibrary(t *testing.T, libraryID uuid.UUID) appdto.LibraryRelease {
	t.Helper()
	svc := newLibraryService()
	lib, err := svc.FindLibraryByID(context.Background(), libraryID)
	if err != nil {
		t.Fatalf("FindLibraryByID: %v", err)
	}
	release, err := svc.Release(context.Background(), libraryID, lib.Version, appdto.ReleaseInput{})
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	return release
}

// pinNewWidgetType creates, in a new library, a WidgetType of input
// (its name made unique), releases it and pins the release in projectID; it
// returns the type and its library.
func pinNewWidgetType(t *testing.T, projectID uuid.UUID, input appdto.WidgetTypeInput) (appdto.WidgetType, appdto.Library) {
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
	wt, err := newWidgetTypeService().CreateWidgetType(ctx, lib.ID, input)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	release := mustReleaseLibrary(t, lib.ID)
	if _, _, err := newLibraryPinService().PutLibraryPin(ctx, projectID, lib.ID, release.Number, 0); err != nil {
		t.Fatalf("PutLibraryPin: %v", err)
	}
	return wt, lib
}
