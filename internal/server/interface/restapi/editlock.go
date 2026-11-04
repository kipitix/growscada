package restapi

import (
	"net/http"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
)

// EditLockHandlers handles the EditLock of a Project's Draft (ADR 0015): its
// state, Check out, Check in and takeover. The client that acts is named by
// the X-GrowSCADA-Client header, which the middleware puts in the request's
// context.
type EditLockHandlers struct {
	locks application.EditLockService
}

// NewEditLockHandler creates the handler over the EditLocks.
func NewEditLockHandler(locks application.EditLockService) *EditLockHandlers {
	return &EditLockHandlers{locks: locks}
}

// GetEditLock handles GET /projects/{project_id}/edit-lock: the state of the
// Draft's lock, 200 free or held.
func (h EditLockHandlers) GetEditLock(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	state, err := h.locks.State(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newEditLockResponse(state))
}

// PostEditLock handles POST /projects/{project_id}/edit-lock: Check out. 201
// with the new lock when it was free, 200 when the same client holds it
// already, 409 with the holder when another client does.
func (h EditLockHandlers) PostEditLock(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	var req apiv0.CheckOutEditLockRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	state, took, err := h.locks.CheckOut(r.Context(), projectID, req.Name)
	if err != nil {
		sendError(w, r, err)
		return
	}
	status := http.StatusOK
	if took {
		status = http.StatusCreated
	}
	sendJSONResponse(w, status, newEditLockResponse(state))
}

// DeleteEditLock handles DELETE /projects/{project_id}/edit-lock: Check in.
// 200 with the free lock; a free lock is 200 too, another client's is 409.
func (h EditLockHandlers) DeleteEditLock(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	state, err := h.locks.CheckIn(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newEditLockResponse(state))
}

// PostEditLockTakeOver handles POST /projects/{project_id}/edit-lock/takeover:
// taking a held lock from its holder, stating the version the client observed.
// 200 with the lock; a stale version or a free lock is 409.
func (h EditLockHandlers) PostEditLockTakeOver(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	var req apiv0.TakeOverEditLockRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	state, err := h.locks.TakeOver(r.Context(), projectID, req.Name, req.Version)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newEditLockResponse(state))
}
