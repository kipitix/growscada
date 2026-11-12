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
	order, err := h.service.SetLibraryOrder(r.Context(), request.Version, request.LibraryIDs)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newLibraryOrderResponse(order))
}
