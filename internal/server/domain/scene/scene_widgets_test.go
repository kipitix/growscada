package scene

import (
	"errors"
	"slices"
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// ── Fixtures ────────────────────────────────────────────────────────────────

func mustPortName(t *testing.T, s string) library.InputPortName {
	t.Helper()
	n, err := library.NewInputPortName(s)
	if err != nil {
		t.Fatalf("NewInputPortName(%q): %v", s, err)
	}
	return n
}

// makeWidgetType builds a WidgetType declaring the given ports.
func makeWidgetType(t *testing.T, ports ...string) library.WidgetType {
	t.Helper()
	name, _ := library.NewWidgetTypeName("gauge")
	html, _ := library.NewHtmlTemplate("<div></div>")
	script, _ := library.NewScript("function update(){}")
	inputPorts := make([]library.InputPort, len(ports))
	for i, p := range ports {
		inputPorts[i] = library.NewInputPort(mustPortName(t, p), "", library.AnyTypeHint())
	}
	wt, err := library.ReconstituteWidgetType(id.NewID[library.WidgetType](), name, html, script,
		library.ScriptLanguageJavaScript, library.DefaultSize(), inputPorts, version.Committed[library.WidgetType]())
	if err != nil {
		t.Fatalf("ReconstituteWidgetType: %v", err)
	}
	return wt
}

// makeWidgetOf builds a Widget of WidgetType wt binding the given ports.
func makeWidgetOf(t *testing.T, wt library.WidgetType, name string, ports ...string) Widget {
	t.Helper()
	widgetName, _ := NewWidgetName(name)
	bindings := make([]PortBinding, len(ports))
	for i, p := range ports {
		bindings[i] = NewPortBinding(mustPortName(t, p), id.NewID[tag.Tag]())
	}
	return NewWidget(id.NewID[Widget](), widgetName, NewPosition(0, 0, 0), library.DefaultSize(),
		DefaultOrigin(), DefaultRotation(), wt.ID(), nil, bindings)
}

// makeStoredScene builds a Scene as read back from a repository: committed, with widgets.
func makeStoredScene(t *testing.T, someWidgets ...Widget) Scene {
	t.Helper()
	name, _ := NewSceneName("boiler_room")
	size, _ := NewSceneSize(1920, 1080)
	ver, _ := version.New(version.WithNumber[Scene](3))
	return ReconstituteScene(id.NewID[Scene](), name, size, NewBackgroundHTML(""), someWidgets, ver)
}

func staleVersion(sc Scene) version.Version[Scene] {
	ver, _ := version.New(version.WithNumber[Scene](sc.Version().Number() - 1))
	return ver
}

func ids(ws []Widget) []id.ID[Widget] {
	out := make([]id.ID[Widget], len(ws))
	for i, w := range ws {
		out[i] = w.ID()
	}
	return out
}

// assertUnchanged checks that a change method left the original Scene as it was.
func assertUnchanged(t *testing.T, sc Scene, wantWidgets []Widget) {
	t.Helper()
	if got := sc.Widgets(); !slices.Equal(got, wantWidgets) {
		t.Errorf("original scene changed: widgets %v, want %v", ids(got), ids(wantWidgets))
	}
}

// ── Update ──────────────────────────────────────────────────────────────────

func TestSceneUpdate_ChangesFieldsKeepsWidgetsAndVersion(t *testing.T) {
	wt := makeWidgetType(t)
	w := makeWidgetOf(t, wt, "w1")
	sc := makeStoredScene(t, w)
	name, _ := NewSceneName("renamed")
	size, _ := NewSceneSize(800, 600)

	after, err := sc.Update(sc.Version(), name, size, NewBackgroundHTML("<svg/>"))

	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if after.Name() != name || after.Size() != size || after.BackgroundHTML().Content() != "<svg/>" {
		t.Errorf("fields not changed: %s", after)
	}
	if after.Version() != sc.Version() {
		t.Errorf("Version: a change keeps it (%d), got %d", sc.Version().Number(), after.Version().Number())
	}
	if !slices.Equal(after.Widgets(), []Widget{w}) {
		t.Errorf("widgets: expected %v, got %v", ids([]Widget{w}), ids(after.Widgets()))
	}
	if sc.Name().String() != "boiler_room" {
		t.Errorf("original scene renamed to %q", sc.Name())
	}
}

func TestSceneUpdate_StaleVersion_ReturnsConflict(t *testing.T) {
	sc := makeStoredScene(t)

	_, err := sc.Update(staleVersion(sc), sc.Name(), sc.Size(), sc.BackgroundHTML())

	if !errors.Is(err, ErrSceneConflict) {
		t.Errorf("expected ErrSceneConflict, got %v", err)
	}
}

// ── AddWidget ───────────────────────────────────────────────────────────────

func TestSceneAddWidget_Valid_AppendsWidgetLeavesOriginal(t *testing.T) {
	wt := makeWidgetType(t, "value")
	existing := makeWidgetOf(t, wt, "w1")
	sc := makeStoredScene(t, existing)
	added := makeWidgetOf(t, wt, "w2", "value")

	after, err := sc.AddWidget(sc.Version(), added, wt)

	if err != nil {
		t.Fatalf("AddWidget: %v", err)
	}
	if got, want := ids(after.Widgets()), ids([]Widget{existing, added}); !slices.Equal(got, want) {
		t.Errorf("widgets: expected %v, got %v", want, got)
	}
	if after.Version() != sc.Version() {
		t.Errorf("Version: a change keeps it (%d), got %d", sc.Version().Number(), after.Version().Number())
	}
	assertUnchanged(t, sc, []Widget{existing})
}

func TestSceneAddWidget_Rejected(t *testing.T) {
	wt := makeWidgetType(t, "value")
	existing := makeWidgetOf(t, wt, "w1")
	sc := makeStoredScene(t, existing)

	tests := map[string]struct {
		expected version.Version[Scene]
		widget   Widget
		wt       library.WidgetType
		want     error
	}{
		"stale version":       {staleVersion(sc), makeWidgetOf(t, wt, "w2"), wt, ErrSceneConflict},
		"widget ID taken":     {sc.Version(), existing, wt, ErrWidgetAlreadyExists},
		"other widget type":   {sc.Version(), makeWidgetOf(t, wt, "w2"), makeWidgetType(t, "value"), ErrWidgetTypeMismatch},
		"undeclared port":     {sc.Version(), makeWidgetOf(t, wt, "w2", "value", "missing"), wt, ErrPortNotDeclared},
		"conflict goes first": {staleVersion(sc), makeWidgetOf(t, wt, "w2", "missing"), wt, ErrSceneConflict},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := sc.AddWidget(tt.expected, tt.widget, tt.wt)
			if !errors.Is(err, tt.want) {
				t.Errorf("expected %v, got %v", tt.want, err)
			}
			assertUnchanged(t, sc, []Widget{existing})
		})
	}
}

