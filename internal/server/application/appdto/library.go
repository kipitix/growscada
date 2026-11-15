package appdto

import (
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/libraryimport"
)

// Library is the application-layer DTO for library data. Its Organization is
// left out: it is the server's own, the same for every library.
type Library struct {
	ID          uuid.UUID
	Name        string
	Description string
	Version     int
	// LatestSavedState is the number of the Draft's last saved state; 0 when it
	// was never saved.
	LatestSavedState int
	// HasUnsavedChanges reports whether the Draft moved since its last saved
	// state.
	HasUnsavedChanges bool
}

// LibraryInput holds the fields of a library a client creates or updates.
// A nil Description leaves the current one as it is, so a client that sends
// only a name does not erase it; a pointer to an empty string clears it.
type LibraryInput struct {
	Name        string
	Description *string
}

// NewLibrary creates a Library DTO from the domain aggregate, the number of
// its last saved state (0 when it was never saved) and whether its Draft moved
// since that saved state (task 74).
func NewLibrary(l library.Library, latestSavedState int, hasUnsavedChanges bool) Library {
	return Library{
		ID:                l.ID().UUID(),
		Name:              l.Name().String(),
		Description:       l.Description().String(),
		Version:           l.Version().Number(),
		LatestSavedState:  latestSavedState,
		HasUnsavedChanges: hasUnsavedChanges,
	}
}

// LibraryImport is the application-layer DTO for a Library Import of a
// Project: the saved state of the Library it names, with that state's title,
// Author, time and labels (task 85_6).
type LibraryImport struct {
	LibraryID        uuid.UUID
	SavedStateNumber int
	Title            string
	Author           string
	SavedAt          time.Time
	Labels           []Label
	Version          int
}

// NewLibraryImport creates a LibraryImport DTO from the domain entity and, when
// this server has it, the saved state it names: a saved state that cannot be
// read leaves the provenance empty and the number as stored.
func NewLibraryImport(i libraryimport.LibraryImport, version int, savedState draft.SavedState, labels []Label) LibraryImport {
	dto := LibraryImport{LibraryID: i.LibraryID().UUID(), SavedStateNumber: i.SavedStateNumber(), Labels: labels, Version: version}
	if savedState != nil {
		dto.Title = savedState.Title().String()
		dto.Author = savedState.Author().String()
		dto.SavedAt = savedState.CreatedAt()
	}
	if dto.Labels == nil {
		dto.Labels = []Label{}
	}
	return dto
}

// AffectedWidget names a Widget of a Project with its Scene and WidgetType.
type AffectedWidget struct {
	SceneID  uuid.UUID
	WidgetID uuid.UUID
	Name     string
	TypeID   uuid.UUID
}

// BrokenBinding is a PortBinding a switch of an import would remove.
type BrokenBinding struct {
	AffectedWidget
	PortName string
	TagID    uuid.UUID
}

// ImportImpact is what switching or removing an import would do to the
// Project's Widgets (libraryimport.Impact).
type ImportImpact struct {
	Orphaned []AffectedWidget
	Broken   []BrokenBinding
}

// NewAffectedWidget creates an AffectedWidget DTO.
func NewAffectedWidget(w libraryimport.PlacedWidget) AffectedWidget {
	return AffectedWidget{
		SceneID:  w.SceneID.UUID(),
		WidgetID: w.Widget.ID().UUID(),
		Name:     w.Widget.Name().String(),
		TypeID:   w.Widget.TypeID().UUID(),
	}
}

// NewImportImpact creates an ImportImpact DTO from the domain's Impact.
func NewImportImpact(i libraryimport.Impact) ImportImpact {
	dto := ImportImpact{Orphaned: []AffectedWidget{}, Broken: []BrokenBinding{}}
	for _, w := range i.Orphaned {
		dto.Orphaned = append(dto.Orphaned, NewAffectedWidget(w))
	}
	for _, b := range i.Broken {
		dto.Broken = append(dto.Broken, BrokenBinding{
			AffectedWidget: NewAffectedWidget(b.PlacedWidget),
			PortName:       b.Binding.PortName().String(),
			TagID:          b.Binding.TagID().UUID(),
		})
	}
	return dto
}
