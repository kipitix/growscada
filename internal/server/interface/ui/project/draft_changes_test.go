package project

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// The DraftChange stack in the Project mode (ADR 0017): the Undo and Redo
// buttons of the header and their descriptions, the keyboard shortcuts, what a
// refusal does and one Version of the Draft. The fixture's API client and the
// mounted Project act on one Draft
// (newServer).

// reloadChanges makes the Project read the stack again, as an event or a
// resync does.
func (f *fixture) reloadChanges() {
	f.p.loadDraftChanges(f.p.compoCtx)
	f.run()
}

// sceneNames returns the names of the Scenes the server holds.
func (f *fixture) sceneNames() []string {
	f.t.Helper()
	scenes, err := f.api.ListScenes(context.Background())
	if err != nil {
		f.t.Fatalf("list scenes: %v", err)
	}
	names := make([]string, len(scenes))
	for i, sc := range scenes {
		names[i] = sc.Name
	}
	return names
}

func TestDraftChangesBar_EmptyStack(t *testing.T) {
	f := startProject(t)
	f.reloadChanges()
	f.noToasts()

	html := app.HTMLString(f.p.renderDraftChangesBar())
	if !strings.Contains(html, "data-undo") || !strings.Contains(html, "data-redo") {
		t.Fatalf("the bar has no Undo and Redo: %s", html)
	}
	if f.p.canUndo() || f.p.canRedo() {
		t.Errorf("an empty stack offers a step: %+v", f.p.changes)
	}
	if !strings.Contains(html, "Nothing to undo") || !strings.Contains(html, "Nothing to redo") {
		t.Errorf("the buttons do not say the stack is empty: %s", html)
	}
}

func TestDraftChangesBar_ShowsTheNextStep(t *testing.T) {
	f := startProject(t)
	createScene(t, f.api, "Pump")
	f.reloadChanges()
	f.noToasts()

	if !f.p.canUndo() {
		t.Fatalf("the stack has a change to undo: %+v", f.p.changes)
	}
	if f.p.canRedo() {
		t.Errorf("a redo is offered after one change: %+v", f.p.changes)
	}
	html := app.HTMLString(f.p.renderDraftChangesBar())
	if !strings.Contains(html, "Undo: Create scene Pump") {
		t.Errorf("the Undo button does not name the step: %s", html)
	}
}

func TestUndo_RestoresTheSceneAndRedoBringsItBack(t *testing.T) {
	f := startProject(t)
	createScene(t, f.api, "Pump")
	f.reloadChanges()

	f.p.undoDraftChange(f.p.compoCtx)
	f.run()

	f.noToasts()
	if names := f.sceneNames(); len(names) != 0 {
		t.Errorf("scenes after undo: %v, want none", names)
	}
	if f.p.changes.CanUndo || !f.p.changes.CanRedo {
		t.Errorf("stack after undo: %+v, want a redo only", f.p.changes)
	}
	if !f.p.canRedo() {
		t.Error("the Redo button is off after an undo")
	}

	// The shortcut is the button's own command.
	f.p.applyKeyAction(f.p.compoCtx, uiutil.KeyRedo)
	f.run()

	f.noToasts()
	if names := f.sceneNames(); len(names) != 1 || names[0] != "Pump" {
		t.Errorf("scenes after redo: %v, want Pump", names)
	}
	if !f.p.changes.CanUndo || f.p.changes.CanRedo {
		t.Errorf("stack after redo: %+v, want an undo only", f.p.changes)
	}
}

// TestUndo_StaleVersion_RereadsTheStackAndTheContent: a change made after the
// stack was read is a 409 with the Draft's state (Problem.draft), handled as a
// refused edit is.
func TestUndo_StaleVersion_RereadsTheStackAndTheContent(t *testing.T) {
	f := startProject(t)
	createScene(t, f.api, "One")
	f.reloadChanges()
	second := createScene(t, f.api, "Two") // the UI has not seen it

	f.p.undoDraftChange(f.p.compoCtx)
	f.run()

	got := f.oneToast()
	if got.Status != 409 || !strings.Contains(got.Detail, "changed concurrently") {
		t.Errorf("toast %+v, want the conflict", got)
	}
	f.knowsVersion(second) // the Draft's Version was forgotten and reread
	if !f.p.changes.CanUndo {
		t.Errorf("stack after the conflict: %+v, want the reread one", f.p.changes)
	}
}

// TestUndo_EmptyStack_IsAToastAndAReread: the state the buttons were built
// from may state the Version the Draft is at and still have no step to apply —
// another client undid the change, or the rows were trimmed past the depth. The
// 409 carries no Problem.draft, so nothing of the content is claimed: a toast
// and a reread of the stack alone.
func TestUndo_EmptyStack_IsAToastAndAReread(t *testing.T) {
	f := startProject(t)
	createScene(t, f.api, "One")
	f.reloadChanges()
	if _, err := f.api.UndoDraftChange(context.Background(), 0); err != nil { // someone else undid it
		t.Fatalf("undo through the API: %v", err)
	}
	state, err := f.api.DraftChanges(context.Background())
	if err != nil {
		t.Fatalf("read the stack: %v", err)
	}
	f.p.changes = apiv0.DraftChangesResponse{
		Version: state.Version, CanUndo: true, UndoDescription: "Create scene One",
	}
	f.p.changesLoaded = true

	f.p.undoDraftChange(f.p.compoCtx)
	f.run()

	got := f.oneToast()
	if got.Status != 409 || !strings.Contains(got.Detail, "Nothing to undo") {
		t.Errorf("toast %+v, want the empty stack", got)
	}
	if f.p.changes.CanUndo || !f.p.changes.CanRedo {
		t.Errorf("stack after the refusal: %+v, want the reread one", f.p.changes)
	}
	if names := f.sceneNames(); len(names) != 0 {
		t.Errorf("scenes: %v, want the undo of the other client", names)
	}
}

