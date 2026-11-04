package editlock

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
)

// Event is a change of an EditLock. It names the Scope it belongs to, so a
// Project's events go to that Project's stream and a Library's to the
// server's.
type Event interface {
	event.Event
	// Scope is the Draft whose lock changed.
	Scope() Scope
	// State is the lock as the change leaves it.
	State() Snapshot
}

// EditLockAcquiredEvent - a free lock was taken for editing (Check out).
type EditLockAcquiredEvent interface {
	Event
}

// EditLockReleasedEvent - the holder gave the lock back (Check in).
type EditLockReleasedEvent interface {
	Event
}

// EditLockTakenOverEvent - another client took a held lock over.
type EditLockTakenOverEvent interface {
	Event
	// Previous is the holder the takeover displaced.
	Previous() Holder
}

// EditLockExpiredEvent - the holder stopped being seen and the lock was
// freed by the reaper.
type EditLockExpiredEvent interface {
	Event
}

type editLockEventImpl struct {
	event.Base
	scope Scope
	state Snapshot
}

var _ Event = (*editLockEventImpl)(nil)

// NO FABRIC METHOD

func (e editLockEventImpl) Scope() Scope    { return e.scope }
func (e editLockEventImpl) State() Snapshot { return e.state }

type editLockAcquiredEventImpl struct{ editLockEventImpl }

var _ EditLockAcquiredEvent = (*editLockAcquiredEventImpl)(nil)

// NewEditLockAcquiredEvent records a Check out of the lock of scope, which
// state describes.
func NewEditLockAcquiredEvent(scope Scope, state Snapshot, opts ...event.EventOption) EditLockAcquiredEvent {
	return &editLockAcquiredEventImpl{editLockEventImpl{
		Base:  event.NewBase(event.EventTypeEditLockAcquired, opts...),
		scope: scope,
		state: state,
	}}
}

type editLockReleasedEventImpl struct{ editLockEventImpl }

var _ EditLockReleasedEvent = (*editLockReleasedEventImpl)(nil)

// NewEditLockReleasedEvent records a Check in of the lock of scope, which
// state describes.
func NewEditLockReleasedEvent(scope Scope, state Snapshot, opts ...event.EventOption) EditLockReleasedEvent {
	return &editLockReleasedEventImpl{editLockEventImpl{
		Base:  event.NewBase(event.EventTypeEditLockReleased, opts...),
		scope: scope,
		state: state,
	}}
}

type editLockTakenOverEventImpl struct {
	editLockEventImpl
	previous Holder
}

var _ EditLockTakenOverEvent = (*editLockTakenOverEventImpl)(nil)

// NewEditLockTakenOverEvent records a takeover of the lock of scope, which
// state describes, from the holder previous.
func NewEditLockTakenOverEvent(scope Scope, state Snapshot, previous Holder, opts ...event.EventOption) EditLockTakenOverEvent {
	return &editLockTakenOverEventImpl{
		editLockEventImpl: editLockEventImpl{
			Base:  event.NewBase(event.EventTypeEditLockTakenOver, opts...),
			scope: scope,
			state: state,
		},
		previous: previous,
	}
}

func (e editLockTakenOverEventImpl) Previous() Holder { return e.previous }

type editLockExpiredEventImpl struct{ editLockEventImpl }

var _ EditLockExpiredEvent = (*editLockExpiredEventImpl)(nil)

// NewEditLockExpiredEvent records the reaper freeing the lock of scope, which
// state describes.
func NewEditLockExpiredEvent(scope Scope, state Snapshot, opts ...event.EventOption) EditLockExpiredEvent {
	return &editLockExpiredEventImpl{editLockEventImpl{
		Base:  event.NewBase(event.EventTypeEditLockExpired, opts...),
		scope: scope,
		state: state,
	}}
}
