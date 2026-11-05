package repositories_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/version"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// The gate of a Library's Draft (ADR 0015): the WidgetTypes of its Draft, a
// Release and the start of a Discard are refused to a client that does not
// hold the Draft's EditLock, and made by the one that does.

func TestGatedLibraryWrite_NoClient_IsRefused(t *testing.T) {
	cleanEditLocks(t)
	repo := newWidgetTypeRepo(t)

	_, err := repo.Save(context.Background(), makeWidgetType(t, "gated", nil, repo))
	if !errors.Is(err, editlock.ErrNoClient) {
		t.Fatalf("write without a client: %v, want ErrNoClient", err)
	}
}

func TestGatedLibraryWrite_FreeLock_IsRefused(t *testing.T) {
	cleanEditLocks(t)
	repo := newWidgetTypeRepo(t)

	ctx := editlock.WithClient(context.Background(), editlock.NewKey())
	_, err := repo.Save(ctx, makeWidgetType(t, "gated", nil, repo))
	if !errors.Is(err, editlock.ErrEditLockNotHeld) {
		t.Fatalf("write of a free Draft: %v, want ErrEditLockNotHeld", err)
	}
	if state, ok := editlock.StateOf(err); !ok || state.Held {
		t.Errorf("the error of a free Draft: %+v", state)
	}
}

func TestGatedLibraryWrite_AnotherHolder_NamesTheHolder(t *testing.T) {
	cleanEditLocks(t)
	repo := newWidgetTypeRepo(t)
	if _, err := editLockRepo().Create(editlock.System(context.Background()),
		createHeldLockOf(t, editlock.LibraryScope(repo.libraryID))); err != nil {
		t.Fatalf("hold the lock: %v", err)
	}

	ctx := editlock.WithClient(context.Background(), editlock.NewKey())
	_, err := repo.Save(ctx, makeWidgetType(t, "gated", nil, repo))
	if !errors.Is(err, editlock.ErrEditLockNotHeld) {
		t.Fatalf("write of another client's Draft: %v, want ErrEditLockNotHeld", err)
	}
	state, ok := editlock.StateOf(err)
	if !ok || !state.Held || state.Holder.Name() != "alice" || state.Version == 0 {
		t.Errorf("the error does not name the holder: %+v", state)
	}
}

func TestGatedLibraryWrite_TheHolder_Writes(t *testing.T) {
	cleanEditLocks(t)
	repo := newWidgetTypeRepo(t)
	lock, err := editLockRepo().Create(editlock.System(context.Background()),
		createHeldLockOf(t, editlock.LibraryScope(repo.libraryID)))
	if err != nil {
		t.Fatalf("hold the lock: %v", err)
	}

	ctx := editlock.WithClient(context.Background(), lock.Holder().Key())
	if _, err := repo.Save(ctx, makeWidgetType(t, "gated", nil, repo)); err != nil {
		t.Fatalf("the holder's write: %v", err)
	}
	found, _, err := editLockRepo().Find(editlock.System(context.Background()), lock.Scope())
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !found.LastSeen().After(lock.LastSeen()) {
		t.Errorf("last seen %s, want it raised by the write", found.LastSeen())
	}
}

