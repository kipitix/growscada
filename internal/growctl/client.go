package growctl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"

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
		apiclient.OnServerVersion(warnOlderServerOnce(warnings)))}
}

// warnOlderServerOnce returns a server version handler that writes a warning
// the first time the server's version is older than growctl's.
func warnOlderServerOnce(w io.Writer) func(contract.SchemaVersion) {
	var once sync.Once
	return func(server contract.SchemaVersion) {
		if !apiv0.SchemaVersion().NewerThan(server) {
			return
		}
		once.Do(func() {
			fmt.Fprintf(w, "warning: the server speaks API %v, older than growctl's %v: update the server\n",
				server, apiv0.SchemaVersion())
		})
	}
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
	_, err := c.Client.CreateTag(ctx, apiv0.CreateTagRequest{
		Name:    m.Name,
		Type:    m.Type,
		Value:   m.InitialValue,
		Quality: m.InitialQuality,
	})
	return err
}
