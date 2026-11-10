package draft

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// undoDraft is a Draft of a Project at position 3, its content at version 5.
func undoDraft(t *testing.T) Draft {
	t.Helper()
	scope := ProjectScope(id.NewID[project.Project]())
	ver, err := version.New(version.WithNumber[Draft](5))
	if err != nil {
		t.Fatal(err)
	}
	return NewDraft(id.NewID[Draft](), scope, ver, ver, 3)
}

// TestDraft_UndoAndRedoStepTheStackAndRaiseTheVersion: undo applies the change
// at the position and steps back, redo the change after it and steps forward;
// both raise the Version of the Draft (ADR 0017).
func TestDraft_UndoAndRedoStepTheStackAndRaiseTheVersion(t *testing.T) {
	d := undoDraft(t)
	undone, err := d.Undo(DraftChange{Seq: 3})
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if undone.Version().Number() != 6 || undone.StackPosition() != 2 {
		t.Errorf("after undo: version %d, position %d; want 6, 2", undone.Version().Number(), undone.StackPosition())
	}
	if d.Version().Number() != 5 || d.StackPosition() != 3 {
		t.Errorf("the original changed: version %d, position %d", d.Version().Number(), d.StackPosition())
	}
	redone, err := undone.Redo(DraftChange{Seq: 3})
	if err != nil {
		t.Fatalf("Redo: %v", err)
	}
	if redone.Version().Number() != 7 || redone.StackPosition() != 3 {
		t.Errorf("after redo: version %d, position %d; want 7, 3", redone.Version().Number(), redone.StackPosition())
	}
}

// TestDraft_UndoAndRedoRefuseTheWrongChange: the stack names the change each
// command applies, so a change that is not at its position is refused.
func TestDraft_UndoAndRedoRefuseTheWrongChange(t *testing.T) {
	d := undoDraft(t)
	if _, err := d.Undo(DraftChange{Seq: 2}); !errors.Is(err, ErrNothingToUndo) {
		t.Errorf("undo of an older change: %v, want ErrNothingToUndo", err)
	}
	if _, err := d.Undo(DraftChange{}); !errors.Is(err, ErrNothingToUndo) {
		t.Errorf("undo of the empty stack: %v, want ErrNothingToUndo", err)
	}
	if _, err := d.Redo(DraftChange{Seq: 3}); !errors.Is(err, ErrNothingToRedo) {
		t.Errorf("redo of the applied change: %v, want ErrNothingToRedo", err)
	}
	if _, err := d.Redo(DraftChange{Seq: 5}); !errors.Is(err, ErrNothingToRedo) {
		t.Errorf("redo of a change beyond the stack: %v, want ErrNothingToRedo", err)
	}
}

// TestSession_UndoOfATagDeletionRestoresItAndTheBindings: the before states of
// the change are written, the events are its inverse — tag_created for the Tag
// and widget_updated for the Scene whose binding returns (ADR 0017).
func TestSession_UndoOfATagDeletionRestoresItAndTheBindings(t *testing.T) {
	session := NewSession(undoDraft(t), nil)
	sceneID, widgetID := uuid.New(), uuid.New()
	bound := widgetSnapshot(sceneID, widgetID, true)
	unbound := widgetSnapshot(sceneID, widgetID, false)

	err := session.Undo(context.Background(), DraftChange{Seq: 3,
		Before: []Snapshot{tagSnapshot(uuid.New()), bound},
		After:  []Snapshot{unbound},
	})
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	before, after := session.Changes()
	if len(before) != 1 || before[0].Kind != KindWidget {
		t.Errorf("the write removes %d snapshots, want the unbound widget: %v", len(before), before)
	}
	if len(after) != 2 || after[0].Kind != KindTag || after[1].Kind != KindWidget {
		t.Fatalf("the write restores %v, want the Tag and the bound widget", after)
	}
	if session.Draft().StackPosition() != 2 || session.Draft().Version().Number() != 6 {
		t.Errorf("the stack is at %d, version %d; want 2, 6",
			session.Draft().StackPosition(), session.Draft().Version().Number())
	}
	types := eventTypes(session.Events())
	if !types[event.EventTypeTagCreated] || !types[event.EventTypeWidgetUpdated] {
		t.Errorf("undo published %v, want tag_created and widget_updated", types)
	}
}

// TestSession_UndoOfASceneDeletionRestoresTheSceneAndItsWidgets: the snapshots
// of a removed Scene carry its Widgets, and both come back.
func TestSession_UndoOfASceneCreationRemovesTheSceneAndItsWidgets(t *testing.T) {
	session := NewSession(undoDraft(t), nil)
	sceneID, widgetID := uuid.New(), uuid.New()
	err := session.Undo(context.Background(), DraftChange{Seq: 3,
		After: []Snapshot{sceneSnapshot(sceneID), widgetSnapshot(sceneID, widgetID, false)},
	})
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	before, after := session.Changes()
	if len(before) != 2 || len(after) != 0 {
		t.Errorf("the write removes %d and restores %d snapshots, want 2 and 0", len(before), len(after))
	}
	types := eventTypes(session.Events())
	if !types[event.EventTypeSceneDeleted] || !types[event.EventTypeWidgetDeleted] {
		t.Errorf("undo published %v, want scene_deleted and widget_deleted", types)
	}
}

