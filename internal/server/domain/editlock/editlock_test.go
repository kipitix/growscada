package editlock_test

import (
	"errors"
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

var at = time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)

func projectScope() editlock.Scope {
	return editlock.ProjectScope(id.NewID[project.Project]())
}

func holderOf(t *testing.T, key editlock.Key, name string) editlock.Holder {
	t.Helper()
	holder, err := editlock.NewHolder(key, name)
	if err != nil {
		t.Fatalf("holder: %v", err)
	}
	return holder
}

func TestCreateEditLock_IsFreeAtTheInitialVersion(t *testing.T) {
	scope := projectScope()
	lock := editlock.CreateEditLock(scope)

	if lock.Scope() != scope || lock.Held() || !lock.Holder().IsZero() || lock.Since() != (time.Time{}) {
		t.Errorf("new lock: %s", lock)
	}
	if lock.Version().Number() != 0 {
		t.Errorf("version %s, want 0", lock.Version())
	}
	if events := lock.PendingEvents(); len(events) != 0 {
		t.Errorf("new lock records %d events, want none", len(events))
	}
}

func TestAcquire_RecordsAcquiredAtTheNextVersion(t *testing.T) {
	lock := editlock.CreateEditLock(projectScope())
	alice := holderOf(t, editlock.NewKey(), "alice")

	acquired, err := lock.Acquire(alice, at)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if !acquired.Held() || acquired.Holder() != alice || !acquired.Since().Equal(at) || !acquired.LastSeen().Equal(at) {
		t.Errorf("acquired: %s", acquired)
	}
	events := acquired.PendingEvents()
	if len(events) != 1 || events[0].Type() != event.EventTypeEditLockAcquired {
		t.Fatalf("events %v, want one edit_lock_acquired", events)
	}
	ev, ok := events[0].(editlock.Event)
	if !ok || ev.Scope() != acquired.Scope() {
		t.Fatalf("event %T does not name the scope", events[0])
	}
	if state := ev.State(); !state.Held || state.Holder != alice || state.Version != 1 {
		t.Errorf("event state %+v, want alice at version 1", state)
	}
}

func TestAcquire_AnotherHolder_IsRefused(t *testing.T) {
	lock := editlock.CreateEditLock(projectScope())
	alice := holderOf(t, editlock.NewKey(), "alice")
	held, err := lock.Acquire(alice, at)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	bob := holderOf(t, editlock.NewKey(), "bob")
	_, err = held.Acquire(bob, at.Add(time.Minute))
	if !errors.Is(err, editlock.ErrEditLockHeld) {
		t.Fatalf("acquire by another: %v, want ErrEditLockHeld", err)
	}
	state, ok := editlock.StateOf(err)
	if !ok || state.Holder != alice {
		t.Errorf("the error carries %+v, want alice", state)
	}
}

func TestAcquire_SameHolder_IsIdempotent(t *testing.T) {
	lock := editlock.CreateEditLock(projectScope())
	alice := holderOf(t, editlock.NewKey(), "alice")
	held, err := lock.Acquire(alice, at)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	held = editlock.ReconstituteEditLock(held.Scope(), alice, at, at, held.Version().Next())

	again, err := held.Acquire(alice, at.Add(time.Hour))
	if err != nil {
		t.Fatalf("acquire again: %v", err)
	}
	if again != held {
		t.Errorf("acquire again changed the lock")
	}
	if len(again.PendingEvents()) != 0 {
		t.Errorf("acquire again records %v, want nothing", again.PendingEvents())
	}
}

func TestRelease_GivesTheDraftBack(t *testing.T) {
	lock := editlock.CreateEditLock(projectScope())
	alice := holderOf(t, editlock.NewKey(), "alice")
	held, _ := lock.Acquire(alice, at)

	released, err := held.Release(alice.Key(), at.Add(time.Minute))
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released.Held() || !released.Since().IsZero() || !released.LastSeen().IsZero() {
		t.Errorf("released: %s", released)
	}
	events := released.PendingEvents()
	if last := events[len(events)-1]; len(events) != 2 || last.Type() != event.EventTypeEditLockReleased {
		t.Fatalf("events %v, want an edit_lock_released after the acquisition", events)
	}
}

func TestRelease_FreeLock_IsIdempotent(t *testing.T) {
	lock := editlock.CreateEditLock(projectScope())
	released, err := lock.Release(editlock.NewKey(), at)
	if err != nil || released != lock {
		t.Errorf("release of a free lock: %v, %s", err, released)
	}
}

func TestRelease_AnotherHolder_IsRefused(t *testing.T) {
	lock := editlock.CreateEditLock(projectScope())
	alice := holderOf(t, editlock.NewKey(), "alice")
	held, _ := lock.Acquire(alice, at)

	if _, err := held.Release(editlock.NewKey(), at); !errors.Is(err, editlock.ErrEditLockNotHeld) {
		t.Errorf("release by another: %v, want ErrEditLockNotHeld", err)
	}
}