// TestContentEvent_ReloadsTheStack: a write of the Draft's content is one
// DraftChange, so the buttons follow its event.
func TestContentEvent_ReloadsTheStack(t *testing.T) {
	f := startProject(t)
	created := createScene(t, f.api, "Pump")

	f.event(apiv0.Event{Type: apiv0.EventSceneCreated, ID: created.String()})

	f.noToasts()
	if !f.p.canUndo() || !strings.Contains(f.p.changes.UndoDescription, "Pump") {
		t.Errorf("stack after the event: %+v, want the creation", f.p.changes)
	}
}

// TestSwitchProject_ResetsTheStack: the stack of the project left is not the
// one of another.
func TestSwitchProject_ResetsTheStack(t *testing.T) {
	f := startProject(t)
	createScene(t, f.api, "One")
	f.reloadChanges()
	if !f.p.changesLoaded {
		t.Fatal("the stack was not read")
	}

	f.p.SetProject(uuid.Nil)
	f.run()

	if f.p.changesLoaded || f.p.changes.CanUndo || f.p.changes.CanRedo {
		t.Errorf("the stack survived the switch: %+v", f.p.changes)
	}
	if f.p.draftVersion() != 0 {
		t.Errorf("the Version of the project left survived: %d", f.p.draftVersion())
	}
}

// TestDraftChange_OneVersionForTheDraft: what an undo states is the Version of
// the whole Draft, so the write after it goes through.
func TestDraftChange_OneVersionForTheDraft(t *testing.T) {
	f := startProject(t)
	createScene(t, f.api, "One")
	f.reloadChanges()

	f.p.undoDraftChange(f.p.compoCtx)
	f.run()
	f.noToasts()

	if _, err := f.api.CreateScene(context.Background(), apiv0.CreateSceneRequest{
		Name: "After undo", Width: 100, Height: 100, Version: f.p.draftVersion(),
	}); err != nil {
		t.Errorf("a write at the Version the undo left: %v", err)
	}
}

// TestShortcut_EmptyStack_SendsNothing: a shortcut is the button's own, and a
// button that offers no step sends no request.
func TestShortcut_EmptyStack_SendsNothing(t *testing.T) {
	url, api := newServer(t)
	f, log := mountLogged(t, url, api)
	f.reloadChanges()
	f.e.TakeToasts()
	log.take()

	f.p.applyKeyAction(f.p.compoCtx, uiutil.KeyUndo)
	f.run()
	f.p.applyKeyAction(f.p.compoCtx, uiutil.KeyRedo)
	f.run()

	f.noToasts()
	if got := log.take(); len(got) != 0 {
		t.Errorf("requests of an empty stack: %v, want none", got)
	}
}

// TestShortcut_DialogOpen_SendsNothing: an undo behind a dialog of the mode
// would change the Draft the dialog shows.
func TestShortcut_DialogOpen_SendsNothing(t *testing.T) {
	url, api := newServer(t)
	createScene(t, api, "One")
	f, log := mountLogged(t, url, api)
	f.reloadChanges()
	f.e.TakeToasts()
	log.take()

	f.p.releasesDialog = true
	f.p.applyKeyAction(f.p.compoCtx, uiutil.KeyUndo)
	f.run()

	f.noToasts()
	if got := log.take(); len(got) != 0 {
		t.Errorf("requests behind the dialog: %v, want none", got)
	}
	state, err := f.api.DraftChanges(context.Background())
	if err != nil {
		t.Fatalf("read the stack: %v", err)
	}
	if !state.CanUndo {
		t.Errorf("the Draft was undone behind the dialog: %+v", state)
	}
}

// TestDraftChangesBar_UnnamedStep_IsOff: a step the stack offers but does not
// name is no step — its description is what the button shows.
func TestDraftChangesBar_UnnamedStep_IsOff(t *testing.T) {
	url, api := newServer(t)
	f, log := mountLogged(t, url, api)
	createScene(t, api, "One")
	f.reloadChanges()
	log.take()

	state, err := api.DraftChanges(context.Background())
	if err != nil {
		t.Fatalf("read the stack: %v", err)
	}
	f.p.changes = apiv0.DraftChangesResponse{Version: state.Version, CanUndo: true}
	f.p.changesLoaded = true

	if f.p.canUndo() {
		t.Error("an unnamed step is offered")
	}
	if got, want := f.p.undoTitle(), "Nothing to undo"; got != want {
		t.Errorf("hover text %q, want %q", got, want)
	}
	f.p.applyKeyAction(f.p.compoCtx, uiutil.KeyUndo)
	f.run()
	f.noToasts()
	if got := log.take(); len(got) != 0 {
		t.Errorf("requests of an unnamed step: %v, want none", got)
	}
}
