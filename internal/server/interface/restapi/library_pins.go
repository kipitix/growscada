package restapi

import (
	"net/http"
	"strconv"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
)

// LibraryPinsHandlers handles HTTP requests related to the Library Pins of a
// project and the WidgetTypes they give it.
type LibraryPinsHandlers struct {
	service application.LibraryPinService
}

func NewLibraryPinsHandler(s application.LibraryPinService) *LibraryPinsHandlers {
	return &LibraryPinsHandlers{service: s}
}

// GetLibraryPins handles GET /projects/{project_id}/library-pins
func (h LibraryPinsHandlers) GetLibraryPins(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	list, err := h.service.FindLibraryPins(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetLibraryPinsResponse(list))
}

// PutLibraryPin handles PUT /projects/{project_id}/library-pins/{library_id}:
// adds the pin (201) or switches it (200). A switch that would leave Widgets
// without their WidgetType is refused (409 naming them).
func (h LibraryPinsHandlers) PutLibraryPin(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	libraryID, ok := pathUUID(w, r, libraryPinResource)
	if !ok {
		return
	}
	var request apiv0.PutLibraryPinRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	saved, created, err := h.service.PutLibraryPin(r.Context(), projectID, libraryID, request.Release, request.Version)
	if err != nil {
		sendError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	sendJSONResponse(w, status, newLibraryPinResponse(saved))
}

// DeleteLibraryPin handles DELETE /projects/{project_id}/library-pins/{library_id}.
// Refused while Widgets of the project are of the pinned WidgetTypes (409
// naming them).
func (h LibraryPinsHandlers) DeleteLibraryPin(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	libraryID, ok := pathUUID(w, r, libraryPinResource)
	if !ok {
		return
	}
	deleted, err := h.service.DeleteLibraryPin(r.Context(), projectID, libraryID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newLibraryPinResponse(deleted))
}

// GetLibraryPinImpact handles GET
// /projects/{project_id}/library-pins/{library_id}/impact?release=<n>: the
// preview of pinning release n of the library, or of removing the pin
// without ?release (404 if the project does not pin the library).
func (h LibraryPinsHandlers) GetLibraryPinImpact(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	libraryID, ok := pathUUID(w, r, libraryPinResource)
	if !ok {
		return
	}
	var release *int
	if query := r.URL.Query(); query.Has(apiv0.LibraryPinImpactQueryRelease) {
		raw := query.Get(apiv0.LibraryPinImpactQueryRelease)
		n, err := strconv.Atoi(raw)
		if err != nil {
			sendProblem(w, NewBadRequest(apiv0.LibraryPinImpactQueryRelease+": '"+raw+"' is not a number", r.URL.Path))
			return
		}
		release = &n
	}
	impact, err := h.service.PinImpact(r.Context(), projectID, libraryID, release)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newPinImpactResponse(impact))
}

// GetPinnedWidgetTypes handles GET /projects/{project_id}/widget-types: the
// WidgetTypes of every release the project pins.
func (h LibraryPinsHandlers) GetPinnedWidgetTypes(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	list, err := h.service.FindPinnedWidgetTypes(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, apiv0.GetPinnedWidgetTypesResponse{WidgetTypes: newReleasedWidgetTypeResponses(list)})
}
