package project

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/servertest"
)

// The EditLock of the Draft in the Project mode (ADR 0015): the row of the
// header, the read-only Draft, Check out and Check in, the takeover, and the
// toast of a lost lock. The fixture's API client and the mounted Project act
// as servertest.ClientKey, the holder of Main's lock (newServer).

// otherClient is a client of the same server acting as another tab: it takes
// the lock over without the UI knowing.
func otherClient(t *testing.T, api *testAPI) *testAPI {
	t.Helper()
	other := api.Client.WithClientKey(uuid.NewString())
	return newTestAPI(other)
}

// reloadLock makes the Project read the lock again, as the resync does.
func (f *fixture) reloadLock() {
	f.p.loadEditLock(f.p.compoCtx)
	f.run()
}

func TestEditLockRow_Mine(t *testing.T) {
	f := startProject(t)
	f.reloadLock()

	html := app.HTMLString(f.p.renderEditLockBar())
	if !strings.Contains(html, `data-edit-lock="mine"`) || !strings.Contains(html, "Editing as servertest") {
		t.Errorf("the row does not show the holder: %s", html)
	}
	if !strings.Contains(html, "Check in") {
		t.Errorf("the row does not offer Check in: %s", html)
	}
	if f.p.readOnly() {
		t.Error("the holder's Draft is read-only")
	}
}

func TestEditLockRow_Free(t *testing.T) {
	f := startProject(t)
	if _, err := f.api.CheckInEditLock(context.Background()); err != nil {
		t.Fatalf("check in: %v", err)
	}
	f.reloadLock()

	html := app.HTMLString(f.p.renderEditLockBar())
	if !strings.Contains(html, `data-edit-lock="free"`) || !strings.Contains(html, "Check out") {
		t.Errorf("the free row: %s", html)
	}
	if !f.p.readOnly() {
		t.Error("a free Draft is editable")
	}
}

func TestEditLockRow_HeldByAnother(t *testing.T) {
	f := startProject(t)
	other := otherClient(t, f.api)
	if _, err := other.TakeOverEditLock(context.Background(), "bob", f.p.editLock.Version); err != nil {
		t.Fatalf("take over: %v", err)
	}
	f.reloadLock()

	html := app.HTMLString(f.p.renderEditLockBar())
	if !strings.Contains(html, `data-edit-lock="other"`) || !strings.Contains(html, "bob is editing") {
		t.Errorf("the row does not name the holder: %s", html)
	}
	if !strings.Contains(html, "Take over") {
		t.Errorf("the row does not offer the takeover: %s", html)
	}
	if !f.p.readOnly() {
		t.Error("a Draft held by another is editable")
	}
}

func TestReadOnly_RefusesTheDraftAndDeploy(t *testing.T) {
	f := startProject(t)
	other := otherClient(t, f.api)
	if _, err := other.TakeOverEditLock(context.Background(), "bob", f.p.editLock.Version); err != nil {
		t.Fatalf("take over: %v", err)
	}
	f.reloadLock()
	f.e.TakeToasts()

	// An edit is refused on this side: no request reaches the server.
	f.p.createScene(f.p.compoCtx)
	f.run()
	if got := f.oneToast(); !strings.Contains(got.Detail, "Read-only") {
		t.Errorf("create scene: toast %+v, want the read-only hint", got)
	}
	if len(f.p.scenes) != 0 {
		t.Errorf("a Scene was created in a read-only Draft: %+v", f.p.scenes)
	}

	// Deploy and Discard are disabled, with the hint.
	html := app.HTMLString(f.p.renderDeployBar())
	if !strings.Contains(html, "data-read-only") {
		t.Errorf("the bar does not say the Draft is read-only: %s", html)
	}
}

func TestCheckOut_AsksTheNameOnceAndTakesTheDraft(t *testing.T) {
	f := startProject(t)
	if _, err := f.api.CheckInEditLock(context.Background()); err != nil {
		t.Fatalf("check in: %v", err)
	}
	f.reloadLock()
	f.e.TakeToasts() // the Check in of the API is this tab losing the lock

	f.p.checkOut(f.p.compoCtx)
	f.run()
	if !f.p.askingName {
		t.Fatal("Check out did not ask the name")
	}
	html := app.HTMLString(f.p.renderEditLockBar())
	if !strings.Contains(html, "data-edit-lock-name") {
		t.Fatalf("the name input is not shown: %s", html)
	}

	f.p.lockNameInput = "alice"
	f.p.confirmLockName(f.p.compoCtx)
	f.run()

	f.noToasts()
	if !f.p.lockMine() {
		t.Fatalf("the Draft is not held after Check out: %+v", f.p.editLock)
	}
	if f.p.editLock.Holder.Name != "alice" {
		t.Errorf("holder %q, want alice", f.p.editLock.Holder.Name)
	}
	// The name is remembered: the next Check out asks nothing.
	if f.p.lockName != "alice" {
		t.Errorf("the declared name was not kept: %q", f.p.lockName)
	}
	// And the Draft is editable now.
	if _, err := f.api.CreateScene(context.Background(), apiv0.CreateSceneRequest{Name: "after check out", Width: 100, Height: 100}); err != nil {
		t.Errorf("create a Scene after Check out: %v", err)
	}
}

