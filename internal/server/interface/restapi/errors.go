package restapi

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/devicetoken"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
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
	libraryResource    = resource{name: "Library", pathParam: "library_id"}
	releaseResource    = resource{name: "LibraryRelease", pathParam: "release_number"}
	// A Library Pin is named by its library within its project.
	libraryPinResource  = resource{name: "LibraryPin", pathParam: "library_id"}
	deviceResource      = resource{name: "Device", pathParam: "device_id"}
	environmentResource = resource{name: "Environment", pathParam: "environment_id"}
	// An Environment Device is named by its Device within its Environment.
	environmentDeviceResource = resource{name: "EnvironmentDevice", pathParam: "device_id"}
	// A Device Token is named by its Device: there is one per Device.
	deviceTokenResource = resource{name: "DeviceToken", pathParam: "device_id"}
	// A Deployment is named by its number within its Project.
	deploymentResource = resource{name: "Deployment", pathParam: "deployment_number"}
)

// wildcard is the router pattern segment that carries the resource's ID.
func (res resource) wildcard() string {
	return "{" + res.pathParam + "}"
}

// errorMapping maps a domain sentinel error to a response. A 404 names the
// resource and its ID from the path, or, for a resource the request names in
// its body (a batch write), from the error itself; 401, 403 and 409 carry a
// fixed detail, so the text of the wrapping errors never reaches the client. A
// row with another status is a 500 until sendError learns it.
type errorMapping struct {
	sentinel error
	status   int
	resource resource
	detail   string // for 409
}