// TestSession_UndoOfAPinSwitchRestoresThePinAndTheBindings: a change of both
// sides is written back on its before side, with the pin and widget events.
func TestSession_UndoOfAPinSwitchRestoresThePinAndTheBindings(t *testing.T) {
	session := NewSession(undoDraft(t), nil)
	libraryID, sceneID, widgetID := uuid.New(), uuid.New(), uuid.New()
	err := session.Undo(context.Background(), DraftChange{Seq: 3,
		Before: []Snapshot{pinSnapshot(libraryID, 1), widgetSnapshot(sceneID, widgetID, true)},
		After:  []Snapshot{pinSnapshot(libraryID, 2), widgetSnapshot(sceneID, widgetID, false)},
	})
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	_, after := session.Changes()
	pin, ok := after[0].State.(LibraryPinState)
	if !ok || pin.Release != 1 {
		t.Errorf("the restored pin is %v, want release 1", after[0].State)
	}
	types := eventTypes(session.Events())
	if !types[event.EventTypeLibraryPinUpdated] || !types[event.EventTypeWidgetUpdated] {
		t.Errorf("undo published %v, want library_pin_updated and widget_updated", types)
	}
}

// TestSession_RedoReappliesTheAfterStates: a redo is the undo's mirror image.
func TestSession_RedoReappliesTheAfterStates(t *testing.T) {
	session := NewSession(undoDraft(t).WithStackPosition(2), nil)
	sceneID, widgetID := uuid.New(), uuid.New()
	renamed := sceneSnapshot(sceneID)
	renamed.State = SceneState{Name: "renamed", Width: 100, Height: 80}
	change := DraftChange{Seq: 3,
		Before: []Snapshot{sceneSnapshot(sceneID)},
		After:  []Snapshot{renamed, widgetSnapshot(sceneID, widgetID, false)},
	}
	if err := session.Redo(context.Background(), change); err != nil {
		t.Fatalf("Redo: %v", err)
	}
	before, after := session.Changes()
	if len(before) != 1 || len(after) != 2 {
		t.Errorf("the write removes %d and restores %d snapshots, want 1 and 2", len(before), len(after))
	}
	if session.Draft().StackPosition() != 3 {
		t.Errorf("the stack is at %d, want 3", session.Draft().StackPosition())
	}
	types := eventTypes(session.Events())
	if !types[event.EventTypeSceneUpdated] || !types[event.EventTypeWidgetCreated] {
		t.Errorf("redo published %v, want scene_updated and widget_created", types)
	}
}

// TestInverseEvents_EveryKind: an inverse event names the entity of its
// snapshot — created, updated or deleted — for a Project's Draft and a
// Library's alike.
func TestInverseEvents_EveryKind(t *testing.T) {
	projectScope := ProjectScope(id.NewID[project.Project]())
	libraryScope := LibraryScope(id.NewID[library.Library]())
	sceneID, typeID, pairID, deviceID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	pinID, widgetID := uuid.New(), uuid.New()
	renamedScene := sceneSnapshot(sceneID)
	renamedScene.State = SceneState{Name: "renamed", Width: 100, Height: 80}
	createdDevice := deviceSnapshot(uuid.New())
	createdDevice.State = DeviceState{Name: "plc", Protocol: "simulation", Config: []byte(`{}`)}
	deletedEnvironment := environmentSnapshot(uuid.New())
	libraryDraftType := widgetTypeSnapshot(uuid.New(), typeID)
	cases := []struct {
		name  string
		scope Scope
		old   []Snapshot
		new   []Snapshot
		want  event.EventType
	}{
		{"tag created", projectScope, nil, []Snapshot{tagSnapshot(uuid.New())}, event.EventTypeTagCreated},
		{"tag deleted", projectScope, []Snapshot{tagSnapshot(uuid.New())}, nil, event.EventTypeTagDeleted},
		{"scene created", projectScope, nil, []Snapshot{sceneSnapshot(sceneID)}, event.EventTypeSceneCreated},
		{"scene updated", projectScope, []Snapshot{sceneSnapshot(sceneID)}, []Snapshot{renamedScene}, event.EventTypeSceneUpdated},
		{"scene deleted", projectScope, []Snapshot{sceneSnapshot(sceneID)}, nil, event.EventTypeSceneDeleted},
		{"widget created", projectScope, nil, []Snapshot{widgetSnapshot(sceneID, widgetID, true)}, event.EventTypeWidgetCreated},
		{"widget updated", projectScope, []Snapshot{widgetSnapshot(sceneID, widgetID, true)},
			[]Snapshot{widgetSnapshot(sceneID, widgetID, false)}, event.EventTypeWidgetUpdated},
		{"widget deleted", projectScope, []Snapshot{widgetSnapshot(sceneID, widgetID, true)}, nil, event.EventTypeWidgetDeleted},
		{"device created", projectScope, nil, []Snapshot{createdDevice}, event.EventTypeDeviceCreated},
		{"device deleted", projectScope, []Snapshot{deviceSnapshot(uuid.New())}, nil, event.EventTypeDeviceDeleted},
		{"environment created", projectScope, nil, []Snapshot{environmentSnapshot(uuid.New())}, event.EventTypeEnvironmentCreated},
		{"environment deleted", projectScope, []Snapshot{deletedEnvironment}, nil, event.EventTypeEnvironmentDeleted},
		{"pair created", projectScope, nil, []Snapshot{pairSnapshot(pairID, deviceID)}, event.EventTypeEnvironmentDeviceCreated},
		{"pair deleted", projectScope, []Snapshot{pairSnapshot(pairID, deviceID)}, nil, event.EventTypeEnvironmentDeviceDeleted},
		{"pin created", projectScope, nil, []Snapshot{pinSnapshot(pinID, 1)}, event.EventTypeLibraryPinCreated},
		{"pin updated", projectScope, []Snapshot{pinSnapshot(pinID, 1)}, []Snapshot{pinSnapshot(pinID, 2)}, event.EventTypeLibraryPinUpdated},
		{"widget type created", libraryScope, nil, []Snapshot{libraryDraftType}, event.EventTypeWidgetTypeCreated},
		{"widget type deleted", libraryScope, []Snapshot{libraryDraftType}, nil, event.EventTypeWidgetTypeDeleted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			events, err := inverseEvents(tc.scope, tc.old, tc.new)
			if err != nil {
				t.Fatalf("inverseEvents: %v", err)
			}
			if len(events) != 1 || events[0].Type() != tc.want {
				t.Errorf("published %v, want one %s", eventTypes(events), tc.want)
			}
		})
	}
}

