package scene

import (
	"errors"
	"slices"
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
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

// tagTypesOf returns TagTypes in which every Tag someWidgets bind exists, as
// an Integer Tag.
func tagTypesOf(someWidgets ...Widget) TagTypes {
	types := map[id.ID[tag.Tag]]tag.TagType{}
	for _, tagID := range BoundTagIDs(someWidgets...) {
		types[tagID] = tag.TagTypeInteger
	}
	return NewTagTypes(testProject, types)
}

// makeTypedWidgetType builds a WidgetType declaring one port, "value", with
// the given type hint ("" for any TagType).
func makeTypedWidgetType(t *testing.T, hint string) library.WidgetType {
	t.Helper()
	wt := makeWidgetType(t)
	return withValueHint(t, wt, hint, wt.Version())
}

// withValueHint builds the WidgetType wt, at aVersion, declaring one port,
// "value", with the given type hint.
func withValueHint(t *testing.T, wt library.WidgetType, hint string, aVersion version.Version[library.WidgetType]) library.WidgetType {
	t.Helper()
	typeHint, err := library.NewPortTypeHint(hint)
	if err != nil {
		t.Fatalf("NewPortTypeHint(%q): %v", hint, err)
	}
	typed, err := library.ReconstituteWidgetType(wt.ID(), wt.Name(), wt.HtmlTemplate(), wt.Script(), wt.ScriptLanguage(),
		wt.DefaultSize(), []library.InputPort{library.NewInputPort(mustPortName(t, "value"), "", typeHint)}, aVersion)
	if err != nil {
		t.Fatalf("ReconstituteWidgetType: %v", err)
	}
	return typed
}

// makeStoredScene builds a Scene as read back from a repository: committed, with widgets.
func makeStoredScene(t *testing.T, someWidgets ...Widget) Scene {
	t.Helper()
	name, _ := NewSceneName("boiler_room")
	size, _ := NewSceneSize(1920, 1080)
	ver, _ := version.New(version.WithNumber[Scene](3))
	return ReconstituteScene(id.NewID[Scene](), testProject, name, size, NewBackgroundHTML(""), someWidgets, ver)
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

	after, err := sc.AddWidget(sc.Version(), added, wt, tagTypesOf(added))

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
			_, err := sc.AddWidget(tt.expected, tt.widget, tt.wt, tagTypesOf(tt.widget))
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

	after, err := sc.UpdateWidget(sc.Version(), changed, wt, tagTypesOf(changed))

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
			_, err := sc.UpdateWidget(tt.expected, tt.widget, tt.wt, tagTypesOf(tt.widget))
			if !errors.Is(err, tt.want) {
				t.Errorf("expected %v, got %v", tt.want, err)
			}
			assertUnchanged(t, sc, []Widget{existing})
		})
	}
}

// ── PortBinding validity ────────────────────────────────────────────────────

// bindValue returns a widget of WidgetType wt (as w1, if given) binding its
// port "value" to tagID.
func bindValue(t *testing.T, wt library.WidgetType, tagID id.ID[tag.Tag], as ...Widget) Widget {
	t.Helper()
	widgetID := id.NewID[Widget]()
	if len(as) > 0 {
		widgetID = as[0].ID()
	}
	name, _ := NewWidgetName("bound")
	return NewWidget(widgetID, name, NewPosition(0, 0, 0), library.DefaultSize(), DefaultOrigin(), DefaultRotation(),
		wt.ID(), nil, []PortBinding{NewPortBinding(mustPortName(t, "value"), tagID)})
}

func TestSceneWidgetChanges_CheckBoundTagTypes(t *testing.T) {
	intTag, strTag, missingTag := id.NewID[tag.Tag](), id.NewID[tag.Tag](), id.NewID[tag.Tag]()
	tagTypes := NewTagTypes(testProject, map[id.ID[tag.Tag]]tag.TagType{
		intTag: tag.TagTypeInteger,
		strTag: tag.TagTypeString,
	})

	tests := map[string]struct {
		hint  string
		tagID id.ID[tag.Tag]
		want  error
	}{
		"hint accepts the tag type":   {"integer", intTag, nil},
		"any hint accepts every type": {"", strTag, nil},
		"hint refuses the tag type":   {"integer", strTag, ErrPortTypeMismatch},
		"tag does not exist":          {"", missingTag, ErrBoundTagNotFound},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			wt := makeTypedWidgetType(t, tt.hint)
			existing := makeWidgetOf(t, wt, "w1")
			sc := makeStoredScene(t, existing)

			_, addErr := sc.AddWidget(sc.Version(), bindValue(t, wt, tt.tagID), wt, tagTypes)
			_, updateErr := sc.UpdateWidget(sc.Version(), bindValue(t, wt, tt.tagID, existing), wt, tagTypes)

			for op, err := range map[string]error{"AddWidget": addErr, "UpdateWidget": updateErr} {
				if tt.want == nil && err != nil {
					t.Errorf("%s: expected no error, got %v", op, err)
				}
				if tt.want != nil && !errors.Is(err, tt.want) {
					t.Errorf("%s: expected %v, got %v", op, tt.want, err)
				}
			}
			assertUnchanged(t, sc, []Widget{existing})
		})
	}
}

