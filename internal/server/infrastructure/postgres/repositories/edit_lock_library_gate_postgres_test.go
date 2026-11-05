package repositories_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/version"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// The gate of a Library's Draft (ADR 0015): the WidgetTypes of its Draft, a
// Release and a Discard are refused to a client that does not hold the Draft's
// EditLock, and made by the one that does.

func TestGatedLibraryWrite_NoClient_IsRefused(t *testing.T) {
	cleanEditLocks(t)
	repo := newWidgetTypeRepo(t)

	err := gatedWidgetTypeWrite(t, repo, context.Background())
	if !errors.Is(err, editlock.ErrNoClient) {
		t.Fatalf("write without a client: %v, want ErrNoClient", err)
	}
}

func TestGatedLibraryWrite_FreeLock_IsRefused(t *testing.T) {
	cleanEditLocks(t)
	repo := newWidgetTypeRepo(t)

	ctx := editlock.WithClient(context.Background(), editlock.NewKey())
	err := gatedWidgetTypeWrite(t, repo, ctx)
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
	err := gatedWidgetTypeWrite(t, repo, ctx)
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
	if err := gatedWidgetTypeWrite(t, repo, ctx); err != nil {
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
	mustStoreWidgetType(t, makeWidgetType(t, "type", nil, repo), repo)
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

// TestGatedLibraryDiscard_NeedsTheLock: a Discard is one transaction over the
// Draft (Replace), so its gate refuses a client that does not hold the lock as
// the writes do (ADR 0015, ADR 0016).
func TestGatedLibraryDiscard_NeedsTheLock(t *testing.T) {
	cleanEditLocks(t)
	repo := newWidgetTypeRepo(t)
	mustStoreWidgetType(t, makeWidgetType(t, "type", nil, repo), repo)
	release := mustRelease(t, repo.libraryID)
	scope := editlock.LibraryScope(repo.libraryID)
	discard := func(ctx context.Context) error {
		_, err := testDrafts().Replace(ctx, scope, func(session *draft.Session) error {
			return session.ReplaceWidgetTypes(ctx, release)
		})
		return err
	}

	if err := discard(context.Background()); !errors.Is(err, editlock.ErrNoClient) {
		t.Errorf("discard without a client: %v, want ErrNoClient", err)
	}
	ctx := editlock.WithClient(context.Background(), editlock.NewKey())
	if err := discard(ctx); !errors.Is(err, editlock.ErrEditLockNotHeld) {
		t.Errorf("discard of a free Draft: %v, want ErrEditLockNotHeld", err)
	}

	lock, err := editLockRepo().Create(editlock.System(context.Background()), createHeldLockOf(t, scope))
	if err != nil {
		t.Fatalf("hold the lock: %v", err)
	}
	holder := editlock.WithClient(context.Background(), lock.Holder().Key())
	if err := discard(holder); err != nil {
		t.Errorf("the holder's discard: %v", err)
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

// versionCommitted is the stored Version of the library's Draft.
func versionCommitted(t *testing.T, libraryID id.ID[library.Library]) version.Version[library.DraftContent] {
	t.Helper()
	found, ok, err := testDrafts().Read(editlock.System(context.Background()), editlock.LibraryScope(libraryID))
	if err != nil {
		t.Fatalf("read draft: %v", err)
	}
	number := 1
	if ok {
		number = found.Version().Number()
	}
	ver, err := version.New(version.WithNumber[library.DraftContent](number))
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	return ver
}

// gatedWidgetTypeWrite creates a WidgetType through the Draft, as a client
// write does, and returns the gate's error.
func gatedWidgetTypeWrite(t *testing.T, repo libraryRepo, ctx context.Context) error {
	t.Helper()
	wt := makeWidgetType(t, "gated", nil, repo)
	_, err := testDrafts().Update(ctx, editlock.LibraryScope(repo.libraryID), libraryDraftVersionOf(t, repo.libraryID),
		"Create widget type gated", func(session *draft.Session) error {
			session.CreateWidgetType(ctx, wt)
			return nil
		})
	return err
}
