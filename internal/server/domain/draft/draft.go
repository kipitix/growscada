// Package draft holds the Draft: the working state of one Project or one
// Library as it is being edited, and the unit of consistency of its content
// (ADR 0016). A Project's Draft holds its Scenes with their Widgets, its Tag
// definitions, its Devices, its Environments with their pairs and its Library
// Pins; a Library's holds its WidgetTypes. The entities keep their own
// packages and tables: the Draft composes them and carries the one Version of
// the whole content, the EditLock of the Draft and the position of its
// DraftChange stack (ADR 0017).
package draft

import (
	"slices"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// Draft is one Draft's identity and metadata: its scope, the Version of its
// content, its EditLock and the position of its DraftChange stack. The
// content itself is loaded and written by the Draft's repository, which opens
// one transaction over this row (ADR 0016).
//
// Draft is immutable: every change returns a new Draft. A content change
// keeps the Version; the repository raises it once when it stores the change.
// The EditLock's own Version grows only when the holder changes.
type Draft interface {
	ID() id.ID[Draft]
	// Scope is the Project or the Library the Draft belongs to; it never
	// changes.
	Scope() editlock.Scope
	// Version is the Version of the Draft's content: the token every write
	// of it states and raises.
	Version() version.Version[Draft]
	// Lock is the EditLock of the Draft, with its own Version.
	Lock() editlock.EditLock
	// StackPosition is the number of DraftChanges applied; the next undo
	// steps back from it.
	StackPosition() int

	// WithVersion returns the Draft at the given Version: what the repository
	// stores and answers after a write.
	WithVersion(version.Version[Draft]) Draft
	// WithStackPosition returns the Draft at the given stack position.
	WithStackPosition(position int) Draft
	// WithLock returns the Draft with lock in place of its EditLock.
	WithLock(lock editlock.EditLock) Draft

	// Undo returns the Draft after undoing change, the DraftChange at the
	// stack's position: its Version is raised by one and its position steps
	// back (ADR 0017). Returns ErrNothingToUndo for a change that is not the
	// one at the position. The rows and the events are the Session's.
	Undo(change DraftChange) (Draft, error)
	// Redo returns the Draft after redoing change, the DraftChange after the
	// stack's position: its Version is raised by one and its position steps
	// forward. Returns ErrNothingToRedo for a change that is not the one after
	// the position.
	Redo(change DraftChange) (Draft, error)

	// PendingEvents are the events of the EditLock changes recorded since the
	// Draft was read; the content's events come from its entities.
	PendingEvents() []event.Event
}

type draftImpl struct {
	id            id.ID[Draft]
	scope         editlock.Scope
	version       version.Version[Draft]
	lock          editlock.EditLock
	stackPosition int
}

var _ Draft = (*draftImpl)(nil)

// NewDraft returns the Draft of scope at anID, at the given Version and stack
// position, with lock as its EditLock.
func NewDraft(anID id.ID[Draft], scope editlock.Scope, aVersion version.Version[Draft], stackPosition int, lock editlock.EditLock) Draft {
	return &draftImpl{id: anID, scope: scope, version: aVersion, lock: lock, stackPosition: stackPosition}
}

func (d *draftImpl) ID() id.ID[Draft]                { return d.id }
func (d *draftImpl) Scope() editlock.Scope           { return d.scope }
func (d *draftImpl) Version() version.Version[Draft] { return d.version }
func (d *draftImpl) Lock() editlock.EditLock         { return d.lock }
func (d *draftImpl) StackPosition() int              { return d.stackPosition }
func (d *draftImpl) WithVersion(v version.Version[Draft]) Draft {
	copied := *d
	copied.version = v
	return &copied
}

func (d *draftImpl) WithStackPosition(position int) Draft {
	copied := *d
	copied.stackPosition = position
	return &copied
}

func (d *draftImpl) WithLock(lock editlock.EditLock) Draft {
	copied := *d
	copied.lock = lock
	return &copied
}

func (d *draftImpl) PendingEvents() []event.Event {
	if d.lock == nil {
		return nil
	}
	return slices.Clone(d.lock.PendingEvents())
}

// Clock is the time source of a Draft write, for tests that must let a lock
// go stale without waiting.
type Clock func() time.Time
