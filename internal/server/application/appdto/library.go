package appdto

import (
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
)

// Library is the application-layer DTO for library data. Its Organization is
// left out: it is the server's own, the same for every library.
type Library struct {
	ID      uuid.UUID
	Name    string
	Version int
	// LatestRelease is the number of the latest Library Release; 0 if none.
	LatestRelease int
	// DiscardingTo is the release a Discard under way resets the Draft to;
	// 0 if none is.
	DiscardingTo int
	// HasUnreleasedChanges reports whether the Draft differs from the
	// latest release (always, for a library never released).
	HasUnreleasedChanges bool
}

// LibraryInput holds the fields of a library a client creates or renames.
type LibraryInput struct {
	Name string
}

// NewLibrary creates a Library DTO from the domain aggregate, its latest
// release (nil for none) and whether its Draft differs from it.
func NewLibrary(l library.Library, latest library.LibraryRelease, unreleased bool) Library {
	dto := Library{
		ID:                   l.ID().UUID(),
		Name:                 l.Name().String(),
		Version:              l.Version().Number(),
		HasUnreleasedChanges: unreleased,
	}
	if latest != nil {
		dto.LatestRelease = latest.Number().Int()
	}
	if to, ok := l.DiscardingTo(); ok {
		dto.DiscardingTo = to.Int()
	}
	return dto
}

// LibraryRelease is the application-layer DTO for a Library Release.
type LibraryRelease struct {
	LibraryID   uuid.UUID
	Number      int
	Note        string
	CreatedAt   time.Time
	WidgetTypes []ReleasedWidgetType
}

// ReleaseInput holds what a client states for a Release.
type ReleaseInput struct {
	Note string
}

// NewLibraryRelease creates a LibraryRelease DTO from the domain aggregate.
func NewLibraryRelease(r library.LibraryRelease) LibraryRelease {
	return LibraryRelease{
		LibraryID:   r.LibraryID().UUID(),
		Number:      r.Number().Int(),
		Note:        r.Note(),
		CreatedAt:   r.CreatedAt(),
		WidgetTypes: NewReleasedWidgetTypeList(r.WidgetTypes()),
	}
}

// NewLibraryReleaseList creates a slice of LibraryRelease DTOs.
func NewLibraryReleaseList(list []library.LibraryRelease) []LibraryRelease {
	result := make([]LibraryRelease, len(list))
	for i, r := range list {
		result[i] = NewLibraryRelease(r)
	}
	return result
}

// LibraryPin is the application-layer DTO for a Library Pin of a Project.
type LibraryPin struct {
	LibraryID uuid.UUID
	Release   int
	Version   int
}

// NewLibraryPin creates a LibraryPin DTO from the domain aggregate.
func NewLibraryPin(p librarypin.LibraryPin) LibraryPin {
	return LibraryPin{LibraryID: p.LibraryID().UUID(), Release: p.Release().Int(), Version: p.Version().Number()}
}

// NewLibraryPinList creates a slice of LibraryPin DTOs.
func NewLibraryPinList(list []librarypin.LibraryPin) []LibraryPin {
	result := make([]LibraryPin, len(list))
	for i, p := range list {
		result[i] = NewLibraryPin(p)
	}
	return result
}

// PinnedWidget names a Widget of a Project with its Scene and WidgetType.
type PinnedWidget struct {
	SceneID  uuid.UUID
	WidgetID uuid.UUID
	Name     string
	TypeID   uuid.UUID
}

// BrokenBinding is a PortBinding a switch of a pin would remove.
type BrokenBinding struct {
	PinnedWidget
	PortName string
	TagID    uuid.UUID
}

// PinImpact is what switching or removing a pin would do to the Project's
// Widgets (librarypin.Impact).
type PinImpact struct {
	Orphaned []PinnedWidget
	Broken   []BrokenBinding
}

// NewPinnedWidget creates a PinnedWidget DTO.
func NewPinnedWidget(w librarypin.PlacedWidget) PinnedWidget {
	return PinnedWidget{
		SceneID:  w.SceneID.UUID(),
		WidgetID: w.Widget.ID().UUID(),
		Name:     w.Widget.Name().String(),
		TypeID:   w.Widget.TypeID().UUID(),
	}
}

// NewPinImpact creates a PinImpact DTO from the domain's Impact.
func NewPinImpact(i librarypin.Impact) PinImpact {
	dto := PinImpact{Orphaned: []PinnedWidget{}, Broken: []BrokenBinding{}}
	for _, w := range i.Orphaned {
		dto.Orphaned = append(dto.Orphaned, NewPinnedWidget(w))
	}
	for _, b := range i.Broken {
		dto.Broken = append(dto.Broken, BrokenBinding{
			PinnedWidget: NewPinnedWidget(b.PlacedWidget),
			PortName:     b.Binding.PortName().String(),
			TagID:        b.Binding.TagID().UUID(),
		})
	}
	return dto
}
