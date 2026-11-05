package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// The EditLock's service (ADR 0015): the Check out, the Check in, the
// takeover, the gate the services and Deploy read, and the reaper.

func cleanEditLocksApp(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(editlock.System(context.Background()), "DELETE FROM engineering.draft_changes; DELETE FROM engineering.drafts"); err != nil {
		t.Fatalf("clean edit locks: %v", err)
	}
}

// editLockServiceOver returns the service over the test database, reading the
// time from now unless an option says otherwise.
func editLockServiceOver(opts ...application.EditLockOption) application.EditLockService {
	content := contentOver(newScopes())
	return application.NewEditLockService(content, newLibraryContent(),
		repositories.NewDraftRepositoryPostgres(testDB, repositories.NewLibraryReleaseRepositoryPostgres(testDB)), opts...)
}

// projectScope is the Scope of the Draft of the Project projectID.
func projectScope(projectID uuid.UUID) editlock.Scope {
	return editlock.ProjectScope(id.NewID(id.IDWithUUID[project.Project](projectID)))
}

func clientContext(key editlock.Key) context.Context {
	return editlock.WithClient(context.Background(), key)
}

func TestEditLockService_CheckOutAndCheckIn(t *testing.T) {
	cleanEditLocksApp(t)
	service := editLockServiceOver()
	key := editlock.NewKey()
	ctx := clientContext(key)

	if state, err := service.State(editlock.System(context.Background()), projectScope(mainProject)); err != nil || state.Held {
		t.Fatalf("state before Check out: %+v, %v", state, err)
	}
	state, took, err := service.CheckOut(ctx, projectScope(mainProject), "alice")
	if err != nil || !took {
		t.Fatalf("check out: %v", err)
	}
	if !state.Held || state.Holder == nil || state.Holder.Name != "alice" || state.Holder.Key != key.UUID() || state.Version != 1 {
		t.Fatalf("checked out: %+v", state)
	}

	if _, took, err := service.CheckOut(ctx, projectScope(mainProject), "alice"); err != nil || took {
		t.Errorf("check out again: took %t, %v; want 200 with the same lock", took, err)
	}

	state, err = service.CheckIn(ctx, projectScope(mainProject))
	if err != nil || state.Held || state.Version != 2 {
		t.Fatalf("check in: %+v, %v", state, err)
	}
	if state, err := service.CheckIn(ctx, projectScope(mainProject)); err != nil || state.Held {
		t.Errorf("check in of a free lock: %+v, %v", state, err)
	}
}

func TestEditLockService_CheckOut_WithoutClient_IsErrNoClient(t *testing.T) {
	cleanEditLocksApp(t)
	_, _, err := editLockServiceOver().CheckOut(context.Background(), projectScope(mainProject), "alice")
	if !errors.Is(err, editlock.ErrNoClient) {
		t.Fatalf("check out without a client: %v, want ErrNoClient", err)
	}
}

func TestEditLockService_CheckOut_EmptyName_IsInvalidInput(t *testing.T) {
	cleanEditLocksApp(t)
	_, _, err := editLockServiceOver().CheckOut(clientContext(editlock.NewKey()), projectScope(mainProject), " ")
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("check out with an empty name: %v, want invalid input", err)
	}
}

func TestEditLockService_CheckOut_HeldByAnother(t *testing.T) {
	cleanEditLocksApp(t)
	service := editLockServiceOver()
	if _, _, err := service.CheckOut(clientContext(editlock.NewKey()), projectScope(mainProject), "alice"); err != nil {
		t.Fatalf("check out: %v", err)
	}
	_, _, err := service.CheckOut(clientContext(editlock.NewKey()), projectScope(mainProject), "bob")
	if !errors.Is(err, editlock.ErrEditLockHeld) {
		t.Fatalf("check out of a held lock: %v, want ErrEditLockHeld", err)
	}
	if state, ok := editlock.StateOf(err); !ok || state.Holder.Name() != "alice" {
		t.Errorf("the error does not name the holder: %+v", state)
	}
}

