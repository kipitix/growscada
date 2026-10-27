package restapi

import (
	"net/http"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
)

// WidgetTypesHandlers handles HTTP requests related to the widget types of a
// library's Draft, under /libraries/{library_id}/widget-types.
type WidgetTypesHandlers struct {
	service application.WidgetTypeService
}

func NewWidgetTypesHandler(s application.WidgetTypeService) *WidgetTypesHandlers {
	return &WidgetTypesHandlers{service: s}
}

// GetWidgetTypes handles GET /libraries/{library_id}/widget-types
func (h WidgetTypesHandlers) GetWidgetTypes(w http.ResponseWriter, r *http.Request) {
	libraryID, ok := pathUUID(w, r, libraryResource)
	if !ok {
		return
	}
	list, err := h.service.FindAllWidgetTypes(r.Context(), libraryID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetWidgetTypesResponse(list))
}

// GetWidgetTypesByID handles GET /libraries/{library_id}/widget-types/{widget_type_id}
func (h WidgetTypesHandlers) GetWidgetTypesByID(w http.ResponseWriter, r *http.Request) {
	libraryID, ok := pathUUID(w, r, libraryResource)
	if !ok {
		return
	}
	inID, ok := pathUUID(w, r, widgetTypeResource)
	if !ok {
		return
	}

	found, err := h.service.FindWidgetTypeByID(r.Context(), libraryID, inID)
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newWidgetTypeResponse(found))
}

// PostWidgetTypes handles POST /libraries/{library_id}/widget-types
func (h WidgetTypesHandlers) PostWidgetTypes(w http.ResponseWriter, r *http.Request) {
	libraryID, ok := pathUUID(w, r, libraryResource)
	if !ok {
		return
	}
	var request apiv0.CreateWidgetTypeRequest
	if !decodeRequest(w, r, &request) {
		return
	}

	created, err := h.service.CreateWidgetType(r.Context(), libraryID, newCreateWidgetTypeInput(request))
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusCreated, newCreateWidgetTypeResponse(created))
}

// PutWidgetTypesByID handles PUT /libraries/{library_id}/widget-types/{widget_type_id}
func (h WidgetTypesHandlers) PutWidgetTypesByID(w http.ResponseWriter, r *http.Request) {
	libraryID, ok := pathUUID(w, r, libraryResource)
	if !ok {
		return
	}
	inID, ok := pathUUID(w, r, widgetTypeResource)
	if !ok {
		return
	}

	var request apiv0.UpdateWidgetTypeRequest
	if !decodeRequest(w, r, &request) {
		return
	}

	updated, err := h.service.UpdateWidgetType(r.Context(), libraryID, inID, request.Version, newUpdateWidgetTypeInput(request))
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newUpdateWidgetTypeResponse(updated))
}

// DeleteWidgetTypesByID handles DELETE /libraries/{library_id}/widget-types/{widget_type_id}
func (h WidgetTypesHandlers) DeleteWidgetTypesByID(w http.ResponseWriter, r *http.Request) {
	libraryID, ok := pathUUID(w, r, libraryResource)
	if !ok {
		return
	}
	inID, ok := pathUUID(w, r, widgetTypeResource)
	if !ok {
		return
	}

	deleted, err := h.service.DeleteWidgetTypeByID(r.Context(), libraryID, inID)
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newWidgetTypeResponse(deleted))
}
