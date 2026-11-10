package draft

import (
	"fmt"
	"slices"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// Commit - an immutable snapshot of a whole Draft (task 72_2; ADR 0021): the
// canonical content of a Project's Draft or the WidgetTypes of a Library's,
// numbered 1, 2, 3… within the Draft, with a title, the Author who saved it,
// the time it was made and the Version of the Draft it fixed. It is an entity
// of the Draft, like DraftChange: a Save writes no DraftChange and raises no
// Version, because it changes no content.
//
// Commit is immutable: renaming its title returns a new Commit and records
// commit_title_updated; its content never changes. A Save that finds the
// Draft at its Clean Version is refused (ErrNothingToSave).
type Commit interface {
	ID() id.ID[Commit]
	// Scope is the Draft the Commit belongs to.
	Scope() Scope
	// Number is the Commit's number within its Draft, from 1.
	Number() int
	Title() CommitTitle
	Author() Author
	CreatedAt() time.Time
	// DraftVersion is the Version of the Draft the Save fixed.
	DraftVersion() version.Version[Draft]
	// Content is the snapshot: a Project's ProjectFile or a Library's
	// WidgetTypes.
	Content() CommitContent

	// WithTitle returns the Commit with its title changed, recording
	// commit_title_updated. The content is untouched.
	WithTitle(aTitle CommitTitle) Commit

	event.Recorder
	fmt.Stringer
}

type commitImpl struct {
	id           id.ID[Commit]
	scope        Scope
	number       int
	title        CommitTitle
	author       Author
	createdAt    time.Time
	draftVersion version.Version[Draft]
	content      CommitContent
	pending      []event.Event
}

var _ Commit = (*commitImpl)(nil)

// NewCommit returns the Commit of scope at anID, numbered number, saving the
// Draft at draftVersion with the given content, and records
// commit_created. Returns an error if the number is not positive or the
// content does not belong to the scope.
func NewCommit(
	scope Scope,
	anID id.ID[Commit],
	number int,
	aTitle CommitTitle,
	anAuthor Author,
	at time.Time,
	draftVersion version.Version[Draft],
	content CommitContent,
) (Commit, error) {
	commit := &commitImpl{
		id:           anID,
		scope:        scope,
		number:       number,
		title:        aTitle,
		author:       anAuthor,
		createdAt:    at,
		draftVersion: draftVersion,
		content:      content,
	}
	if err := commit.checkScope(); err != nil {
		return nil, err
	}
	commit.pending = []event.Event{NewCommitCreatedEvent(scope, number)}
	return commit, nil
}

// ReconstituteCommit rebuilds a Commit as stored; it records no event. For
// repositories.
func ReconstituteCommit(
	scope Scope,
	anID id.ID[Commit],
	number int,
	aTitle CommitTitle,
	anAuthor Author,
	at time.Time,
	draftVersion version.Version[Draft],
	content CommitContent,
) (Commit, error) {
	commit := &commitImpl{
		id:           anID,
		scope:        scope,
		number:       number,
		title:        aTitle,
		author:       anAuthor,
		createdAt:    at,
		draftVersion: draftVersion,
		content:      content,
	}
	if err := commit.checkScope(); err != nil {
		return nil, err
	}
	return commit, nil
}

// checkScope refuses a number that is no number and content of the other
// kind of scope: a Library's Commit holds WidgetTypes, a Project's a
// ProjectFile.
func (c *commitImpl) checkScope() error {
	if c.number < 1 {
		return fmt.Errorf("commit number must be positive, got %d", c.number)
	}
	switch {
	case c.scope.IsProject() && !c.content.IsProject():
		return fmt.Errorf("cannot commit project %s: its snapshot is not a project file", c.scope)
	case c.scope.IsLibrary() && !c.content.IsLibrary():
		return fmt.Errorf("cannot commit %s: its snapshot is not widget types", c.scope)
	}
	return nil
}

func (c *commitImpl) ID() id.ID[Commit]                    { return c.id }
func (c *commitImpl) Scope() Scope                         { return c.scope }
func (c *commitImpl) Number() int                          { return c.number }
func (c *commitImpl) Title() CommitTitle                   { return c.title }
func (c *commitImpl) Author() Author                       { return c.author }
func (c *commitImpl) CreatedAt() time.Time                 { return c.createdAt }
func (c *commitImpl) DraftVersion() version.Version[Draft] { return c.draftVersion }
func (c *commitImpl) Content() CommitContent               { return c.content }
func (c *commitImpl) PendingEvents() []event.Event         { return slices.Clone(c.pending) }

func (c *commitImpl) WithTitle(aTitle CommitTitle) Commit {
	copied := *c
	copied.title = aTitle
	copied.pending = []event.Event{NewCommitTitleUpdatedEvent(c.scope, c.number)}
	return &copied
}

// String implements [fmt.Stringer].
func (c *commitImpl) String() string {
	return fmt.Sprintf("Commit: %d %q by %s", c.number, c.title, c.author)
}
