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
		    (id, library_id, name, html_template, script, script_language, input_ports, default_width, default_height)
		 VALUES ($2, $1, $3, '<div></div>', 'function update(){}', 'javascript', '[]', 100, 100)
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
		`INSERT INTO engineering.library_pins (project_id, library_id, release_number)
		 VALUES ($1, $2, 1) ON CONFLICT DO NOTHING`,
		projectID, libraryID,
	); err != nil {
		t.Fatalf("ensurePinnedType: pin: %v", err)
	}
}

// newLibraryContent opens the Libraries over the test database, their writes
// through the Drafts.
func newLibraryContent(opts ...repositories.Option) application.LibraryContent {
	return application.NewLibraryContent(repositories.NewLibraryRepositoryPostgres(testDB, opts...),
		repositories.NewDraftRepositoryPostgres(testDB, repositories.NewLibraryReleaseRepositoryPostgres(testDB, opts...), opts...),
		soleOrganization())
}

func newLibraryService() application.LibraryService {
	return application.NewLibraryService(newLibraryContent(), repositories.NewLibraryRepositoryPostgres(testDB),
		repositories.NewLibraryReleaseRepositoryPostgres(testDB), repositories.NewWidgetTypeRepositories(testDB), newRuntimeNode())
}

// libraryDraftVersionOf returns the Version of a Library's Draft, 1 for one
// never written.
func libraryDraftVersionOf(t *testing.T, libraryID uuid.UUID) int {
	t.Helper()
	drafts := repositories.NewDraftRepositoryPostgres(testDB, nil)
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

// mustCommitLibrary saves the library's Draft as a Commit as it is.
func mustCommitLibrary(t *testing.T, libraryID uuid.UUID) appdto.Commit {
	t.Helper()
	commit, err := newCommitService().Save(context.Background(),
		draft.LibraryScope(id.NewID(id.IDWithUUID[library.Library](libraryID))),
		libraryDraftVersionOf(t, libraryID), "Save", "Ada")
	if err != nil {
		t.Fatalf("Save commit: %v", err)
	}
	return commit
}

// mustReleaseLibrary saves the library's Draft as a Commit and publishes it as
// the next Library Release (task 72_5; ADR 0021).
func mustReleaseLibrary(t *testing.T, libraryID uuid.UUID) appdto.LibraryRelease {
	t.Helper()
	svc := newLibraryService()
	if _, err := svc.FindLibraryByID(context.Background(), libraryID); err != nil {
		t.Fatalf("FindLibraryByID: %v", err)
	}
	commit := mustCommitLibrary(t, libraryID)
	release, err := svc.Release(context.Background(), libraryID, appdto.ReleaseInput{Commit: commit.Number, Author: "Ada"})
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
	wt, err := newWidgetTypeService().CreateWidgetType(ctx, lib.ID, libraryDraftVersionOf(t, lib.ID), input)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	release := mustReleaseLibrary(t, lib.ID)
	if _, _, err := newLibraryPinService().PutLibraryPin(ctx, projectID, lib.ID, release.Number, draftVersionOf(t, projectID)); err != nil {
		t.Fatalf("PutLibraryPin: %v", err)
	}
	return wt, lib
}