func TestTakeOver_TakesAHeldLockOver(t *testing.T) {
	lock := editlock.CreateEditLock(projectScope())
	alice := holderOf(t, editlock.NewKey(), "alice")
	held, _ := lock.Acquire(alice, at)
	bob := holderOf(t, editlock.NewKey(), "bob")

	taken, err := held.TakeOver(bob, held.Version(), at.Add(time.Minute))
	if err != nil {
		t.Fatalf("take over: %v", err)
	}
	if taken.Holder() != bob || !taken.Since().Equal(at.Add(time.Minute)) {
		t.Errorf("taken: %s", taken)
	}
	events := taken.PendingEvents()
	last := events[len(events)-1]
	if last.Type() != event.EventTypeEditLockTakenOver {
		t.Fatalf("events %v, want an edit_lock_taken_over", events)
	}
	if previous := last.(editlock.EditLockTakenOverEvent).Previous(); previous != alice {
		t.Errorf("previous holder %s, want alice", previous)
	}
}

func TestTakeOver_StaleVersion_IsRefused(t *testing.T) {
	lock := editlock.CreateEditLock(projectScope())
	alice := holderOf(t, editlock.NewKey(), "alice")
	held, _ := lock.Acquire(alice, at)

	stale := held.Version().Next()
	if _, err := held.TakeOver(holderOf(t, editlock.NewKey(), "bob"), stale, at); !errors.Is(err, editlock.ErrEditLockVersionStale) {
		t.Errorf("stale takeover: %v, want ErrEditLockVersionStale", err)
	}
}

func TestTakeOver_FreeLock_IsRefused(t *testing.T) {
	lock := editlock.CreateEditLock(projectScope())
	if _, err := lock.TakeOver(holderOf(t, editlock.NewKey(), "bob"), lock.Version(), at); !errors.Is(err, editlock.ErrEditLockFree) {
		t.Errorf("takeover of a free lock: %v, want ErrEditLockFree", err)
	}
}

func TestTakeOver_OwnLock_IsIdempotent(t *testing.T) {
	lock := editlock.CreateEditLock(projectScope())
	alice := holderOf(t, editlock.NewKey(), "alice")
	held, _ := lock.Acquire(alice, at)

	taken, err := held.TakeOver(alice, held.Version(), at.Add(time.Hour))
	if err != nil || taken != held {
		t.Errorf("own takeover: %v, %s", err, taken)
	}
}

func TestExpire_FreesTheLock(t *testing.T) {
	lock := editlock.CreateEditLock(projectScope())
	alice := holderOf(t, editlock.NewKey(), "alice")
	held, _ := lock.Acquire(alice, at)

	expired, err := held.Expire(at.Add(5 * time.Minute))
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	if expired.Held() {
		t.Errorf("expired: %s", expired)
	}
	events := expired.PendingEvents()
	if last := events[len(events)-1]; last.Type() != event.EventTypeEditLockExpired {
		t.Fatalf("events %v, want an edit_lock_expired", events)
	}
	if _, err := expired.Expire(at); !errors.Is(err, editlock.ErrEditLockFree) {
		t.Errorf("expire of a free lock: %v, want ErrEditLockFree", err)
	}
}

func TestSeen_RaisesLastSeenOnly(t *testing.T) {
	lock := editlock.CreateEditLock(projectScope())
	alice := holderOf(t, editlock.NewKey(), "alice")
	held, _ := lock.Acquire(alice, at)

	seen := held.Seen(at.Add(5 * time.Minute / 2))
	if !seen.LastSeen().Equal(at.Add(5*time.Minute/2)) || !seen.Since().Equal(at) {
		t.Errorf("seen: %s", seen)
	}
	if seen.Version() != held.Version() {
		t.Errorf("seen raised the version to %s", seen.Version())
	}
	if len(seen.PendingEvents()) != len(held.PendingEvents()) {
		t.Errorf("seen records %d more events than the lock it came from", len(seen.PendingEvents())-len(held.PendingEvents()))
	}
}

func TestHolder_EmptyName_IsRefused(t *testing.T) {
	if _, err := editlock.NewHolder(editlock.NewKey(), "  "); !errors.Is(err, editlock.ErrEditLockNameEmpty) {
		t.Errorf("empty name: %v, want ErrEditLockNameEmpty", err)
	}
}

func TestParseKey(t *testing.T) {
	key := editlock.NewKey()
	parsed, err := editlock.ParseKey(key.String())
	if err != nil || parsed != key {
		t.Errorf("parse %q: %v, %s", key, err, parsed)
	}
	if _, err := editlock.ParseKey(""); !errors.Is(err, editlock.ErrNoClient) {
		t.Errorf("empty key: %v, want ErrNoClient", err)
	}
	if _, err := editlock.ParseKey("not-a-uuid"); err == nil {
		t.Errorf("a malformed key was accepted")
	}
}

func TestScope_NamesOneDraft(t *testing.T) {
	projectID := id.NewID[project.Project]()
	libraryID := id.NewID[library.Library]()

	ofProject := editlock.ProjectScope(projectID)
	if !ofProject.IsProject() || ofProject.IsLibrary() || ofProject.ProjectID() != projectID || !ofProject.LibraryID().IsZero() {
		t.Errorf("project scope: %+v", ofProject)
	}
	ofLibrary := editlock.LibraryScope(libraryID)
	if !ofLibrary.IsLibrary() || ofLibrary.IsProject() || ofLibrary.LibraryID() != libraryID || !ofLibrary.ProjectID().IsZero() {
		t.Errorf("library scope: %+v", ofLibrary)
	}
}
