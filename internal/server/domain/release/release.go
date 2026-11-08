package release

import (
	"fmt"
	"slices"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// Release - the publication of a Commit (task 72_3; ADR 0021): an immutable,
// numbered snapshot with a note, the Author who published it, the time, the
// commit it came from and a copy of that Commit's content. It is an aggregate
// of the Engineering node's, outside the Draft: it copies the content, so it
// outlives the Commit, the Draft and the Library the content came from, and it
// changes nothing of them.
//
// A Release is never removed and its content never changes. Only its note is
// metadata and changes on its own (WithNote, release_note_updated).
type Release interface {
	ID() id.ID[Release]
	// Scope is the Project (or, from task 72_5, the Library) the Release
	// belongs to.
	Scope() draft.Scope
	// Number is the Release's number within its scope, from 1.
	Number() Number
	// CommitNumber is the number of the Commit published; 0 for a Release
	// made from a project file (task 72_4).
	CommitNumber() int
	// CommitTitle is the title of the Commit at the moment of publication; it
	// is a snapshot and never changes.
	CommitTitle() draft.CommitTitle
	Note() Note
	Author() draft.Author
	CreatedAt() time.Time
	// Origin tells where the content came from: a Commit or a project file.
	Origin() Origin
	// Content is the copy of what was published: a ProjectFile or a Library's
	// WidgetTypes.
	Content() draft.CommitContent

	// WithNote returns the Release with its note changed, recording
	// release_note_updated. The content is untouched.
	WithNote(aNote Note) Release

	event.Recorder
	fmt.Stringer
}

type releaseImpl struct {
	id           id.ID[Release]
	scope        draft.Scope
	number       Number
	commitNumber int
	commitTitle  draft.CommitTitle
	note         Note
	author       draft.Author
	createdAt    time.Time
	origin       Origin
	content      draft.CommitContent
	pending      []event.Event
}

var _ Release = (*releaseImpl)(nil)

// CreateRelease publishes the Commit commit of scope as Release number, with
// the note and the Author who made it, and records release_created. The Commit
// must belong to the same scope; its title and content are copied, so the
// Release does not depend on the Commit afterwards.
// Returns an error if the number is not positive or the Commit is of another
// scope.
func CreateRelease(scope draft.Scope, commit draft.Commit, number Number, aNote Note, anAuthor draft.Author, at time.Time) (Release, error) {
	if commit.Scope() != scope {
		return nil, fmt.Errorf("cannot release %s from a commit of %s", scope, commit.Scope())
	}
	created, err := ReconstituteRelease(scope, id.NewID[Release](), number,
		commit.Number(), commit.Title(), aNote, anAuthor, at, OriginCommit, commit.Content())
	if err != nil {
		return nil, err
	}
	impl := created.(*releaseImpl)
	impl.pending = []event.Event{NewReleaseCreatedEvent(scope, number)}
	return impl, nil
}

// ReconstituteRelease rebuilds a Release as stored; it records no event. For
// repositories.
func ReconstituteRelease(
	scope draft.Scope,
	anID id.ID[Release],
	number Number,
	commitNumber int,
	commitTitle draft.CommitTitle,
	aNote Note,
	anAuthor draft.Author,
	at time.Time,
	origin Origin,
	content draft.CommitContent,
) (Release, error) {
	if !number.IsValid() {
		return nil, fmt.Errorf("cannot create release: invalid number")
	}
	if origin == OriginCommit && commitNumber < 1 {
		return nil, fmt.Errorf("cannot create release: a release of a commit names its number")
	}
	if !origin.IsValid() {
		return nil, fmt.Errorf("cannot create release: invalid origin")
	}
	switch {
	case scope.IsProject() && !content.IsProject():
		return nil, fmt.Errorf("cannot release project %s: its content is not a project file", scope)
	case scope.IsLibrary() && !content.IsLibrary():
		return nil, fmt.Errorf("cannot release %s: its content is not widget types", scope)
	}
	return &releaseImpl{
		id:           anID,
		scope:        scope,
		number:       number,
		commitNumber: commitNumber,
		commitTitle:  commitTitle,
		note:         aNote,
		author:       anAuthor,
		createdAt:    at,
		origin:       origin,
		content:      content,
	}, nil
}

func (r *releaseImpl) ID() id.ID[Release]             { return r.id }
func (r *releaseImpl) Scope() draft.Scope             { return r.scope }
func (r *releaseImpl) Number() Number                 { return r.number }
func (r *releaseImpl) CommitNumber() int              { return r.commitNumber }
func (r *releaseImpl) CommitTitle() draft.CommitTitle { return r.commitTitle }
func (r *releaseImpl) Note() Note                     { return r.note }
func (r *releaseImpl) Author() draft.Author           { return r.author }
func (r *releaseImpl) CreatedAt() time.Time           { return r.createdAt }
func (r *releaseImpl) Origin() Origin                 { return r.origin }
func (r *releaseImpl) Content() draft.CommitContent   { return r.content }
func (r *releaseImpl) PendingEvents() []event.Event   { return slices.Clone(r.pending) }

func (r *releaseImpl) WithNote(aNote Note) Release {
	copied := *r
	copied.note = aNote
	copied.pending = []event.Event{NewReleaseNoteUpdatedEvent(r.scope, r.number)}
	return &copied
}

// String implements [fmt.Stringer].
func (r *releaseImpl) String() string {
	return fmt.Sprintf("Release: %s of %s, %d widget types", r.number, r.scope, r.contentWidgetCount())
}

// contentWidgetCount is the number of WidgetTypes a Library's release holds;
// 0 for a Project's.
func (r *releaseImpl) contentWidgetCount() int {
	types, _ := r.content.WidgetTypes()
	return len(types)
}
