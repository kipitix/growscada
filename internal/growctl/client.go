package growctl

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
)

// Client is the REST API client with the operations growctl needs on top of
// the plain API calls.
type Client struct {
	*apiclient.Client
}

// NewClient creates a client for the server at baseURL (e.g.
// http://localhost:9090). A server that speaks an older API than growctl gets
// one warning on warnings for the client's lifetime — a run of growctl; a
// newer one is fine, growctl reads its responses tolerantly.
func NewClient(baseURL string, httpClient *http.Client, warnings io.Writer) *Client {
	return &Client{Client: apiclient.New(baseURL, httpClient,
		apiclient.WarnOlderServer(apiclient.WarnOnce, func(server, own contract.SchemaVersion) {
			fmt.Fprintf(warnings, "warning: the server speaks API %v, older than growctl's %v: update the server\n", server, own)
		}))}
}

// ProjectTags returns the client of the Tags of the project with the given
// name. A project that does not exist is an error.
func (c *Client) ProjectTags(ctx context.Context, project string) (*Tags, error) {
	found, ok, err := c.FindProjectByName(ctx, project)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("project/%s not found", project)
	}
	return c.TagsOf(found), nil
}

// TagsOf returns the client of the Tags of the project p.
func (c *Client) TagsOf(p apiv0.ProjectResponse) *Tags {
	return &Tags{ProjectClient: c.Project(p.ID), Name: p.Name}
}

// Tags is the client of the Tags of one project, with the operations growctl
// needs on top of the plain API calls.
type Tags struct {
	*apiclient.ProjectClient
	// Name is the project's name.
	Name string
}

// ListTagsMatching returns the project's tags selected by the filter. The
// result is filtered here as well: a server that predates
// ?name_pattern=/?name_regex= ignores them and returns every tag, which must
// never widen the selection (e.g. apply --prune --pattern would otherwise
// delete everything).
func (t *Tags) ListTagsMatching(ctx context.Context, f NameFilter) ([]ServerTag, error) {
	tags, err := t.ListTagsFiltered(ctx, f.param, f.expr)
	if err != nil {
		return nil, err
	}
	matched := tags[:0]
	for _, tg := range tags {
		if f.Matches(tg.Name) {
			matched = append(matched, tg)
		}
	}
	return matched, nil
}

// CreateTag creates a tag from its manifest.
func (t *Tags) CreateTag(ctx context.Context, m TagManifest) error {
	_, err := t.ProjectClient.CreateTag(ctx, apiv0.CreateTagRequest{Name: m.Name, Type: m.Type})
	return err
}
