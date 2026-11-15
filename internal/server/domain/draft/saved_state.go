package draft

import (
	"fmt"
	"slices"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// SavedState - a saved state of a whole Draft (task 72_2; ADR 0021,
// ADR 0026): the canonical content of a Project's Draft or the WidgetTypes of
// a Library's, numbered 1, 2, 3… within the Draft, with a title, the Author
// who saved it, the time it was made and the Version of the Draft it fixed. It
// is an entity of the Draft, like DraftChange: a Save writes no DraftChange
// and raises no Version, because it changes no content.
//
// A saved state is immutable: renaming its title returns a new one and records
// saved_state_title_updated; its content never changes. A Save that finds the Draft
// at its Clean Version is refused (ErrNothingToSave).
type SavedState interface {
	ID() id.ID[SavedState]
	// Scope is the Draft the saved state belongs to.
	Scope() Scope
	// Number is the saved state's number within its Draft, from 1.
	Number() int
	Title() SavedStateTitle
	Author() Author
	CreatedAt() time.Time
	// DraftVersion is the Version of the Draft the Save fixed.
	DraftVersion() version.Version[Draft]
	// Content is the snapshot: a Project's ProjectFile or a Library's
	// WidgetTypes.
	Content() SavedStateContent

	// WithTitle returns the saved state with its title changed, recording
	// saved_state_title_updated. The content is untouched.
	WithTitle(aTitle SavedStateTitle) SavedState

	event.Recorder
	fmt.Stringer
}

type savedStateImpl struct {
	id           id.ID[SavedState]
	scope        Scope
	number       int
	title        SavedStateTitle
	author       Author
	createdAt    time.Time
	draftVersion version.Version[Draft]
	content      SavedStateContent
	pending      []event.Event
}

var _ SavedState = (*savedStateImpl)(nil)

// NewSavedState returns the saved state of scope at anID, numbered number,
// saving the Draft at draftVersion with the given content, and records
// saved_state_created. Returns an error if the number is not positive or the
// content does not belong to the scope.
func NewSavedState(
	scope Scope,
	anID id.ID[SavedState],
	number int,
	aTitle SavedStateTitle,
	anAuthor Author,
	at time.Time,
	draftVersion version.Version[Draft],
	content SavedStateContent,
) (SavedState, error) {
	savedState := &savedStateImpl{
		id:           anID,
		scope:        scope,
		number:       number,
		title:        aTitle,
		author:       anAuthor,
		createdAt:    at,
		draftVersion: draftVersion,
		content:      content,
	}
	if err := savedState.checkScope(); err != nil {
		return nil, err
	}
	savedState.pending = []event.Event{NewSavedStateCreatedEvent(scope, number)}
	return savedState, nil
}

// ReconstituteSavedState rebuilds a saved state as stored; it records no event.
// For repositories.
func ReconstituteSavedState(
	scope Scope,
	anID id.ID[SavedState],
	number int,
	aTitle SavedStateTitle,
	anAuthor Author,
	at time.Time,
	draftVersion version.Version[Draft],
	content SavedStateContent,
) (SavedState, error) {
	savedState := &savedStateImpl{
		id:           anID,
		scope:        scope,
		number:       number,
		title:        aTitle,
		author:       anAuthor,
		createdAt:    at,
		draftVersion: draftVersion,
		content:      content,
	}
	if err := savedState.checkScope(); err != nil {
		return nil, err
	}
	return savedState, nil
}

// checkScope refuses a number that is no number and content of the other
// kind of scope: a Library's saved state holds WidgetTypes, a Project's a
// ProjectFile.
func (c *savedStateImpl) checkScope() error {
	if c.number < 1 {
		return fmt.Errorf("saved state number must be positive, got %d", c.number)
	}
	switch {
	case c.scope.IsProject() && !c.content.IsProject():
		return fmt.Errorf("cannot make a saved state of project %s: its snapshot is not a project file", c.scope)
	case c.scope.IsLibrary() && !c.content.IsLibrary():
		return fmt.Errorf("cannot make a saved state of %s: its snapshot is not widget types", c.scope)
	}
	return nil
}

func (c *savedStateImpl) ID() id.ID[SavedState]                { return c.id }
func (c *savedStateImpl) Scope() Scope                         { return c.scope }
func (c *savedStateImpl) Number() int                          { return c.number }
func (c *savedStateImpl) Title() SavedStateTitle               { return c.title }
func (c *savedStateImpl) Author() Author                       { return c.author }
func (c *savedStateImpl) CreatedAt() time.Time                 { return c.createdAt }
func (c *savedStateImpl) DraftVersion() version.Version[Draft] { return c.draftVersion }
func (c *savedStateImpl) Content() SavedStateContent           { return c.content }
func (c *savedStateImpl) PendingEvents() []event.Event         { return slices.Clone(c.pending) }

func (c *savedStateImpl) WithTitle(aTitle SavedStateTitle) SavedState {
	copied := *c
	copied.title = aTitle
	copied.pending = []event.Event{NewSavedStateTitleUpdatedEvent(c.scope, c.number)}
	return &copied
}

// String implements [fmt.Stringer].
func (c *savedStateImpl) String() string {
	return fmt.Sprintf("Saved state: %d %q by %s", c.number, c.title, c.author)
}
