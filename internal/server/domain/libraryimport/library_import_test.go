package libraryimport_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/libraryimport"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

var (
	testProject = id.NewID[project.Project]()
	testLibrary = id.NewID[library.Library]()
)

func portName(t *testing.T, s string) library.InputPortName {
	t.Helper()
	n, err := library.NewInputPortName(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// saved returns a WidgetType anID as a saved state holds it, declaring the
// port "value" with the given hint, or no port for hint "-".
func saved(t *testing.T, anID id.ID[library.WidgetType], hint string) library.WidgetTypeCopy {
	t.Helper()
	name, _ := library.NewWidgetTypeName("gauge")
	html, _ := library.NewHtmlTemplate("")
	script, _ := library.NewScript("")
	var ports []library.InputPort
	if hint != "-" {
		h, err := library.NewPortTypeHint(hint)
		if err != nil {
			t.Fatal(err)
		}
		ports = []library.InputPort{library.NewInputPort(portName(t, "value"), "", h)}
	}
	wt, err := library.NewWidgetTypeCopy(anID, testLibrary, name, html, script, library.ScriptLanguageJavaScript, library.DefaultSize(), ports)
	if err != nil {
		t.Fatal(err)
	}
	return wt
}

// commit returns the saved state number of the test Library holding types.
func commit(t *testing.T, number int, types ...library.WidgetTypeCopy) libraryimport.Commit {
	t.Helper()
	found, err := libraryimport.NewCommit(uuid.New(), testLibrary, number, types)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func placed(t *testing.T, typeID id.ID[library.WidgetType], tagID id.ID[tag.Tag]) libraryimport.PlacedWidget {
	t.Helper()
	name, _ := scene.NewWidgetName("w")
	var bindings []scene.PortBinding
	if tagID != (id.ID[tag.Tag]{}) {
		bindings = []scene.PortBinding{scene.NewPortBinding(portName(t, "value"), tagID)}
	}
	w := scene.NewWidget(id.NewID[scene.Widget](), name, scene.NewPosition(0, 0, 0), library.DefaultSize(),
		scene.DefaultOrigin(), scene.DefaultRotation(), typeID, nil, bindings)
	return libraryimport.PlacedWidget{SceneID: id.NewID[scene.Scene](), Widget: w}
}

func TestImportImpactOf_OrphanedAndBroken(t *testing.T) {
	kept, dropped, narrowed, foreign := id.NewID[library.WidgetType](), id.NewID[library.WidgetType](), id.NewID[library.WidgetType](), id.NewID[library.WidgetType]()
	intTag, strTag, goneTag := id.NewID[tag.Tag](), id.NewID[tag.Tag](), id.NewID[tag.Tag]()
	tagTypes := scene.NewTagTypes(testProject, map[id.ID[tag.Tag]]tag.TagType{intTag: tag.TagTypeInteger, strTag: tag.TagTypeString})

	from := commit(t, 1, saved(t, kept, ""), saved(t, dropped, ""), saved(t, narrowed, ""))
	to := commit(t, 2, saved(t, kept, "-"), saved(t, narrowed, "integer"))

	widgets := []libraryimport.PlacedWidget{
		placed(t, kept, intTag),      // port dropped: broken
		placed(t, dropped, intTag),   // type dropped: orphaned
		placed(t, narrowed, intTag),  // still accepted
		placed(t, narrowed, strTag),  // narrowed: broken
		placed(t, narrowed, goneTag), // Tag gone: no matter of the import
		placed(t, foreign, strTag),   // of another Library
	}
	impact := libraryimport.ImportImpactOf(widgets, &from, &to, tagTypes)

	if len(impact.Orphaned) != 1 || impact.Orphaned[0].Widget != widgets[1].Widget {
		t.Errorf("Orphaned = %v, want the widget of the dropped type", impact.Orphaned)
	}
	if len(impact.Broken) != 2 || impact.Broken[0].Widget != widgets[0].Widget || impact.Broken[1].Widget != widgets[3].Widget {
		t.Errorf("Broken = %v, want the dropped port and the narrowed hint", impact.Broken)
	}
	var orphans libraryimport.OrphansError
	if err := impact.Check(); !errors.Is(err, libraryimport.ErrWidgetsOrphaned) || !errors.As(err, &orphans) || len(orphans.Widgets) != 1 {
		t.Errorf("Check: %v", err)
	}

	removal := libraryimport.ImportImpactOf(widgets, &from, nil, tagTypes)
	if len(removal.Orphaned) != 5 || len(removal.Broken) != 0 {
		t.Errorf("removal: %d orphaned, %d broken; want 5, 0", len(removal.Orphaned), len(removal.Broken))
	}
	if adding := libraryimport.ImportImpactOf(widgets, nil, &to, tagTypes); adding.Check() != nil || len(adding.Broken) != 0 {
		t.Errorf("adding an import touches nothing: %v", adding)
	}
}

func TestLibraryImport_SwitchAndDelete(t *testing.T) {
	first, second := commit(t, 1), commit(t, 2)
	imported := libraryimport.CreateLibraryImport(testProject, first)
	if imported.LibraryID() != testLibrary || imported.CommitNumber() != first.Number() || imported.CommitID() != first.ID() {
		t.Fatalf("unexpected import %s", imported)
	}
	if ev, ok := imported.PendingEvents()[0].(libraryimport.LibraryImportEvent); !ok || ev.Type() != event.EventTypeLibraryImportCreated || ev.ProjectID() != testProject || ev.LibraryID() != testLibrary {
		t.Errorf("expected library_import_created of the import, got %v", imported.PendingEvents())
	}

	stored := libraryimport.ReconstituteLibraryImport(testProject, testLibrary, first.ID(), first.Number())
	other, err := libraryimport.NewCommit(uuid.New(), id.NewID[library.Library](), second.Number(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stored.Switch(other); err == nil {
		t.Error("a saved state of another Library cannot be imported in place")
	}
	switched, err := stored.Switch(second)
	if err != nil || switched.CommitNumber() != second.Number() || switched.CommitID() != second.ID() ||
		stored.CommitNumber() != first.Number() {
		t.Fatalf("Switch: %v, %s", err, switched)
	}
	if got := switched.PendingEvents(); len(got) != 1 || got[0].Type() != event.EventTypeLibraryImportUpdated {
		t.Errorf("expected [library_import_updated], got %v", got)
	}
	if got := stored.Delete().PendingEvents(); len(got) != 1 || got[0].Type() != event.EventTypeLibraryImportDeleted {
		t.Errorf("expected [library_import_deleted], got %v", got)
	}
	published := switched.PendingEvents()[0]
	if _, ok := any(published).(project.ContentEvent); !ok {
		t.Error("an import's event is content of its Project")
	}
}

func TestNewCommit_Checks(t *testing.T) {
	if _, err := libraryimport.NewCommit(uuid.Nil, testLibrary, 1, nil); err == nil {
		t.Error("a commit needs an ID")
	}
	if _, err := libraryimport.NewCommit(uuid.New(), id.ID[library.Library]{}, 1, nil); err == nil {
		t.Error("a commit needs its library")
	}
	if _, err := libraryimport.NewCommit(uuid.New(), testLibrary, 0, nil); err == nil {
		t.Error("a commit number is positive")
	}
}
