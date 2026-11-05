package restapi

import (
	"net/http"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// EditLockHandlers handles the EditLock of a Draft (ADR 0015) — a Project's or
// a Library's: its state, Check out, Check in and takeover. The client that
// acts is named by the X-GrowSCADA-Client header, which the middleware puts in
// the request's context.
type EditLockHandlers struct {
	locks application.EditLockService
}

// NewEditLockHandler creates the handler over the EditLocks.
func NewEditLockHandler(locks application.EditLockService) *EditLockHandlers {
	return &EditLockHandlers{locks: locks}
}

// draftScope is the Scope of the Draft named by the ID of res's path: a
// Project's or a Library's.
func draftScope(res resource, rawID uuid.UUID) editlock.Scope {
	if res.pathParam == libraryResource.pathParam {
		return editlock.LibraryScope(id.NewID(id.IDWithUUID[library.Library](rawID)))
	}
	return editlock.ProjectScope(id.NewID(id.IDWithUUID[project.Project](rawID)))
}

// GetEditLock handles GET /projects/{project_id}/edit-lock: the state of the
// Draft's lock, 200 free or held.
func (h EditLockHandlers) GetEditLock(w http.ResponseWriter, r *http.Request) {
	h.get(w, r, projectResource)
}

// GetLibraryEditLock is GetEditLock for a Library's Draft.
func (h EditLockHandlers) GetLibraryEditLock(w http.ResponseWriter, r *http.Request) {
	h.get(w, r, libraryResource)
}

func (h EditLockHandlers) get(w http.ResponseWriter, r *http.Request, res resource) {
	rawID, ok := pathUUID(w, r, res)
	if !ok {
		return
	}
	state, err := h.locks.State(r.Context(), draftScope(res, rawID))
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
	h.post(w, r, projectResource)
}

// PostLibraryEditLock is PostEditLock for a Library's Draft.
func (h EditLockHandlers) PostLibraryEditLock(w http.ResponseWriter, r *http.Request) {
	h.post(w, r, libraryResource)
}

func (h EditLockHandlers) post(w http.ResponseWriter, r *http.Request, res resource) {
	rawID, ok := pathUUID(w, r, res)
	if !ok {
		return
	}
	var req apiv0.CheckOutEditLockRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	state, took, err := h.locks.CheckOut(r.Context(), draftScope(res, rawID), req.Name)
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
	h.delete(w, r, projectResource)
}

// DeleteLibraryEditLock is DeleteEditLock for a Library's Draft.
func (h EditLockHandlers) DeleteLibraryEditLock(w http.ResponseWriter, r *http.Request) {
	h.delete(w, r, libraryResource)
}

func (h EditLockHandlers) delete(w http.ResponseWriter, r *http.Request, res resource) {
	rawID, ok := pathUUID(w, r, res)
	if !ok {
		return
	}
	state, err := h.locks.CheckIn(r.Context(), draftScope(res, rawID))
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
	h.postTakeOver(w, r, projectResource)
}

// PostLibraryEditLockTakeOver is PostEditLockTakeOver for a Library's Draft.
func (h EditLockHandlers) PostLibraryEditLockTakeOver(w http.ResponseWriter, r *http.Request) {
	h.postTakeOver(w, r, libraryResource)
}

func (h EditLockHandlers) postTakeOver(w http.ResponseWriter, r *http.Request, res resource) {
	rawID, ok := pathUUID(w, r, res)
	if !ok {
		return
	}
	var req apiv0.TakeOverEditLockRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	state, err := h.locks.TakeOver(r.Context(), draftScope(res, rawID), req.Name, req.Version)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newEditLockResponse(state))
}
