// Package editlock holds the EditLock: the marker that one holder — today a
// Client, later a User — currently holds one Draft for editing (ADR 0015).
// Without it the Draft is read-only.
//
// One EditLock belongs to one Scope: the Draft of a Project or of a Library.
// The row stays when the lock is free, so its Version grows monotonically
// across holders and a client can tell a stale observation from a fresh one.
//
// EditLock is immutable: every change returns a new EditLock that also records
// the change's event. A change keeps the Version; the repository raises it
// once when it saves the result. Refreshing LastSeen is no change: it raises
// nothing and records nothing.
package editlock

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

var (
	// ErrEditLockHeld is returned by a Check out of a lock another holder has.
	ErrEditLockHeld = errors.New("edit lock is held")
	// ErrEditLockFree is returned by a takeover of a free lock.
	ErrEditLockFree = errors.New("edit lock is free")
	// ErrEditLockNotHeld is returned when the caller is not the holder: by a
	// Check in of another holder's lock, and by a write of a Draft whose lock
	// is free or held by another client.
	ErrEditLockNotHeld = errors.New("edit lock is not held by the client")
	// ErrEditLockVersionStale is returned by a takeover that states a version
	// other than the stored one: the client observed an older state.
	ErrEditLockVersionStale = errors.New("edit lock version is stale")
	// ErrEditLockNameEmpty is returned by a holder with an empty name.
	ErrEditLockNameEmpty = errors.New("holder name is empty")
	// ErrEditLockExists is returned by Create when the Draft's lock row was
	// created concurrently: the caller rereads it.
	ErrEditLockExists = errors.New("edit lock row already exists")
	// ErrNoClient is returned by a gated write of a request that names no
	// client: the key of the tab travels in X-GrowSCADA-Client.
	ErrNoClient = errors.New("request names no client")
)

// Scope is the Draft an EditLock belongs to: a Project's or a Library's.
// Exactly one ID is set.
type Scope struct {
	projectID id.ID[project.Project]
	libraryID id.ID[library.Library]
}

// ProjectScope is the Scope of the Draft of the Project projectID.
func ProjectScope(projectID id.ID[project.Project]) Scope {
	return Scope{projectID: projectID}
}

// LibraryScope is the Scope of the Draft of the Library libraryID.
func LibraryScope(libraryID id.ID[library.Library]) Scope {
	return Scope{libraryID: libraryID}
}

// ProjectID is the Project of the Scope, a zero ID for a Library's.
func (s Scope) ProjectID() id.ID[project.Project] { return s.projectID }

// LibraryID is the Library of the Scope, a zero ID for a Project's.
func (s Scope) LibraryID() id.ID[library.Library] { return s.libraryID }

// IsProject reports whether the Scope is a Project's Draft.
func (s Scope) IsProject() bool { return !s.projectID.IsZero() }

// IsLibrary reports whether the Scope is a Library's Draft.
func (s Scope) IsLibrary() bool { return !s.libraryID.IsZero() }

// String implements [fmt.Stringer].
func (s Scope) String() string {
	if s.IsLibrary() {
		return "library " + s.libraryID.String()
	}
	return "project " + s.projectID.String()
}

// Key is the identity a client keeps for itself: a UUID the tab holds in
// sessionStorage, one per tab, surviving a reload. It is coordination, not
// authentication: there is no secret in it (ADR 0015).
type Key struct {
	uuid uuid.UUID
}

// NewKey returns a new random key.
func NewKey() Key { return Key{uuid: uuid.New()} }

// ParseKey reads the key a request presented. An empty value is ErrNoClient,
// anything else that is not a UUID is an error.
func ParseKey(raw string) (Key, error) {
	if raw == "" {
		return Key{}, ErrNoClient
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return Key{}, fmt.Errorf("client key: %q is not a valid UUID: %w", raw, err)
	}
	return Key{uuid: parsed}, nil
}

// UUID is the key's UUID.
func (k Key) UUID() uuid.UUID { return k.uuid }

// IsZero reports whether the key names no client.
func (k Key) IsZero() bool { return k.uuid == uuid.Nil }

// String implements [fmt.Stringer].
func (k Key) String() string { return k.uuid.String() }

// Holder is who holds the lock: a Client's key and the name the client
// declared. The name is not checked against anything: there are no Users yet
// (task 49).
type Holder struct {
	key  Key
	name string
}

