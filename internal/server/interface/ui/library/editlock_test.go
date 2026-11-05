package library

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
)

// The EditLock of the Draft in the Library mode (ADR 0015): the row above the
// libraries, the read-only Draft, Check out and Check in, the takeover, the
// toast of a lost lock, and the Check in a switch of library sends. The
// fixture's API client and the mounted Library act as servertest.ClientKey,
// the holder of the library's lock (newTestAPI).

// reloadLock makes the Library read the lock again, as the resync does.
func (f *fixture) reloadLock() {
	f.l.loadEditLock(f.l.compoCtx)
	f.run()
}

func TestEditLockRow_Mine(t *testing.T) {
	f, _ := selected(t)
	f.reloadLock()

	html := app.HTMLString(f.l.renderEditLockBar())
	if !strings.Contains(html, `data-edit-lock="mine"`) || !strings.Contains(html, "Editing as servertest") {
		t.Errorf("the row does not show the holder: %s", html)
	}
	if !strings.Contains(html, "Check in") {
		t.Errorf("the row does not offer Check in: %s", html)
	}
	if f.l.readOnly() {
		t.Error("the holder's Draft is read-only")
	}
}

func TestEditLockRow_Free(t *testing.T) {
	f, id := selected(t)
	if _, err := f.api.CheckInEditLock(context.Background()); err != nil {
		t.Fatalf("check in: %v", err)
	}
	f.reloadLock()
	f.e.TakeToasts() // the Check in of this tab's client is losing the lock

	html := app.HTMLString(f.l.renderEditLockBar())
	if !strings.Contains(html, `data-edit-lock="free"`) || !strings.Contains(html, "Check out") {
		t.Errorf("the free row: %s", html)
	}
	if !f.l.readOnly() {
		t.Error("a free Draft is editable")
	}

	// No edit reaches the server.
	f.l.createItem(f.l.compoCtx)
	f.run()
	if list, _ := f.api.ListWidgetTypes(context.Background()); len(list) != 1 {
		t.Errorf("an edit of a free Draft reached the server: %+v", list)
	}
	if wt := f.widgetType(id); wt.ID != id {
		t.Errorf("the WidgetType is gone: %+v", wt)
	}

	// Release and Discard are disabled, with the hint.
	if html := app.HTMLString(f.l.renderLibraryStatus()); !strings.Contains(html, readOnlyHint) {
		t.Errorf("the Release and Discard buttons do not say the Draft is read-only: %s", html)
	}
	if html := app.HTMLString(f.l.renderListButtons()); !strings.Contains(html, readOnlyHint) {
		t.Errorf("the WidgetType buttons do not say the Draft is read-only: %s", html)
	}
}

func TestEditLockRow_HeldByAnother(t *testing.T) {
	f, _ := selected(t)
	other := f.api.Client.WithClientKey(uuid.NewString())
	if _, err := other.Library(f.l.libraryID).TakeOverEditLock(context.Background(), "bob", f.l.editLock.Version); err != nil {
		t.Fatalf("take over: %v", err)
	}
	f.reloadLock()
	f.e.TakeToasts()

	html := app.HTMLString(f.l.renderEditLockBar())
	if !strings.Contains(html, `data-edit-lock="other"`) || !strings.Contains(html, "bob is editing") {
		t.Errorf("the row does not name the holder: %s", html)
	}
	if !strings.Contains(html, "Take over") {
		t.Errorf("the row does not offer the takeover: %s", html)
	}
	if !f.l.readOnly() {
		t.Error("a Draft held by another is editable")
	}
}

func TestCheckOut_AsksTheNameOnceAndTakesTheDraft(t *testing.T) {
	f, _ := selected(t)
	if _, err := f.api.CheckInEditLock(context.Background()); err != nil {
		t.Fatalf("check in: %v", err)
	}
	f.reloadLock()
	f.e.TakeToasts()

	f.l.checkOut(f.l.compoCtx)
	f.run()
	if !f.l.askingName {
		t.Fatal("Check out did not ask the name")
	}
	if html := app.HTMLString(f.l.renderEditLockBar()); !strings.Contains(html, "data-edit-lock-name") {
		t.Fatalf("the name input is not shown: %s", html)
	}

	f.l.lockNameInput = "alice"
	f.l.confirmLockName(f.l.compoCtx)
	f.run()

	f.noToasts()
	if !f.l.lockMine() {
		t.Fatalf("the Draft is not held after Check out: %+v", f.l.editLock)
	}
	if f.l.editLock.Holder.Name != "alice" {
		t.Errorf("holder %q, want alice", f.l.editLock.Holder.Name)
	}
	if f.l.lockName != "alice" {
		t.Errorf("the declared name was not kept: %q", f.l.lockName)
	}
	if f.l.readOnly() {
		t.Error("the Draft is read-only after Check out")
	}
}

