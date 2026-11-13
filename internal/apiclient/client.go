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
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	recordv0 "github.com/kipitix/growscada/contract/record/v0"
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
	// deviceToken, when set, is sent as Authorization: Bearer <token> on every
	// request: the client acts as the Device the token names.
	deviceToken string
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

// WithDeviceToken returns a client of the same server whose every request
// authenticates as the Device the Device Token names
// (Authorization: Bearer gsc_dev_…). An empty token is no token: the returned
// client sends none. The receiver is left as it is, so the UI and growctl can
// share one client with the Gateway's.
func (c *Client) WithDeviceToken(token string) *Client {
	gateway := *c
	gateway.deviceToken = token
	return &gateway
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

// doOneOf is do for a call that may answer with one of several statuses: the
// value of out is the same for each.
func (p *ProjectClient) doOneOf(ctx context.Context, method, path string, body any, out any, wantStatuses ...int) error {
	return p.c.doOneOf(ctx, method, p.prefix+path, body, out, wantStatuses...)
}

// ── Per-user settings ────────────────────────────────────────────────────────

// LibraryOrder returns the library order of the person the server acts for
// (task 81): the Version the next write states, and the IDs of the libraries
// they placed, first to last. Version 0 with no ID is a person who never
// reordered.
func (c *Client) LibraryOrder(ctx context.Context) (apiv0.LibraryOrderResponse, error) {
	var resp apiv0.LibraryOrderResponse
	err := c.do(ctx, http.MethodGet, apiv0.LibraryOrderPath, nil, http.StatusOK, &resp)
	return resp, err
}

// SetLibraryOrder stores the whole library order, stating the Version it was
// read at (0 when there was none). The error matches ErrConflict when another
// write came first: the newer order is never overwritten silently.
func (c *Client) SetLibraryOrder(ctx context.Context, version int, libraryIDs []uuid.UUID) (apiv0.LibraryOrderResponse, error) {
	// The request states library_ids as an array: a nil slice would marshal to
	// null, which the schema forbids and the server refuses.
	if libraryIDs == nil {
		libraryIDs = []uuid.UUID{}
	}
	var resp apiv0.LibraryOrderResponse
	err := c.do(ctx, http.MethodPut, apiv0.LibraryOrderPath, apiv0.SetLibraryOrderRequest{
		Version:    version,
		LibraryIDs: libraryIDs,
	}, http.StatusOK, &resp)
	return resp, err
}

// ProjectOrder returns the project order of the person the server acts for
// (task 83): the IDs they placed, first to last, and the Version their next
// write states. A person who never reordered is Version 0 and no ID.
func (c *Client) ProjectOrder(ctx context.Context) (apiv0.ProjectOrderResponse, error) {
	var resp apiv0.ProjectOrderResponse
	err := c.do(ctx, http.MethodGet, apiv0.ProjectOrderPath, nil, http.StatusOK, &resp)
	return resp, err
}

// SetProjectOrder stores the whole project order, stating the Version it was
// read at (0 when there was none), and returns the Version stored (task 83).
// A stale Version is ErrConflict.
func (c *Client) SetProjectOrder(ctx context.Context, version int, projectIDs []uuid.UUID) (apiv0.ProjectOrderResponse, error) {
	var resp apiv0.ProjectOrderResponse
	err := c.do(ctx, http.MethodPut, apiv0.ProjectOrderPath, apiv0.SetProjectOrderRequest{
		Version:    version,
		ProjectIDs: projectIDs,
	}, http.StatusOK, &resp)
	return resp, err
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

// doOneOf is do for a call that may answer with one of several statuses: the
// value of out is the same for each.
func (l *LibraryClient) doOneOf(ctx context.Context, method, path string, body any, out any, wantStatuses ...int) error {
	return l.c.doOneOf(ctx, method, l.prefix+path, body, out, wantStatuses...)
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

// DraftState returns the Version of the library's Draft: what every write of
// its content states and every DTO of it carries (ADR 0016).
func (l *LibraryClient) DraftState(ctx context.Context) (apiv0.DraftStateResponse, error) {
	var resp apiv0.DraftStateResponse
	err := l.do(ctx, http.MethodGet, apiv0.DraftPath, nil, http.StatusOK, &resp)
	return resp, err
}

// DraftChanges returns the state of the library's DraftChange stack (ADR
// 0017), as the project's does.
func (l *LibraryClient) DraftChanges(ctx context.Context) (apiv0.DraftChangesResponse, error) {
	var resp apiv0.DraftChangesResponse
	err := l.do(ctx, http.MethodGet, apiv0.DraftChangesPath, nil, http.StatusOK, &resp)
	return resp, err
}

// UndoDraftChange undoes the change at the stack's position, stating version
// (0: the Version of the Draft the client reads now).
func (l *LibraryClient) UndoDraftChange(ctx context.Context, version int) (apiv0.DraftChangesResponse, error) {
	return l.draftChangeCommand(ctx, apiv0.DraftChangesUndoPath, version)
}

// RedoDraftChange redoes the change after the stack's position.
func (l *LibraryClient) RedoDraftChange(ctx context.Context, version int) (apiv0.DraftChangesResponse, error) {
	return l.draftChangeCommand(ctx, apiv0.DraftChangesRedoPath, version)
}

func (l *LibraryClient) draftChangeCommand(ctx context.Context, path string, stated int) (apiv0.DraftChangesResponse, error) {
	var resp apiv0.DraftChangesResponse
	version, err := l.draftVersionOf(ctx, stated)
	if err != nil {
		return resp, err
	}
	err = l.do(ctx, http.MethodPost, path, apiv0.DraftChangeCommandRequest{Version: version}, http.StatusOK, &resp)
	return resp, err
}

// draftVersionOf returns stated, or the Version of the library's Draft when
// the client states none.
func (l *LibraryClient) draftVersionOf(ctx context.Context, stated int) (int, error) {
	if stated != 0 {
		return stated, nil
	}
	state, err := l.DraftState(ctx)
	if err != nil {
		return 0, err
	}
	return state.Version, nil
}

// CreateWidgetType creates a WidgetType in the Draft. The error matches
// ErrConflict if its name is taken in the library or req.Version is stale.
func (l *LibraryClient) CreateWidgetType(ctx context.Context, req apiv0.CreateWidgetTypeRequest) (apiv0.CreateWidgetTypeResponse, error) {
	var resp apiv0.CreateWidgetTypeResponse
	version, err := l.draftVersionOf(ctx, req.Version)
	if err != nil {
		return resp, err
	}
	req.Version = version
	err = l.do(ctx, http.MethodPost, "/widget-types", req, http.StatusCreated, &resp)
	return resp, err
}

// UpdateWidgetType replaces a WidgetType of the Draft, expecting req.Version
// to be the Version of the Draft the client saw. The error matches
// ErrConflict if the version is stale or the name taken.
func (l *LibraryClient) UpdateWidgetType(ctx context.Context, id uuid.UUID, req apiv0.UpdateWidgetTypeRequest) (apiv0.UpdateWidgetTypeResponse, error) {
	var resp apiv0.UpdateWidgetTypeResponse
	version, err := l.draftVersionOf(ctx, req.Version)
	if err != nil {
		return resp, err
	}
	req.Version = version
	err = l.do(ctx, http.MethodPut, widgetTypePath(id), req, http.StatusOK, &resp)
	return resp, err
}

// DeleteWidgetType removes a WidgetType from the Draft, stating the Version of
// the Draft the client saw, and returns its last state. It touches no project:
// they use the releases they pin.
func (l *LibraryClient) DeleteWidgetType(ctx context.Context, id uuid.UUID, version int) (apiv0.WidgetTypeResponse, error) {
	var resp apiv0.WidgetTypeResponse
	version, err := l.draftVersionOf(ctx, version)
	if err != nil {
		return resp, err
	}
	err = l.do(ctx, http.MethodDelete, withVersion(widgetTypePath(id), version), nil, http.StatusOK, &resp)
	return resp, err
}

func widgetTypePath(id uuid.UUID) string { return "/widget-types/" + id.String() }

// Release publishes a Commit of the library's Draft as the next Library
// Release (task 72_5; ADR 0021). The error matches ErrConflict if the Commit
// is released already and ErrNotFound for a library or a Commit it does not
// have.
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

// UpdateReleaseNote changes the note of a Library Release (task 72_5). It
// states no Draft Version: the content never changes.
func (l *LibraryClient) UpdateReleaseNote(ctx context.Context, n int, req apiv0.UpdateLibraryReleaseNoteRequest) (apiv0.LibraryReleaseResponse, error) {
	var resp apiv0.LibraryReleaseResponse
	err := l.do(ctx, http.MethodPatch, "/releases/"+strconv.Itoa(n), req, http.StatusOK, &resp)
	return resp, err
}

// DraftState returns the Version of the project's Draft: what every content
// write states and every content DTO carries (ADR 0016).
func (p *ProjectClient) DraftState(ctx context.Context) (apiv0.DraftStateResponse, error) {
	var resp apiv0.DraftStateResponse
	err := p.do(ctx, http.MethodGet, apiv0.DraftPath, nil, http.StatusOK, &resp)
	return resp, err
}

// DraftChanges returns the state of the project's DraftChange stack (ADR 0017):
// its Version, whether an undo and a redo are possible and the description of
// each step.
func (p *ProjectClient) DraftChanges(ctx context.Context) (apiv0.DraftChangesResponse, error) {
	var resp apiv0.DraftChangesResponse
	err := p.do(ctx, http.MethodGet, apiv0.DraftChangesPath, nil, http.StatusOK, &resp)
	return resp, err
}

// UndoDraftChange undoes the change at the stack's position and returns the
// state after it, stating version (0: the Version of the Draft the client
// reads now). The error matches ErrConflict for an empty stack or a stale
// Version.
func (p *ProjectClient) UndoDraftChange(ctx context.Context, version int) (apiv0.DraftChangesResponse, error) {
	return p.draftChangeCommand(ctx, apiv0.DraftChangesUndoPath, version)
}

// RedoDraftChange redoes the change after the stack's position.
func (p *ProjectClient) RedoDraftChange(ctx context.Context, version int) (apiv0.DraftChangesResponse, error) {
	return p.draftChangeCommand(ctx, apiv0.DraftChangesRedoPath, version)
}

func (p *ProjectClient) draftChangeCommand(ctx context.Context, path string, stated int) (apiv0.DraftChangesResponse, error) {
	var resp apiv0.DraftChangesResponse
	version, err := p.draftVersionOf(ctx, stated)
	if err != nil {
		return resp, err
	}
	err = p.do(ctx, http.MethodPost, path, apiv0.DraftChangeCommandRequest{Version: version}, http.StatusOK, &resp)
	return resp, err
}

// Discard resets the library's Draft to its last Commit in one transaction and
// returns the Draft's new state (task 72_5). The error matches ErrConflict if
// the library has no Commit.
func (l *LibraryClient) Discard(ctx context.Context) (apiv0.DiscardLibraryResponse, error) {
	var resp apiv0.DiscardLibraryResponse
	err := l.do(ctx, http.MethodPost, "/discard", nil, http.StatusOK, &resp)
	return resp, err
}

// RevertToRelease resets the library's Draft to its latest Release in one
// transaction and returns the Draft's new state (task 72_5). The error matches
// ErrConflict if the library was never released.
func (l *LibraryClient) RevertToRelease(ctx context.Context) (apiv0.RevertLibraryResponse, error) {
	var resp apiv0.RevertLibraryResponse
	err := l.do(ctx, http.MethodPost, apiv0.RevertToReleasePath, nil, http.StatusOK, &resp)
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

// PinLibrary pins release req.Release of the library, adding the pin or
// switching it; req.Version is the Version of the Draft the client saw. The
// error matches ErrConflict if the version is stale or the switch would leave
// Widgets without their WidgetType (the StatusError's Problem names them).
func (p *ProjectClient) PinLibrary(ctx context.Context, libraryID uuid.UUID, req apiv0.PutLibraryPinRequest) (apiv0.LibraryPinResponse, error) {
	var resp apiv0.LibraryPinResponse
	version, err := p.draftVersionOf(ctx, req.Version)
	if err != nil {
		return resp, err
	}
	req.Version = version
	err = p.doOneOf(ctx, http.MethodPut, libraryPinPath(libraryID), req, &resp, http.StatusOK, http.StatusCreated)
	return resp, err
}

// UnpinLibrary removes the pin of the library, stating the Version of the
// Draft the client saw. The error matches ErrConflict while Widgets of the
// project are of its WidgetTypes.
func (p *ProjectClient) UnpinLibrary(ctx context.Context, libraryID uuid.UUID, version int) (apiv0.LibraryPinResponse, error) {
	var resp apiv0.LibraryPinResponse
	version, err := p.draftVersionOf(ctx, version)
	if err != nil {
		return resp, err
	}
	err = p.do(ctx, http.MethodDelete, withVersion(libraryPinPath(libraryID), version), nil, http.StatusOK, &resp)
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
	version, err := p.draftVersionOf(ctx, req.Version)
	if err != nil {
		return resp, err
	}
	req.Version = version
	err = p.do(ctx, http.MethodPost, "/tags", req, http.StatusCreated, &resp)
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

// DeleteTag deletes a tag by ID, stating the Version of the Draft the client
// saw, and returns its last state. The error matches ErrNotFound if it is
// already gone, ErrConflict if the version is stale.
func (p *ProjectClient) DeleteTag(ctx context.Context, id uuid.UUID, version int) (apiv0.TagResponse, error) {
	var resp apiv0.TagResponse
	version, err := p.draftVersionOf(ctx, version)
	if err != nil {
		return resp, err
	}
	err = p.do(ctx, http.MethodDelete, withVersion(tagPath(id), version), nil, http.StatusOK, &resp)
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
	version, err := p.draftVersionOf(ctx, req.Version)
	if err != nil {
		return resp, err
	}
	req.Version = version
	err = p.do(ctx, http.MethodPost, "/scenes", req, http.StatusCreated, &resp)
	return resp, err
}

// UpdateScene replaces a Scene's properties, expecting req.Version to be its
// current version. The error matches ErrConflict if the version is stale.
func (p *ProjectClient) UpdateScene(ctx context.Context, id uuid.UUID, req apiv0.UpdateSceneRequest) (apiv0.UpdateSceneResponse, error) {
	var resp apiv0.UpdateSceneResponse
	version, err := p.draftVersionOf(ctx, req.Version)
	if err != nil {
		return resp, err
	}
	req.Version = version
	err = p.do(ctx, http.MethodPut, scenePath(id), req, http.StatusOK, &resp)
	return resp, err
}

// DeleteScene deletes a Scene with its Widgets, stating the Version of the
// Draft the client saw, and returns its last state.
func (p *ProjectClient) DeleteScene(ctx context.Context, id uuid.UUID, version int) (apiv0.SceneResponse, error) {
	var resp apiv0.SceneResponse
	version, err := p.draftVersionOf(ctx, version)
	if err != nil {
		return resp, err
	}
	err = p.do(ctx, http.MethodDelete, withVersion(scenePath(id), version), nil, http.StatusOK, &resp)
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

// CreateWidget places a Widget on a Scene, expecting req.Version to be the
// Draft's current Version. The error matches ErrConflict if it is stale.
func (p *ProjectClient) CreateWidget(ctx context.Context, sceneID uuid.UUID, req apiv0.CreateWidgetRequest) (apiv0.CreateWidgetResponse, error) {
	var resp apiv0.CreateWidgetResponse
	version, err := p.draftVersionOf(ctx, req.Version)
	if err != nil {
		return resp, err
	}
	req.Version = version
	err = p.do(ctx, http.MethodPost, scenePath(sceneID)+"/widgets", req, http.StatusCreated, &resp)
	return resp, err
}

// UpdateWidget replaces a Widget, expecting req.Version to be the Draft's
// current Version. The error matches ErrConflict if it is stale.
func (p *ProjectClient) UpdateWidget(ctx context.Context, sceneID, widgetID uuid.UUID, req apiv0.UpdateWidgetRequest) (apiv0.UpdateWidgetResponse, error) {
	var resp apiv0.UpdateWidgetResponse
	version, err := p.draftVersionOf(ctx, req.Version)
	if err != nil {
		return resp, err
	}
	req.Version = version
	err = p.do(ctx, http.MethodPut, widgetPath(sceneID, widgetID), req, http.StatusOK, &resp)
	return resp, err
}

// DeleteWidget removes a Widget from its Scene, stating the Version of the
// Draft the client saw, and returns its last state.
func (p *ProjectClient) DeleteWidget(ctx context.Context, sceneID, widgetID uuid.UUID, version int) (apiv0.WidgetResponse, error) {
	var resp apiv0.WidgetResponse
	version, err := p.draftVersionOf(ctx, version)
	if err != nil {
		return resp, err
	}
	err = p.do(ctx, http.MethodDelete, withVersion(widgetPath(sceneID, widgetID), version), nil, http.StatusOK, &resp)
	return resp, err
}

// MoveWidgets moves several Widgets of one Scene at once — one change of the
// Draft (ADR 0010, ADR 0017) — expecting req.Version to be the Draft's current
// Version. The answer is the Version after the change; the error matches
// ErrConflict if the stated Version is stale.
func (p *ProjectClient) MoveWidgets(ctx context.Context, sceneID uuid.UUID, req apiv0.MoveWidgetsRequest) (apiv0.MoveWidgetsResponse, error) {
	var resp apiv0.MoveWidgetsResponse
	version, err := p.draftVersionOf(ctx, req.Version)
	if err != nil {
		return resp, err
	}
	req.Version = version
	err = p.do(ctx, http.MethodPatch, scenePath(sceneID)+"/widgets", req, http.StatusOK, &resp)
	return resp, err
}

// withVersion states the Version of the Draft on a DELETE (ADR 0016).
func withVersion(path string, version int) string {
	return path + "?" + apiv0.VersionQuery + "=" + strconv.Itoa(version)
}

// draftVersionOf returns the Version a write states: the stated one, or the
// Draft's current one when the caller stated none (0). A caller that read
// nothing of the Draft yet — an empty Project — states none, and the write
// takes the version it read a moment before.
func (p *ProjectClient) draftVersionOf(ctx context.Context, stated int) (int, error) {
	if stated != 0 {
		return stated, nil
	}
	state, err := p.DraftState(ctx)
	if err != nil {
		return 0, err
	}
	return state.Version, nil
}

func widgetPath(sceneID, widgetID uuid.UUID) string {
	return scenePath(sceneID) + "/widgets/" + widgetID.String()
}

// ── Commits ───────────────────────────────────────────────────────────────────

// SaveCommit fixes the Draft as a Commit at the stated Version (task 72_2;
// ADR 0021). The error matches ErrConflict for nothing_to_save and for a stale
// draft_conflict; an empty title or author is a 400.
func (p *ProjectClient) SaveCommit(ctx context.Context, req apiv0.CreateCommitRequest) (apiv0.CommitResponse, error) {
	var resp apiv0.CommitResponse
	err := p.do(ctx, http.MethodPost, apiv0.CommitPath, req, http.StatusCreated, &resp)
	return resp, err
}

// Commits returns the Project's Commit history, newest first, without content.
func (p *ProjectClient) Commits(ctx context.Context) ([]apiv0.CommitResponse, error) {
	var resp apiv0.GetCommitsResponse
	err := p.do(ctx, http.MethodGet, apiv0.CommitPath, nil, http.StatusOK, &resp)
	return resp.Commits, err
}

// Commit returns one Commit. A number the Project does not have matches
// ErrNotFound.
func (p *ProjectClient) Commit(ctx context.Context, number int) (apiv0.CommitResponse, error) {
	var resp apiv0.CommitResponse
	err := p.do(ctx, http.MethodGet, apiv0.CommitPathOf(number), nil, http.StatusOK, &resp)
	return resp, err
}

// RenameCommit changes a Commit's title. It states no Draft Version.
func (p *ProjectClient) RenameCommit(ctx context.Context, number int, req apiv0.RenameCommitRequest) (apiv0.CommitResponse, error) {
	var resp apiv0.CommitResponse
	err := p.do(ctx, http.MethodPatch, apiv0.CommitPathOf(number), req, http.StatusOK, &resp)
	return resp, err
}

// CommitProjectFile returns the ProjectFile a Commit fixed, as a download.
// The error matches ErrNotFound for a number the Project does not have.
func (p *ProjectClient) CommitProjectFile(ctx context.Context, number int) (ProjectFileDownload, error) {
	return p.downloadProjectFile(ctx, apiv0.CommitPathOf(number)+"/projectfile")
}

// CreateRelease publishes a Commit as the next Release of the Project (task
// 72_3; ADR 0021). The error matches ErrConflict if the Commit is released
// already and ErrNotFound for a Project or a Commit it does not have.
func (p *ProjectClient) CreateRelease(ctx context.Context, req apiv0.CreateProjectReleaseRequest) (apiv0.ReleaseResponse, error) {
	var resp apiv0.ReleaseResponse
	err := p.do(ctx, http.MethodPost, apiv0.ReleasePath, req, http.StatusCreated, &resp)
	return resp, err
}

// Releases returns the Project's Releases, newest first, without content.
func (p *ProjectClient) Releases(ctx context.Context) ([]apiv0.ReleaseResponse, error) {
	var resp apiv0.GetReleasesResponse
	err := p.do(ctx, http.MethodGet, apiv0.ReleasePath, nil, http.StatusOK, &resp)
	return resp.Releases, err
}

// Release returns one Release. A number the Project does not have matches
// ErrNotFound.
func (p *ProjectClient) Release(ctx context.Context, number int) (apiv0.ReleaseResponse, error) {
	var resp apiv0.ReleaseResponse
	err := p.do(ctx, http.MethodGet, apiv0.ReleasePathOf(number), nil, http.StatusOK, &resp)
	return resp, err
}

// UpdateReleaseNote changes a Release's note. It states no Draft Version.
func (p *ProjectClient) UpdateReleaseNote(ctx context.Context, number int, req apiv0.UpdateReleaseNoteRequest) (apiv0.ReleaseResponse, error) {
	var resp apiv0.ReleaseResponse
	err := p.do(ctx, http.MethodPatch, apiv0.ReleasePathOf(number), req, http.StatusOK, &resp)
	return resp, err
}

// ReleaseProjectFile returns the ProjectFile a Release copied, as a download.
// The error matches ErrNotFound for a number the Project does not have.
func (p *ProjectClient) ReleaseProjectFile(ctx context.Context, number int) (ProjectFileDownload, error) {
	return p.downloadProjectFile(ctx, apiv0.ReleaseProjectFilePathOf(number))
}

// SaveCommit fixes the Library's Draft as a Commit at the stated Version
// (task 72_2; ADR 0021).
func (l *LibraryClient) SaveCommit(ctx context.Context, req apiv0.CreateCommitRequest) (apiv0.CommitResponse, error) {
	var resp apiv0.CommitResponse
	err := l.do(ctx, http.MethodPost, apiv0.CommitPath, req, http.StatusCreated, &resp)
	return resp, err
}

// Commits returns the Library's Commit history, newest first, without
// content.
func (l *LibraryClient) Commits(ctx context.Context) ([]apiv0.CommitResponse, error) {
	var resp apiv0.GetCommitsResponse
	err := l.do(ctx, http.MethodGet, apiv0.CommitPath, nil, http.StatusOK, &resp)
	return resp.Commits, err
}

// Commit returns one Commit, with the WidgetTypes of its snapshot.
func (l *LibraryClient) Commit(ctx context.Context, number int) (apiv0.CommitResponse, error) {
	var resp apiv0.CommitResponse
	err := l.do(ctx, http.MethodGet, apiv0.CommitPathOf(number), nil, http.StatusOK, &resp)
	return resp, err
}

// RenameCommit changes a Commit's title. It states no Draft Version.
func (l *LibraryClient) RenameCommit(ctx context.Context, number int, req apiv0.RenameCommitRequest) (apiv0.CommitResponse, error) {
	var resp apiv0.CommitResponse
	err := l.do(ctx, http.MethodPatch, apiv0.CommitPathOf(number), req, http.StatusOK, &resp)
	return resp, err
}

// ── Deploy and Deployment ──────────────────────────────────────────────────────

// DeploymentState is one Project's running Deployment, with its ProjectFile.
// Deployed is false until the Project's first Deploy: the call then matched a
// 404, which here means "no Deployment", not "no Project" (a Project that is
// gone makes the loads of its content fail as well).
type DeploymentState struct {
	Deployment apiv0.DeploymentResponse
	Deployed   bool
	// ReleaseNumber is the Project Release the running Deployment made; nil
	// for a Deployment from a file or one made before Releases existed
	// (task 72_4).
	ReleaseNumber *int
}

// Deployment returns the Deployment the project runs. Deployed is false when the
// project was never deployed; an error matching ErrNotFound is not used for
// that.
func (p *ProjectClient) Deployment(ctx context.Context) (DeploymentState, error) {
	var resp apiv0.DeploymentResponse
	err := p.do(ctx, http.MethodGet, "/deployment", nil, http.StatusOK, &resp)
	if errors.Is(err, ErrNotFound) {
		return DeploymentState{}, nil
	}
	return DeploymentState{Deployment: resp, Deployed: err == nil, ReleaseNumber: resp.ReleaseNumber}, err
}

// TagStates returns the states of the Tags of the running Deployment, by Tag ID:
// the snapshot of a reader that does not follow the Journal, since Operation
// reads its starting state from …/operation (ADR 0020).
func (p *ProjectClient) TagStates(ctx context.Context) (map[string]apiv0.TagStateResponse, error) {
	var resp apiv0.GetTagStatesResponse
	err := p.do(ctx, http.MethodGet, "/tag-states", nil, http.StatusOK, &resp)
	return resp.TagStates, err
}

// Operation returns the starting state of Operation (task 43, ADR 0020): the
// Project's Journal position, the Deployment it runs (nil until the first
// Deploy) and the states of that Deployment's Tags by ID. A live Project answers
// even before its first Deploy, so a nil Deployment is not an error; a Project
// that is gone matches ErrNotFound.
func (p *ProjectClient) Operation(ctx context.Context) (apiv0.OperationStateResponse, error) {
	var resp apiv0.OperationStateResponse
	err := p.do(ctx, http.MethodGet, apiv0.OperationPath, nil, http.StatusOK, &resp)
	return resp, err
}

// OperationAt returns the starting state of Operation at a past Journal
// position (task 44, ADR 0023): the nearest Checkpoint at or before position,
// its Deployment and its Tag states, with the Checkpoint's seq in Position —
// the moment History reads the Journal from. A position beyond the Project's
// current one is clamped to it, before the first Checkpoint the answer is the
// empty state at position 0, and a negative position is a 400 (*StatusError).
// A Project that is gone matches ErrNotFound.
func (p *ProjectClient) OperationAt(ctx context.Context, position int64) (apiv0.OperationStateResponse, error) {
	var resp apiv0.OperationStateResponse
	query := url.Values{apiv0.OperationQueryAt: {strconv.FormatInt(position, 10)}}
	err := p.do(ctx, http.MethodGet, apiv0.OperationPath+"?"+query.Encode(), nil, http.StatusOK, &resp)
	return resp, err
}

// Journal returns the Project's Journal entries after the position from, in
// the order of their positions, at most limit of them (task 42, ADR 0019).
// from is exclusive; a Project that is not live matches ErrNotFound.
func (p *ProjectClient) Journal(ctx context.Context, from int64, limit int) ([]recordv0.Entry, error) {
	var resp apiv0.JournalResponse
	query := url.Values{
		apiv0.JournalQueryFrom:  {strconv.FormatInt(from, 10)},
		apiv0.JournalQueryLimit: {strconv.Itoa(limit)},
	}
	err := p.do(ctx, http.MethodGet, apiv0.JournalPath+"?"+query.Encode(), nil, http.StatusOK, &resp)
	return resp.Entries, err
}

// DeployRelease makes Release number the Project's running Deployment (task
// 72_4). The error matches ErrConflict if that Release runs already and
// ErrNotFound for a Release the Project does not have.
func (p *ProjectClient) DeployRelease(ctx context.Context, number int) (apiv0.DeployResponse, error) {
	var resp apiv0.DeployResponse
	err := p.do(ctx, http.MethodPost, apiv0.ReleaseDeployPathOf(number), nil, http.StatusCreated, &resp)
	return resp, err
}

// Discard resets the Draft to the Project's last Commit, dropping the unsaved
// changes, and returns its answer at once (task 72_4). The error matches
// ErrConflict if the Project has no Commit.
func (p *ProjectClient) Discard(ctx context.Context) (apiv0.DiscardProjectResponse, error) {
	var resp apiv0.DiscardProjectResponse
	err := p.do(ctx, http.MethodPost, "/discard", nil, http.StatusOK, &resp)
	return resp, err
}

// RevertToDeployed resets the Draft to the Deployment the Project runs and
// returns its answer (task 72_4). The error matches ErrConflict if the Project
// runs no Deployment.
func (p *ProjectClient) RevertToDeployed(ctx context.Context) (apiv0.DiscardProjectResponse, error) {
	var resp apiv0.DiscardProjectResponse
	err := p.do(ctx, http.MethodPost, apiv0.RevertToDeployedPath, nil, http.StatusOK, &resp)
	return resp, err
}

// ProjectFileDownload is a ProjectFile the server sends as an attachment: the
// document as it is and the name Content-Disposition gives the file (ADR 0018).
type ProjectFileDownload struct {
	Data     []byte
	Filename string
}

// DraftProjectFile returns the Draft of the Project as a ProjectFile. The
// error matches ErrConflict when the Draft is not deployable.
func (p *ProjectClient) DraftProjectFile(ctx context.Context) (ProjectFileDownload, error) {
	return p.downloadProjectFile(ctx, "/projectfile")
}

// DeploymentProjectFile returns the ProjectFile the Runtime node stored for
// Deployment number. The error matches ErrNotFound if the Project has no such
// Deployment.
func (p *ProjectClient) DeploymentProjectFile(ctx context.Context, number int) (ProjectFileDownload, error) {
	return p.downloadProjectFile(ctx, deploymentPath(number)+"/projectfile")
}

func (p *ProjectClient) downloadProjectFile(ctx context.Context, path string) (ProjectFileDownload, error) {
	data, header, err := p.c.doBytes(ctx, http.MethodGet, p.prefix+path, nil, http.StatusOK)
	if err != nil {
		return ProjectFileDownload{}, err
	}
	return ProjectFileDownload{Data: data, Filename: filenameOf(header)}, nil
}

// OpenProjectFile replaces the whole Draft with document, stating the Version
// the client saw: one DraftChange (ADR 0018). The error matches ErrConflict
// for nothing_to_open and carries the file's problems in its Problem when the
// document does not validate.
func (p *ProjectClient) OpenProjectFile(ctx context.Context, version int, document []byte) (apiv0.DraftStateResponse, error) {
	var resp apiv0.DraftStateResponse
	err := p.doBytes(ctx, http.MethodPost,
		"/projectfile?"+apiv0.VersionQuery+"="+strconv.Itoa(version), document, http.StatusOK, &resp)
	return resp, err
}

// DeployProjectFile makes document the Project's new Deployment without the
// Draft (ADR 0018). The error matches ErrConflict when the document is the
// running Deployment's.
func (p *ProjectClient) DeployProjectFile(ctx context.Context, document []byte) (apiv0.DeployResponse, error) {
	var resp apiv0.DeployResponse
	err := p.doBytes(ctx, http.MethodPost, "/projectfile/deploy", document, http.StatusCreated, &resp)
	return resp, err
}

func (p *ProjectClient) doBytes(ctx context.Context, method, path string, body []byte, wantStatus int, out any) error {
	data, _, err := p.c.doBytes(ctx, method, p.prefix+path, body, wantStatus)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s %s: %w: %w", method, path, ErrBadResponse, err)
	}
	return nil
}

// filenameOf reads the filename of a Content-Disposition attachment, the
// only form the server sends; an absent or different header has no filename.
func filenameOf(header http.Header) string {
	value := header.Get("Content-Disposition")
	const marker = `filename="`
	index := strings.Index(value, marker)
	if index < 0 {
		return ""
	}
	rest := value[index+len(marker):]
	if end := strings.Index(rest, `"`); end >= 0 {
		return rest[:end]
	}
	return ""
}

// Deployments returns the Deployments of the Project, newest first, without their
// documents; a Project without a Deployment has an empty list.
func (p *ProjectClient) Deployments(ctx context.Context) ([]apiv0.DeploymentSummaryResponse, error) {
	var resp apiv0.ListDeploymentsResponse
	err := p.do(ctx, http.MethodGet, "/deployments", nil, http.StatusOK, &resp)
	return resp.Deployments, err
}

// DeploymentByNumber returns one Deployment with its ProjectFile. The error
// matches ErrNotFound if the Project has no such Deployment.
func (p *ProjectClient) DeploymentByNumber(ctx context.Context, number int) (apiv0.DeploymentResponse, error) {
	var resp apiv0.DeploymentResponse
	err := p.do(ctx, http.MethodGet, deploymentPath(number), nil, http.StatusOK, &resp)
	return resp, err
}

// RollbackDeployment makes the Deployment's document the Project's new
// Deployment and returns its summary. The error matches ErrConflict if its
// document is the running one or a Discard is under way, ErrNotFound if the
// Project has no such Deployment.
func (p *ProjectClient) RollbackDeployment(ctx context.Context, number int) (apiv0.DeploymentSummaryResponse, error) {
	var resp apiv0.DeploymentSummaryResponse
	err := p.do(ctx, http.MethodPost, deploymentPath(number)+"/rollback", nil, http.StatusCreated, &resp)
	return resp, err
}

func deploymentPath(number int) string { return "/deployments/" + strconv.Itoa(number) }

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
	return c.doOneOf(ctx, method, path, body, out, wantStatus)
}

// doOneOf is do for a response that may come with one of several statuses.
func (c *Client) doOneOf(ctx context.Context, method, path string, body any, out any, wantStatuses ...int) error {
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
	c.authenticate(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err // *url.Error already names the method and URL
	}
	defer drainAndClose(resp.Body)
	c.reportServerVersion(resp)

	if !slices.Contains(wantStatuses, resp.StatusCode) {
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

// doBytes sends a request whose body is already encoded — a project file as
// it came — and returns the successful response's body and headers. It is the
// download side of a ProjectFile (ADR 0018); an error is a *StatusError with
// the Problem Details, its file_issues included.
func (c *Client) doBytes(ctx context.Context, method, path string, body []byte, wantStatus int) ([]byte, http.Header, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+apiv0.PathPrefix+path, reader)
	if err != nil {
		return nil, nil, fmt.Errorf("%s %s: cannot build request: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(contract.SchemaVersionHeader, apiv0.SchemaVersion().String())
	}
	c.authenticate(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer drainAndClose(resp.Body)
	c.reportServerVersion(resp)
	if resp.StatusCode != wantStatus {
		return nil, nil, newStatusError(req, resp)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.Header, fmt.Errorf("%s %s: %w: %w", method, req.URL, ErrBadResponse, err)
	}
	return data, resp.Header, nil
}

// drainAndClose reads what is left of body and closes it, so the connection
// can be reused: json.Decoder stops at the end of the value, leaving the rest
// (the encoder's newline, a chunked body's last chunk) unread.
func drainAndClose(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maxDrain))
	_ = body.Close()
}

// authenticate adds the Device Token of the client, if it has one.
func (c *Client) authenticate(req *http.Request) {
	if c.deviceToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.deviceToken)
	}
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