// NewHolder is the holder of the key that declared aName.
// Returns ErrEditLockNameEmpty if the name is blank.
func NewHolder(key Key, aName string) (Holder, error) {
	name := strings.TrimSpace(aName)
	if name == "" {
		return Holder{}, ErrEditLockNameEmpty
	}
	return Holder{key: key, name: name}, nil
}

// Key is the holder's client key.
func (h Holder) Key() Key { return h.key }

// Name is the name the holder declared.
func (h Holder) Name() string { return h.name }

// IsZero reports whether the holder names nobody.
func (h Holder) IsZero() bool { return h.key.IsZero() }

// String implements [fmt.Stringer].
func (h Holder) String() string {
	return fmt.Sprintf("%s (%s)", h.name, h.key)
}

// heldError carries the lock state an ErrEditLockHeld or an ErrEditLockNotHeld
// is about, so the API can name the holder in the Problem Details.
type heldError struct {
	sentinel error
	state    Snapshot
}

func (e heldError) Error() string {
	if !e.state.Held {
		return fmt.Sprintf("%v: the lock is free", e.sentinel)
	}
	return fmt.Sprintf("%v: held by %s", e.sentinel, e.state.Holder)
}

func (e heldError) Is(target error) bool { return target == e.sentinel }

// HeldError is the error of a Check out of a lock another holder has; it
// carries that lock's state.
func HeldError(state Snapshot) error {
	return heldError{sentinel: ErrEditLockHeld, state: state}
}

// NotHeldError is the error of a write of a Draft (or a Check in) whose lock
// is free or held by another client; it carries that lock's state.
func NotHeldError(state Snapshot) error {
	return heldError{sentinel: ErrEditLockNotHeld, state: state}
}

// StateOf returns the lock state an error carries, if it carries one.
func StateOf(err error) (Snapshot, bool) {
	var held heldError
	if errors.As(err, &held) {
		return held.state, true
	}
	return Snapshot{}, false
}

// Snapshot is the state of an EditLock at a moment: what a reader and an
// event see. A free lock has no holder and no times.
type Snapshot struct {
	Held     bool
	Holder   Holder
	Since    time.Time
	LastSeen time.Time
	Version  int
}

// EditLock - aggregate of one Draft's editing: the holder, since when, when it
// was last seen to be alive, and the Version of the row.
type EditLock interface {
	// Scope is the Draft the lock belongs to; it never changes.
	Scope() Scope
	// Held reports whether a holder has the lock.
	Held() bool
	// Holder is the current holder, a zero Holder when free.
	Holder() Holder
	// Since is when the current holder took the lock; zero when free.
	Since() time.Time
	// LastSeen is when the holder was last seen alive; zero when free.
	LastSeen() time.Time
	Version() version.Version[EditLock]

	// Snapshot is the state as a value.
	Snapshot() Snapshot

	// Acquire takes the lock for holder and records an EditLockAcquiredEvent.
	// Returns HeldError if another client holds it; the same client's key
	// again is the lock as it is (an idempotent Check out).
	Acquire(holder Holder, at time.Time) (EditLock, error)

	// Release gives the lock back and records an EditLockReleasedEvent. A
	// free lock is the lock as it is (an idempotent Check in); another
	// client's key is NotHeldError.
	Release(key Key, at time.Time) (EditLock, error)

	// TakeOver takes a held lock for holder, stating the Version the client
	// observed, and records an EditLockTakenOverEvent. A stale expected
	// version is ErrEditLockVersionStale, a free lock ErrEditLockFree; the
	// holder's own key again is the lock as it is.
	TakeOver(holder Holder, expected version.Version[EditLock], at time.Time) (EditLock, error)

	// Expire frees a lock whose holder stopped being seen and records an
	// EditLockExpiredEvent; a free lock is ErrEditLockFree.
	Expire(at time.Time) (EditLock, error)

	// Seen is the lock with its LastSeen raised to at: no change of the
	// holder, so no event and no new Version. A free lock is as it is.
	Seen(at time.Time) EditLock

	event.Recorder
	fmt.Stringer
}

type editLockImpl struct {
	scope    Scope
	holder   Holder
	since    time.Time
	lastSeen time.Time
	version  version.Version[EditLock]
	pending  []event.Event
}

var _ EditLock = (*editLockImpl)(nil)

// CreateEditLock returns a new, not yet saved free lock of the Draft scope.
func CreateEditLock(scope Scope) EditLock {
	return &editLockImpl{scope: scope}
}

