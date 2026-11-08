package library

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// The DraftChange stack in the Library mode (ADR 0017): the Undo and Redo
// buttons of the libraries column and their descriptions, the keyboard
// shortcuts, what a refusal does and one Version of the Draft.

// reloadChanges makes the Library read the stack again, as an event or a
// resync does.
func (f *fixture) reloadChanges() {
	f.l.loadDraftChanges(f.l.compoCtx)
	f.run()
}

// event dispatches a server event into the mounted Library, as the Bar does.
func (f *fixture) event(ev apiv0.Event) {
	f.l.compoCtx.NewActionWithValue(eventlog.ActionServerEvent, ev)
	f.run()
}

// typeNames returns the names of the WidgetTypes the server holds.
func (f *fixture) typeNames() []string {
	f.t.Helper()
	list, err := f.api.ListWidgetTypes(context.Background())
	if err != nil {
		f.t.Fatalf("list widget types: %v", err)
	}
	names := make([]string, len(list))
	for i, wt := range list {
		names[i] = wt.Name
	}
	return names
}

func TestDraftChangesBar_EmptyStack(t *testing.T) {
	url, api := newServer(t)
	f := mount(t, url, api)
	f.reloadChanges()
	f.noToasts()

	html := app.HTMLString(f.l.renderDraftChangesBar())
	if !strings.Contains(html, "Undo") || !strings.Contains(html, "Redo") {
		t.Fatalf("the bar has no Undo and Redo: %s", html)
	}
	if f.l.canUndo() || f.l.canRedo() {
		t.Errorf("an empty stack offers a step: %+v", f.l.changes)
	}
	if !strings.Contains(html, "Nothing to undo") || !strings.Contains(html, "Nothing to redo") {
		t.Errorf("the buttons do not say the stack is empty: %s", html)
	}
}

func TestDraftChangesBar_ShowsTheNextStep(t *testing.T) {
	url, api := newServer(t)
	f := mount(t, url, api)
	f.l.createItem(f.l.compoCtx)
	f.run()
	f.reloadChanges()
	f.noToasts()

	if !f.l.canUndo() {
		t.Fatalf("the stack has a change to undo: %+v", f.l.changes)
	}
	html := app.HTMLString(f.l.renderDraftChangesBar())
	if !strings.Contains(html, "Undo: Create widget type New Widget Type") {
		t.Errorf("the Undo button does not name the step: %s", html)
	}
}

func TestUndo_RestoresTheWidgetTypeAndRedoBringsItBack(t *testing.T) {
	url, api := newServer(t)
	f := mount(t, url, api)
	f.l.createItem(f.l.compoCtx)
	f.run()
	f.reloadChanges()

	f.l.undoDraftChange(f.l.compoCtx)
	f.run()

	f.noToasts()
	if names := f.typeNames(); len(names) != 0 {
		t.Errorf("widget types after undo: %v, want none", names)
	}
	if f.l.changes.CanUndo || !f.l.changes.CanRedo {
		t.Errorf("stack after undo: %+v, want a redo only", f.l.changes)
	}

	// The shortcut is the button's own command.
	f.l.applyKeyAction(f.l.compoCtx, uiutil.KeyRedo)
	f.run()

	f.noToasts()
	if names := f.typeNames(); len(names) != 1 || names[0] != "New Widget Type" {
		t.Errorf("widget types after redo: %v, want the redone type", names)
	}

	// The undo stated the Draft's new Version, and the Draft of a library
	// without a WidgetType states it in no content DTO: the write after the
	// redo goes through.
	if _, err := api.CreateWidgetType(context.Background(), apiv0.CreateWidgetTypeRequest{
		Name: "Gauge", HtmlTemplate: "<div></div>", Script: "function render(inputs) {}",
		ScriptLanguage: "javascript", DefaultWidth: 150, DefaultHeight: 70,
		Version: f.l.draftVersion(),
	}); err != nil {
		t.Errorf("a write at the Version the redo left: %v", err)
	}
}

// TestUndo_StaleVersion_RereadsTheStackAndTheList: a change made after the
// stack was read is a 409 with the Draft's state (Problem.draft).
func TestUndo_StaleVersion_RereadsTheStackAndTheList(t *testing.T) {
	url, api := newServer(t)
	f := mount(t, url, api)
	f.l.createItem(f.l.compoCtx)
	f.run()
	f.reloadChanges()
	createWidgetType(t, api, "Gauge") // the UI has not seen it

	f.l.undoDraftChange(f.l.compoCtx)
	f.run()

	got := f.oneToast()
	if got.Status != 409 || !strings.Contains(got.Detail, "changed concurrently") {
		t.Errorf("toast %+v, want the conflict", got)
	}
	if list, err := api.ListWidgetTypes(context.Background()); err != nil || len(list) != 2 {
		t.Errorf("widget types: %v, %v, want both kept", list, err)
	}
	if !f.l.changes.CanUndo {
		t.Errorf("stack after the conflict: %+v, want the reread one", f.l.changes)
	}
}