// ── UpdateWidget ────────────────────────────────────────────────────────────

func TestSceneUpdateWidget_Valid_ReplacesWidgetInPlaceLeavesOriginal(t *testing.T) {
	wt := makeWidgetType(t, "value")
	w1 := makeWidgetOf(t, wt, "w1")
	w2 := makeWidgetOf(t, wt, "w2")
	sc := makeStoredScene(t, w1, w2)
	newName, _ := NewWidgetName("w1-renamed")
	changed := NewWidget(w1.ID(), newName, NewPosition(5, 5, 0), w1.Size(), w1.Origin(), w1.Rotation(),
		w1.TypeID(), nil, []PortBinding{NewPortBinding(mustPortName(t, "value"), id.NewID[tag.Tag]())})

	after, err := sc.UpdateWidget(sc.Version(), changed, wt)

	if err != nil {
		t.Fatalf("UpdateWidget: %v", err)
	}
	if got := after.Widgets(); !slices.Equal(got, []Widget{changed, w2}) {
		t.Errorf("widgets: expected the changed widget in place, got %v", ids(got))
	}
	assertUnchanged(t, sc, []Widget{w1, w2})
}

func TestSceneUpdateWidget_Rejected(t *testing.T) {
	wt := makeWidgetType(t, "value")
	existing := makeWidgetOf(t, wt, "w1")
	sc := makeStoredScene(t, existing)
	withPorts := func(ports ...string) Widget {
		bindings := make([]PortBinding, len(ports))
		for i, p := range ports {
			bindings[i] = NewPortBinding(mustPortName(t, p), id.NewID[tag.Tag]())
		}
		return NewWidget(existing.ID(), existing.Name(), existing.Position(), existing.Size(), existing.Origin(),
			existing.Rotation(), existing.TypeID(), nil, bindings)
	}

	tests := map[string]struct {
		expected version.Version[Scene]
		widget   Widget
		wt       library.WidgetType
		want     error
	}{
		"stale version":     {staleVersion(sc), withPorts(), wt, ErrSceneConflict},
		"unknown widget":    {sc.Version(), makeWidgetOf(t, wt, "ghost"), wt, ErrWidgetNotFound},
		"other widget type": {sc.Version(), withPorts(), makeWidgetType(t, "value"), ErrWidgetTypeMismatch},
		"undeclared port":   {sc.Version(), withPorts("missing"), wt, ErrPortNotDeclared},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := sc.UpdateWidget(tt.expected, tt.widget, tt.wt)
			if !errors.Is(err, tt.want) {
				t.Errorf("expected %v, got %v", tt.want, err)
			}
			assertUnchanged(t, sc, []Widget{existing})
		})
	}
}

// ── CheckVersion ────────────────────────────────────────────────────────────

