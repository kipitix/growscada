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

// LibraryInput holds the fields of a library a client creates or updates.
// A nil Description leaves the current one as it is, so a client that sends
// only a name does not erase it; a pointer to an empty string clears it.
type LibraryInput struct {
	Name        string
	Description *string
}

// NewLibrary creates a Library DTO from the domain aggregate and the state its
// Commits and Releases are in.
func NewLibrary(l library.Library, state library.ReleaseState) Library {
	return Library{
		ID:                    l.ID().UUID(),
		Name:                  l.Name().String(),
		Description:           l.Description().String(),
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

// LibraryImport is the application-layer DTO for a Library Import of a
// Project: the saved state of the Library it names, with that state's title,
// Author, time and labels (task 85_6).
type LibraryImport struct {
	LibraryID    uuid.UUID
	CommitNumber int
	Title        string
	Author       string
	SavedAt      time.Time
	Labels       []Label
	Version      int
}

// NewLibraryImport creates a LibraryImport DTO from the domain entity and, when
// this server has it, the Commit it names: a Commit that cannot be read leaves
// the provenance empty and the number as stored.
func NewLibraryImport(i libraryimport.LibraryImport, version int, commit draft.Commit, labels []Label) LibraryImport {
	dto := LibraryImport{LibraryID: i.LibraryID().UUID(), CommitNumber: i.CommitNumber(), Labels: labels, Version: version}
	if commit != nil {
		dto.Title = commit.Title().String()
		dto.Author = commit.Author().String()
		dto.SavedAt = commit.CreatedAt()
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
