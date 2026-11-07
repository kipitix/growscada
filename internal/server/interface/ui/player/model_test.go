package player

import (
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	recordv0 "github.com/kipitix/growscada/contract/record/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// id is a stable ID named for readability.
func id(name string) uuid.UUID { return uuid.NewSHA1(uuid.Nil, []byte(name)) }

func revision(number int, tags []projectv0.TagDefinition, scenes []projectv0.Scene, types []projectv0.WidgetType) apiv0.RevisionResponse {
	return apiv0.RevisionResponse{
		Number: number,
		Content: projectv0.ProjectFile{
			Header: projectv0.Header{
				SchemaVersion: projectv0.SchemaVersion(),
				ProjectID:     id("project"),
			},
			Tags:        tags,
			Scenes:      scenes,
			WidgetTypes: types,
		},
	}
}

func tagEntry(seq int64, tagID uuid.UUID, value, quality string) recordv0.Entry {
	return recordv0.Entry{
		Header:    recordv0.Header{SchemaVersion: recordv0.SchemaVersion()},
		Seq:       seq,
		Timestamp: "2026-11-07T10:00:00Z",
		Type:      recordv0.EntryTypeTagState,
		TagState:  &recordv0.TagState{TagID: tagID, Value: value, Quality: quality},
	}
}

func revisionEntry(seq int64, number int) recordv0.Entry {
	return recordv0.Entry{
		Header:   recordv0.Header{SchemaVersion: recordv0.SchemaVersion()},
		Seq:      seq,
		Type:     recordv0.EntryTypeRevision,
		Revision: &recordv0.Revision{Number: number, Cause: recordv0.RevisionCauseDeploy},
	}
}

// TestApply_TagStateBySeq: a tag_state entry is applied unless the known state
// has the same or a greater position, and it keeps the entry's moment as
// WrittenAt.
func TestApply_TagStateBySeq(t *testing.T) {
	tagID := id("speed")
	m := &Model{}
	m.SetRevision(revision(1, []projectv0.TagDefinition{{ID: tagID, Name: "speed", Type: "integer"}}, nil, nil))

	if got := m.Apply(tagEntry(5, tagID, "42", "good")); !got.Changed {
		t.Fatal("the first state was not applied")
	}
	state, ok := m.TagState(tagID)
	if !ok || state.Value != "42" || state.Quality != "good" || state.Seq != 5 {
		t.Fatalf("state = %+v, %v", state, ok)
	}
	if state.WrittenAt == nil || *state.WrittenAt != "2026-11-07T10:00:00Z" {
		t.Errorf("written_at = %v, want the entry's timestamp", state.WrittenAt)
	}
	if m.Position() != 5 {
		t.Errorf("position = %d, want 5", m.Position())
	}

	// An older entry, and the same one again, change nothing.
	for _, entry := range []recordv0.Entry{
		tagEntry(4, tagID, "old", "bad"),
		tagEntry(5, tagID, "same", "bad"),
	} {
		if got := m.Apply(entry); got.Changed {
			t.Errorf("entry at %d changed the state", entry.Seq)
		}
	}
	if state, _ := m.TagState(tagID); state.Value != "42" || state.Quality != "good" {
		t.Errorf("an older entry rolled the state back: %+v", state)
	}
	if m.Position() != 5 {
		t.Errorf("position = %d, want it not to move back", m.Position())
	}

	if got := m.Apply(tagEntry(6, tagID, "43", "uncertain")); !got.Changed {
		t.Fatal("a newer state was not applied")
	}
	if state, _ := m.TagState(tagID); state.Value != "43" || state.Quality != "uncertain" || state.Seq != 6 {
		t.Errorf("state = %+v, want 43 uncertain at 6", state)
	}
	if m.Position() != 6 {
		t.Errorf("position = %d, want 6", m.Position())
	}
}

// TestApply_TagOutsideConfiguration: the state of a Tag the running Revision
// does not name is kept by ID, so a later Revision that names it finds it, but
// it is not rendered.
func TestApply_TagOutsideConfiguration(t *testing.T) {
	inConfig, outside := id("speed"), id("pressure")
	m := &Model{}
	m.SetRevision(revision(1, []projectv0.TagDefinition{{ID: inConfig, Name: "speed", Type: "integer"}}, nil, nil))

	if got := m.Apply(tagEntry(3, outside, "7", "good")); !got.Changed {
		t.Fatal("the state of a Tag outside the configuration was dropped")
	}
	if state, ok := m.TagState(outside); !ok || state.Value != "7" {
		t.Errorf("state outside the configuration = %+v, %v", state, ok)
	}
	view := m.View()
	if _, ok := view.Tags[outside]; ok {
		t.Error("a Tag outside the configuration is rendered")
	}
	if tag := view.Tags[inConfig]; tag.Value != "0" || tag.Quality != "bad" || tag.Seq != 0 {
		t.Errorf("the configured Tag = %+v, want its type's zero value at bad", tag)
	}
}

// TestApply_RevisionByNumber: a revision entry asks for the document of a
// greater number alone; applying it keeps the Tag states by ID, gives a new
// Tag its zero value and stops rendering a removed one.
func TestApply_RevisionByNumber(t *testing.T) {
	kept, removed, added := id("kept"), id("removed"), id("added")
	m := &Model{}
	m.SetRevision(revision(1,
		[]projectv0.TagDefinition{
			{ID: kept, Name: "kept", Type: "integer"},
			{ID: removed, Name: "removed", Type: "boolean"},
		},
		nil, nil))
	m.Apply(tagEntry(4, kept, "42", "good"))
	m.Apply(tagEntry(5, removed, "true", "good"))

	if got := m.Apply(revisionEntry(6, 2)); got.Revision != 2 || got.Changed {
		t.Fatalf("revision entry = %+v, want a request for 2", got)
	}
	if !m.SetRevision(revision(2,
		[]projectv0.TagDefinition{
			{ID: kept, Name: "kept", Type: "integer"},
			{ID: added, Name: "added", Type: "string"},
		},
		nil, nil)) {
		t.Fatal("Revision 2 was refused")
	}

	view := m.View()
	if tag := view.Tags[kept]; tag.Value != "42" || tag.Quality != "good" {
		t.Errorf("a kept Tag lost its state: %+v", tag)
	}
	if tag := view.Tags[added]; tag.Value != "" || tag.Quality != "bad" || tag.Seq != 0 {
		t.Errorf("a new Tag = %+v, want its type's zero value at bad", tag)
	}
	if _, ok := view.Tags[removed]; ok {
		t.Error("a removed Tag is still rendered")
	}
	if state, ok := m.TagState(removed); !ok || state.Value != "true" {
		t.Errorf("the state of a removed Tag was dropped: %+v, %v", state, ok)
	}

	for _, entry := range []recordv0.Entry{revisionEntry(7, 2), revisionEntry(8, 1)} {
		if got := m.Apply(entry); got.Revision != 0 {
			t.Errorf("revision entry for #%d asked for a document", entry.Revision.Number)
		}
	}
	if m.SetRevision(revision(1, nil, nil, nil)) {
		t.Error("an older Revision replaced a newer one")
	}
	if m.Apply(revisionEntry(9, 3)).Revision != 3 {
		t.Error("a newer Revision did not ask for its document")
	}
}

// TestApply_IgnoresWhatItCannotApply: an unknown entry type, a foreign MAJOR
// and an entry without its payload change nothing; the position still moves
// past every entry consumed.
func TestApply_IgnoresWhatItCannotApply(t *testing.T) {
	tagID := id("speed")
	m := &Model{}
	m.SetRevision(revision(1, []projectv0.TagDefinition{{ID: tagID, Name: "speed", Type: "integer"}}, nil, nil))
	m.Apply(tagEntry(3, tagID, "42", "good"))

	future := tagEntry(4, tagID, "43", "good")
	future.Type = recordv0.EntryType("future")
	foreign := tagEntry(5, tagID, "44", "good")
	foreign.SchemaVersion.Major = recordv0.SchemaVersion().Major + 1
	foreignRevision := revisionEntry(6, 2)
	foreignRevision.SchemaVersion.Major = recordv0.SchemaVersion().Major + 1
	withoutPayload := tagEntry(7, tagID, "45", "good")
	withoutPayload.TagState = nil
	withoutRevision := revisionEntry(8, 2)
	withoutRevision.Revision = nil

	for _, entry := range []recordv0.Entry{future, foreign, foreignRevision, withoutPayload, withoutRevision} {
		if got := m.Apply(entry); got != (Result{}) {
			t.Errorf("entry %q at %d applied as %+v", entry.Type, entry.Seq, got)
		}
	}
	if state, _ := m.TagState(tagID); state.Value != "42" {
		t.Errorf("an entry the reader cannot apply changed the state: %+v", state)
	}
	if m.Revision().Number != 1 {
		t.Errorf("a foreign or empty revision entry changed the configuration: #%d", m.Revision().Number)
	}
	if m.Position() != 8 {
		t.Errorf("position = %d, want the cursor past every consumed entry", m.Position())
	}
}

// TestMergeSnapshot_NewerStateWins: a starting state read after entries have
// arrived never rolls them back; the position only moves forward.
func TestMergeSnapshot_NewerStateWins(t *testing.T) {
	tagID := id("speed")
	m := &Model{}
	m.SetRevision(revision(2, []projectv0.TagDefinition{{ID: tagID, Name: "speed", Type: "integer"}}, nil, nil))
	m.Apply(tagEntry(6, tagID, "43", "good"))

	written := "2026-11-07T09:00:00Z"
	m.MergeSnapshot(apiv0.OperationStateResponse{
		Position:  10,
		Revision:  ptr(revision(1, nil, nil, nil)),
		TagStates: map[string]apiv0.TagStateResponse{tagID.String(): {Value: "42", Quality: "bad", Seq: 4, WrittenAt: &written}},
	})
	if state, _ := m.TagState(tagID); state.Value != "43" || state.Seq != 6 {
		t.Errorf("the starting state rolled the Tag back: %+v", state)
	}
	if m.Revision().Number != 2 {
		t.Errorf("the starting state rolled the Revision back to #%d", m.Revision().Number)
	}
	if m.Position() != 10 {
		t.Errorf("position = %d, want the starting state's 10", m.Position())
	}
}

// TestView_MapsProjectFile: the Revision document is mapped to the shapes
// livescene renders — Scenes, Widgets with their geometry's matrix,
// WidgetTypes with their ports and Tags with their states.
func TestView_MapsProjectFile(t *testing.T) {
	tagID, typeID, sceneID := id("speed"), id("gauge"), id("overview")
	widget := projectv0.Widget{
		ID:       id("widget"),
		Name:     "Speed",
		Position: projectv0.Position{X: 10, Y: 20, Z: 3},
		Size:     projectv0.Size{Width: 100, Height: 50},
		Origin:   projectv0.Origin{X: 0.5, Y: 0.5},
		Rotation: projectv0.Rotation{Degrees: 30},
		TypeID:   typeID,
		Labels:   []string{"speed"},
		PortBindings: []projectv0.PortBinding{
			{PortName: "value", TagID: tagID},
		},
	}
	m := &Model{}
	m.SetRevision(revision(1,
		[]projectv0.TagDefinition{
			{ID: tagID, Name: "speed", Type: "integer"},
			{ID: id("name"), Name: "name", Type: "string"},
			{ID: id("flag"), Name: "flag", Type: "boolean"},
		},
		[]projectv0.Scene{{
			ID:             sceneID,
			Name:           "Overview",
			Width:          800,
			Height:         600,
			BackgroundHTML: "<b>bg</b>",
			Widgets:        []projectv0.Widget{widget},
		}},
		[]projectv0.WidgetType{{
			ID:             typeID,
			LibraryID:      id("library"),
			Name:           "Gauge",
			HTMLTemplate:   "<p></p>",
			Script:         "render()",
			ScriptLanguage: "javascript",
			DefaultWidth:   100,
			DefaultHeight:  50,
			InputPorts:     []projectv0.InputPort{{Name: "value", Description: "the value", TypeHint: "integer"}},
		}},
	))
	m.Apply(tagEntry(5, tagID, "42", "good"))

	view := m.View()
	if !view.Deployed() || len(view.Scenes) != 1 {
		t.Fatalf("view = %+v", view)
	}
	scene := view.Scenes[0]
	if scene.ID != sceneID || scene.Name != "Overview" || scene.Width != 800 || scene.Height != 600 || scene.BackgroundHTML != "<b>bg</b>" {
		t.Errorf("scene = %+v", scene)
	}
	widgets := view.Widgets(sceneID)
	if len(widgets) != 1 {
		t.Fatalf("widgets = %+v", widgets)
	}
	w := widgets[0]
	wantMatrix := uiutil.MatrixCSS(10, 20, 0.5, 0.5, 30, 100, 50)
	if w.ID != widget.ID || w.Name != "Speed" || w.SceneID != sceneID || w.TypeID != typeID {
		t.Errorf("widget = %+v", w)
	}
	if w.Position != (apiv0.PositionResponse{X: 10, Y: 20, Z: 3}) ||
		w.Size != (apiv0.SizeResponse{Width: 100, Height: 50}) ||
		w.Origin != (apiv0.OriginResponse{X: 0.5, Y: 0.5}) ||
		w.Rotation != (apiv0.RotationResponse{Degrees: 30}) {
		t.Errorf("widget geometry = %+v", w)
	}
	if w.TransformMatrix.CSS != wantMatrix {
		t.Errorf("matrix = %q, want %q", w.TransformMatrix.CSS, wantMatrix)
	}
	if len(w.PortBindings) != 1 || w.PortBindings[0].PortName != "value" || w.PortBindings[0].TagID != tagID {
		t.Errorf("port bindings = %+v", w.PortBindings)
	}
	if len(w.Labels) != 1 || w.Labels[0] != "speed" {
		t.Errorf("labels = %+v", w.Labels)
	}

	widgetType, ok := view.WidgetTypes[typeID]
	if !ok || widgetType.Name != "Gauge" || widgetType.HtmlTemplate != "<p></p>" || widgetType.Script != "render()" {
		t.Errorf("widget type = %+v, %v", widgetType, ok)
	}
	if len(widgetType.InputPorts) != 1 || widgetType.InputPorts[0].TypeHint != "integer" {
		t.Errorf("input ports = %+v", widgetType.InputPorts)
	}

	if tag := view.Tags[tagID]; tag.Value != "42" || tag.Quality != "good" || tag.Seq != 5 || tag.Type != "integer" {
		t.Errorf("tag with a state = %+v", tag)
	}
	for name, want := range map[string]string{"name": "", "flag": "false"} {
		if tag := view.Tags[id(name)]; tag.Value != want || tag.Quality != "bad" {
			t.Errorf("tag %s = %+v, want the zero value %q at bad", name, tag, want)
		}
	}
}

// TestView_SceneSwitch: each Scene renders its own Widgets, and a Scene the
// Revision does not hold renders none.
func TestView_SceneSwitch(t *testing.T) {
	first, second := id("first"), id("second")
	scene := func(sceneID uuid.UUID, widgetName string) projectv0.Scene {
		return projectv0.Scene{
			ID: sceneID, Name: widgetName, Width: 100, Height: 100,
			Widgets: []projectv0.Widget{{ID: id(widgetName), Name: widgetName}},
		}
	}
	m := &Model{}
	m.SetRevision(revision(1, nil, []projectv0.Scene{scene(first, "one"), scene(second, "two")}, nil))

	view := m.View()
	if widgets := view.Widgets(first); len(widgets) != 1 || widgets[0].Name != "one" {
		t.Errorf("first scene's widgets = %+v", widgets)
	}
	if widgets := view.Widgets(second); len(widgets) != 1 || widgets[0].Name != "two" {
		t.Errorf("second scene's widgets = %+v", widgets)
	}
	if widgets := view.Widgets(id("gone")); len(widgets) != 0 {
		t.Errorf("a Scene the Revision lacks rendered %+v", widgets)
	}
}

func ptr[T any](v T) *T { return &v }
