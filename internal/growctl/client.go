package growctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/interface/restapi/restdto"
)

// errNotFound matches (errors.Is) an error for a 404 response.
var errNotFound = errors.New("not found")

// statusError is a response with an unexpected HTTP status. Its message names
// the request, so a --server pointing at the wrong place is easy to spot.
type statusError struct {
	status int
	msg    string
}

func (e *statusError) Error() string { return e.msg }

// Is makes a 404 match errNotFound.
func (e *statusError) Is(target error) bool {
	return target == errNotFound && e.status == http.StatusNotFound
}

// Client talks to the GrowSCADA REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a client for the server at baseURL (e.g. http://localhost:9090).
func NewClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient}
}

// ListTags returns every tag on the server.
func (c *Client) ListTags(ctx context.Context) ([]ServerTag, error) {
	return c.getTags(ctx, "/api/v1/tags")
}

// ListTagsMatching returns the server tags selected by the filter. The result
// is filtered here as well: a server that predates ?name_pattern=/?name_regex=
// ignores them and returns every tag, which must never widen the selection
// (e.g. apply --prune --pattern would otherwise delete everything).
func (c *Client) ListTagsMatching(ctx context.Context, f NameFilter) ([]ServerTag, error) {
	tags, err := c.getTags(ctx, "/api/v1/tags?"+f.param+"="+url.QueryEscape(f.expr))
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

// FindTagByName returns the tag with the given name, or false if there is none.
func (c *Client) FindTagByName(ctx context.Context, name string) (ServerTag, bool, error) {
	tags, err := c.getTags(ctx, "/api/v1/tags?"+restdto.TagsQueryName+"="+url.QueryEscape(name))
	if err != nil {
		return ServerTag{}, false, err
	}
	// Match the name here as well: a server that predates the ?name= filter
	// ignores it and returns every tag.
	for _, t := range tags {
		if t.Name == name {
			return t, true, nil
		}
	}
	return ServerTag{}, false, nil
}

// CreateTag creates a tag from its manifest.
func (c *Client) CreateTag(ctx context.Context, m TagManifest) error {
	body, err := json.Marshal(restdto.CreateTagRequest{
		Name:    m.Name,
		Type:    m.Type,
		Value:   m.InitialValue,
		Quality: m.InitialQuality,
	})
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, "/api/v1/tags", body, http.StatusCreated, nil)
}

// DeleteTag deletes a tag by ID. The error matches errNotFound if it is already gone.
func (c *Client) DeleteTag(ctx context.Context, id uuid.UUID) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/tags/"+id.String(), nil, http.StatusOK, nil)
}

func (c *Client) getTags(ctx context.Context, path string) ([]ServerTag, error) {
	var resp restdto.GetTagsResponse
	if err := c.do(ctx, http.MethodGet, path, nil, http.StatusOK, &resp); err != nil {
		return nil, err
	}
	tags := make([]ServerTag, len(resp.Tags))
	for i, t := range resp.Tags {
		tags[i] = ServerTag{ID: t.ID, Name: t.Name, Type: t.Type, Value: t.Value, Quality: t.Quality, Version: t.Version}
	}
	return tags, nil
}

// do sends a request and decodes a successful JSON response into out (if not nil).
// Any other status is a *statusError carrying the Problem Details detail.
func (c *Client) do(ctx context.Context, method, path string, body []byte, wantStatus int, out any) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err // *url.Error already names the method and URL
	}
	defer resp.Body.Close()

	if resp.StatusCode != wantStatus {
		return &statusError{
			status: resp.StatusCode,
			msg:    fmt.Sprintf("%s %s: %s%s", method, req.URL, resp.Status, problemDetail(resp.Body)),
		}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s %s: cannot decode response: %w", method, req.URL, err)
	}
	return nil
}

// problemDetail extracts the detail of an RFC 7807 response body, if any.
func problemDetail(body io.Reader) string {
	var problem struct {
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(body).Decode(&problem); err != nil || problem.Detail == "" {
		return ""
	}
	return ": " + problem.Detail
}
