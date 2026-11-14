package restapi

import (
	"net/http"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
)

// LabelHandlers serves the Labels of the server's Organization (task 85_2;
// ADR 0025). A label is metadata, not content: no request states a Draft
// Version.
type LabelHandlers struct {
	service application.LabelService
}

// NewLabelHandler creates the handler of the labels.
func NewLabelHandler(s application.LabelService) *LabelHandlers {
	return &LabelHandlers{service: s}
}

// GetLabels handles GET /labels: the Organization's labels, ordered by name
// ignoring case.
func (h LabelHandlers) GetLabels(w http.ResponseWriter, r *http.Request) {
	list, err := h.service.List(r.Context())
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetLabelsResponse(list))
}

// PostLabels handles POST /labels.
func (h LabelHandlers) PostLabels(w http.ResponseWriter, r *http.Request) {
	var request apiv0.CreateLabelRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	created, err := h.service.Create(r.Context(), request.Name, request.Color)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusCreated, newLabelResponse(created))
}

// PatchLabelsByID handles PATCH /labels/{label_id}: the rename and/or the
// recolor of a label.
func (h LabelHandlers) PatchLabelsByID(w http.ResponseWriter, r *http.Request) {
	labelID, ok := pathUUID(w, r, labelResource)
	if !ok {
		return
	}
	var request apiv0.UpdateLabelRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	updated, err := h.service.Update(r.Context(), labelID, request.Name, request.Color)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newLabelResponse(updated))
}

// DeleteLabelsByID handles DELETE /labels/{label_id}: the label and its
// assignments go, 204.
func (h LabelHandlers) DeleteLabelsByID(w http.ResponseWriter, r *http.Request) {
	labelID, ok := pathUUID(w, r, labelResource)
	if !ok {
		return
	}
	if err := h.service.Delete(r.Context(), labelID); err != nil {
		sendError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
