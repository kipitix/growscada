package restapi

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/server/application"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// resource is a REST resource as the client sees it: its name from the
// glossary and the path parameter that carries its ID in the router's
// patterns (wildcard), named like the API's fields since it appears in 400
// details.
type resource struct {
	name      string
	pathParam string
}

var (
	projectResource    = resource{name: "Project", pathParam: "project_id"}
	tagResource        = resource{name: "Tag", pathParam: "tag_id"}
	widgetTypeResource = resource{name: "WidgetType", pathParam: "widget_type_id"}
	sceneResource      = resource{name: "Scene", pathParam: "scene_id"}
	widgetResource     = resource{name: "Widget", pathParam: "widget_id"}
)

// wildcard is the router pattern segment that carries the resource's ID.
func (res resource) wildcard() string {
	return "{" + res.pathParam + "}"
}

// errorMapping maps a domain sentinel error to a response. A 404 names the
// resource and its ID from the path; a 409 carries a fixed detail, so the text
// of the wrapping errors never reaches the client. sendError answers only 404
// and 409; a row with another status is a 500 until sendError learns it.
type errorMapping struct {
	sentinel error
	status   int
	resource resource
	detail   string // for 409
}

// errorMappings is the only place the restapi knows the domain sentinel errors.
var errorMappings = []errorMapping{
	{project.ErrProjectNotFound, http.StatusNotFound, projectResource, ""},
	{project.ErrProjectConflict, http.StatusConflict, projectResource, "Project was changed concurrently; reload it and retry"},
	{project.ErrProjectNameTaken, http.StatusConflict, projectResource, "Project name is already taken"},
	{tag.ErrTagNotFound, http.StatusNotFound, tagResource, ""},
	{tag.ErrTagConflict, http.StatusConflict, tagResource, "Tag was changed concurrently; reload it and retry"},
	{tag.ErrTagNameTaken, http.StatusConflict, tagResource, "Tag name is already taken in the project"},
	{library.ErrWidgetTypeNotFound, http.StatusNotFound, widgetTypeResource, ""},
	{library.ErrWidgetTypeConflict, http.StatusConflict, widgetTypeResource, "WidgetType was changed concurrently; reload it and retry"},
	{library.ErrWidgetTypeInUseByDeletedProjects, http.StatusConflict, widgetTypeResource,
		"WidgetType is used only by Widgets of deleted Projects, still being removed; retry shortly"},
	{library.ErrWidgetTypeInUse, http.StatusConflict, widgetTypeResource, "WidgetType is used by Widgets"},
	{scene.ErrSceneNotFound, http.StatusNotFound, sceneResource, ""},
	{scene.ErrSceneConflict, http.StatusConflict, sceneResource, "Scene was changed concurrently; reload it and retry"},
	{scene.ErrWidgetNotFound, http.StatusNotFound, widgetResource, ""},
}

// sendError responds to an error from the application layer. Invalid input is
// a 400 naming the cause and goes first: a reference from the request body to
// something that does not exist is the caller's input, 404 is only for the
// resource in the path. Then the rows of errorMappings; anything else — and a
// 404 for a resource the path does not name, which is a service fault — is a
// 500 that logs the error and tells the client nothing internal.
func sendError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, application.ErrInvalidInput) || errors.Is(err, runtimeapp.ErrInvalidInput) {
		sendProblem(w, NewBadRequest(err.Error(), r.URL.Path))
		return
	}
	for _, m := range errorMappings {
		if !errors.Is(err, m.sentinel) {
			continue
		}
		switch m.status {
		case http.StatusNotFound:
			pathID, ok := notFoundInPath(r, m, err)
			if !ok {
				sendInternalError(w, r, fmt.Errorf("%s not found, but not the one the path names by %s: %w",
					m.resource.name, m.resource.pathParam, err))
				return
			}
			sendProblem(w, NewNotFound(m.resource.name, pathID, r.URL.Path))
		case http.StatusConflict:
			detail := m.detail
			if errors.Is(err, application.ErrRaceLost) {
				// The request stated no version: nothing to reload, the server
				// has already retried.
				detail = m.resource.name + " kept changing while the request ran; retry it later"
			}
			sendProblem(w, NewConflict(m.resource.name, detail, r.URL.Path))
		default:
			sendInternalError(w, r, fmt.Errorf("error mapping for %d has no response: %w", m.status, err))
		}
		return
	}
	sendInternalError(w, r, err)
}

// notFoundInPath returns the path's ID of m's resource if err, a not-found of
// that resource, is about it. An error that does not say which ID was not
// found (no id.NotFoundError) is taken to be about the path's.
func notFoundInPath(r *http.Request, m errorMapping, err error) (string, bool) {
	pathID := r.PathValue(m.resource.pathParam)
	if pathID == "" {
		return "", false
	}
	var nf id.NotFoundError
	if errors.As(err, &nf) && nf.Err == m.sentinel {
		parsed, perr := uuid.Parse(pathID)
		if perr != nil || parsed != nf.ID {
			return "", false
		}
	}
	return pathID, true
}

// isNotFound reports whether err says that res does not exist, for a handler
// whose answer to that is not an error (a lookup by name finding nothing).
func isNotFound(err error, res resource) bool {
	for _, m := range errorMappings {
		if m.status == http.StatusNotFound && m.resource == res && errors.Is(err, m.sentinel) {
			return true
		}
	}
	return false
}

// pathUUID parses the ID of res from the request path. On failure it has
// already sent a 400 and returns false.
func pathUUID(w http.ResponseWriter, r *http.Request, res resource) (uuid.UUID, bool) {
	raw := r.PathValue(res.pathParam)
	id, err := uuid.Parse(raw)
	if err != nil {
		sendProblem(w, NewBadRequest(fmt.Sprintf("%s: '%s' is not a valid UUID", res.pathParam, raw), r.URL.Path))
		return uuid.Nil, false
	}
	return id, true
}

// sendInternalError logs the full error and sends a generic 500 to the client.
func sendInternalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("internal server error", "path", r.URL.Path, "method", r.Method, "error", err)
	sendProblem(w, NewInternalError("an internal error occurred", r.URL.Path))
}
