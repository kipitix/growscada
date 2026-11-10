package library

import (
	"fmt"
	"slices"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// LibraryRelease - an immutable snapshot of a Library's Draft (ADR 0007): a
// numbered copy of all its WidgetTypes under their own IDs, with the time it
// was made, an optional note and the Author who published it. It is made from
// a Commit of the Library's Draft (task 72_5; ADR 0021) or, for a Release
// older than Commits, from the Draft directly (OriginLegacy). A release is
// never changed nor removed, so it has no Version; its number is its identity
// within the Library. Its content never changes: only its note is metadata
// and changes on its own (WithNote, library_release_note_updated).
type LibraryRelease interface {
	LibraryID() id.ID[Library]
	Number() ReleaseNumber
	// CommitNumber is the number of the Commit the Release published; nil for
	// a legacy Release, which names none.
	CommitNumber() *int
	// Author is the Engineer who published the Release.
	Author() Author
	// Origin tells whether the Release came from a Commit or predates them.
	Origin() Origin
	Note() string
	CreatedAt() time.Time
	// WidgetTypes returns the release's WidgetTypes, in the order of the
	// Commit they were copied from.
	WidgetTypes() []ReleasedWidgetType
	// FindWidgetType returns the release's copy of the WidgetType with the
	// given ID; false if the release does not hold it.
	FindWidgetType(id.ID[WidgetType]) (ReleasedWidgetType, bool)

	// WithNote returns the Release with its note changed, recording
	// library_release_note_updated. The content is untouched.
	WithNote(aNote string) LibraryRelease

	event.Recorder
	fmt.Stringer
}

type libraryReleaseImpl struct {
	libraryID    id.ID[Library]
	number       ReleaseNumber
	commitNumber int
	author       Author
	origin       Origin
	note         string
	createdAt    time.Time
	widgetTypes  []ReleasedWidgetType
	pending      []event.Event
}

var _ LibraryRelease = (*libraryReleaseImpl)(nil)

// CreateRelease publishes commitNumber of lib as Release number with the
// copies someWidgetTypes of the Commit's content, the note aNote and the
// Author anAuthor, and records a LibraryReleasedEvent.
// Returns an error if the Commit number is not positive or a WidgetType is of
// another Library.
func CreateRelease(lib Library, number ReleaseNumber, commitNumber int, aNote string, anAuthor Author,
	at time.Time, someWidgetTypes []ReleasedWidgetType) (LibraryRelease, error) {
	created, err := newLibraryRelease(lib.ID(), number, commitNumber, anAuthor, OriginCommit, aNote, at, someWidgetTypes)
	if err != nil {
		return nil, err
	}
	impl := created.(*libraryReleaseImpl)
	impl.pending = []event.Event{NewLibraryReleasedEvent(lib.ID(), number)}
	return impl, nil
}

// ReconstituteRelease rebuilds a Library Release as stored; it records no
// event. For repositories.
// Returns an error if the origin is unknown, a Release of a Commit names no
// Commit (or a legacy one does), or a WidgetType is of another Library.
func ReconstituteRelease(aLibraryID id.ID[Library], aNumber ReleaseNumber, aCommitNumber *int, anAuthor Author,
	anOrigin Origin, aNote string, at time.Time, someWidgetTypes []ReleasedWidgetType) (LibraryRelease, error) {
	commitNumber := 0
	if aCommitNumber != nil {
		commitNumber = *aCommitNumber
	}
	return newLibraryRelease(aLibraryID, aNumber, commitNumber, anAuthor, anOrigin, aNote, at, someWidgetTypes)
}

// newLibraryRelease builds a Release and checks its invariants.
func newLibraryRelease(aLibraryID id.ID[Library], aNumber ReleaseNumber, aCommitNumber int, anAuthor Author,
	anOrigin Origin, aNote string, at time.Time, someWidgetTypes []ReleasedWidgetType) (LibraryRelease, error) {
	if !aNumber.IsValid() {
		return nil, fmt.Errorf("cannot create release: invalid number")
	}
	if !anOrigin.IsValid() {
		return nil, fmt.Errorf("cannot create release: invalid origin")
	}
	if anOrigin.IsCommit() && aCommitNumber < 1 {
		return nil, fmt.Errorf("cannot create release: a release of a commit names its number")
	}
	if !anOrigin.IsCommit() && aCommitNumber != 0 {
		return nil, fmt.Errorf("cannot create release: a legacy release names no commit")
	}
	for _, wt := range someWidgetTypes {
		if wt.LibraryID() != aLibraryID {
			return nil, fmt.Errorf("cannot release library %s: widget type %s is of library %s",
				aLibraryID, wt.ID(), wt.LibraryID())
		}
	}
	return &libraryReleaseImpl{
		libraryID:    aLibraryID,
		number:       aNumber,
		commitNumber: aCommitNumber,
		author:       anAuthor,
		origin:       anOrigin,
		note:         aNote,
		createdAt:    at,
		widgetTypes:  slices.Clone(someWidgetTypes),
	}, nil
}

func (r *libraryReleaseImpl) LibraryID() id.ID[Library] { return r.libraryID }
func (r *libraryReleaseImpl) Number() ReleaseNumber     { return r.number }

func (r *libraryReleaseImpl) CommitNumber() *int {
	if r.commitNumber == 0 {
		return nil
	}
	number := r.commitNumber
	return &number
}

func (r *libraryReleaseImpl) Author() Author       { return r.author }
func (r *libraryReleaseImpl) Origin() Origin       { return r.origin }
func (r *libraryReleaseImpl) Note() string         { return r.note }
func (r *libraryReleaseImpl) CreatedAt() time.Time { return r.createdAt }

func (r *libraryReleaseImpl) WidgetTypes() []ReleasedWidgetType {
	return slices.Clone(r.widgetTypes)
}

func (r *libraryReleaseImpl) FindWidgetType(typeID id.ID[WidgetType]) (ReleasedWidgetType, bool) {
	i := slices.IndexFunc(r.widgetTypes, func(wt ReleasedWidgetType) bool { return wt.ID() == typeID })
	if i < 0 {
		return ReleasedWidgetType{}, false
	}
	return r.widgetTypes[i], true
}

func (r *libraryReleaseImpl) WithNote(aNote string) LibraryRelease {
	copied := *r
	copied.note = aNote
	copied.pending = []event.Event{NewLibraryReleaseNoteUpdatedEvent(r.libraryID, r.number)}
	return &copied
}

func (r *libraryReleaseImpl) PendingEvents() []event.Event {
	return slices.Clone(r.pending)
}

// String implements [fmt.Stringer].
func (r *libraryReleaseImpl) String() string {
	return fmt.Sprintf("LibraryRelease: %s of %s, %d widget types", r.number, r.libraryID, len(r.widgetTypes))
}

// ReleaseState is what a Library's Commits and Releases tell of it.
type ReleaseState struct {
	// Latest is the number of the latest Release; 0 for none.
	Latest ReleaseNumber
	// LatestCommit is the number of the Draft's last Commit; 0 when it was
	// never saved.
	LatestCommit int
	// LatestReleaseCommit is the Commit the latest Release published; 0 for no
	// Release and for a legacy one.
	LatestReleaseCommit int
	// UnsavedChanges reports whether the Draft moved since its content last
	// equalled its last Commit: the Draft's own flag (ADR 0016, amendment
	// 2026-11-10).
	UnsavedChanges bool
	// UnpublishedChanges reports whether the last Commit is newer than the one
	// the latest Release published.
	UnpublishedChanges bool
}

// ReleaseStateOf returns the ReleaseState of a Library whose latest Release is
// latest (nil for none) and whose last Commit, number lastCommitNumber, is
// published or not; unsavedChanges is the Draft's own unsaved-changes flag.
func ReleaseStateOf(latest LibraryRelease, lastCommitNumber int, unsavedChanges bool) ReleaseState {
	state := ReleaseState{
		LatestCommit:   lastCommitNumber,
		UnsavedChanges: unsavedChanges,
	}
	if latest != nil {
		state.Latest = latest.Number()
		if commitNumber := latest.CommitNumber(); commitNumber != nil {
			state.LatestReleaseCommit = *commitNumber
		}
	}
	state.UnpublishedChanges = lastCommitNumber > state.LatestReleaseCommit
	return state
}
