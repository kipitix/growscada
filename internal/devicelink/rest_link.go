package devicelink

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/kipitix/growscada/internal/apiclient"
)

// RESTLink is the Link over the server's REST API, to the Tags of the project
// its Device Token names (ADR 0004, amendment 2026-11-03): the token alone
// decides the project, and a token whose project is gone stops resolving. The
// project is read off GET /api/v0/device on the first Resolve, and again after
// a 404 of a request to it. A write states no version: the Device is the tag's
// single writer, and the latest write wins (ADR 0011).
type RESTLink struct {
	client *apiclient.Client

	mu      sync.Mutex
	project *apiclient.ProjectClient // nil until resolved
}

var _ Link = (*RESTLink)(nil)

// NewRESTLink creates a Link over the given REST API client that acts as the
// Device the token names. The client's own Device Token is ignored: the token
// given here is the one every request carries.
func NewRESTLink(client *apiclient.Client, token string) *RESTLink {
	return &RESTLink{client: client.WithDeviceToken(token)}
}

// Resolve looks a tag up with GET /api/v0/projects/{id}/tags?name=, the
// project found first with GET /api/v0/device. The list is 404 only when the
// project found earlier is gone: it is then looked up once more. The error
// matches ErrTagNotFound if there is no such tag.
func (l *RESTLink) Resolve(ctx context.Context, name string) (Tag, error) {
	for again := true; ; again = false {
		project, err := l.resolveProject(ctx)
		if err != nil {
			return Tag{}, err
		}
		found, ok, err := project.FindTagByName(ctx, name)
		if errors.Is(err, apiclient.ErrNotFound) {
			l.dropProject(project)
			if again {
				continue
			}
			return Tag{}, fmt.Errorf("%w: %q: the project is gone: %w", ErrTagNotFound, name, err)
		}
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
		return Tag{ID: found.ID, Name: found.Name, Type: tagType}, nil
	}
}

// resolveProject returns the client of the token's project, asking the server
// which project that is the first time.
func (l *RESTLink) resolveProject(ctx context.Context) (*apiclient.ProjectClient, error) {
	l.mu.Lock()
	project := l.project
	l.mu.Unlock()
	if project != nil {
		return project, nil
	}
	identity, err := l.client.DeviceIdentity(ctx)
	if err != nil {
		return nil, err
	}
	project = l.client.Project(identity.ProjectID)
	l.mu.Lock()
	l.project = project
	l.mu.Unlock()
	return project, nil
}

// dropProject forgets the project looked up, unless it was replaced
// meanwhile: the next request asks the server again.
func (l *RESTLink) dropProject(project *apiclient.ProjectClient) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.project == project {
		l.project = nil
	}
}

// Write sets the value with PATCH /api/v0/projects/{id}/tags/{id}/value. A
// 404 is the tag's; the tag is gone. A 401 or 403 is the server refusing the
// Device (IsNoPermission): a revoked token, a Tag it does not supply, or a
// Device that left the running configuration.
func (l *RESTLink) Write(ctx context.Context, tag Tag, value string, quality Quality) error {
	project, err := l.resolveProject(ctx)
	if err != nil {
		return err
	}
	_, err = project.SetTagValue(ctx, tag.ID, value, quality.String())
	if errors.Is(err, apiclient.ErrNotFound) {
		l.dropProject(project)
		return fmt.Errorf("%w: %q: %w", ErrTagNotFound, tag.Name, err)
	}
	if err != nil {
		return fmt.Errorf("tag %q: %w", tag.Name, err)
	}
	return nil
}