// ReconstituteEditLock rebuilds a lock as stored, at the given version; it
// records no event. For repositories.
func ReconstituteEditLock(scope Scope, holder Holder, since, lastSeen time.Time, aVersion version.Version[EditLock]) EditLock {
	return &editLockImpl{scope: scope, holder: holder, since: since, lastSeen: lastSeen, version: aVersion}
}

func (l *editLockImpl) Scope() Scope                       { return l.scope }
func (l *editLockImpl) Held() bool                         { return !l.holder.IsZero() }
func (l *editLockImpl) Holder() Holder                     { return l.holder }
func (l *editLockImpl) Since() time.Time                   { return l.since }
func (l *editLockImpl) LastSeen() time.Time                { return l.lastSeen }
func (l *editLockImpl) Version() version.Version[EditLock] { return l.version }

func (l *editLockImpl) Snapshot() Snapshot {
	return Snapshot{
		Held:     l.Held(),
		Holder:   l.holder,
		Since:    l.since,
		LastSeen: l.lastSeen,
		Version:  l.version.Number(),
	}
}

func (l *editLockImpl) Acquire(holder Holder, at time.Time) (EditLock, error) {
	if holder.IsZero() {
		return nil, ErrNoClient
	}
	if l.Held() {
		if l.holder.Key() == holder.Key() {
			return l, nil // the same client checks out again
		}
		return nil, HeldError(l.Snapshot())
	}
	acquired := *l
	acquired.holder = holder
	acquired.since = at
	acquired.lastSeen = at
	acquired.pending = append(slices.Clone(l.pending), NewEditLockAcquiredEvent(l.scope, l.snapshotOf(acquired)))
	return &acquired, nil
}

func (l *editLockImpl) Release(key Key, at time.Time) (EditLock, error) {
	if !l.Held() {
		return l, nil // nothing to give back
	}
	if l.holder.Key() != key {
		return nil, NotHeldError(l.Snapshot())
	}
	released := *l
	released.holder = Holder{}
	released.since = time.Time{}
	released.lastSeen = time.Time{}
	released.pending = append(slices.Clone(l.pending), NewEditLockReleasedEvent(l.scope, l.snapshotOf(released)))
	return &released, nil
}

func (l *editLockImpl) TakeOver(holder Holder, expected version.Version[EditLock], at time.Time) (EditLock, error) {
	if holder.IsZero() {
		return nil, ErrNoClient
	}
	if expected != l.version {
		return nil, fmt.Errorf("%w: expected version %s, current %s", ErrEditLockVersionStale, expected, l.version)
	}
	if !l.Held() {
		return nil, ErrEditLockFree
	}
	if l.holder.Key() == holder.Key() {
		return l, nil // its own lock taken over: nothing changes
	}
	previous := l.holder
	taken := *l
	taken.holder = holder
	taken.since = at
	taken.lastSeen = at
	taken.pending = append(slices.Clone(l.pending), NewEditLockTakenOverEvent(l.scope, l.snapshotOf(taken), previous))
	return &taken, nil
}

func (l *editLockImpl) Expire(at time.Time) (EditLock, error) {
	if !l.Held() {
		return nil, ErrEditLockFree
	}
	expired := *l
	expired.holder = Holder{}
	expired.since = time.Time{}
	expired.lastSeen = time.Time{}
	expired.pending = append(slices.Clone(l.pending), NewEditLockExpiredEvent(l.scope, l.snapshotOf(expired)))
	return &expired, nil
}

func (l *editLockImpl) Seen(at time.Time) EditLock {
	if !l.Held() {
		return l
	}
	seen := *l
	seen.lastSeen = at
	return &seen
}

// snapshotOf is the state of the copy next, the state its event announces.
func (l *editLockImpl) snapshotOf(next editLockImpl) Snapshot {
	return Snapshot{
		Held:     !next.holder.IsZero(),
		Holder:   next.holder,
		Since:    next.since,
		LastSeen: next.lastSeen,
		Version:  l.version.Next().Number(),
	}
}

func (l *editLockImpl) PendingEvents() []event.Event {
	return slices.Clone(l.pending)
}

// String implements [fmt.Stringer].
func (l *editLockImpl) String() string {
	if !l.Held() {
		return fmt.Sprintf("EditLock: free, %s, Version: %s", l.scope, l.version)
	}
	return fmt.Sprintf("EditLock: held by %s since %s, %s, Version: %s", l.holder, l.since.Format(time.RFC3339), l.scope, l.version)
}
