package restapi

import (
	"net/http"
	"strconv"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
)

// LibrariesHandlers handles HTTP requests related to libraries, their
// releases and Discards. The WidgetTypes of a library's Draft are served by
// WidgetTypesHandlers.
type LibrariesHandlers struct {
	service application.LibraryService
}

func NewLibrariesHandler(s application.LibraryService) *LibrariesHandlers {
	return &LibrariesHandlers{service: s}
}

// GetLibraries handles GET /libraries, the live libraries in creation order.
// ?name=<name> selects the library with that exact name: the list holds 0
// or 1 libraries.
func (h LibrariesHandlers) GetLibraries(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if query.Has(apiv0.LibrariesQueryName) {
		found, err := h.service.FindLibraryByName(r.Context(), query.Get(apiv0.LibrariesQueryName))
		if err != nil {
			if isNotFound(err, libraryResource) {
				sendJSONResponse(w, http.StatusOK, newGetLibrariesResponse(nil))
				return
			}
			sendError(w, r, err)
			return
		}
		sendJSONResponse(w, http.StatusOK, newGetLibrariesResponse([]appdto.Library{found}))
		return
	}

	list, err := h.service.FindAllLibraries(r.Context())
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetLibrariesResponse(list))
}

// GetLibrariesByID handles GET /libraries/{library_id}
func (h LibrariesHandlers) GetLibrariesByID(w http.ResponseWriter, r *http.Request) {
	libraryID, ok := pathUUID(w, r, libraryResource)
	if !ok {
		return
	}
	found, err := h.service.FindLibraryByID(r.Context(), libraryID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newLibraryResponse(found))
}

// PostLibraries handles POST /libraries
func (h LibrariesHandlers) PostLibraries(w http.ResponseWriter, r *http.Request) {
	var request apiv0.CreateLibraryRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	created, err := h.service.CreateLibrary(r.Context(), appdto.LibraryInput{Name: request.Name})
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusCreated, apiv0.CreateLibraryResponse{ID: created.ID, Version: created.Version})
}

// PutLibrariesByID handles PUT /libraries/{library_id}: a rename.
func (h LibrariesHandlers) PutLibrariesByID(w http.ResponseWriter, r *http.Request) {
	libraryID, ok := pathUUID(w, r, libraryResource)
	if !ok {
		return
	}
	var request apiv0.UpdateLibraryRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	updated, err := h.service.UpdateLibrary(r.Context(), libraryID, request.Version, appdto.LibraryInput{Name: request.Name})
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, apiv0.UpdateLibraryResponse{Version: updated.Version})
}

// DeleteLibrariesByID handles DELETE /libraries/{library_id}. The library is
// gone for the API at once; its Draft and releases are removed after the
// response. Refused while a live project pins it.
func (h LibrariesHandlers) DeleteLibrariesByID(w http.ResponseWriter, r *http.Request) {
	libraryID, ok := pathUUID(w, r, libraryResource)
	if !ok {
		return
	}
	deleted, err := h.service.DeleteLibraryByID(r.Context(), libraryID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newLibraryResponse(deleted))
}

// PostReleases handles POST /libraries/{library_id}/releases: a Release of
// the library's Draft.
func (h LibrariesHandlers) PostReleases(w http.ResponseWriter, r *http.Request) {
	libraryID, ok := pathUUID(w, r, libraryResource)
	if !ok {
		return
	}
	var request apiv0.CreateReleaseRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	created, err := h.service.Release(r.Context(), libraryID, request.Version, appdto.ReleaseInput{Note: request.Note})
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusCreated, newLibraryReleaseResponse(created))
}

// GetReleases handles GET /libraries/{library_id}/releases, oldest first.
func (h LibrariesHandlers) GetReleases(w http.ResponseWriter, r *http.Request) {
	libraryID, ok := pathUUID(w, r, libraryResource)
	if !ok {
		return
	}
	list, err := h.service.FindReleases(r.Context(), libraryID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetLibraryReleasesResponse(list))
}

// GetReleasesByNumber handles GET /libraries/{library_id}/releases/{release_number}
func (h LibrariesHandlers) GetReleasesByNumber(w http.ResponseWriter, r *http.Request) {
	libraryID, ok := pathUUID(w, r, libraryResource)
	if !ok {
		return
	}
	number, ok := pathInt(w, r, releaseResource)
	if !ok {
		return
	}
	found, err := h.service.FindRelease(r.Context(), libraryID, number)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newLibraryReleaseResponse(found))
}

// PostDiscard handles POST /libraries/{library_id}/discard: the Draft is
// reset to the latest release after the response; until then it is
// read-only.
func (h LibrariesHandlers) PostDiscard(w http.ResponseWriter, r *http.Request) {
	libraryID, ok := pathUUID(w, r, libraryResource)
	if !ok {
		return
	}
	marked, err := h.service.Discard(r.Context(), libraryID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusAccepted, newLibraryResponse(marked))
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