// TestInverseEvents_UnchangedRowPublishesNothing: a snapshot on both sides with
// the same state is not a change.
func TestInverseEvents_UnchangedRowPublishesNothing(t *testing.T) {
	snapshot := sceneSnapshot(uuid.New())
	events, err := inverseEvents(ProjectScope(id.NewID[project.Project]()), []Snapshot{snapshot}, []Snapshot{snapshot})
	if err != nil {
		t.Fatalf("inverseEvents: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("published %v, want nothing", eventTypes(events))
	}
}

func eventTypes(events []event.Event) map[event.EventType]bool {
	types := make(map[event.EventType]bool, len(events))
	for _, ev := range events {
		types[ev.Type()] = true
	}
	return types
}

func tagSnapshot(tagID uuid.UUID) Snapshot {
	return Snapshot{Kind: KindTag, ID: tagID, State: TagState{Name: "pressure", Type: "integer"}}
}

func sceneSnapshot(sceneID uuid.UUID) Snapshot {
	return Snapshot{Kind: KindScene, ID: sceneID, State: SceneState{
		Name: "main", Width: 100, Height: 80, BackgroundHTML: "<div/>",
	}}
}

// widgetSnapshot is a Widget's snapshot; bound says whether its port is bound
// to a Tag.
func widgetSnapshot(sceneID, widgetID uuid.UUID, bound bool) Snapshot {
	bindings := []PortBindingState{}
	if bound {
		bindings = append(bindings, PortBindingState{PortName: "value", TagID: uuid.New()})
	}
	return Snapshot{Kind: KindWidget, ID: widgetID, State: WidgetState{
		SceneID: sceneID, Name: "gauge", X: 1, Y: 2, Z: 0, Width: 10, Height: 5,
		TypeID: uuid.New(), Labels: []string{}, PortBindings: bindings,
	}}
}

func deviceSnapshot(deviceID uuid.UUID) Snapshot {
	return Snapshot{Kind: KindDevice, ID: deviceID, State: DeviceState{
		Name: "sim", Protocol: "simulation", Config: []byte(`{}`),
	}}
}

func environmentSnapshot(environmentID uuid.UUID) Snapshot {
	return Snapshot{Kind: KindEnvironment, ID: environmentID, State: EnvironmentState{Name: "bench"}}
}

func pairSnapshot(environmentID, deviceID uuid.UUID) Snapshot {
	return Snapshot{Kind: KindEnvironmentDevice, ID: environmentID, State: EnvironmentDeviceState{
		EnvironmentID: environmentID, DeviceID: deviceID, Params: []byte(`{}`),
	}}
}

func pinSnapshot(libraryID uuid.UUID, release int) Snapshot {
	return Snapshot{Kind: KindLibraryPin, ID: libraryID, State: LibraryPinState{
		LibraryID: libraryID, Release: release,
	}}
}

func widgetTypeSnapshot(libraryID, typeID uuid.UUID) Snapshot {
	return Snapshot{Kind: KindWidgetType, ID: typeID, State: WidgetTypeState{
		LibraryID: libraryID, Name: "Gauge", HtmlTemplate: "<div/>", Script: "function render() {}",
		ScriptLanguage: "javascript", DefaultWidth: 10, DefaultHeight: 5,
	}}
}
