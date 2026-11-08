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
	// LatestCommit is the number of the Draft's last Commit; 0 when it was
	// never saved.
	LatestCommit int
	// HasUnsavedChanges reports whether the Draft moved since its last Commit.
	HasUnsavedChanges bool
	// HasUnpublishedChanges reports whether the last Commit is newer than the
	// one the latest Release published.
	HasUnpublishedChanges bool
}

// LibraryInput holds the fields of a library a client creates or renames.
type LibraryInput struct {
	Name string
}

// NewLibrary creates a Library DTO from the domain aggregate and the state its
// Commits and Releases are in.
func NewLibrary(l library.Library, state library.ReleaseState) Library {
	return Library{
		ID:                    l.ID().UUID(),
		Name:                  l.Name().String(),
		Version:               l.Version().Number(),
		LatestRelease:         state.Latest.Int(),
		LatestCommit:          state.LatestCommit,
		HasUnsavedChanges:     state.UnsavedChanges,
		HasUnpublishedChanges: state.UnpublishedChanges,
	}
}

// LibraryRelease is the application-layer DTO for a Library Release (task
// 72_5; ADR 0021): its number, the Commit it published (nil for a legacy
// Release), that Commit's title, its Author, origin, note, time, the copies of
// its WidgetTypes and the counts of its snapshot.
type LibraryRelease struct {
	LibraryID    uuid.UUID
	Number       int
	CommitNumber *int
	Title        string
	Author       string
	Origin       string
	Note         string
	CreatedAt    time.Time
	WidgetTypes  []ReleasedWidgetType
	Counts       CommitCounts
}

// ReleaseInput holds what a client states for a Library Release: the Commit to
// publish, an optional note and the Author who publishes it.
type ReleaseInput struct {
	Commit int
	Note   string
	Author string
}

// NewLibraryRelease creates a LibraryRelease DTO from the domain aggregate; a
// title is the Commit's title at the moment of the read, empty for a legacy
// Release.
func NewLibraryRelease(r library.LibraryRelease, title string) LibraryRelease {
	return LibraryRelease{
		LibraryID:    r.LibraryID().UUID(),
		Number:       r.Number().Int(),
		CommitNumber: r.CommitNumber(),
		Title:        title,
		Author:       r.Author().String(),
		Origin:       r.Origin().String(),
		Note:         r.Note(),
		CreatedAt:    r.CreatedAt(),
		WidgetTypes:  NewReleasedWidgetTypeList(r.WidgetTypes()),
		Counts:       CommitCounts{WidgetTypes: len(r.WidgetTypes())},
	}
}

// NewLibraryReleaseList creates a slice of LibraryRelease DTOs, oldest first;
// titles maps a Commit number to its current title.
func NewLibraryReleaseList(list []library.LibraryRelease, titles map[int]string) []LibraryRelease {
	result := make([]LibraryRelease, len(list))
	for i, r := range list {
		title := ""
		if number := r.CommitNumber(); number != nil {
			title = titles[*number]
		}
		result[i] = NewLibraryRelease(r, title)
	}
	return result
}

// LibraryPin is the application-layer DTO for a Library Pin of a Project: the
// Release it names and, for a Release made from a Commit, its provenance.
type LibraryPin struct {
	LibraryID    uuid.UUID
	Release      int
	Version      int
	CommitNumber *int
	Title        string
	Author       string
	Origin       string
	Note         string
}

// NewLibraryPin creates a LibraryPin DTO from the domain aggregate and the
// Release it pins; a nil release leaves the provenance empty.
func NewLibraryPin(p librarypin.LibraryPin, version int, release library.LibraryRelease) LibraryPin {
	dto := LibraryPin{LibraryID: p.LibraryID().UUID(), Release: p.Release().Int(), Version: version}
	if release != nil {
		dto.CommitNumber = release.CommitNumber()
		dto.Author = release.Author().String()
		dto.Origin = release.Origin().String()
		dto.Note = release.Note()
	}
	return dto
}

// NewLibraryPinList creates a slice of LibraryPin DTOs; releases maps a pinned
// release number to the Release itself.
func NewLibraryPinList(list []librarypin.LibraryPin, version int, releases map[library.ReleaseNumber]library.LibraryRelease) []LibraryPin {
	result := make([]LibraryPin, len(list))
	for i, p := range list {
		result[i] = NewLibraryPin(p, version, releases[p.Release()])
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
