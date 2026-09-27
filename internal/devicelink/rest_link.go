package devicelink

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/apiclient"
)

// RESTLink is the Link over the server's REST API. It keeps each tag's
// Version: a write sends the version it last saw; on 409 Conflict (someone
// else changed the tag) it rereads the tag and retries once.
type RESTLink struct {
	client *apiclient.Client

	mu       sync.Mutex
	versions map[uuid.UUID]int
}

var _ Link = (*RESTLink)(nil)

// NewRESTLink creates a Link over the given REST API client.
func NewRESTLink(client *apiclient.Client) *RESTLink {
	return &RESTLink{client: client, versions: make(map[uuid.UUID]int)}
}

// Resolve looks a tag up with GET /api/v1/tags?name=.
func (l *RESTLink) Resolve(ctx context.Context, name string) (Tag, error) {
	found, ok, err := l.client.FindTagByName(ctx, name)
	if err != nil {
		return Tag{}, err
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

// Write sets the value with PATCH /api/v1/tags/{id}/value.
func (l *RESTLink) Write(ctx context.Context, tag Tag, value string, quality Quality) error {
	version, known := l.version(tag.ID)
	if !known {
		if err := l.reread(ctx, tag); err != nil {
			return err
		}
		version, _ = l.version(tag.ID)
	}

	newVersion, err := l.client.SetTagValue(ctx, tag.ID, value, quality.String(), version)
	if errors.Is(err, apiclient.ErrConflict) {
		if err := l.reread(ctx, tag); err != nil {
			return err
		}
		version, _ = l.version(tag.ID)
		newVersion, err = l.client.SetTagValue(ctx, tag.ID, value, quality.String(), version)
	}
	if errors.Is(err, apiclient.ErrNotFound) {
		l.forget(tag.ID)
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

// reread refreshes the tag's version from GET /api/v1/tags/{id}.
func (l *RESTLink) reread(ctx context.Context, tag Tag) error {
	current, err := l.client.GetTag(ctx, tag.ID)
	if errors.Is(err, apiclient.ErrNotFound) {
		l.forget(tag.ID)
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
