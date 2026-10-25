package devicelink

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
)

// RESTLink is the Link over the server's REST API, to the Tags of one
// project named by configuration (until the Device Token names it, task 34).
// The project is looked up by name on the first Resolve, and again after a
// 404 of a request to it: the project may have been deleted, or deleted and
// created anew under the name (a new ID). It keeps each tag's
// Version: a write sends the version it last saw; on 409 Conflict (someone
// else changed the tag) it rereads the tag and retries once.
type RESTLink struct {
	client      *apiclient.Client
	projectName string

	mu       sync.Mutex
	project  *apiclient.ProjectClient // nil until resolved
	versions map[uuid.UUID]int
}

var _ Link = (*RESTLink)(nil)

// NewRESTLink creates a Link over the given REST API client to the Tags of
// the project named projectName.
func NewRESTLink(client *apiclient.Client, projectName string) *RESTLink {
	return &RESTLink{client: client, projectName: projectName, versions: make(map[uuid.UUID]int)}
}

// Resolve looks a tag up with GET /api/v0/projects/{id}/tags?name=, the
// project found first with GET /api/v0/projects?name=. The list is 404 only
// when the project found earlier is gone: it is then looked up by name once
// more. The error matches ErrProjectNotFound if there is no such project.
func (l *RESTLink) Resolve(ctx context.Context, name string) (Tag, error) {
	var found apiv0.TagResponse
	var ok bool
	for again := true; ; again = false {
		project, err := l.resolveProject(ctx)
		if err != nil {
			return Tag{}, err
		}
		found, ok, err = project.FindTagByName(ctx, name)
		if errors.Is(err, apiclient.ErrNotFound) {
			l.dropProject(project)
			if again {
				continue
			}
			return Tag{}, fmt.Errorf("%w: %q: %w", ErrProjectNotFound, l.projectName, err)
		}
		if err != nil {
			return Tag{}, err
		}
		break
	}
	if !ok {
		return Tag{}, fmt.Errorf("%w: %q", ErrTagNotFound, name)
	}
	tagType, err := ParseTagType(found.Type)
	if err != nil {
		return Tag{}, fmt.Errorf("tag %q: %w", name, err)
	}
	l.setVersion(found.ID, found.Version)
	return Tag{ID: found.ID, Name: found.Name, Type: tagType}, nil
}

// resolveProject returns the client of the link's project, looking it up by
// name the first time.
func (l *RESTLink) resolveProject(ctx context.Context) (*apiclient.ProjectClient, error) {
	l.mu.Lock()
	project := l.project
	l.mu.Unlock()
	if project != nil {
		return project, nil
	}
	found, ok, err := l.client.FindProjectByName(ctx, l.projectName)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrProjectNotFound, l.projectName)
	}
	project = l.client.Project(found.ID)
	l.mu.Lock()
	l.project = project
	l.mu.Unlock()
	return project, nil
}

// dropProject forgets the project looked up, unless it was replaced
// meanwhile: the next request looks it up by name again.
func (l *RESTLink) dropProject(project *apiclient.ProjectClient) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.project == project {
		l.project = nil
	}
}

// Write sets the value with PATCH /api/v0/projects/{id}/tags/{id}/value. A
// 404 is the tag's or its project's; either way the tag is gone (a project
// created anew under the name has tags of new IDs).
func (l *RESTLink) Write(ctx context.Context, tag Tag, value string, quality Quality) error {
	project, err := l.resolveProject(ctx)
	if err != nil {
		return err
	}
	version, known := l.version(tag.ID)
	if !known {
		if err := l.reread(ctx, tag); err != nil {
			return err
		}
		version, _ = l.version(tag.ID)
	}

	newVersion, err := project.SetTagValue(ctx, tag.ID, value, quality.String(), version)
	if errors.Is(err, apiclient.ErrConflict) {
		if err := l.reread(ctx, tag); err != nil {
			return err
		}
		version, _ = l.version(tag.ID)
		newVersion, err = project.SetTagValue(ctx, tag.ID, value, quality.String(), version)
	}
	if errors.Is(err, apiclient.ErrNotFound) {
		l.forget(tag.ID)
		l.dropProject(project)
		return fmt.Errorf("%w: %q: %w", ErrTagNotFound, tag.Name, err)
	}
	if err != nil {
		if errors.Is(err, apiclient.ErrConflict) {
			l.forget(tag.ID) // reread before the next write
		}
		return fmt.Errorf("tag %q: %w", tag.Name, err)
	}
	l.setVersion(tag.ID, newVersion)
	return nil
}

// reread refreshes the tag's version from GET /api/v0/projects/{id}/tags/{id}.
func (l *RESTLink) reread(ctx context.Context, tag Tag) error {
	project, err := l.resolveProject(ctx)
	if err != nil {
		return err
	}
	current, err := project.GetTag(ctx, tag.ID)
	if errors.Is(err, apiclient.ErrNotFound) {
		l.forget(tag.ID)
		l.dropProject(project)
		return fmt.Errorf("%w: %q: %w", ErrTagNotFound, tag.Name, err)
	}
	if err != nil {
		return fmt.Errorf("tag %q: %w", tag.Name, err)
	}
	l.setVersion(tag.ID, current.Version)
	return nil
}

func (l *RESTLink) version(id uuid.UUID) (int, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok := l.versions[id]
	return v, ok
}

func (l *RESTLink) setVersion(id uuid.UUID, v int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.versions[id] = v
}

func (l *RESTLink) forget(id uuid.UUID) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.versions, id)
}
