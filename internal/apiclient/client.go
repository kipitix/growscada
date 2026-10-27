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
	"reflect"
	"strconv"
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
	// ErrBadResponse matches (errors.Is) an error for a successful response
	// whose body cannot be read: the server answered, not as expected.
	ErrBadResponse = errors.New("cannot decode response")
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

// Client talks to the GrowSCADA REST API. The client itself keeps no state
// between calls; its Options may (WarnOlderServer remembers what it warned
// about), so create one Client and reuse it.
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
// goroutine of the call. Several such options (WarnOlderServer is one) are
// all called, in the order they were given.
func OnServerVersion(f func(contract.SchemaVersion)) Option {
	return func(c *Client) {
		prev := c.onServerVersion
		if prev == nil {
			c.onServerVersion = f
			return
		}
		c.onServerVersion = func(v contract.SchemaVersion) {
			prev(v)
			f(v)
		}
	}
}

// New creates a client for the server at baseURL (e.g. http://localhost:9090).
func New(baseURL string, httpClient *http.Client, opts ...Option) *Client {
	c := &Client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ── Projects ─────────────────────────────────────────────────────────────────

// ListProjects returns every live project of the server, in creation order.
func (c *Client) ListProjects(ctx context.Context) ([]apiv0.ProjectResponse, error) {
	return c.getProjects(ctx, "/projects")
}

// FindProjectByName returns the project with the given name, or false if
// there is none.
func (c *Client) FindProjectByName(ctx context.Context, name string) (apiv0.ProjectResponse, bool, error) {
	projects, err := c.getProjects(ctx, "/projects?"+apiv0.ProjectsQueryName+"="+url.QueryEscape(name))
	if err != nil {
		return apiv0.ProjectResponse{}, false, err
	}
	// Match the name here as well: a server that does not know the filter
	// ignores it and returns every project.
	for _, p := range projects {
		if p.Name == name {
			return p, true, nil
		}
	}
	return apiv0.ProjectResponse{}, false, nil
}

// GetProject returns a project by ID. The error matches ErrNotFound if there
// is none (or it is deleted).
func (c *Client) GetProject(ctx context.Context, id uuid.UUID) (apiv0.ProjectResponse, error) {
	var resp apiv0.ProjectResponse
	err := c.do(ctx, http.MethodGet, apiv0.ProjectPath(id.String()), nil, http.StatusOK, &resp)
	return resp, err
}

// CreateProject creates a project. The error matches ErrConflict if its name
// is taken.
func (c *Client) CreateProject(ctx context.Context, req apiv0.CreateProjectRequest) (apiv0.CreateProjectResponse, error) {
	var resp apiv0.CreateProjectResponse
	err := c.do(ctx, http.MethodPost, "/projects", req, http.StatusCreated, &resp)
	return resp, err
}

// UpdateProject renames a project, expecting req.Version to be its current
// version. The error matches ErrConflict if the version is stale or the name
// is taken.
func (c *Client) UpdateProject(ctx context.Context, id uuid.UUID, req apiv0.UpdateProjectRequest) (apiv0.UpdateProjectResponse, error) {
	var resp apiv0.UpdateProjectResponse
	err := c.do(ctx, http.MethodPut, apiv0.ProjectPath(id.String()), req, http.StatusOK, &resp)
	return resp, err
}

// DeleteProject deletes a project with all its content and returns its last
// state. The project is gone at once; its content is removed after.
func (c *Client) DeleteProject(ctx context.Context, id uuid.UUID) (apiv0.ProjectResponse, error) {
	var resp apiv0.ProjectResponse
	err := c.do(ctx, http.MethodDelete, apiv0.ProjectPath(id.String()), nil, http.StatusOK, &resp)
	return resp, err
}

func (c *Client) getProjects(ctx context.Context, path string) ([]apiv0.ProjectResponse, error) {
	var resp apiv0.GetProjectsResponse
	err := c.do(ctx, http.MethodGet, path, nil, http.StatusOK, &resp)
	return resp.Projects, err
}

// ProjectClient talks to the content of one project: its Tags, Scenes,
// Widgets and Library Pins, under the project's path (ADR 0006). A project that is gone makes
// every call fail with an error matching ErrNotFound.
type ProjectClient struct {
	c      *Client
	id     uuid.UUID
	prefix string
}

// Project returns the client of the content of the project id. It sends
// nothing: whether the project exists is learned with the first call.
func (c *Client) Project(id uuid.UUID) *ProjectClient {
	return &ProjectClient{c: c, id: id, prefix: apiv0.ProjectPath(id.String())}
}

// ID returns the project the client talks to.
func (p *ProjectClient) ID() uuid.UUID { return p.id }

func (p *ProjectClient) do(ctx context.Context, method, path string, body any, wantStatus int, out any) error {
	return p.c.do(ctx, method, p.prefix+path, body, wantStatus, out)
}

// ── Libraries ────────────────────────────────────────────────────────────────

// ListLibraries returns every live library of the server, in creation order.
func (c *Client) ListLibraries(ctx context.Context) ([]apiv0.LibraryResponse, error) {
	return c.getLibraries(ctx, "/libraries")
}

// FindLibraryByName returns the library with the given name, or false if
// there is none.
func (c *Client) FindLibraryByName(ctx context.Context, name string) (apiv0.LibraryResponse, bool, error) {
	libraries, err := c.getLibraries(ctx, "/libraries?"+apiv0.LibrariesQueryName+"="+url.QueryEscape(name))
	if err != nil {
		return apiv0.LibraryResponse{}, false, err
	}
	for _, l := range libraries {
		if l.Name == name {
			return l, true, nil
		}
	}
	return apiv0.LibraryResponse{}, false, nil
}

// GetLibrary returns a library by ID. The error matches ErrNotFound if there
// is none (or it is deleted).
func (c *Client) GetLibrary(ctx context.Context, id uuid.UUID) (apiv0.LibraryResponse, error) {
	var resp apiv0.LibraryResponse
	err := c.do(ctx, http.MethodGet, apiv0.LibraryPath(id.String()), nil, http.StatusOK, &resp)
	return resp, err
}

// CreateLibrary creates a library. The error matches ErrConflict if its name
// is taken.
func (c *Client) CreateLibrary(ctx context.Context, req apiv0.CreateLibraryRequest) (apiv0.CreateLibraryResponse, error) {
	var resp apiv0.CreateLibraryResponse
	err := c.do(ctx, http.MethodPost, "/libraries", req, http.StatusCreated, &resp)
	return resp, err
}

// UpdateLibrary renames a library, expecting req.Version to be its current
// version. The error matches ErrConflict if the version is stale or the name
// is taken.
func (c *Client) UpdateLibrary(ctx context.Context, id uuid.UUID, req apiv0.UpdateLibraryRequest) (apiv0.UpdateLibraryResponse, error) {
	var resp apiv0.UpdateLibraryResponse
	err := c.do(ctx, http.MethodPut, apiv0.LibraryPath(id.String()), req, http.StatusOK, &resp)
	return resp, err
}

// DeleteLibrary deletes a library with its Draft and releases and returns
// its last state. The error matches ErrConflict while a project pins it.
func (c *Client) DeleteLibrary(ctx context.Context, id uuid.UUID) (apiv0.LibraryResponse, error) {
	var resp apiv0.LibraryResponse
	err := c.do(ctx, http.MethodDelete, apiv0.LibraryPath(id.String()), nil, http.StatusOK, &resp)
	return resp, err
}

func (c *Client) getLibraries(ctx context.Context, path string) ([]apiv0.LibraryResponse, error) {
	var resp apiv0.GetLibrariesResponse
	err := c.do(ctx, http.MethodGet, path, nil, http.StatusOK, &resp)
	return resp.Libraries, err
}

// LibraryClient talks to one library: the WidgetTypes of its Draft, its
// releases and Discards, under the library's path. A library that is gone
// makes every call fail with an error matching ErrNotFound.
type LibraryClient struct {
	c      *Client
	id     uuid.UUID
	prefix string
}

// Library returns the client of the library id. It sends nothing.
func (c *Client) Library(id uuid.UUID) *LibraryClient {
	return &LibraryClient{c: c, id: id, prefix: apiv0.LibraryPath(id.String())}
}

// ID returns the library the client talks to.
func (l *LibraryClient) ID() uuid.UUID { return l.id }

func (l *LibraryClient) do(ctx context.Context, method, path string, body any, wantStatus int, out any) error {
	return l.c.do(ctx, method, l.prefix+path, body, wantStatus, out)
}

// ListWidgetTypes returns every WidgetType of the library's Draft.
func (l *LibraryClient) ListWidgetTypes(ctx context.Context) ([]apiv0.WidgetTypeResponse, error) {
	var resp apiv0.GetWidgetTypesResponse
	err := l.do(ctx, http.MethodGet, "/widget-types", nil, http.StatusOK, &resp)
	return resp.WidgetTypes, err
}

// GetWidgetType returns a WidgetType of the Draft by ID. The error matches
// ErrNotFound if there is none.
func (l *LibraryClient) GetWidgetType(ctx context.Context, id uuid.UUID) (apiv0.WidgetTypeResponse, error) {
	var resp apiv0.WidgetTypeResponse
	err := l.do(ctx, http.MethodGet, widgetTypePath(id), nil, http.StatusOK, &resp)
	return resp, err
}

// CreateWidgetType creates a WidgetType in the Draft. The error matches
// ErrConflict if its name is taken in the library, or while a Discard is
// under way.
func (l *LibraryClient) CreateWidgetType(ctx context.Context, req apiv0.CreateWidgetTypeRequest) (apiv0.CreateWidgetTypeResponse, error) {
	var resp apiv0.CreateWidgetTypeResponse
	err := l.do(ctx, http.MethodPost, "/widget-types", req, http.StatusCreated, &resp)
	return resp, err
}

// UpdateWidgetType replaces a WidgetType of the Draft, expecting req.Version
// to be its current version. The error matches ErrConflict if the version is
// stale, the name taken, or a Discard under way.
func (l *LibraryClient) UpdateWidgetType(ctx context.Context, id uuid.UUID, req apiv0.UpdateWidgetTypeRequest) (apiv0.UpdateWidgetTypeResponse, error) {
	var resp apiv0.UpdateWidgetTypeResponse
	err := l.do(ctx, http.MethodPut, widgetTypePath(id), req, http.StatusOK, &resp)
	return resp, err
}

// DeleteWidgetType removes a WidgetType from the Draft and returns its last
// state. It touches no project: they use the releases they pin.
func (l *LibraryClient) DeleteWidgetType(ctx context.Context, id uuid.UUID) (apiv0.WidgetTypeResponse, error) {
	var resp apiv0.WidgetTypeResponse
	err := l.do(ctx, http.MethodDelete, widgetTypePath(id), nil, http.StatusOK, &resp)
	return resp, err
}

func widgetTypePath(id uuid.UUID) string { return "/widget-types/" + id.String() }

// Release makes a release of the library's Draft. The error matches
// ErrConflict if req.Version is stale, the Draft has not changed since the
// latest release, or a Discard is under way.
func (l *LibraryClient) Release(ctx context.Context, req apiv0.CreateReleaseRequest) (apiv0.LibraryReleaseResponse, error) {
	var resp apiv0.LibraryReleaseResponse
	err := l.do(ctx, http.MethodPost, "/releases", req, http.StatusCreated, &resp)
	return resp, err
}

// ListReleases returns every release of the library, oldest first.
func (l *LibraryClient) ListReleases(ctx context.Context) ([]apiv0.LibraryReleaseResponse, error) {
	var resp apiv0.GetLibraryReleasesResponse
	err := l.do(ctx, http.MethodGet, "/releases", nil, http.StatusOK, &resp)
	return resp.Releases, err
}

// GetRelease returns the release numbered n. The error matches ErrNotFound if
// there is none.
func (l *LibraryClient) GetRelease(ctx context.Context, n int) (apiv0.LibraryReleaseResponse, error) {
	var resp apiv0.LibraryReleaseResponse
	err := l.do(ctx, http.MethodGet, "/releases/"+strconv.Itoa(n), nil, http.StatusOK, &resp)
	return resp, err
}

// Discard starts resetting the library's Draft to its latest release and
// returns the library, marked as being discarded. The error matches
// ErrConflict if it was never released or a Discard is already under way.
func (l *LibraryClient) Discard(ctx context.Context) (apiv0.LibraryResponse, error) {
	var resp apiv0.LibraryResponse
	err := l.do(ctx, http.MethodPost, "/discard", nil, http.StatusAccepted, &resp)
	return resp, err
}

// ── Library Pins ─────────────────────────────────────────────────────────────

// ListLibraryPins returns the project's Library Pins, in the order they were
// added.
func (p *ProjectClient) ListLibraryPins(ctx context.Context) ([]apiv0.LibraryPinResponse, error) {
	var resp apiv0.GetLibraryPinsResponse
	err := p.do(ctx, http.MethodGet, "/library-pins", nil, http.StatusOK, &resp)
	return resp.LibraryPins, err
}

// PinLibrary pins release req.Release of the library: req.Version 0 adds the
// pin, any other switches it. The error matches ErrConflict if the version
// is stale or the switch would leave Widgets without their WidgetType (the
// StatusError's Problem names them).
func (p *ProjectClient) PinLibrary(ctx context.Context, libraryID uuid.UUID, req apiv0.PutLibraryPinRequest) (apiv0.LibraryPinResponse, error) {
	var resp apiv0.LibraryPinResponse
	want := http.StatusOK
	if req.Version == 0 {
		want = http.StatusCreated
	}
	err := p.do(ctx, http.MethodPut, libraryPinPath(libraryID), req, want, &resp)
	return resp, err
}

// UnpinLibrary removes the pin of the library. The error matches ErrConflict
// while Widgets of the project are of its WidgetTypes.
func (p *ProjectClient) UnpinLibrary(ctx context.Context, libraryID uuid.UUID) (apiv0.LibraryPinResponse, error) {
	var resp apiv0.LibraryPinResponse
	err := p.do(ctx, http.MethodDelete, libraryPinPath(libraryID), nil, http.StatusOK, &resp)
	return resp, err
}

// LibraryPinImpact previews pinning the release numbered release of the
// library; release 0 previews removing the pin.
func (p *ProjectClient) LibraryPinImpact(ctx context.Context, libraryID uuid.UUID, release int) (apiv0.PinImpactResponse, error) {
	path := libraryPinPath(libraryID) + "/impact"
	if release != 0 {
		path += "?" + apiv0.LibraryPinImpactQueryRelease + "=" + strconv.Itoa(release)
	}
	var resp apiv0.PinImpactResponse
	err := p.do(ctx, http.MethodGet, path, nil, http.StatusOK, &resp)
	return resp, err
}

// ListWidgetTypes returns the WidgetTypes of every release the project pins:
// those its Widgets may be of.
func (p *ProjectClient) ListWidgetTypes(ctx context.Context) ([]apiv0.ReleasedWidgetTypeResponse, error) {
	var resp apiv0.GetPinnedWidgetTypesResponse
	err := p.do(ctx, http.MethodGet, "/widget-types", nil, http.StatusOK, &resp)
	return resp.WidgetTypes, err
}

func libraryPinPath(libraryID uuid.UUID) string { return "/library-pins/" + libraryID.String() }

// ── Tags ─────────────────────────────────────────────────────────────────────

// ListTags returns every tag of the project.
func (p *ProjectClient) ListTags(ctx context.Context) ([]apiv0.TagResponse, error) {
	return p.getTags(ctx, "/tags")
}

// ListTagsFiltered returns the tags selected by one GET /tags name filter
// (apiv0.TagsQueryName, TagsQueryNamePattern or TagsQueryNameRegex).
// A server that does not know the filter ignores it and returns every tag, so
// callers that must not widen the selection filter the result themselves.
func (p *ProjectClient) ListTagsFiltered(ctx context.Context, param, value string) ([]apiv0.TagResponse, error) {
	return p.getTags(ctx, "/tags?"+param+"="+url.QueryEscape(value))
}

// FindTagByName returns the tag of the project with the given name, or false
// if there is none.
func (p *ProjectClient) FindTagByName(ctx context.Context, name string) (apiv0.TagResponse, bool, error) {
	tags, err := p.ListTagsFiltered(ctx, apiv0.TagsQueryName, name)
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
func (p *ProjectClient) GetTag(ctx context.Context, id uuid.UUID) (apiv0.TagResponse, error) {
	var resp apiv0.TagResponse
	err := p.do(ctx, http.MethodGet, tagPath(id), nil, http.StatusOK, &resp)
	return resp, err
}

// CreateTag creates a tag.
func (p *ProjectClient) CreateTag(ctx context.Context, req apiv0.CreateTagRequest) (apiv0.CreateTagResponse, error) {
	var resp apiv0.CreateTagResponse
	err := p.do(ctx, http.MethodPost, "/tags", req, http.StatusCreated, &resp)
	return resp, err
}

// SetTagValue writes a tag's value and quality — the latest write wins, no
// version is stated — and returns the seq the write took. The error matches
// ErrNotFound if the tag is gone.
func (p *ProjectClient) SetTagValue(ctx context.Context, id uuid.UUID, value, quality string) (int64, error) {
	var resp apiv0.UpdateTagResponse
	req := apiv0.UpdateTagRequest{Value: value, Quality: quality}
	if err := p.do(ctx, http.MethodPatch, tagPath(id)+"/value", req, http.StatusOK, &resp); err != nil {
		return 0, err
	}
	return resp.Seq, nil
}

// DeleteTag deletes a tag by ID and returns its last state. The error matches
// ErrNotFound if it is already gone.
func (p *ProjectClient) DeleteTag(ctx context.Context, id uuid.UUID) (apiv0.TagResponse, error) {
	var resp apiv0.TagResponse
	err := p.do(ctx, http.MethodDelete, tagPath(id), nil, http.StatusOK, &resp)
	return resp, err
}

func (p *ProjectClient) getTags(ctx context.Context, path string) ([]apiv0.TagResponse, error) {
	var resp apiv0.GetTagsResponse
	err := p.do(ctx, http.MethodGet, path, nil, http.StatusOK, &resp)
	return resp.Tags, err
}

func tagPath(id uuid.UUID) string { return "/tags/" + id.String() }

// ── Scenes ───────────────────────────────────────────────────────────────────

// ListScenes returns every Scene of the project.
func (p *ProjectClient) ListScenes(ctx context.Context) ([]apiv0.SceneResponse, error) {
	var resp apiv0.GetScenesResponse
	err := p.do(ctx, http.MethodGet, "/scenes", nil, http.StatusOK, &resp)
	return resp.Scenes, err
}

// GetScene returns a Scene by ID. The error matches ErrNotFound if there is
// none.
func (p *ProjectClient) GetScene(ctx context.Context, id uuid.UUID) (apiv0.SceneResponse, error) {
	var resp apiv0.SceneResponse
	err := p.do(ctx, http.MethodGet, scenePath(id), nil, http.StatusOK, &resp)
	return resp, err
}

// CreateScene creates a Scene.
func (p *ProjectClient) CreateScene(ctx context.Context, req apiv0.CreateSceneRequest) (apiv0.CreateSceneResponse, error) {
	var resp apiv0.CreateSceneResponse
	err := p.do(ctx, http.MethodPost, "/scenes", req, http.StatusCreated, &resp)
	return resp, err
}

// UpdateScene replaces a Scene's properties, expecting req.Version to be its
// current version. The error matches ErrConflict if the version is stale.
func (p *ProjectClient) UpdateScene(ctx context.Context, id uuid.UUID, req apiv0.UpdateSceneRequest) (apiv0.UpdateSceneResponse, error) {
	var resp apiv0.UpdateSceneResponse
	err := p.do(ctx, http.MethodPut, scenePath(id), req, http.StatusOK, &resp)
	return resp, err
}

// DeleteScene deletes a Scene with its Widgets and returns its last state.
func (p *ProjectClient) DeleteScene(ctx context.Context, id uuid.UUID) (apiv0.SceneResponse, error) {
	var resp apiv0.SceneResponse
	err := p.do(ctx, http.MethodDelete, scenePath(id), nil, http.StatusOK, &resp)
	return resp, err
}

func scenePath(id uuid.UUID) string { return "/scenes/" + id.String() }

// ── Widgets ──────────────────────────────────────────────────────────────────

// ListWidgets returns the Widgets of a Scene.
func (p *ProjectClient) ListWidgets(ctx context.Context, sceneID uuid.UUID) ([]apiv0.WidgetResponse, error) {
	var resp apiv0.GetWidgetsResponse
	err := p.do(ctx, http.MethodGet, scenePath(sceneID)+"/widgets", nil, http.StatusOK, &resp)
	return resp.Widgets, err
}

// GetWidget returns a Widget of a Scene. The error matches ErrNotFound if
// there is none.
func (p *ProjectClient) GetWidget(ctx context.Context, sceneID, widgetID uuid.UUID) (apiv0.WidgetResponse, error) {
	var resp apiv0.WidgetResponse
	err := p.do(ctx, http.MethodGet, widgetPath(sceneID, widgetID), nil, http.StatusOK, &resp)
	return resp, err
}

// CreateWidget places a Widget on a Scene, expecting req.SceneVersion to be
// the Scene's current version. The error matches ErrConflict if it is stale.
func (p *ProjectClient) CreateWidget(ctx context.Context, sceneID uuid.UUID, req apiv0.CreateWidgetRequest) (apiv0.CreateWidgetResponse, error) {
	var resp apiv0.CreateWidgetResponse
	err := p.do(ctx, http.MethodPost, scenePath(sceneID)+"/widgets", req, http.StatusCreated, &resp)
	return resp, err
}

// UpdateWidget replaces a Widget, expecting req.SceneVersion to be its
// Scene's current version. The error matches ErrConflict if it is stale.
func (p *ProjectClient) UpdateWidget(ctx context.Context, sceneID, widgetID uuid.UUID, req apiv0.UpdateWidgetRequest) (apiv0.UpdateWidgetResponse, error) {
	var resp apiv0.UpdateWidgetResponse
	err := p.do(ctx, http.MethodPut, widgetPath(sceneID, widgetID), req, http.StatusOK, &resp)
	return resp, err
}

// DeleteWidget removes a Widget from its Scene and returns its last state,
// with the Scene's new version.
func (p *ProjectClient) DeleteWidget(ctx context.Context, sceneID, widgetID uuid.UUID) (apiv0.WidgetResponse, error) {
	var resp apiv0.WidgetResponse
	err := p.do(ctx, http.MethodDelete, widgetPath(sceneID, widgetID), nil, http.StatusOK, &resp)
	return resp, err
}

func widgetPath(sceneID, widgetID uuid.UUID) string {
	return scenePath(sceneID) + "/widgets/" + widgetID.String()
}

// ── Transport ────────────────────────────────────────────────────────────────

// maxProblemBody caps how much of an error response is read.
const maxProblemBody = 64 << 10

// maxDrain caps how much of a response's rest is read before its body is
// closed: a body read to EOF lets the transport reuse the connection, a larger
// rest is cheaper to drop with the connection.
const maxDrain = 64 << 10

// maxFallbackDetail caps the Detail taken from an error body that is not
// Problem Details (e.g. a proxy's HTML page).
const maxFallbackDetail = 200

// do sends a request to path under apiv0.PathPrefix — the body, if not nil,
// marshalled as JSON — and decodes a successful response into out (if not
// nil). Any other status is a *StatusError carrying the Problem Details.
// A DELETE's status alone tells it succeeded: a body that does not decode
// (empty, or e.g. a proxy's "OK") is no error, out then being left zero —
// the last state in out is only extra information, and a caller reads a zero
// value as "unknown". A request that cannot be built (e.g. a malformed baseURL) is an
// error that is not a *url.Error: nothing was sent.
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
		// %v, not %w: the *url.Error inside would read as a network failure.
		return fmt.Errorf("%s %s: cannot build request: %v", method, path, err)
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
	defer drainAndClose(resp.Body)
	c.reportServerVersion(resp)

	if resp.StatusCode != wantStatus {
		return newStatusError(req, resp)
	}
	if out == nil {
		return nil
	}
	// Tolerant: a newer server may add fields (ADR 0005).
	err = json.NewDecoder(resp.Body).Decode(out)
	switch {
	case err == nil:
		return nil
	case method == http.MethodDelete:
		// A body cut short may have filled out in part.
		reflect.ValueOf(out).Elem().SetZero()
		return nil
	default:
		return fmt.Errorf("%s %s: %w: %w", method, req.URL, ErrBadResponse, err)
	}
}

// drainAndClose reads what is left of body and closes it, so the connection
// can be reused: json.Decoder stops at the end of the value, leaving the rest
// (the encoder's newline, a chunked body's last chunk) unread.
func drainAndClose(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maxDrain))
	_ = body.Close()
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
// (e.g. an HTML page or a JSON error of a proxy) gives a Problem titled by
// the status, with the start of the body as its detail.
func newStatusError(req *http.Request, resp *http.Response) *StatusError {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxProblemBody))
	var problem apiv0.Problem
	if err := json.Unmarshal(data, &problem); err != nil || (problem.Title == "" && problem.Detail == "") {
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
