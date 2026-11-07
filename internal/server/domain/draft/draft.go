// Package draft holds the Draft: the working state of one Project or one
// Library as it is being edited, and the unit of consistency of its content
// (ADR 0016). A Project's Draft holds its Scenes with their Widgets, its Tag
// definitions, its Devices, its Environments with their pairs and its Library
// Pins; a Library's holds its WidgetTypes. The entities keep their own
// packages and tables: the Draft composes them and carries the one Version of
// the whole content and the position of its DraftChange stack (ADR 0017).
//
// A Draft is edited without a lock: the Version of its content is the only
// concurrency token (ADR 0022).
package draft

import (
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// Draft is one Draft's identity and metadata: its scope, the Version of its
// content and the position of its DraftChange stack. The content itself is
// loaded and written by the Draft's repository, which opens one transaction
// over this row (ADR 0016).
//
// Draft is immutable: every change returns a new Draft. A content change
// keeps the Version; the repository raises it once when it stores the change.
type Draft interface {
	ID() id.ID[Draft]
	// Scope is the Project or the Library the Draft belongs to; it never
	// changes.
	Scope() Scope
	// Version is the Version of the Draft's content: the token every write
	// of it states and raises.
	Version() version.Version[Draft]
	// StackPosition is the number of DraftChanges applied; the next undo
	// steps back from it.
	StackPosition() int

	// WithVersion returns the Draft at the given Version: what the repository
	// stores and answers after a write.
	WithVersion(version.Version[Draft]) Draft
	// WithStackPosition returns the Draft at the given stack position.
	WithStackPosition(position int) Draft

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
}

type draftImpl struct {
	id            id.ID[Draft]
	scope         Scope
	version       version.Version[Draft]
	stackPosition int
}

var _ Draft = (*draftImpl)(nil)

// NewDraft returns the Draft of scope at anID, at the given Version and stack
// position.
func NewDraft(anID id.ID[Draft], scope Scope, aVersion version.Version[Draft], stackPosition int) Draft {
	return &draftImpl{id: anID, scope: scope, version: aVersion, stackPosition: stackPosition}
}

func (d *draftImpl) ID() id.ID[Draft]                { return d.id }
func (d *draftImpl) Scope() Scope                    { return d.scope }
func (d *draftImpl) Version() version.Version[Draft] { return d.version }
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