func TestCheckIn_GivesTheDraftBack(t *testing.T) {
	f := startProject(t)
	f.p.checkIn(f.p.compoCtx)
	f.run()

	f.noToasts()
	if f.p.editLock.Held {
		t.Errorf("the lock is still held: %+v", f.p.editLock)
	}
	if !f.p.readOnly() {
		t.Error("the Draft is editable after Check in")
	}
}

func TestTakeOver_ConfirmsAndTakesTheDraft(t *testing.T) {
	f := startProject(t)
	other := otherClient(t, f.api)
	if _, err := other.TakeOverEditLock(context.Background(), "bob", f.p.editLock.Version); err != nil {
		t.Fatalf("take over: %v", err)
	}
	f.reloadLock()
	f.e.TakeToasts()

	f.p.openTakeOver(f.p.compoCtx)
	f.run()
	if !f.p.takeOverDialog {
		t.Fatal("no confirmation of the takeover")
	}
	html := app.HTMLString(f.p.renderEditLockBar())
	if !strings.Contains(html, "data-takeover-dialog") || !strings.Contains(html, "bob has been editing this project") {
		t.Errorf("the confirmation does not name the holder: %s", html)
	}

	f.p.lockNameInput = "bob"
	f.p.doTakeOver(f.p.compoCtx)
	f.run()

	f.noToasts()
	if !f.p.lockMine() {
		t.Fatalf("the takeover did not take the Draft: %+v", f.p.editLock)
	}
	if f.p.editLock.Holder.Name != "bob" {
		t.Errorf("holder %q, want the declared bob", f.p.editLock.Holder.Name)
	}
}

func TestLostLock_IsAToastAndReadOnly(t *testing.T) {
	f := startProject(t)
	holderBefore := f.p.editLock.Holder.Name

	other := otherClient(t, f.api)
	taken, err := other.TakeOverEditLock(context.Background(), "bob", f.p.editLock.Version)
	if err != nil {
		t.Fatalf("take over: %v", err)
	}

	// The event of the takeover reaches this tab's stream.
	f.event(apiv0.Event{
		Type:     apiv0.EventEditLockTakenOver,
		ID:       servertest.MainProject.String(),
		EditLock: lockResponse(taken),
	})

	got := f.oneToast()
	if !strings.Contains(got.Detail, "no longer hold the lock") {
		t.Errorf("toast %+v, want the loss of the lock", got)
	}
	if !f.p.readOnly() {
		t.Errorf("the Draft is still editable after losing the lock (was %s)", holderBefore)
	}
}

// TestOwnCheckIn_IsNoLossToast: the release event of this tab's own Check in
// may reach the stream before the answer to the request. It is not a loss of
// the lock, so the tab raises no toast (ADR 0015).
func TestOwnCheckIn_IsNoLossToast(t *testing.T) {
	f := startProject(t)
	f.reloadLock()
	f.e.TakeToasts()

	f.p.checkingOut = true // the Check in is in flight
	f.event(apiv0.Event{
		Type:     apiv0.EventEditLockReleased,
		ID:       servertest.MainProject.String(),
		EditLock: lockResponse(apiv0.EditLockResponse{Held: false, Version: 2}),
	})

	if toasts := f.e.TakeToasts(); len(toasts) != 0 {
		t.Errorf("the tab's own Check in raised a toast: %+v", toasts)
	}
	if !f.p.readOnly() {
		t.Error("the Draft is editable after the release event")
	}
}

func TestEditLockEvent_AppliesTheStateWithoutAReread(t *testing.T) {
	f := startProject(t)
	state, err := f.api.StateEditLock(context.Background())
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if _, err := f.api.CheckInEditLock(context.Background()); err != nil {
		t.Fatalf("check in: %v", err)
	}

	f.event(apiv0.Event{
		Type:     apiv0.EventEditLockReleased,
		ID:       servertest.MainProject.String(),
		EditLock: lockResponse(state),
	})
	if !f.p.editLock.Held {
		t.Errorf("the event's state was not applied: %+v", f.p.editLock)
	}
}

func lockResponse(state apiv0.EditLockResponse) *apiv0.EditLockResponse {
	return &state
}