// errorMappings is the only place the restapi knows the domain and the
// application sentinel errors.
var errorMappings = []errorMapping{
	{draft.ErrDraftConflict, http.StatusConflict, projectResource,
		"Draft was changed concurrently; reload it and retry"},
	{draft.ErrNothingToUndo, http.StatusConflict, projectResource,
		"Nothing to undo: the Draft has no change at the stack's position"},
	{draft.ErrNothingToRedo, http.StatusConflict, projectResource,
		"Nothing to redo: nothing was undone, or a new change replaced the redo tail"},
	{application.ErrNothingToDeploy, http.StatusConflict, projectResource,
		"Nothing to deploy: the Draft of the project is the deployment it runs"},
	{application.ErrNoDeploymentToDiscard, http.StatusConflict, projectResource,
		"Nothing to discard: the project runs no deployment"},
	{application.ErrNothingToOpen, http.StatusConflict, projectResource,
		"Nothing to open: the Draft of the project is the project file"},
	{application.ErrDraftNotDeployable, http.StatusConflict, projectResource,
		"The Draft cannot be exported: it is not deployable; repair the Draft first"},
	{application.ErrDeploymentNotDiscardable, http.StatusConflict, deploymentResource,
		"The Deployment names Library Releases this server does not have; the Draft is left as it is"},
	{runtimeapp.ErrNothingToRollback, http.StatusConflict, deploymentResource,
		"Nothing to rollback: the deployment holds the document the project runs"},
	{runtimeapp.ErrDeploymentNotFound, http.StatusNotFound, deploymentResource, ""},
	{project.ErrProjectNotFound, http.StatusNotFound, projectResource, ""},
	{project.ErrProjectConflict, http.StatusConflict, projectResource, "Project was changed concurrently; reload it and retry"},
	{project.ErrProjectNameTaken, http.StatusConflict, projectResource, "Project name is already taken"},
	{tag.ErrTagNotFound, http.StatusNotFound, tagResource, ""},
	{tag.ErrTagNameTaken, http.StatusConflict, tagResource, "Tag name is already taken in the project"},
	{library.ErrLibraryNotFound, http.StatusNotFound, libraryResource, ""},
	{library.ErrLibraryConflict, http.StatusConflict, libraryResource, "Library was changed concurrently; reload it and retry"},
	{library.ErrLibraryNameTaken, http.StatusConflict, libraryResource, "Library name is already taken"},
	{library.ErrLibraryPinned, http.StatusConflict, libraryResource, "Library is pinned by Projects; unpin it first"},
	{library.ErrNoRelease, http.StatusConflict, libraryResource, "Library has never been released: there is nothing to discard to"},
	{library.ErrNothingToRelease, http.StatusConflict, libraryResource, "Library's Draft has not changed since its latest release"},
	{library.ErrReleaseNumberTaken, http.StatusConflict, libraryResource, "Library was released concurrently; retry"},
	{library.ErrReleaseNotFound, http.StatusNotFound, releaseResource, ""},
	{library.ErrWidgetTypeNotFound, http.StatusNotFound, widgetTypeResource, ""},
	{library.ErrWidgetTypeNameTaken, http.StatusConflict, widgetTypeResource, "WidgetType name is already taken in the library"},
	{librarypin.ErrLibraryPinNotFound, http.StatusNotFound, libraryPinResource, ""},
	{librarypin.ErrLibraryPinExists, http.StatusConflict, libraryPinResource, "Library is already pinned in the project"},
	{librarypin.ErrWidgetsOrphaned, http.StatusConflict, libraryPinResource,
		"Widgets of the project would be left without their WidgetType; remove them or change their type first"},
	{scene.ErrSceneNotFound, http.StatusNotFound, sceneResource, ""},
	{scene.ErrWidgetNotFound, http.StatusNotFound, widgetResource, ""},
	{scene.ErrNothingToMove, http.StatusConflict, sceneResource,
		"Nothing to move: the widgets already hold these positions"},
	{device.ErrDeviceNotFound, http.StatusNotFound, deviceResource, ""},
	{device.ErrDeviceNameTaken, http.StatusConflict, deviceResource, "Device name is already taken in the project"},
	{device.ErrDeviceInUse, http.StatusConflict, deviceResource,
		"Device is still in use: a Tag is supplied by it, or it stands in an environment"},
	{device.ErrEnvironmentNotFound, http.StatusNotFound, environmentResource, ""},
	{device.ErrEnvironmentNameTaken, http.StatusConflict, environmentResource, "Environment name is already taken in the project"},
	{device.ErrEnvironmentInUse, http.StatusConflict, environmentResource,
		"Environment still holds devices: remove their pairs first"},
	{device.ErrEnvironmentDeviceNotFound, http.StatusNotFound, environmentDeviceResource, ""},
	{device.ErrEnvironmentDeviceExists, http.StatusConflict, environmentDeviceResource, "Device is already in the environment"},
	{tag.ErrTagNotSuppliedByDevice, http.StatusForbidden, tagResource,
		"Tag is supplied by a Device: only that Device writes its value"},
	{devicetoken.ErrTokenNotFound, http.StatusNotFound, deviceTokenResource, ""},
	{devicetoken.ErrTokenExists, http.StatusConflict, deviceTokenResource,
		"Device already has a token; revoke or rotate it"},
}

