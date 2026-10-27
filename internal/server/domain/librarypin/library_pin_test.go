package librarypin_test

import (
	"errors"
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/version"
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

// released returns a WidgetType anID as a release holds it, declaring the
// port "value" with the given hint, or no port for hint "-".
func released(t *testing.T, anID id.ID[library.WidgetType], hint string) library.ReleasedWidgetType {
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
	wt, err := library.NewReleasedWidgetType(anID, testLibrary, name, html, script, library.ScriptLanguageJavaScript, library.DefaultSize(), ports)
	if err != nil {
		t.Fatal(err)
	}
	return wt
}

func release(n int, types ...library.ReleasedWidgetType) library.LibraryRelease {
	number, _ := library.NewReleaseNumber(n)
	return library.ReconstituteRelease(testLibrary, number, "", time.Now(), types)
}

func placed(t *testing.T, typeID id.ID[library.WidgetType], tagID id.ID[tag.Tag]) librarypin.PlacedWidget {
	t.Helper()
	name, _ := scene.NewWidgetName("w")
	var bindings []scene.PortBinding
	if tagID != (id.ID[tag.Tag]{}) {
		bindings = []scene.PortBinding{scene.NewPortBinding(portName(t, "value"), tagID)}
	}
	w := scene.NewWidget(id.NewID[scene.Widget](), name, scene.NewPosition(0, 0, 0), library.DefaultSize(),
		scene.DefaultOrigin(), scene.DefaultRotation(), typeID, nil, bindings)
	return librarypin.PlacedWidget{SceneID: id.NewID[scene.Scene](), Widget: w}
}

func TestImpactOf_OrphanedAndBroken(t *testing.T) {
	kept, dropped, narrowed, foreign := id.NewID[library.WidgetType](), id.NewID[library.WidgetType](), id.NewID[library.WidgetType](), id.NewID[library.WidgetType]()
	intTag, strTag, goneTag := id.NewID[tag.Tag](), id.NewID[tag.Tag](), id.NewID[tag.Tag]()
	tagTypes := scene.NewTagTypes(testProject, map[id.ID[tag.Tag]]tag.TagType{intTag: tag.TagTypeInteger, strTag: tag.TagTypeString})

	from := release(1, released(t, kept, ""), released(t, dropped, ""), released(t, narrowed, ""))
	to := release(2, released(t, kept, "-"), released(t, narrowed, "integer"))

	widgets := []librarypin.PlacedWidget{
		placed(t, kept, intTag),      // port dropped: broken
		placed(t, dropped, intTag),   // type dropped: orphaned
		placed(t, narrowed, intTag),  // still accepted
		placed(t, narrowed, strTag),  // narrowed: broken
		placed(t, narrowed, goneTag), // Tag gone: no matter of the pin
		placed(t, foreign, strTag),   // of another Library
	}
	impact := librarypin.ImpactOf(widgets, from, to, tagTypes)

	if len(impact.Orphaned) != 1 || impact.Orphaned[0].Widget != widgets[1].Widget {
		t.Errorf("Orphaned = %v, want the widget of the dropped type", impact.Orphaned)
	}
	if len(impact.Broken) != 2 || impact.Broken[0].Widget != widgets[0].Widget || impact.Broken[1].Widget != widgets[3].Widget {
		t.Errorf("Broken = %v, want the dropped port and the narrowed hint", impact.Broken)
	}
	var orphans librarypin.OrphansError
	if err := impact.Check(); !errors.Is(err, librarypin.ErrWidgetsOrphaned) || !errors.As(err, &orphans) || len(orphans.Widgets) != 1 {
		t.Errorf("Check: %v", err)
	}

	removal := librarypin.ImpactOf(widgets, from, nil, tagTypes)
	if len(removal.Orphaned) != 5 || len(removal.Broken) != 0 {
		t.Errorf("removal: %d orphaned, %d broken; want 5, 0", len(removal.Orphaned), len(removal.Broken))
	}
	if adding := librarypin.ImpactOf(widgets, nil, to, tagTypes); adding.Check() != nil || len(adding.Broken) != 0 {
		t.Errorf("adding a pin touches nothing: %v", adding)
	}
}

func TestLibraryPin_SwitchAndDelete(t *testing.T) {
	first, second := release(1), release(2)
	pin := librarypin.CreateLibraryPin(testProject, first)
	if pin.LibraryID() != testLibrary || pin.Release() != first.Number() || pin.Version() != version.Initial[librarypin.LibraryPin]() {
		t.Fatalf("unexpected pin %s", pin)
	}
	if ev, ok := pin.PendingEvents()[0].(librarypin.LibraryPinEvent); !ok || ev.Type() != event.EventTypeLibraryPinCreated || ev.ProjectID() != testProject || ev.LibraryID() != testLibrary {
		t.Errorf("expected library_pin_created of the pin, got %v", pin.PendingEvents())
	}

	stored := librarypin.ReconstituteLibraryPin(testProject, testLibrary, first.Number(), version.Committed[librarypin.LibraryPin]())
	if _, err := stored.Switch(version.Initial[librarypin.LibraryPin](), second); !errors.Is(err, librarypin.ErrLibraryPinConflict) {
		t.Errorf("stale switch: expected ErrLibraryPinConflict, got %v", err)
	}
	other := library.ReconstituteRelease(id.NewID[library.Library](), second.Number(), "", time.Now(), nil)
	if _, err := stored.Switch(stored.Version(), other); err == nil {
		t.Error("a release of another Library cannot be pinned in place")
	}
	switched, err := stored.Switch(stored.Version(), second)
	if err != nil || switched.Release() != second.Number() || stored.Release() != first.Number() {
		t.Fatalf("Switch: %v, %s", err, switched)
	}
	if got := switched.PendingEvents(); len(got) != 1 || got[0].Type() != event.EventTypeLibraryPinUpdated {
		t.Errorf("expected [library_pin_updated], got %v", got)
	}
	if got := stored.Delete().PendingEvents(); len(got) != 1 || got[0].Type() != event.EventTypeLibraryPinDeleted {
		t.Errorf("expected [library_pin_deleted], got %v", got)
	}
	if _, ok := any(pin.PendingEvents()[0]).(project.ContentEvent); !ok {
		t.Error("a pin's event is content of its Project")
	}
}