// TestUndo_EmptyStack_IsAToastAndAReread: the state the buttons were built from
// may state the Version the Draft is at and still have no step to apply. The
// 409 carries no Problem.draft, so nothing of the WidgetTypes is claimed: a
// toast and a reread of the stack alone.
func TestUndo_EmptyStack_IsAToastAndAReread(t *testing.T) {
	url, api := newServer(t)
	f := mount(t, url, api)
	f.l.createItem(f.l.compoCtx)
	f.run()
	f.reloadChanges()
	if _, err := api.UndoDraftChange(context.Background(), 0); err != nil { // someone else undid it
		t.Fatalf("undo through the API: %v", err)
	}
	state, err := api.DraftChanges(context.Background())
	if err != nil {
		t.Fatalf("read the stack: %v", err)
	}
	f.l.changes = apiv0.DraftChangesResponse{
		Version: state.Version, CanUndo: true, UndoDescription: "Create widget type New Widget Type",
	}
	f.l.changesLoaded = true

	f.l.undoDraftChange(f.l.compoCtx)
	f.run()

	got := f.oneToast()
	if got.Status != 409 || !strings.Contains(got.Detail, "Nothing to undo") {
		t.Errorf("toast %+v, want the empty stack", got)
	}
	if f.l.changes.CanUndo || !f.l.changes.CanRedo {
		t.Errorf("stack after the refusal: %+v, want the reread one", f.l.changes)
	}
	if names := f.typeNames(); len(names) != 0 {
		t.Errorf("widget types: %v, want the undo of the other client", names)
	}
}

// TestDraftChanges_AReleaseShownIsReadOnly: what is shown is a release, not the
// Draft, so there is nothing to undo and no request to make.
func TestDraftChanges_AReleaseShownIsReadOnly(t *testing.T) {
	url, api := newServer(t)
	f := mount(t, url, api)
	f.l.createItem(f.l.compoCtx)
	f.run()
	state, err := api.DraftState(context.Background())
	if err != nil {
		t.Fatalf("DraftState: %v", err)
	}
	commit, err := api.SaveCommit(context.Background(), apiv0.CreateCommitRequest{Version: state.Version, Title: "Save", Author: "Ada"})
	if err != nil {
		t.Fatalf("SaveCommit: %v", err)
	}
	if _, err := api.Release(context.Background(), apiv0.CreateReleaseRequest{Commit: commit.Number, Note: "one", Author: "Ada"}); err != nil {
		t.Fatalf("release: %v", err)
	}
	createWidgetType(t, api, "Gauge") // a change after the release, which cleared the stack
	f.l.loadReleases(f.l.compoCtx)
	f.run()
	f.reloadChanges()
	if !f.l.canUndo() {
		t.Fatalf("the Draft has a step to undo: %+v", f.l.changes)
	}

	f.l.view(1)
	f.run()

	if f.l.canUndo() || f.l.canRedo() {
		t.Errorf("a release shown offers a step: %+v", f.l.changes)
	}
	html := app.HTMLString(f.l.renderDraftChangesBar())
	if !strings.Contains(html, "open the Draft to edit it") {
		t.Errorf("the buttons do not say a release is shown: %s", html)
	}
}

// TestContentEvent_ReloadsTheStack: a write of the Draft's content is one
// DraftChange, so the buttons follow its event.
func TestContentEvent_ReloadsTheStack(t *testing.T) {
	url, api := newServer(t)
	f := mount(t, url, api)
	createWidgetType(t, api, "Gauge")

	f.event(apiv0.Event{Type: apiv0.EventWidgetTypeCreated, ID: api.LibraryClient.ID().String()})

	f.noToasts()
	if !f.l.canUndo() || !strings.Contains(f.l.changes.UndoDescription, "Gauge") {
		t.Errorf("stack after the event: %+v, want the creation", f.l.changes)
	}
}

// TestSwitchLibrary_ResetsTheStack: the stack of the Draft left is not the one
// of another.
func TestSwitchLibrary_ResetsTheStack(t *testing.T) {
	url, api := newServer(t)
	f := mount(t, url, api)
	f.l.createItem(f.l.compoCtx)
	f.run()
	f.reloadChanges()
	if !f.l.changesLoaded {
		t.Fatal("the stack was not read")
	}

	f.l.selectLibrary(f.l.compoCtx, uuid.Nil)
	f.run()

	if f.l.changesLoaded || f.l.changes.CanUndo || f.l.changes.CanRedo {
		t.Errorf("the stack survived the switch: %+v", f.l.changes)
	}
	if f.l.draftVersion() != 0 {
		t.Errorf("the Version of the Draft left survived: %d", f.l.draftVersion())
	}
}

// TestDraftChangesBar_UnnamedStep_IsOff: a step the stack offers but does not
// name is no step — its description is what the button shows.
func TestDraftChangesBar_UnnamedStep_IsOff(t *testing.T) {
	url, api := newServer(t)
	f := mount(t, url, api)
	f.l.createItem(f.l.compoCtx)
	f.run()
	f.reloadChanges()
	f.noToasts()

	state, err := api.DraftChanges(context.Background())
	if err != nil {
		t.Fatalf("read the stack: %v", err)
	}
	f.l.changes = apiv0.DraftChangesResponse{Version: state.Version, CanUndo: true}
	f.l.changesLoaded = true

	if f.l.canUndo() {
		t.Error("an unnamed step is offered")
	}
	if got, want := f.l.undoTitle(), "Nothing to undo"; got != want {
		t.Errorf("hover text %q, want %q", got, want)
	}
	f.l.applyKeyAction(f.l.compoCtx, uiutil.KeyUndo)
	f.run()
	f.noToasts()
	if got := f.typeNames(); len(got) != 1 {
		t.Errorf("widget types after the silent key: %v, want the one created", got)
	}
}
