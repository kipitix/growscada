// Package apiclient is a Go client of the GrowSCADA REST API, shared by the
// tools that talk to the server from outside (growctl, the device simulator).
package apiclient

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

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

var (
	// ErrNotFound matches (errors.Is) an error for a 404 response.
	ErrNotFound = errors.New("not found")
	// ErrConflict matches (errors.Is) an error for a 409 response.
	ErrConflict = errors.New("conflict")
)

// StatusError is a response with an unexpected HTTP status. Its message names
// the request, so a server URL pointing at the wrong place is easy to spot.
type StatusError struct {
	Status int
	msg    string
}

func (e *StatusError) Error() string { return e.msg }

// Is makes a 404 match ErrNotFound and a 409 match ErrConflict.
func (e *StatusError) Is(target error) bool {
	switch target {
	case ErrNotFound:
		return e.Status == http.StatusNotFound
	case ErrConflict:
		return e.Status == http.StatusConflict
	}
	return false
}

// Tag is a tag as the server reports it.
type Tag struct {
	ID      uuid.UUID
	Name    string
	Type    string
	Value   string
	Quality string
	Version int
}

// Client talks to the GrowSCADA REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a client for the server at baseURL (e.g. http://localhost:9090).
func New(baseURL string, httpClient *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient}
}

// ListTags returns every tag on the server.
func (c *Client) ListTags(ctx context.Context) ([]Tag, error) {
	return c.getTags(ctx, apiv0.PathPrefix+"/tags")
}

// ListTagsFiltered returns the tags selected by one GET /tags name filter
// (apiv0.TagsQueryName, TagsQueryNamePattern or TagsQueryNameRegex).
// A server that does not know the filter ignores it and returns every tag, so
// callers that must not widen the selection filter the result themselves.
func (c *Client) ListTagsFiltered(ctx context.Context, param, value string) ([]Tag, error) {
	return c.getTags(ctx, apiv0.PathPrefix+"/tags?"+param+"="+url.QueryEscape(value))
}

// FindTagByName returns the tag with the given name, or false if there is none.
func (c *Client) FindTagByName(ctx context.Context, name string) (Tag, bool, error) {
	tags, err := c.ListTagsFiltered(ctx, apiv0.TagsQueryName, name)
	if err != nil {
		return Tag{}, false, err
	}
	// Match the name here as well: a server that predates the ?name= filter
	// ignores it and returns every tag.
	for _, t := range tags {
		if t.Name == name {
			return t, true, nil
		}
	}
	return Tag{}, false, nil
}

// GetTag returns a tag by ID. The error matches ErrNotFound if there is none.
func (c *Client) GetTag(ctx context.Context, id uuid.UUID) (Tag, error) {
	var resp apiv0.TagResponse
	if err := c.do(ctx, http.MethodGet, apiv0.PathPrefix+"/tags/"+id.String(), nil, http.StatusOK, &resp); err != nil {
		return Tag{}, err
	}
	return newTag(resp), nil
}

// CreateTag creates a tag.
func (c *Client) CreateTag(ctx context.Context, req apiv0.CreateTagRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, apiv0.PathPrefix+"/tags", body, http.StatusCreated, nil)
}

// SetTagValue writes a tag's value and quality, expecting its current version
// (optimistic locking), and returns the new version. The error matches
// ErrConflict if the version is stale and ErrNotFound if the tag is gone.
func (c *Client) SetTagValue(ctx context.Context, id uuid.UUID, value, quality string, version int) (int, error) {
	body, err := json.Marshal(apiv0.UpdateTagRequest{Value: value, Quality: quality, Version: version})
	if err != nil {
		return 0, err
	}
	var resp apiv0.UpdateTagResponse
	if err := c.do(ctx, http.MethodPatch, apiv0.PathPrefix+"/tags/"+id.String()+"/value", body, http.StatusOK, &resp); err != nil {
		return 0, err
	}
	return resp.Version, nil
}

// DeleteTag deletes a tag by ID. The error matches ErrNotFound if it is already gone.
func (c *Client) DeleteTag(ctx context.Context, id uuid.UUID) error {
	return c.do(ctx, http.MethodDelete, apiv0.PathPrefix+"/tags/"+id.String(), nil, http.StatusOK, nil)
}

func (c *Client) getTags(ctx context.Context, path string) ([]Tag, error) {
	var resp apiv0.GetTagsResponse
	if err := c.do(ctx, http.MethodGet, path, nil, http.StatusOK, &resp); err != nil {
		return nil, err
	}
	tags := make([]Tag, len(resp.Tags))
	for i, t := range resp.Tags {
		tags[i] = newTag(t)
	}
	return tags, nil
}

func newTag(t apiv0.TagResponse) Tag {
	return Tag{ID: t.ID, Name: t.Name, Type: t.Type, Value: t.Value, Quality: t.Quality, Version: t.Version}
}

// do sends a request and decodes a successful JSON response into out (if not nil).
// Any other status is a *StatusError carrying the Problem Details detail.
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
	req.Header.Set(contract.SchemaVersionHeader, apiv0.SchemaVersion().String())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err // *url.Error already names the method and URL
	}
	defer resp.Body.Close()

	if resp.StatusCode != wantStatus {
		return &StatusError{
			Status: resp.StatusCode,
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
