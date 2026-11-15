package libraryimport

import (
	"fmt"
	"slices"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
)

// Commit is a saved state of a Library as an Import names it (task 85_6): its
// identity, its Library, its number within the Library's Draft and the copies
// of the WidgetTypes it holds. It is the content an Import uses — a Project
// sees the WidgetTypes of the saved state it imported, never the Library's Draft —
// and the value the Import's impact is computed over.
//
// It is a value of this package, not the Draft's saved state: an Import is an
// entity of a Project's Draft, and the Draft package imports this one.
type Commit struct {
	id          uuid.UUID
	libraryID   id.ID[library.Library]
	number      int
	widgetTypes []library.WidgetTypeCopy
}

var _ fmt.Stringer = Commit{}

// NewCommit returns the saved state number of the Library aLibraryID, held by
// the saved state anID, with the copies of its WidgetTypes. Returns an error for a
// zero identity, a zero Library or a number below 1.
func NewCommit(anID uuid.UUID, aLibraryID id.ID[library.Library], number int, widgetTypes []library.WidgetTypeCopy) (Commit, error) {
	if anID == uuid.Nil {
		return Commit{}, fmt.Errorf("a saved state needs an ID")
	}
	if aLibraryID.IsZero() {
		return Commit{}, fmt.Errorf("a saved state needs its library")
	}
	if number < 1 {
		return Commit{}, fmt.Errorf("commit number must be positive, got %d", number)
	}
	return Commit{
		id: anID, libraryID: aLibraryID, number: number,
		widgetTypes: slices.Clone(widgetTypes),
	}, nil
}

// ReconstituteCommit rebuilds a saved state as stored; it checks nothing. For
// repositories.
func ReconstituteCommit(anID uuid.UUID, aLibraryID id.ID[library.Library], number int, widgetTypes []library.WidgetTypeCopy) Commit {
	return Commit{
		id: anID, libraryID: aLibraryID, number: number,
		widgetTypes: slices.Clone(widgetTypes),
	}
}

// ID is the saved state's identity.
func (c Commit) ID() uuid.UUID { return c.id }

// LibraryID is the Library the saved state belongs to.
func (c Commit) LibraryID() id.ID[library.Library] { return c.libraryID }

// Number is the saved state's number within the Library's Draft, from 1.
func (c Commit) Number() int { return c.number }

// WidgetTypes are the copies of the WidgetTypes the saved state holds.
func (c Commit) WidgetTypes() []library.WidgetTypeCopy { return slices.Clone(c.widgetTypes) }

// FindWidgetType returns the copy of the WidgetType typeID the saved state holds.
func (c Commit) FindWidgetType(typeID id.ID[library.WidgetType]) (library.WidgetTypeCopy, bool) {
	for _, wt := range c.widgetTypes {
		if wt.ID() == typeID {
			return wt, true
		}
	}
	return library.WidgetTypeCopy{}, false
}

// String implements [fmt.Stringer].
func (c Commit) String() string {
	return fmt.Sprintf("library saved state %d of %s (%s)", c.number, c.libraryID, c.id)
}