func TestCheckIn_GivesTheDraftBack(t *testing.T) {
	f, _ := selected(t)
	f.l.checkIn(f.l.compoCtx)
	f.run()

	f.noToasts()
	if f.l.editLock.Held {
		t.Errorf("the lock is still held: %+v", f.l.editLock)
	}
	if !f.l.readOnly() {
		t.Error("the Draft is editable after Check in")
	}
}

func TestTakeOver_ConfirmsAndTakesTheDraft(t *testing.T) {
	f, _ := selected(t)
	other := f.api.Client.WithClientKey(uuid.NewString())
	if _, err := other.Library(f.l.libraryID).TakeOverEditLock(context.Background(), "bob", f.l.editLock.Version); err != nil {
		t.Fatalf("take over: %v", err)
	}
	f.reloadLock()
	f.e.TakeToasts()

	f.l.openTakeOver(f.l.compoCtx)
	f.run()
	if !f.l.takeOverDialog {
		t.Fatal("no confirmation of the takeover")
	}
	if html := app.HTMLString(f.l.renderEditLockBar()); !strings.Contains(html, "data-takeover-dialog") || !strings.Contains(html, "bob has been editing this library") {
		t.Errorf("the confirmation does not name the holder: %s", html)
	}

	f.l.lockNameInput = "carol"
	f.l.doTakeOver(f.l.compoCtx)
	f.run()

	f.noToasts()
	if !f.l.lockMine() {
		t.Fatalf("the takeover did not take the Draft: %+v", f.l.editLock)
	}
	if f.l.editLock.Holder.Name != "carol" {
		t.Errorf("holder %q, want the declared carol", f.l.editLock.Holder.Name)
	}
}

func TestLostLock_IsAToastAndReadOnly(t *testing.T) {
	f, _ := selected(t)

	other := f.api.Client.WithClientKey(uuid.NewString())
	taken, err := other.Library(f.l.libraryID).TakeOverEditLock(context.Background(), "bob", f.l.editLock.Version)
	if err != nil {
		t.Fatalf("take over: %v", err)
	}

	// The event of the takeover reaches this tab's stream.
	f.event(apiv0.Event{
		Type:     apiv0.EventEditLockTakenOver,
		ID:       f.l.libraryID.String(),
		EditLock: &taken,
	})

	got := f.oneToast()
	if !strings.Contains(got.Detail, "no longer hold the lock") {
		t.Errorf("toast %+v, want the loss of the lock", got)
	}
	if !f.l.readOnly() {
		t.Error("the Draft is still editable after losing the lock")
	}
}

func TestOwnCheckIn_IsNoLossToast(t *testing.T) {
	f, _ := selected(t)
	f.reloadLock()
	f.e.TakeToasts()

	f.l.checkingOut = true // the Check in is in flight
	f.event(apiv0.Event{
		Type:     apiv0.EventEditLockReleased,
		ID:       f.l.libraryID.String(),
		EditLock: &apiv0.EditLockResponse{Held: false, Version: 2},
	})

	if toasts := f.e.TakeToasts(); len(toasts) != 0 {
		t.Errorf("the tab's own Check in raised a toast: %+v", toasts)
	}
	if !f.l.readOnly() {
		t.Error("the Draft is editable after the release event")
	}
}

// TestSwitchLibrary_ChecksTheDraftIn: a tab edits one Draft at a time, so a
// switch of library gives the Draft left back (ADR 0015).
func TestSwitchLibrary_ChecksTheDraftIn(t *testing.T) {
	url, api := newServer(t)
	other, err := api.CreateLibrary(context.Background(), apiv0.CreateLibraryRequest{Name: "Other"})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	f := mount(t, url, api)
	f.run()
	if !f.l.lockMine() {
		t.Fatalf("the fixture does not hold the Draft: %+v", f.l.editLock)
	}

	f.l.selectLibrary(f.l.compoCtx, other.ID)
	f.run()

	state, err := f.api.Library(api.ID()).StateEditLock(context.Background())
	if err != nil {
		t.Fatalf("state of the library left: %v", err)
	}
	if state.Held {
		t.Errorf("the Draft left is still held: %+v", state)
	}
}

// event dispatches a server event into the mounted Library, as the Bar does.
func (f *fixture) event(ev apiv0.Event) {
	f.l.compoCtx.NewActionWithValue(eventlog.ActionServerEvent, ev)
	f.run()
}
