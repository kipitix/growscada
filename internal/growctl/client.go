package growctl

import (
	"context"
	"net/http"

	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/restapi/restdto"
)

// errNotFound matches (errors.Is) an error for a 404 response.
var errNotFound = apiclient.ErrNotFound

// Client is the REST API client with the operations growctl needs on top of
// the plain API calls.
type Client struct {
	*apiclient.Client
}

// NewClient creates a client for the server at baseURL (e.g. http://localhost:9090).
func NewClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{Client: apiclient.New(baseURL, httpClient)}
}

// ListTagsMatching returns the server tags selected by the filter. The result
// is filtered here as well: a server that predates ?name_pattern=/?name_regex=
// ignores them and returns every tag, which must never widen the selection
// (e.g. apply --prune --pattern would otherwise delete everything).
func (c *Client) ListTagsMatching(ctx context.Context, f NameFilter) ([]ServerTag, error) {
	tags, err := c.ListTagsFiltered(ctx, f.param, f.expr)
	if err != nil {
		return nil, err
	}
	matched := tags[:0]
	for _, t := range tags {
		if f.Matches(t.Name) {
			matched = append(matched, t)
		}
	}
	return matched, nil
}

// CreateTag creates a tag from its manifest.
func (c *Client) CreateTag(ctx context.Context, m TagManifest) error {
	return c.Client.CreateTag(ctx, restdto.CreateTagRequest{
		Name:    m.Name,
		Type:    m.Type,
		Value:   m.InitialValue,
		Quality: m.InitialQuality,
	})
}