// mustReconcile is ReconcileWith failing the test on an error.
func mustReconcile(t *testing.T, sc Scene, wt library.WidgetType, tagTypes TagTypes) (Scene, bool) {
	t.Helper()
	reconciled, changed, err := sc.ReconcileWith(wt, tagTypes)
	if err != nil {
		t.Fatalf("ReconcileWith: %v", err)
	}
	return reconciled, changed
}

// TestSceneWidgetChanges_BindOnlyTagsOfTheScenesProject checks that a Tag
// known only as a Tag of another Project cannot be bound (ADR 0006): the
// Scene takes the TagTypes of its own Project only.
func TestSceneWidgetChanges_BindOnlyTagsOfTheScenesProject(t *testing.T) {
	foreignTag := id.NewID[tag.Tag]()
	ofOtherProject := NewTagTypes(id.NewID[project.Project](), map[id.ID[tag.Tag]]tag.TagType{foreignTag: tag.TagTypeInteger})
	wt := makeTypedWidgetType(t, "")
	existing := makeWidgetOf(t, wt, "w1")
	sc := makeStoredScene(t, existing)

	_, addErr := sc.AddWidget(sc.Version(), bindValue(t, wt, foreignTag), wt, ofOtherProject)
	_, updateErr := sc.UpdateWidget(sc.Version(), bindValue(t, wt, foreignTag, existing), wt, ofOtherProject)

	for op, err := range map[string]error{"AddWidget": addErr, "UpdateWidget": updateErr} {
		if !errors.Is(err, ErrTagTypesOfAnotherProject) {
			t.Errorf("%s: expected ErrTagTypesOfAnotherProject, got %v", op, err)
		}
	}
}