func TestEditLockService_TakeOver(t *testing.T) {
	cleanEditLocksApp(t)
	service := editLockServiceOver()
	alice, bob := editlock.NewKey(), editlock.NewKey()
	if _, _, err := service.CheckOut(clientContext(alice), projectScope(mainProject), "alice"); err != nil {
		t.Fatalf("check out: %v", err)
	}

	if _, err := service.TakeOver(clientContext(bob), projectScope(mainProject), "bob", 9); !errors.Is(err, editlock.ErrEditLockVersionStale) {
		t.Errorf("stale takeover: %v, want ErrEditLockVersionStale", err)
	}
	state, err := service.TakeOver(clientContext(bob), projectScope(mainProject), "bob", 1)
	if err != nil {
		t.Fatalf("take over: %v", err)
	}
	if state.Holder == nil || state.Holder.Name != "bob" || state.Version != 2 {
		t.Errorf("taken over: %+v", state)
	}
}

func TestEditLockService_CheckHeld(t *testing.T) {
	cleanEditLocksApp(t)
	service := editLockServiceOver()
	key := editlock.NewKey()
	ctx := clientContext(key)

	if err := service.CheckHeld(editlock.System(context.Background()), projectScope(mainProject)); err != nil {
		t.Errorf("the server's own write: %v", err)
	}
	if err := service.CheckHeld(context.Background(), projectScope(mainProject)); !errors.Is(err, editlock.ErrNoClient) {
		t.Errorf("a write naming no client: %v, want ErrNoClient", err)
	}
	if err := service.CheckHeld(ctx, projectScope(mainProject)); !errors.Is(err, editlock.ErrEditLockNotHeld) {
		t.Errorf("a write of a free Draft: %v, want ErrEditLockNotHeld", err)
	}

	if _, _, err := service.CheckOut(ctx, projectScope(mainProject), "alice"); err != nil {
		t.Fatalf("check out: %v", err)
	}
	if err := service.CheckHeld(ctx, projectScope(mainProject)); err != nil {
		t.Errorf("the holder's write: %v", err)
	}
	if err := service.CheckHeld(clientContext(editlock.NewKey()), projectScope(mainProject)); !errors.Is(err, editlock.ErrEditLockNotHeld) {
		t.Errorf("another client's write: %v, want ErrEditLockNotHeld", err)
	}
}

func TestEditLockService_ReapOnce_ExpiresASilentHolder(t *testing.T) {
	cleanEditLocksApp(t)
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	service := editLockServiceOver(application.WithEditLockClock(clock))
	ctx := clientContext(editlock.NewKey())
	if _, _, err := service.CheckOut(ctx, projectScope(mainProject), "alice"); err != nil {
		t.Fatalf("check out: %v", err)
	}

	// Within the TTL the absent holder keeps the lock.
	now = now.Add(application.EditLockTTL - time.Minute)
	if err := service.ReapOnce(editlock.System(context.Background())); err != nil {
		t.Fatalf("reap: %v", err)
	}
	if state, _ := service.State(editlock.System(context.Background()), projectScope(mainProject)); !state.Held {
		t.Fatalf("the lock expired before the TTL: %+v", state)
	}

	now = now.Add(2 * time.Minute)
	if err := service.ReapOnce(editlock.System(context.Background())); err != nil {
		t.Fatalf("reap: %v", err)
	}
	state, err := service.State(editlock.System(context.Background()), projectScope(mainProject))
	if err != nil || state.Held || state.Version != 2 {
		t.Errorf("after the TTL: %+v, %v; want it free at version 2", state, err)
	}
}

func TestEditLockService_ReapOnce_KeepsAPresentHolder(t *testing.T) {
	cleanEditLocksApp(t)
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	service := editLockServiceOver(application.WithEditLockClock(clock))
	key := editlock.NewKey()
	if _, _, err := service.CheckOut(clientContext(key), projectScope(mainProject), "alice"); err != nil {
		t.Fatalf("check out: %v", err)
	}

	// An open stream of the holder keeps the lock alive past the TTL.
	service.Attach(key)
	now = now.Add(application.EditLockTTL + time.Minute)
	if err := service.ReapOnce(editlock.System(context.Background())); err != nil {
		t.Fatalf("reap: %v", err)
	}
	if state, _ := service.State(editlock.System(context.Background()), projectScope(mainProject)); !state.Held || state.Holder.Key != key.UUID() {
		t.Fatalf("the present holder's lock was freed: %+v", state)
	}

	// Closed, it expires one TTL later.
	service.Detach(key)
	now = now.Add(application.EditLockTTL + time.Minute)
	if err := service.ReapOnce(editlock.System(context.Background())); err != nil {
		t.Fatalf("reap: %v", err)
	}
	if state, _ := service.State(editlock.System(context.Background()), projectScope(mainProject)); state.Held {
		t.Errorf("the lock outlived its holder by more than the TTL: %+v", state)
	}
}

