// Package apiclient is a Go client of the GrowSCADA REST API, shared by
// everything that talks to the server from outside: growctl, the device
// simulator and the PWA UI (it builds for GOOS=js GOARCH=wasm too). It speaks
// the shapes of contract/api/v0 as they are: requests are marshalled from the
// apiv0 request types, responses and Problem Details are read tolerantly into
// the apiv0 response types (ADR 0005).
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
	"unicode/utf8"

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

// StatusError is a response with an unexpected HTTP status. Problem is the
// server's Problem Details; Problem.Status is always the response's status.
// Its message names the request, so a server URL pointing at the wrong place
// is easy to spot.
type StatusError struct {
	Method  string
	Problem apiv0.Problem
	msg     string
}

func (e *StatusError) Error() string { return e.msg }

// Is makes a 404 match ErrNotFound and a 409 match ErrConflict.
func (e *StatusError) Is(target error) bool {
	switch target {
	case ErrNotFound:
		return e.Problem.Status == http.StatusNotFound
	case ErrConflict:
		return e.Problem.Status == http.StatusConflict
	}
	return false
}

// Client talks to the GrowSCADA REST API. It keeps no state between calls.
type Client struct {
	baseURL         string
	httpClient      *http.Client
	onServerVersion func(contract.SchemaVersion)
}

// Option configures a Client.
type Option func(*Client)

// OnServerVersion makes the client call f with the server API SchemaVersion
// stated by every response, an error response included. A response without
// the header, or with one that does not parse, does not call f. f runs on the
// goroutine of the call.
func OnServerVersion(f func(contract.SchemaVersion)) Option {
	return func(c *Client) { c.onServerVersion = f }
}

