package restapi

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// resource is a REST resource as the client sees it: its name from the
// glossary and the path parameter that carries its ID in the router's
// patterns.
type resource struct {
	name      string
	pathParam string
}

var (
	tagResource        = resource{name: "Tag", pathParam: "tagId"}
	widgetTypeResource = resource{name: "WidgetType", pathParam: "widgetTypeId"}
	sceneResource      = resource{name: "Scene", pathParam: "sceneId"}
	widgetResource     = resource{name: "Widget", pathParam: "widgetId"}
)

// errorMapping maps a domain sentinel error to a response. A 404 names the
// resource and its ID from the path; a 409 carries a fixed detail, so the text
// of the wrapping errors never reaches the client.
type errorMapping struct {
	sentinel error
	status   int
	resource resource
	detail   string // for 409
}

// errorMappings is the only place the restapi knows the domain sentinel errors.
var errorMappings = []errorMapping{
	{tag.ErrTagNotFound, http.StatusNotFound, tagResource, ""},
	{tag.ErrTagConflict, http.StatusConflict, tagResource, "Tag was changed concurrently; reload it and retry"},
	{tag.ErrTagNameTaken, http.StatusConflict, tagResource, "Tag name is already taken"},
	{library.ErrWidgetTypeNotFound, http.StatusNotFound, widgetTypeResource, ""},
	{library.ErrWidgetTypeConflict, http.StatusConflict, widgetTypeResource, "WidgetType was changed concurrently; reload it and retry"},
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
	if errors.Is(err, application.ErrInvalidInput) {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}
	for _, m := range errorMappings {
		if !errors.Is(err, m.sentinel) {
			continue
		}
		switch m.status {
		case http.StatusNotFound:
			id := r.PathValue(m.resource.pathParam)
			if id == "" {
				sendInternalError(w, r, fmt.Errorf("%s not found, but the path has no %s: %w",
					m.resource.name, m.resource.pathParam, err))
				return
			}
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(m.resource.name, id, r.URL.Path))
		case http.StatusConflict:
			sendJSONResponse(w, http.StatusConflict, NewConflict(m.resource.name, m.detail, r.URL.Path))
		}
		return
	}
	sendInternalError(w, r, err)
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
		sendJSONResponse(w, http.StatusBadRequest,
			NewBadRequest(fmt.Sprintf("%s: '%s' is not a valid UUID", res.pathParam, raw), r.URL.Path))
		return uuid.Nil, false
	}
	return id, true
}

// sendInternalError logs the full error and sends a generic 500 to the client.
func sendInternalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("internal server error", "path", r.URL.Path, "method", r.Method, "error", err)
	sendJSONResponse(w, http.StatusInternalServerError, NewInternalError("an internal error occurred", r.URL.Path))
}
