package restapi

import (
	"net/http"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
)

// SettingsHandlers serves the per-user settings of the server (task 81,
// ADR 0024): the way the person the server acts for sees its content. The
// owner is never named by the path — the service knows it, and with
// authentication it comes from the request's principal (task 50).
type SettingsHandlers struct {
	service application.SettingsService
}

func NewSettingsHandler(s application.SettingsService) *SettingsHandlers {
	return &SettingsHandlers{service: s}
}

// GetLibraryOrder handles GET /settings/library-order: the order the person
// stated, and the Version their next write states. A person who never
// reordered is 200 with Version 0 and no library, never a 404.
func (h SettingsHandlers) GetLibraryOrder(w http.ResponseWriter, r *http.Request) {
	order, err := h.service.LibraryOrder(r.Context())
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newLibraryOrderResponse(order))
}

// PutLibraryOrder handles PUT /settings/library-order: the whole order the
// client shows, stored at the Version the request states. A stale Version is
// 409; repetitions are dropped and the IDs of missing libraries are tolerated
// (ADR 0024).
func (h SettingsHandlers) PutLibraryOrder(w http.ResponseWriter, r *http.Request) {
	var request apiv0.SetLibraryOrderRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	// library_ids is required, as the schema states: an absent one and an empty
	// one decode alike, so the absent one is refused here — a client that
	// forgets the field must not wipe the order. [] is a person who placed no
	// library.
	if request.LibraryIDs == nil {
		sendProblem(w, NewBadRequest("library_ids is required: send the whole order, [] for none", r.URL.Path))
		return
	}
	order, err := h.service.SetLibraryOrder(r.Context(), request.Version, request.LibraryIDs)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newLibraryOrderResponse(order))
}

// GetProjectOrder handles GET /settings/project-order: the project order the
// person stated, and the Version their next write states (task 83). A person
// who never reordered is 200 with Version 0 and no project, never a 404.
func (h SettingsHandlers) GetProjectOrder(w http.ResponseWriter, r *http.Request) {
	order, err := h.service.ProjectOrder(r.Context())
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newProjectOrderResponse(order))
}

// PutProjectOrder handles PUT /settings/project-order: the whole order the
// client shows, stored at the Version the request states (task 83). A stale
// Version is 409; repetitions are dropped and the IDs of missing projects are
// tolerated (ADR 0024).
func (h SettingsHandlers) PutProjectOrder(w http.ResponseWriter, r *http.Request) {
	var request apiv0.SetProjectOrderRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	// project_ids is required, as library_ids is: a client that forgets the
	// field must not wipe the order. [] is a person who placed no project.
	if request.ProjectIDs == nil {
		sendProblem(w, NewBadRequest("project_ids is required: send the whole order, [] for none", r.URL.Path))
		return
	}
	order, err := h.service.SetProjectOrder(r.Context(), request.Version, request.ProjectIDs)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newProjectOrderResponse(order))
}