// New creates a client for the server at baseURL (e.g. http://localhost:9090).
func New(baseURL string, httpClient *http.Client, opts ...Option) *Client {
	c := &Client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ── Tags ─────────────────────────────────────────────────────────────────────

// ListTags returns every tag on the server.
func (c *Client) ListTags(ctx context.Context) ([]apiv0.TagResponse, error) {
	return c.getTags(ctx, "/tags")
}

// ListTagsFiltered returns the tags selected by one GET /tags name filter
// (apiv0.TagsQueryName, TagsQueryNamePattern or TagsQueryNameRegex).
// A server that does not know the filter ignores it and returns every tag, so
// callers that must not widen the selection filter the result themselves.
func (c *Client) ListTagsFiltered(ctx context.Context, param, value string) ([]apiv0.TagResponse, error) {
	return c.getTags(ctx, "/tags?"+param+"="+url.QueryEscape(value))
}

// FindTagByName returns the tag with the given name, or false if there is none.
func (c *Client) FindTagByName(ctx context.Context, name string) (apiv0.TagResponse, bool, error) {
	tags, err := c.ListTagsFiltered(ctx, apiv0.TagsQueryName, name)
	if err != nil {
		return apiv0.TagResponse{}, false, err
	}
	// Match the name here as well: a server that predates the ?name= filter
	// ignores it and returns every tag.
	for _, t := range tags {
		if t.Name == name {
			return t, true, nil
		}
	}
	return apiv0.TagResponse{}, false, nil
}

// GetTag returns a tag by ID. The error matches ErrNotFound if there is none.
func (c *Client) GetTag(ctx context.Context, id uuid.UUID) (apiv0.TagResponse, error) {
	var resp apiv0.TagResponse
	err := c.do(ctx, http.MethodGet, tagPath(id), nil, http.StatusOK, &resp)
	return resp, err
}

// CreateTag creates a tag.
func (c *Client) CreateTag(ctx context.Context, req apiv0.CreateTagRequest) (apiv0.CreateTagResponse, error) {
	var resp apiv0.CreateTagResponse
	err := c.do(ctx, http.MethodPost, "/tags", req, http.StatusCreated, &resp)
	return resp, err
}

// SetTagValue writes a tag's value and quality, expecting its current version
// (optimistic locking), and returns the new version. The error matches
// ErrConflict if the version is stale and ErrNotFound if the tag is gone.
func (c *Client) SetTagValue(ctx context.Context, id uuid.UUID, value, quality string, version int) (int, error) {
	var resp apiv0.UpdateTagResponse
	req := apiv0.UpdateTagRequest{Value: value, Quality: quality, Version: version}
	if err := c.do(ctx, http.MethodPatch, tagPath(id)+"/value", req, http.StatusOK, &resp); err != nil {
		return 0, err
	}
	return resp.Version, nil
}

// DeleteTag deletes a tag by ID and returns its last state. The error matches
// ErrNotFound if it is already gone.
func (c *Client) DeleteTag(ctx context.Context, id uuid.UUID) (apiv0.TagResponse, error) {
	var resp apiv0.TagResponse
	err := c.do(ctx, http.MethodDelete, tagPath(id), nil, http.StatusOK, &resp)
	return resp, err
}

func (c *Client) getTags(ctx context.Context, path string) ([]apiv0.TagResponse, error) {
	var resp apiv0.GetTagsResponse
	err := c.do(ctx, http.MethodGet, path, nil, http.StatusOK, &resp)
	return resp.Tags, err
}

func tagPath(id uuid.UUID) string { return "/tags/" + id.String() }

// ── WidgetTypes ──────────────────────────────────────────────────────────────

// ListWidgetTypes returns every WidgetType.
func (c *Client) ListWidgetTypes(ctx context.Context) ([]apiv0.WidgetTypeResponse, error) {
	var resp apiv0.GetWidgetTypesResponse
	err := c.do(ctx, http.MethodGet, "/widget-types", nil, http.StatusOK, &resp)
	return resp.WidgetTypes, err
}

// GetWidgetType returns a WidgetType by ID. The error matches ErrNotFound if
// there is none.
func (c *Client) GetWidgetType(ctx context.Context, id uuid.UUID) (apiv0.WidgetTypeResponse, error) {
	var resp apiv0.WidgetTypeResponse
	err := c.do(ctx, http.MethodGet, widgetTypePath(id), nil, http.StatusOK, &resp)
	return resp, err
}

// CreateWidgetType creates a WidgetType.
func (c *Client) CreateWidgetType(ctx context.Context, req apiv0.CreateWidgetTypeRequest) (apiv0.CreateWidgetTypeResponse, error) {
	var resp apiv0.CreateWidgetTypeResponse
	err := c.do(ctx, http.MethodPost, "/widget-types", req, http.StatusCreated, &resp)
	return resp, err
}

// UpdateWidgetType replaces a WidgetType, expecting req.Version to be its
// current version. The error matches ErrConflict if the version is stale.
func (c *Client) UpdateWidgetType(ctx context.Context, id uuid.UUID, req apiv0.UpdateWidgetTypeRequest) (apiv0.UpdateWidgetTypeResponse, error) {
	var resp apiv0.UpdateWidgetTypeResponse
	err := c.do(ctx, http.MethodPut, widgetTypePath(id), req, http.StatusOK, &resp)
	return resp, err
}

// DeleteWidgetType deletes a WidgetType and returns its last state. The error
// matches ErrConflict if Widgets still use it.
func (c *Client) DeleteWidgetType(ctx context.Context, id uuid.UUID) (apiv0.WidgetTypeResponse, error) {
	var resp apiv0.WidgetTypeResponse
	err := c.do(ctx, http.MethodDelete, widgetTypePath(id), nil, http.StatusOK, &resp)
	return resp, err
}

func widgetTypePath(id uuid.UUID) string { return "/widget-types/" + id.String() }

// ── Scenes ───────────────────────────────────────────────────────────────────

// ListScenes returns every Scene.
func (c *Client) ListScenes(ctx context.Context) ([]apiv0.SceneResponse, error) {
	var resp apiv0.GetScenesResponse
	err := c.do(ctx, http.MethodGet, "/scenes", nil, http.StatusOK, &resp)
	return resp.Scenes, err
}

// GetScene returns a Scene by ID. The error matches ErrNotFound if there is
// none.
func (c *Client) GetScene(ctx context.Context, id uuid.UUID) (apiv0.SceneResponse, error) {
	var resp apiv0.SceneResponse
	err := c.do(ctx, http.MethodGet, scenePath(id), nil, http.StatusOK, &resp)
	return resp, err
}

// CreateScene creates a Scene.
func (c *Client) CreateScene(ctx context.Context, req apiv0.CreateSceneRequest) (apiv0.CreateSceneResponse, error) {
	var resp apiv0.CreateSceneResponse
	err := c.do(ctx, http.MethodPost, "/scenes", req, http.StatusCreated, &resp)
	return resp, err
}

// UpdateScene replaces a Scene's properties, expecting req.Version to be its
// current version. The error matches ErrConflict if the version is stale.
func (c *Client) UpdateScene(ctx context.Context, id uuid.UUID, req apiv0.UpdateSceneRequest) (apiv0.UpdateSceneResponse, error) {
	var resp apiv0.UpdateSceneResponse
	err := c.do(ctx, http.MethodPut, scenePath(id), req, http.StatusOK, &resp)
	return resp, err
}

// DeleteScene deletes a Scene with its Widgets and returns its last state.
func (c *Client) DeleteScene(ctx context.Context, id uuid.UUID) (apiv0.SceneResponse, error) {
	var resp apiv0.SceneResponse
	err := c.do(ctx, http.MethodDelete, scenePath(id), nil, http.StatusOK, &resp)
	return resp, err
}

func scenePath(id uuid.UUID) string { return "/scenes/" + id.String() }

// ── Widgets ──────────────────────────────────────────────────────────────────

// ListWidgets returns the Widgets of a Scene.
func (c *Client) ListWidgets(ctx context.Context, sceneID uuid.UUID) ([]apiv0.WidgetResponse, error) {
	var resp apiv0.GetWidgetsResponse
	err := c.do(ctx, http.MethodGet, scenePath(sceneID)+"/widgets", nil, http.StatusOK, &resp)
	return resp.Widgets, err
}

// GetWidget returns a Widget of a Scene. The error matches ErrNotFound if
// there is none.
func (c *Client) GetWidget(ctx context.Context, sceneID, widgetID uuid.UUID) (apiv0.WidgetResponse, error) {
	var resp apiv0.WidgetResponse
	err := c.do(ctx, http.MethodGet, widgetPath(sceneID, widgetID), nil, http.StatusOK, &resp)
	return resp, err
}

// CreateWidget places a Widget on a Scene, expecting req.SceneVersion to be
// the Scene's current version. The error matches ErrConflict if it is stale.
func (c *Client) CreateWidget(ctx context.Context, sceneID uuid.UUID, req apiv0.CreateWidgetRequest) (apiv0.CreateWidgetResponse, error) {
	var resp apiv0.CreateWidgetResponse
	err := c.do(ctx, http.MethodPost, scenePath(sceneID)+"/widgets", req, http.StatusCreated, &resp)
	return resp, err
}

// UpdateWidget replaces a Widget, expecting req.SceneVersion to be its
// Scene's current version. The error matches ErrConflict if it is stale.
func (c *Client) UpdateWidget(ctx context.Context, sceneID, widgetID uuid.UUID, req apiv0.UpdateWidgetRequest) (apiv0.UpdateWidgetResponse, error) {
	var resp apiv0.UpdateWidgetResponse
	err := c.do(ctx, http.MethodPut, widgetPath(sceneID, widgetID), req, http.StatusOK, &resp)
	return resp, err
}

// DeleteWidget removes a Widget from its Scene and returns its last state,
// with the Scene's new version.
func (c *Client) DeleteWidget(ctx context.Context, sceneID, widgetID uuid.UUID) (apiv0.WidgetResponse, error) {
	var resp apiv0.WidgetResponse
	err := c.do(ctx, http.MethodDelete, widgetPath(sceneID, widgetID), nil, http.StatusOK, &resp)
	return resp, err
}

func widgetPath(sceneID, widgetID uuid.UUID) string {
	return scenePath(sceneID) + "/widgets/" + widgetID.String()
}

// ── Transport ────────────────────────────────────────────────────────────────

// maxProblemBody caps how much of an error response is read.
const maxProblemBody = 64 << 10

// maxFallbackDetail caps the Detail taken from an error body that is not
// Problem Details (e.g. a proxy's HTML page).
const maxFallbackDetail = 200

// do sends a request to path under apiv0.PathPrefix — the body, if not nil,
// marshalled as JSON — and decodes a successful response into out (if not
// nil). Any other status is a *StatusError carrying the Problem Details.
func (c *Client) do(ctx context.Context, method, path string, body any, wantStatus int, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%s %s: cannot encode request: %w", method, path, err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+apiv0.PathPrefix+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		// The server reads the header only when it decodes a body; on a GET
		// a custom header would also cost a browser a CORS preflight.
		req.Header.Set(contract.SchemaVersionHeader, apiv0.SchemaVersion().String())
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err // *url.Error already names the method and URL
	}
	defer resp.Body.Close()
	c.reportServerVersion(resp)

	if resp.StatusCode != wantStatus {
		return newStatusError(req, resp)
	}
	if out == nil {
		return nil
	}
	// Tolerant: a newer server may add fields (ADR 0005).
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s %s: cannot decode response: %w", method, req.URL, err)
	}
	return nil
}

func (c *Client) reportServerVersion(resp *http.Response) {
	if c.onServerVersion == nil {
		return
	}
	v, err := contract.ParseSchemaVersion(resp.Header.Get(contract.SchemaVersionHeader))
	if err != nil {
		return
	}
	c.onServerVersion(v)
}

// newStatusError reads resp's body as Problem Details. A body that is not
// (e.g. an HTML page of a proxy) gives a Problem titled by the status, with
// the start of the body as its detail.
func newStatusError(req *http.Request, resp *http.Response) *StatusError {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxProblemBody))
	var problem apiv0.Problem
	if err := json.Unmarshal(data, &problem); err != nil {
		problem = apiv0.Problem{Detail: truncate(strings.TrimSpace(string(data)), maxFallbackDetail)}
	}
	problem.Status = resp.StatusCode
	if problem.Title == "" {
		problem.Title = http.StatusText(resp.StatusCode)
	}

	msg := fmt.Sprintf("%s %s: %s", req.Method, req.URL, resp.Status)
	if problem.Detail != "" {
		msg += ": " + problem.Detail
	}
	return &StatusError{Method: req.Method, Problem: problem, msg: msg}
}

// truncate cuts s to at most n bytes without splitting a UTF-8 sequence.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "…"
}