func TestEditLockService_State_UnknownProject_IsNotFound(t *testing.T) {
	if _, err := editLockServiceOver().State(editlock.System(context.Background()), projectScope(uuid.New())); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("state of an unknown project: %v", err)
	}
}

// libraryScope is the Scope of the Draft of the Library libraryID.
func libraryScope(libraryID uuid.UUID) editlock.Scope {
	return editlock.LibraryScope(id.NewID(id.IDWithUUID[library.Library](libraryID)))
}

// TestEditLockService_LibraryDraft: the same Check out, Check in, takeover and
// gate over the Draft of a Library (ADR 0015).
func TestEditLockService_LibraryDraft(t *testing.T) {
	cleanEditLocksApp(t)
	service := editLockServiceOver()
	lib := mustCreateLibrary(t)
	scope := libraryScope(lib.ID)
	alice, bob := editlock.NewKey(), editlock.NewKey()

	if state, err := service.State(editlock.System(context.Background()), scope); err != nil || state.Held {
		t.Fatalf("state before Check out: %+v, %v", state, err)
	}
	state, took, err := service.CheckOut(clientContext(alice), scope, "alice")
	if err != nil || !took || !state.Held || state.Holder.Key != alice.UUID() {
		t.Fatalf("check out of a library: %+v, took %t, %v", state, took, err)
	}
	if _, _, err := service.CheckOut(clientContext(bob), scope, "bob"); !errors.Is(err, editlock.ErrEditLockHeld) {
		t.Errorf("check out of a held library: %v, want ErrEditLockHeld", err)
	}
	if err := service.CheckHeld(clientContext(alice), scope); err != nil {
		t.Errorf("the holder's write: %v", err)
	}
	if err := service.CheckHeld(clientContext(bob), scope); !errors.Is(err, editlock.ErrEditLockNotHeld) {
		t.Errorf("another client's write: %v, want ErrEditLockNotHeld", err)
	}
	if _, err := service.TakeOver(clientContext(bob), scope, "bob", state.Version); err != nil {
		t.Fatalf("take over a library: %v", err)
	}
	if state, err := service.CheckIn(clientContext(bob), scope); err != nil || state.Held {
		t.Fatalf("check in of a library: %+v, %v", state, err)
	}
}

func TestEditLockService_State_UnknownLibrary_IsNotFound(t *testing.T) {
	if _, err := editLockServiceOver().State(editlock.System(context.Background()), libraryScope(uuid.New())); !errors.Is(err, library.ErrLibraryNotFound) {
		t.Errorf("state of an unknown library: %v", err)
	}
}

func TestDeploy_NeedsTheEditLock(t *testing.T) {
	deploy, _ := newDeployService()
	// The Draft of Main is deployed by other tests; the gate refuses before
	// anything is built.
	if _, err := deploy.Deploy(context.Background(), mainProject); !errors.Is(err, editlock.ErrNoClient) {
		t.Errorf("deploy without a client: %v, want ErrNoClient", err)
	}
	if _, err := deploy.Deploy(clientContext(editlock.NewKey()), mainProject); !errors.Is(err, editlock.ErrEditLockNotHeld) {
		t.Errorf("deploy of a free Draft: %v, want ErrEditLockNotHeld", err)
	}
}

func TestDiscard_NeedsTheEditLock(t *testing.T) {
	cleanEditLocksApp(t)
	deployProject(t, mainProject) // a Discard needs a Revision to reset the Draft to
	service := newDiscardService()
	if _, err := service.Discard(context.Background(), mainProject); !errors.Is(err, editlock.ErrNoClient) {
		t.Errorf("discard without a client: %v, want ErrNoClient", err)
	}
}
