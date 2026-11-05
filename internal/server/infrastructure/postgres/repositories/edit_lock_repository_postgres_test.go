package repositories_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// cleanEditLocks removes every EditLock row, so a test starts with none.
func cleanEditLocks(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(editlock.System(context.Background()), "DELETE FROM engineering.edit_locks"); err != nil {
		t.Fatalf("cleanEditLocks: %v", err)
	}
}

func editLockRepo() editlock.Repository {
	return repositories.NewEditLockRepositoryPostgres(testDB)
}

func aHolder(t *testing.T, name string) editlock.Holder {
	t.Helper()
	holder, err := editlock.NewHolder(editlock.NewKey(), name)
	if err != nil {
		t.Fatalf("holder %q: %v", name, err)
	}
	return holder
}

func TestEditLockRepository_Find_NoRow_IsNotFound(t *testing.T) {
	cleanEditLocks(t)
	lock, found, err := editLockRepo().Find(editlock.System(context.Background()), editlock.ProjectScope(mainProject))
	if err != nil || found || lock != nil {
		t.Fatalf("Find: %v, %v, %v; want no row", lock, found, err)
	}
}

func TestEditLockRepository_CreateAndFind(t *testing.T) {
	cleanEditLocks(t)
	repo := editLockRepo()
	ctx := editlock.System(context.Background())

	created, err := repo.Create(ctx, createHeldLock(t))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Version().Number() != 1 || !created.Held() {
		t.Fatalf("created %s", created)
	}
	found, ok, err := repo.Find(ctx, editlock.ProjectScope(mainProject))
	if err != nil || !ok {
		t.Fatalf("find: %v, %v", ok, err)
	}
	if found.Holder() != created.Holder() || !found.Since().Equal(created.Since()) {
		t.Errorf("found %s, want alice at %s", found, created.Since())
	}
}

func TestEditLockRepository_Save_StaleVersion_IsRefused(t *testing.T) {
	cleanEditLocks(t)
	repo := editLockRepo()
	ctx := editlock.System(context.Background())

	lock, err := repo.Create(ctx, createHeldLock(t))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// A second read of the same version loses the race: another holder saved
	// in between.
	if _, err := repo.Save(ctx, lock); err != nil {
		t.Fatalf("first save: %v", err)
	}
	other := editlock.ReconstituteEditLock(lock.Scope(), aHolder(t, "bob"), time.Now(), time.Now(), lock.Version())
	if _, err := repo.Save(ctx, other); !errors.Is(err, editlock.ErrEditLockVersionStale) {
		t.Fatalf("save at a stale version: %v, want ErrEditLockVersionStale", err)
	}
}

func TestEditLockRepository_Touch_RaisesLastSeenNotVersion(t *testing.T) {
	cleanEditLocks(t)
	repo := editLockRepo()
	ctx := editlock.System(context.Background())

	lock, err := repo.Create(ctx, createHeldLock(t))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	later := time.Now().Add(time.Minute).UTC().Truncate(time.Microsecond)
	if err := repo.Touch(ctx, lock.Scope(), lock.Holder().Key(), later); err != nil {
		t.Fatalf("touch: %v", err)
	}
	found, _, err := repo.Find(ctx, lock.Scope())
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !found.LastSeen().Equal(later) {
		t.Errorf("last seen %s, want %s", found.LastSeen(), later)
	}
	if found.Version() != lock.Version() {
		t.Errorf("touch raised the version to %s", found.Version())
	}
}

func TestEditLockRepository_FindAllHeld(t *testing.T) {
	cleanEditLocks(t)
	repo := editLockRepo()
	ctx := editlock.System(context.Background())

	lock, err := repo.Create(ctx, createHeldLock(t))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	held, err := repo.FindAllHeld(ctx)
	if err != nil {
		t.Fatalf("find all held: %v", err)
	}
	if len(held) != 1 || held[0].Scope() != lock.Scope() {
		t.Fatalf("held %v, want the one of project %s", held, lock.Scope())
	}

	released, err := lock.Release(lock.Holder().Key(), time.Now().UTC())
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := repo.Save(ctx, released); err != nil {
		t.Fatalf("save released: %v", err)
	}
	if held, err := repo.FindAllHeld(ctx); err != nil || len(held) != 0 {
		t.Errorf("a free lock is listed as held: %v, %v", held, err)
	}
}

func TestEditLockRepository_DeletingTheProjectTakesItsLock(t *testing.T) {
	cleanProjects(t)
	projects := repositories.NewProjectRepositoryPostgres(testDB)
	ctx := editlock.System(context.Background())

	orgID, err := repositories.FindSoleOrganization(ctx, testDB)
	if err != nil {
		t.Fatalf("organization: %v", err)
	}
	name, _ := project.NewProjectName("Cascade " + uuid.NewString())
	created, err := projects.Save(ctx, project.CreateProject(projects.NextID(), orgID, name))
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	repo := editLockRepo()
	scope := editlock.ProjectScope(created.ID())
	if _, err := repo.Create(ctx, createHeldLockOf(t, scope)); err != nil {
		t.Fatalf("create lock: %v", err)
	}

	deleted, err := projects.Save(ctx, created.Delete())
	if err != nil {
		t.Fatalf("mark deleted: %v", err)
	}
	if err := projects.Delete(ctx, deleted); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, found, err := repo.Find(ctx, scope); err != nil || found {
		t.Errorf("the lock outlived its project: found %v, %v", found, err)
	}
}

