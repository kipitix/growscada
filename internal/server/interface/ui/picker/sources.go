package picker

import (
	"context"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
)

// The LocalStorage keys of the selection, one per mode: the tab opens the
// entity under its key and the selection page keeps it there.
const (
	ProjectKey = "project:projectID"
	LibraryKey = "library:selectedLibrary"
)

// Badge texts of an entity's state.
const (
	StateUnsavedChanges    = "Unsaved changes"
	StateUnpublishedCommit = "unpublished commit"
	StatePublished         = "published"
	StateLegacy            = "legacy"
	StateNeverSaved        = "never saved"
)

// NewProjectSource returns the selection page's server side for the Projects.
func NewProjectSource(client *apiclient.Client) Source { return projectSource{client: client} }

type projectSource struct{ client *apiclient.Client }

func (s projectSource) Title() string    { return "Projects" }
func (s projectSource) Singular() string { return "project" }
func (s projectSource) Plural() string   { return "projects" }

func (s projectSource) List(ctx context.Context) ([]Item, error) {
	list, err := s.client.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]Item, len(list))
	for i, p := range list {
		item := Item{ID: p.ID, Name: p.Name, Description: p.Description, Version: p.Version}
		if p.HasUnsavedChanges {
			item.State, item.StateKind = StateUnsavedChanges, "unsaved"
		}
		items[i] = item
	}
	return items, nil
}

func (s projectSource) Create(ctx context.Context, name, description string) (uuid.UUID, error) {
	created, err := s.client.CreateProject(ctx, apiv0.CreateProjectRequest{Name: name, Description: description})
	return created.ID, err
}

func (s projectSource) Update(ctx context.Context, id uuid.UUID, version int, name, description string) error {
	_, err := s.client.UpdateProject(ctx, id, apiv0.UpdateProjectRequest{Name: name, Description: &description, Version: version})
	return err
}

func (s projectSource) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.client.DeleteProject(ctx, id)
	return err
}

func (s projectSource) DeleteQuestion(name string) string {
	return "Delete project " + name + " with all its scenes, widgets and tags?"
}

// NewLibrarySource returns the selection page's server side for the Libraries.
func NewLibrarySource(client *apiclient.Client) Source { return librarySource{client: client} }

type librarySource struct{ client *apiclient.Client }

func (s librarySource) Title() string    { return "Libraries" }
func (s librarySource) Singular() string { return "library" }
func (s librarySource) Plural() string   { return "libraries" }

func (s librarySource) List(ctx context.Context) ([]Item, error) {
	list, err := s.client.ListLibraries(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]Item, len(list))
	for i, l := range list {
		item := Item{ID: l.ID, Name: l.Name, Description: l.Description, Version: l.Version}
		item.State, item.StateKind = libraryState(l)
		items[i] = item
	}
	return items, nil
}

// libraryState is a Library's state as its selection row shows it: unsaved
// changes first, then the Commit and Release state the response reports
// (task 72_7, task 73).
func libraryState(l apiv0.LibraryResponse) (text, kind string) {
	switch {
	case l.HasUnsavedChanges:
		return StateUnsavedChanges, "unsaved"
	case l.HasUnpublishedChanges:
		return StateUnpublishedCommit, "unpublished"
	case l.LatestCommit > 0:
		return StatePublished, "published"
	case l.LatestRelease > 0:
		return StateLegacy, "legacy"
	default:
		return StateNeverSaved, "new"
	}
}

func (s librarySource) Create(ctx context.Context, name, description string) (uuid.UUID, error) {
	created, err := s.client.CreateLibrary(ctx, apiv0.CreateLibraryRequest{Name: name, Description: description})
	return created.ID, err
}

func (s librarySource) Update(ctx context.Context, id uuid.UUID, version int, name, description string) error {
	_, err := s.client.UpdateLibrary(ctx, id, apiv0.UpdateLibraryRequest{Name: name, Description: &description, Version: version})
	return err
}

func (s librarySource) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.client.DeleteLibrary(ctx, id)
	return err
}

func (s librarySource) DeleteQuestion(name string) string {
	return "Delete the library " + name + " with its Draft and releases? A project that pins it, or a running Deployment that names one of its releases, refuses the deletion. Commits and Releases of the past keep a link to the deleted library."
}