func TestSceneAddWidget_UndeclaredPortGoesBeforeMissingTag(t *testing.T) {
	wt := makeWidgetType(t, "value")
	sc := makeStoredScene(t)

	_, err := sc.AddWidget(sc.Version(), makeWidgetOf(t, wt, "w1", "missing"), wt, NewTagTypes(testProject, nil))

	if !errors.Is(err, ErrPortNotDeclared) {
		t.Errorf("expected ErrPortNotDeclared, got %v", err)
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

	reconciled, changed := mustReconcile(t, sc, after, tagTypesOf(sc.Widgets()...))

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

	reconciled, changed := mustReconcile(t, sc, wt, tagTypesOf(sc.Widgets()...))

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

func TestSceneReconcileWith_RemovesBindingsTheNarrowedHintRefuses(t *testing.T) {
	anyType := makeTypedWidgetType(t, "")
	intTag, strTag := id.NewID[tag.Tag](), id.NewID[tag.Tag]()
	tagTypes := NewTagTypes(testProject, map[id.ID[tag.Tag]]tag.TagType{
		intTag: tag.TagTypeInteger,
		strTag: tag.TagTypeString,
	})
	kept := bindValue(t, anyType, intTag)
	refused := bindValue(t, anyType, strTag)
	sc := makeStoredScene(t, kept, refused)
	narrowed := withValueHint(t, anyType, "integer", anyType.Version().Next())

	reconciled, changed := mustReconcile(t, sc, narrowed, tagTypes)

	if !changed {
		t.Fatal("expected a change")
	}
	got := reconciled.Widgets()
	if got[0] != kept {
		t.Error("a binding the hint still accepts must stay")
	}
	if len(got[1].PortBindings()) != 0 {
		t.Errorf("refused binding: expected it removed, got %v", got[1].PortBindings())
	}
}

func TestSceneReconcileWith_RemovesBindingsOfMissingTags(t *testing.T) {
	wt := makeWidgetType(t, "value", "alarm")
	w := makeWidgetOf(t, wt, "w1", "value", "alarm")
	alarmTag := w.PortBindings()[1].TagID()
	sc := makeStoredScene(t, w)
	onlyValue := NewTagTypes(testProject, map[id.ID[tag.Tag]]tag.TagType{w.PortBindings()[0].TagID(): tag.TagTypeInteger})

	reconciled, changed := mustReconcile(t, sc, wt, onlyValue)

	if !changed {
		t.Fatal("expected a change")
	}
	for _, b := range reconciled.Widgets()[0].PortBindings() {
		if b.TagID() == alarmTag {
			t.Errorf("binding of the missing tag %s not removed", alarmTag)
		}
	}
	if n := len(reconciled.Widgets()[0].PortBindings()); n != 1 {
		t.Errorf("bindings: expected 1, got %d", n)
	}
}

// TestSceneReconcileWith_TagTypesOfAnotherProject_FailsRemovingNothing: the
// TagTypes of another Project are a programming error of the caller, not
// Tags that do not exist: no binding is lost to it.
func TestSceneReconcileWith_TagTypesOfAnotherProject_FailsRemovingNothing(t *testing.T) {
	wt := makeWidgetType(t, "value")
	w := makeWidgetOf(t, wt, "w1", "value")
	sc := makeStoredScene(t, w)

	reconciled, changed, err := sc.ReconcileWith(wt, NewTagTypes(id.NewID[project.Project](), map[id.ID[tag.Tag]]tag.TagType{
		w.PortBindings()[0].TagID(): tag.TagTypeInteger,
	}))

	if !errors.Is(err, ErrTagTypesOfAnotherProject) || changed || len(reconciled.Widgets()[0].PortBindings()) != 1 {
		t.Errorf("tag types of another project: %v, changed %t, bindings %v; want the error and the binding kept",
			err, changed, reconciled.Widgets()[0].PortBindings())
	}
}

func TestSceneUpdateWidget_ChecksOnlyBindingsTheWidgetDidNotHold(t *testing.T) {
	wt := makeWidgetType(t, "value", "alarm")
	stored := makeWidgetOf(t, wt, "w1", "value")
	sc := makeStoredScene(t, stored)
	// The Tag of the held binding no longer exists.
	held := stored.PortBindings()[0]
	added := NewPortBinding(mustPortName(t, "alarm"), id.NewID[tag.Tag]())
	updated := NewWidget(stored.ID(), stored.Name(), NewPosition(10, 0, 0), stored.Size(), stored.Origin(), stored.Rotation(),
		wt.ID(), nil, []PortBinding{held, added})

	if _, err := sc.UpdateWidget(sc.Version(), updated, wt, NewTagTypes(testProject, map[id.ID[tag.Tag]]tag.TagType{added.TagID(): tag.TagTypeInteger})); err != nil {
		t.Errorf("held binding broken since: expected no error, got %v", err)
	}
	if _, err := sc.UpdateWidget(sc.Version(), updated, wt, NewTagTypes(testProject, nil)); !errors.Is(err, ErrBoundTagNotFound) {
		t.Errorf("new binding to a missing tag: expected ErrBoundTagNotFound, got %v", err)
	}
}

// ── UnbindTag ───────────────────────────────────────────────────────────────

func TestSceneUnbindTag_RemovesBindingsOfTheTagFromWidgetsOfEveryType(t *testing.T) {
	gone := id.NewID[tag.Tag]()
	wt1 := makeWidgetType(t, "value")
	wt2 := makeTypedWidgetType(t, "integer")
	w1 := bindValue(t, wt1, gone)
	w2 := bindValue(t, wt2, gone)
	untouched := makeWidgetOf(t, wt1, "w3", "value")
	sc := makeStoredScene(t, w1, w2, untouched)

	unbound, changed := sc.UnbindTag(gone)

	if !changed {
		t.Fatal("expected a change")
	}
	got := unbound.Widgets()
	if len(got[0].PortBindings()) != 0 || len(got[1].PortBindings()) != 0 {
		t.Errorf("bindings of the tag not removed: %v, %v", got[0].PortBindings(), got[1].PortBindings())
	}
	if got[2] != untouched {
		t.Error("a widget not bound to the tag must not change")
	}
	assertUnchanged(t, sc, []Widget{w1, w2, untouched})
	assertPending(t, unbound,
		recorded{event.EventTypeWidgetUpdated, w1.ID().String()},
		recorded{event.EventTypeWidgetUpdated, w2.ID().String()},
	)
	for _, e := range unbound.PendingEvents() {
		if ev, ok := e.(WidgetUpdatedEvent); !ok || !ev.ByReconciliation() {
			t.Errorf("event %v: expected a WidgetUpdatedEvent ByReconciliation", e)
		}
	}
}

func TestSceneUnbindTag_TagNotBound_ReportsNoChange(t *testing.T) {
	sc := makeStoredScene(t, makeWidgetOf(t, makeWidgetType(t, "value"), "w1", "value"))

	unbound, changed := sc.UnbindTag(id.NewID[tag.Tag]())

	if changed || unbound != sc {
		t.Error("expected the same scene and no change")
	}
}
