package library

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// ErrNothingToRelease is returned by CreateRelease when the Draft is the
// same as the latest Library Release: a release would hold nothing new.
var ErrNothingToRelease = errors.New("library draft has not changed since the latest release")

// LibraryRelease - an immutable snapshot of a Library's Draft (ADR 0007):
// copies of all its WidgetTypes under their own IDs, numbered 1, 2, 3…
// within the Library, with the time it was made and an optional note. A
// release is never changed nor removed, so it has no Version; its number is
// its identity within the Library. It changes no other aggregate: its
// number is the latest one plus one, and a release that lost the race for
// it is made again on top of the winner (ADR 0012).
type LibraryRelease interface {
	LibraryID() id.ID[Library]
	Number() ReleaseNumber
	Note() string
	CreatedAt() time.Time
	// WidgetTypes returns the release's WidgetTypes, in the order of the
	// Draft they were copied from.
	WidgetTypes() []ReleasedWidgetType
	// FindWidgetType returns the release's copy of the WidgetType with the
	// given ID; false if the release does not hold it.
	FindWidgetType(id.ID[WidgetType]) (ReleasedWidgetType, bool)

	event.Recorder
	fmt.Stringer
}

type libraryReleaseImpl struct {
	libraryID   id.ID[Library]
	number      ReleaseNumber
	note        string
	createdAt   time.Time
	widgetTypes []ReleasedWidgetType
	pending     []event.Event
}

var _ LibraryRelease = (*libraryReleaseImpl)(nil)

// CreateRelease makes the next Library Release of the Library lib from its
// Draft, draft — the WidgetTypes of lib —, on top of latest, its latest
// release (nil for none), and records a LibraryReleasedEvent.
// Returns ErrLibraryDiscarding while a Discard of lib is under way,
// ErrNothingToRelease if the Draft does not differ from latest, and an error
// if a WidgetType of draft or latest is of another Library.
func CreateRelease(lib Library, draft []WidgetType, latest LibraryRelease, aNote string, at time.Time) (LibraryRelease, error) {
	if err := lib.CheckEditable(); err != nil {
		return nil, err
	}
	for _, wt := range draft {
		if wt.LibraryID() != lib.ID() {
			return nil, fmt.Errorf("cannot release library %s: widget type %s is of library %s", lib.ID(), wt.ID(), wt.LibraryID())
		}
	}
	number := FirstRelease()
	if latest != nil {
		if latest.LibraryID() != lib.ID() {
			return nil, fmt.Errorf("cannot release library %s on top of a release of library %s", lib.ID(), latest.LibraryID())
		}
		if !DraftDiffers(draft, latest) {
			return nil, ErrNothingToRelease
		}
		number = latest.Number().Next()
	}
	copies := make([]ReleasedWidgetType, len(draft))
	for i, wt := range draft {
		copies[i] = ReleasedFrom(wt)
	}
	return &libraryReleaseImpl{
		libraryID:   lib.ID(),
		number:      number,
		note:        aNote,
		createdAt:   at,
		widgetTypes: copies,
		pending:     []event.Event{NewLibraryReleasedEvent(lib.ID(), number)},
	}, nil
}

// ReconstituteRelease rebuilds a Library Release as stored; it records no
// event. For repositories.
func ReconstituteRelease(aLibraryID id.ID[Library], aNumber ReleaseNumber, aNote string, at time.Time, someWidgetTypes []ReleasedWidgetType) LibraryRelease {
	return &libraryReleaseImpl{
		libraryID:   aLibraryID,
		number:      aNumber,
		note:        aNote,
		createdAt:   at,
		widgetTypes: slices.Clone(someWidgetTypes),
	}
}

func (r *libraryReleaseImpl) LibraryID() id.ID[Library] { return r.libraryID }
func (r *libraryReleaseImpl) Number() ReleaseNumber     { return r.number }
func (r *libraryReleaseImpl) Note() string              { return r.note }
func (r *libraryReleaseImpl) CreatedAt() time.Time      { return r.createdAt }

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

// WithNumber returns the release, not yet saved, renumbered as the one after
// latest: for a release that lost the race for its number. Its event is
// recorded anew with the new number.
func WithNumber(r LibraryRelease, number ReleaseNumber) LibraryRelease {
	return &libraryReleaseImpl{
		libraryID:   r.LibraryID(),
		number:      number,
		note:        r.Note(),
		createdAt:   r.CreatedAt(),
		widgetTypes: r.WidgetTypes(),
		pending:     []event.Event{NewLibraryReleasedEvent(r.LibraryID(), number)},
	}
}

func (r *libraryReleaseImpl) PendingEvents() []event.Event {
	return slices.Clone(r.pending)
}

// String implements [fmt.Stringer].
func (r *libraryReleaseImpl) String() string {
	return fmt.Sprintf("LibraryRelease: %s of %s, %d widget types", r.number, r.libraryID, len(r.widgetTypes))
}

// ReleaseState is what a Library's releases tell of it: its latest release
// (the zero ReleaseNumber for none) and whether its Draft differs from it.
type ReleaseState struct {
	Latest       ReleaseNumber
	DraftDiffers bool
}

// ReleaseStateOf returns the ReleaseState of a Library whose Draft is draft
// and whose latest release is latest (nil for none), as DraftDiffers decides.
func ReleaseStateOf(draft []WidgetType, latest LibraryRelease) ReleaseState {
	state := ReleaseState{DraftDiffers: DraftDiffers(draft, latest)}
	if latest != nil {
		state.Latest = latest.Number()
	}
	return state
}

// DraftDiffers reports whether the Draft draft — the WidgetTypes of a
// Library — differs from its Library Release release: a WidgetType added,
// removed or changed since. A nil release (none yet) differs from any Draft.
// It decides whether a Release is refused and whether the Library has
// changes not released yet.
func DraftDiffers(draft []WidgetType, release LibraryRelease) bool {
	if release == nil {
		return true
	}
	released := release.WidgetTypes()
	if len(draft) != len(released) {
		return true
	}
	for _, wt := range draft {
		r, ok := release.FindWidgetType(wt.ID())
		if !ok || !r.Matches(wt) {
			return true
		}
	}
	return false
}