func TestSceneCheckVersion(t *testing.T) {
	sc := makeStoredScene(t)

	if err := sc.CheckVersion(sc.Version()); err != nil {
		t.Errorf("current version: expected no error, got %v", err)
	}
	if err := sc.CheckVersion(staleVersion(sc)); !errors.Is(err, ErrSceneConflict) {
		t.Errorf("stale version: expected ErrSceneConflict, got %v", err)
	}
}

// ── RemoveWidget ────────────────────────────────────────────────────────────

func TestSceneRemoveWidget_Existing_RemovesItLeavesOriginal(t *testing.T) {
	wt := makeWidgetType(t)
	w1 := makeWidgetOf(t, wt, "w1")
	w2 := makeWidgetOf(t, wt, "w2")
	w3 := makeWidgetOf(t, wt, "w3")
	sc := makeStoredScene(t, w1, w2, w3)

	after, removed, err := sc.RemoveWidget(w2.ID())

	if err != nil {
		t.Fatalf("RemoveWidget: %v", err)
	}
	if removed.ID() != w2.ID() {
		t.Errorf("removed: expected %s, got %s", w2.ID(), removed.ID())
	}
	if got, want := ids(after.Widgets()), ids([]Widget{w1, w3}); !slices.Equal(got, want) {
		t.Errorf("widgets: expected %v, got %v", want, got)
	}
	if after.Version() != sc.Version() {
		t.Errorf("Version: a change keeps it (%d), got %d", sc.Version().Number(), after.Version().Number())
	}
	assertUnchanged(t, sc, []Widget{w1, w2, w3})
}

func TestSceneRemoveWidget_Unknown_ReturnsErrWidgetNotFound(t *testing.T) {
	sc := makeStoredScene(t, makeWidgetOf(t, makeWidgetType(t), "w1"))

	_, _, err := sc.RemoveWidget(id.NewID[Widget]())

	if !errors.Is(err, ErrWidgetNotFound) {
		t.Errorf("expected ErrWidgetNotFound, got %v", err)
	}
}

// ── FindWidget ──────────────────────────────────────────────────────────────

func TestSceneFindWidget(t *testing.T) {
	w := makeWidgetOf(t, makeWidgetType(t), "w1")
	sc := makeStoredScene(t, w)

	if found, err := sc.FindWidget(w.ID()); err != nil || found != w {
		t.Errorf("FindWidget(existing): got %v, %v", found, err)
	}
	if _, err := sc.FindWidget(id.NewID[Widget]()); !errors.Is(err, ErrWidgetNotFound) {
		t.Errorf("FindWidget(unknown): expected ErrWidgetNotFound, got %v", err)
	}
}

// ── ReconcileWith ───────────────────────────────────────────────────────────

func TestSceneReconcileWith_RemovesBindingsOfUndeclaredPortsOnly(t *testing.T) {
	before := makeWidgetType(t, "value", "alarm")
	other := makeWidgetType(t, "alarm")
	w := makeWidgetOf(t, before, "w1", "value", "alarm")
	untouched := makeWidgetOf(t, other, "w2", "alarm")
	sc := makeStoredScene(t, w, untouched)
	after := makeWidgetTypeWithID(t, before.ID(), "value")

	reconciled, changed := sc.ReconcileWith(after)

	if !changed {
		t.Fatal("expected a change")
	}
	got := reconciled.Widgets()
	if len(got[0].PortBindings()) != 1 || got[0].PortBindings()[0].PortName().String() != "value" {
		t.Errorf("w1 bindings: expected only 'value', got %v", got[0].PortBindings())
	}
	if got[1] != untouched {
		t.Error("a widget of another type must not change")
	}
	if reconciled.Version() != sc.Version() {
		t.Errorf("Version: a change keeps it (%d), got %d", sc.Version().Number(), reconciled.Version().Number())
	}
	assertUnchanged(t, sc, []Widget{w, untouched})
	if len(w.PortBindings()) != 2 {
		t.Errorf("original widget changed: %d bindings, want 2", len(w.PortBindings()))
	}
}

func TestSceneReconcileWith_NothingUndeclared_ReportsNoChange(t *testing.T) {
	wt := makeWidgetType(t, "value")
	sc := makeStoredScene(t, makeWidgetOf(t, wt, "w1", "value"))

	reconciled, changed := sc.ReconcileWith(wt)

	if changed {
		t.Error("expected no change")
	}
	if reconciled != sc {
		t.Error("expected the same scene when nothing changes")
	}
}

// makeWidgetTypeWithID builds the WidgetType anID as it looks after an
// update that left it only the given ports.
func makeWidgetTypeWithID(t *testing.T, anID id.ID[library.WidgetType], ports ...string) library.WidgetType {
	t.Helper()
	wt := makeWidgetType(t, ports...)
	updated, err := library.ReconstituteWidgetType(anID, wt.Name(), wt.HtmlTemplate(), wt.Script(), wt.ScriptLanguage(),
		wt.DefaultSize(), wt.InputPorts(), wt.Version().Next())
	if err != nil {
		t.Fatalf("ReconstituteWidgetType: %v", err)
	}
	return updated
}