// ── The gate of a Draft's writes ─────────────────────────────────────────────

func TestGatedWrite_NoClient_IsRefused(t *testing.T) {
	cleanTags(t)
	cleanEditLocks(t)
	repo := repositories.NewTagRepositoryPostgres(testDB, mainProject)

	_, err := repo.Save(context.Background(), makeTag(t, "gated", repo))
	if !errors.Is(err, editlock.ErrNoClient) {
		t.Fatalf("write without a client: %v, want ErrNoClient", err)
	}
}

func TestGatedWrite_FreeLock_IsRefused(t *testing.T) {
	cleanTags(t)
	cleanEditLocks(t)
	repo := repositories.NewTagRepositoryPostgres(testDB, mainProject)

	ctx := editlock.WithClient(context.Background(), editlock.NewKey())
	_, err := repo.Save(ctx, makeTag(t, "gated", repo))
	if !errors.Is(err, editlock.ErrEditLockNotHeld) {
		t.Fatalf("write of a free Draft: %v, want ErrEditLockNotHeld", err)
	}
	// The refusal carries the state, so the Problem can say the Draft is
	// free rather than name a holder that nobody is.
	state, ok := editlock.StateOf(err)
	if !ok || state.Held || !state.Holder.IsZero() {
		t.Errorf("the error of a free Draft: %+v", state)
	}
}

func TestGatedWrite_AnotherHolder_NamesTheHolder(t *testing.T) {
	cleanTags(t)
	cleanEditLocks(t)
	repo := repositories.NewTagRepositoryPostgres(testDB, mainProject)
	if _, err := editLockRepo().Create(editlock.System(context.Background()), createHeldLock(t)); err != nil {
		t.Fatalf("hold the lock: %v", err)
	}

	ctx := editlock.WithClient(context.Background(), editlock.NewKey())
	_, err := repo.Save(ctx, makeTag(t, "gated", repo))
	if !errors.Is(err, editlock.ErrEditLockNotHeld) {
		t.Fatalf("write of another client's Draft: %v, want ErrEditLockNotHeld", err)
	}
	state, ok := editlock.StateOf(err)
	if !ok || !state.Held || state.Holder.Name() == "" {
		t.Errorf("the error does not name the holder: %+v", state)
	}
	// The state is the lock's, not a bare holder: the Version lets a client
	// offer a takeover, and the times say since when.
	if state.Version == 0 || state.Since.IsZero() || state.LastSeen.IsZero() {
		t.Errorf("the error carries no usable lock state: %+v", state)
	}
}

// TestGatedWrite_TheHolder_WritesConcurrently runs the holder's writes at the
// same time: the gate serializes them on its lock row, so none of them fails.
// (The gate once read the row FOR SHARE and then updated it, which deadlocked
// two of the holder's writes against each other.)
func TestGatedWrite_TheHolder_WritesConcurrently(t *testing.T) {
	cleanTags(t)
	cleanEditLocks(t)
	lock, err := editLockRepo().Create(editlock.System(context.Background()), createHeldLock(t))
	if err != nil {
		t.Fatalf("hold the lock: %v", err)
	}
	ctx := editlock.WithClient(context.Background(), lock.Holder().Key())

	const writers = 8
	// The aggregates are made here: the goroutines only write.
	repo := repositories.NewTagRepositoryPostgres(testDB, mainProject)
	tags := make([]tag.Tag, writers)
	for i := range tags {
		tags[i] = makeTag(t, fmt.Sprintf("gated.%d", i), repo)
	}

	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for i := range tags {
		wg.Add(1)
		go func(one tag.Tag) {
			defer wg.Done()
			writer := repositories.NewTagRepositoryPostgres(testDB, mainProject)
			_, err := writer.Save(ctx, one)
			errs <- err
		}(tags[i])
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("a write of the holder: %v", err)
		}
	}
}

func TestGatedWrite_TheHolder_Writes(t *testing.T) {
	cleanTags(t)
	cleanEditLocks(t)
	repo := repositories.NewTagRepositoryPostgres(testDB, mainProject)
	lock, err := editLockRepo().Create(editlock.System(context.Background()), createHeldLock(t))
	if err != nil {
		t.Fatalf("hold the lock: %v", err)
	}

	ctx := editlock.WithClient(context.Background(), lock.Holder().Key())
	if _, err := repo.Save(ctx, makeTag(t, "gated", repo)); err != nil {
		t.Fatalf("the holder's write: %v", err)
	}
	// The write raised last_seen: a client without a stream stays alive by
	// writing.
	found, _, err := editLockRepo().Find(editlock.System(context.Background()), lock.Scope())
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !found.LastSeen().After(lock.LastSeen()) {
		t.Errorf("last seen %s, want it raised by the write", found.LastSeen())
	}
}

// createHeldLock is a held lock of the main project, not yet stored.
func createHeldLock(t *testing.T) editlock.EditLock {
	t.Helper()
	return createHeldLockOf(t, editlock.ProjectScope(mainProject))
}

func createHeldLockOf(t *testing.T, scope editlock.Scope) editlock.EditLock {
	t.Helper()
	free := editlock.CreateEditLock(scope)
	held, err := free.Acquire(aHolder(t, "alice"), time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	return held
}