// sendError responds to an error from the application layer. Invalid input is
// a 400 naming the cause and goes first: a reference from the request body to
// something that does not exist is the caller's input, while a 404 answers a
// resource the request names — in the path, or in the body of a batch write
// (see notFoundInPath). Then the rows of errorMappings: 404 naming that
// resource, 401, 403 and 409 with a fixed detail, so the text of
// the wrapping errors never reaches the client. Anything else — and a 404
// that does not say which ID was not found, which is a service fault — is a
// 500 that logs the error and tells the client nothing internal.
func sendError(w http.ResponseWriter, r *http.Request, err error) {
	// A project file that does not validate answers every problem found, not
	// the first: the 400 carries them as file_issues (ADR 0018).
	var invalidFile application.ProjectFileError
	if errors.As(err, &invalidFile) {
		problem := NewBadRequest(err.Error(), r.URL.Path)
		problem.FileIssues = newFileIssues(invalidFile.Issues)
		sendProblem(w, problem)
		return
	}
	// A Draft the builder refuses names its reason in the 409's detail.
	var notDeployable application.DraftNotDeployableError
	if errors.As(err, &notDeployable) {
		sendProblem(w, NewConflict("Draft", notDeployable.Reason, r.URL.Path))
		return
	}
	// A Discard refused for foreign pins names the missing ones as file_issues.
	var notDiscardable application.DeploymentNotDiscardableError
	if errors.As(err, &notDiscardable) {
		problem := NewConflict(deploymentResource.name,
			"The Deployment names Library Releases this server does not have; the Draft is left as it is", r.URL.Path)
		problem.FileIssues = newFileIssues(notDiscardable.Issues)
		sendProblem(w, problem)
		return
	}
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
		case http.StatusUnauthorized:
			sendProblem(w, NewUnauthorized(m.detail, r.URL.Path))
		case http.StatusForbidden:
			sendProblem(w, NewForbidden(m.detail, r.URL.Path))
		case http.StatusConflict:
			detail := m.detail
			if errors.Is(err, application.ErrRaceLost) {
				// The request stated no version: nothing to reload, the server
				// has already retried.
				detail = m.resource.name + " kept changing while the request ran; retry it later"
			}
			problem := NewConflict(m.resource.name, detail, r.URL.Path)
			var conflict draft.ConflictError
			if errors.As(err, &conflict) {
				// The client states the Version it read; a newer one lets it
				// retry without rereading the Draft (ADR 0016). A Library's
				// Draft names the Library, a Project's the Project.
				if conflict.Scope.IsLibrary() {
					problem.Title = "Conflict: " + libraryResource.name
				}
				problem.Draft = &apiv0.DraftStateResponse{Version: conflict.Current}
			}
			var nothingToStep draft.NothingToStepError
			if errors.As(err, &nothingToStep) && nothingToStep.Scope.IsLibrary() {
				problem.Title = "Conflict: " + libraryResource.name
			}
			var orphans librarypin.OrphansError
			if errors.As(err, &orphans) {
				for _, pw := range orphans.Widgets {
					problem.OrphanedWidgets = append(problem.OrphanedWidgets, newPinnedWidgetResponse(appdto.NewPinnedWidget(pw)))
				}
			}
			sendProblem(w, problem)
		case http.StatusInternalServerError:
			// A fault the client cannot fix, with an honest detail that says
			// what state the server is in; the wrapping error goes to the log.
			slog.Error("internal server error", "path", r.URL.Path, "method", r.Method, "error", err)
			sendProblem(w, NewInternalError(m.detail, r.URL.Path))
		default:
			sendInternalError(w, r, fmt.Errorf("error mapping for %d has no response: %w", m.status, err))
		}
		return
	}
	sendInternalError(w, r, err)
}

// notFoundInPath returns the ID of m's resource if err, a not-found of that
// resource, is about it. The ID comes from the path; a route that names its
// resource in the body instead of the path (a batch write) has no such path
// parameter, and the answer names the ID the error itself carries. An error
// that does not say which ID was not found (no id.NotFoundError) is taken to
// be about the path's.
func notFoundInPath(r *http.Request, m errorMapping, err error) (string, bool) {
	var nf id.NotFoundError
	namesID := errors.As(err, &nf) && nf.Err == m.sentinel
	pathID := r.PathValue(m.resource.pathParam)
	if pathID == "" {
		if namesID {
			return nf.ID.String(), true
		}
		return "", false
	}
	if namesID {
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

// pathInt parses the number of res from the request path. On failure it has
// already sent a 400 and returns false.
func pathInt(w http.ResponseWriter, r *http.Request, res resource) (int, bool) {
	raw := r.PathValue(res.pathParam)
	n, err := strconv.Atoi(raw)
	if err != nil {
		sendProblem(w, NewBadRequest(res.pathParam+": '"+raw+"' is not a number", r.URL.Path))
		return 0, false
	}
	return n, true
}

// sendInternalError logs the full error and sends a generic 500 to the client.
func sendInternalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("internal server error", "path", r.URL.Path, "method", r.Method, "error", err)
	sendProblem(w, NewInternalError("an internal error occurred", r.URL.Path))
}