func TestGatedLibraryRelease_NeedsTheLock(t *testing.T) {
	cleanEditLocks(t)
	repo := newWidgetTypeRepo(t)
	if _, err := repo.Save(editlock.System(context.Background()), makeWidgetType(t, "type", nil, repo)); err != nil {
		t.Fatalf("save widget type: %v", err)
	}
	release := newReleaseOf(t, repo.libraryID)

	if _, err := repositories.NewLibraryReleaseRepositoryPostgres(testDB).Save(context.Background(), release, versionCommitted(t, repo.libraryID)); !errors.Is(err, editlock.ErrNoClient) {
		t.Errorf("release without a client: %v, want ErrNoClient", err)
	}

	ctx := editlock.WithClient(context.Background(), editlock.NewKey())
	if _, err := repositories.NewLibraryReleaseRepositoryPostgres(testDB).Save(ctx, release, versionCommitted(t, repo.libraryID)); !errors.Is(err, editlock.ErrEditLockNotHeld) {
		t.Errorf("release of a free Draft: %v, want ErrEditLockNotHeld", err)
	}

	lock, err := editLockRepo().Create(editlock.System(context.Background()), createHeldLockOf(t, editlock.LibraryScope(repo.libraryID)))
	if err != nil {
		t.Fatalf("hold the lock: %v", err)
	}
	holder := editlock.WithClient(context.Background(), lock.Holder().Key())
	if _, err := repositories.NewLibraryReleaseRepositoryPostgres(testDB).Save(holder, release, versionCommitted(t, repo.libraryID)); err != nil {
		t.Errorf("the holder's release: %v", err)
	}
}

func TestGatedLibraryDiscard_NeedsTheLock(t *testing.T) {
	cleanEditLocks(t)
	lib := mustSaveLibrary(t)
	libraries := repositories.NewLibraryRepositoryPostgres(testDB)
	// A Discard needs a release to reset the Draft to; its number is not read
	// by the gate, which runs before the mark is written.
	marked, err := lib.StartDiscard(releaseNumber(t, 0))
	if err != nil {
		t.Fatalf("start discard: %v", err)
	}

	if _, err := libraries.Save(context.Background(), marked); !errors.Is(err, editlock.ErrNoClient) {
		t.Errorf("discard without a client: %v, want ErrNoClient", err)
	}
	ctx := editlock.WithClient(context.Background(), editlock.NewKey())
	if _, err := libraries.Save(ctx, marked); !errors.Is(err, editlock.ErrEditLockNotHeld) {
		t.Errorf("discard of a free Draft: %v, want ErrEditLockNotHeld", err)
	}

	lock, err := editLockRepo().Create(editlock.System(context.Background()), createHeldLockOf(t, editlock.LibraryScope(lib.ID())))
	if err != nil {
		t.Fatalf("hold the lock: %v", err)
	}
	holder := editlock.WithClient(context.Background(), lock.Holder().Key())
	// The gate passes; the mark's own check (the latest release) is the
	// repository's business and may refuse here, which is not the lock's.
	if _, err := libraries.Save(holder, marked); errors.Is(err, editlock.ErrEditLockNotHeld) {
		t.Errorf("the holder's discard is refused by the gate")
	}
}

// newReleaseOf builds the next release of the library's Draft as it is, not
// stored.
func newReleaseOf(t *testing.T, libraryID id.ID[library.Library]) library.LibraryRelease {
	t.Helper()
	ctx := editlock.System(context.Background())
	lib, err := repositories.NewLibraryRepositoryPostgres(testDB).FindByID(ctx, libraryID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	draft, err := repositories.NewWidgetTypeRepositoryPostgres(testDB, libraryID).FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	latest, err := repositories.NewLibraryReleaseRepositoryPostgres(testDB).FindLatest(ctx, libraryID)
	if err != nil {
		t.Fatalf("FindLatest: %v", err)
	}
	release, err := library.CreateRelease(lib, draft, latest, "", time.Now())
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	return release
}

// versionCommitted is the stored Version of the library.
func versionCommitted(t *testing.T, libraryID id.ID[library.Library]) version.Version[library.Library] {
	t.Helper()
	lib, err := repositories.NewLibraryRepositoryPostgres(testDB).FindByID(editlock.System(context.Background()), libraryID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	return lib.Version()
}

// releaseNumber is a ReleaseNumber of n (n > 0).
func releaseNumber(t *testing.T, n int) library.ReleaseNumber {
	t.Helper()
	if n == 0 {
		n = 1
	}
	number, err := library.NewReleaseNumber(n)
	if err != nil {
		t.Fatalf("ReleaseNumber: %v", err)
	}
	return number
}
