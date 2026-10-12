package restapi

import (
	"net/http"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
)

// WidgetTypesHandlers handles HTTP requests related to widget types.
type WidgetTypesHandlers struct {
	service application.WidgetTypeService
}

func NewWidgetTypesHandler(s application.WidgetTypeService) *WidgetTypesHandlers {
	return &WidgetTypesHandlers{service: s}
}

// GetWidgetTypes handles GET /widget-types
func (h WidgetTypesHandlers) GetWidgetTypes(w http.ResponseWriter, r *http.Request) {
	list, err := h.service.FindAllWidgetTypes(r.Context())
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetWidgetTypesResponse(list))
}

// GetWidgetTypesByID handles GET /widget-types/{widget_type_id}
func (h WidgetTypesHandlers) GetWidgetTypesByID(w http.ResponseWriter, r *http.Request) {
	inID, ok := pathUUID(w, r, widgetTypeResource)
	if !ok {
		return
	}

	found, err := h.service.FindWidgetTypeByID(r.Context(), inID)
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newWidgetTypeResponse(found))
}

// PostWidgetTypes handles POST /widget-types
func (h WidgetTypesHandlers) PostWidgetTypes(w http.ResponseWriter, r *http.Request) {
	var request apiv0.CreateWidgetTypeRequest
	if !decodeRequest(w, r, &request) {
		return
	}

	created, err := h.service.CreateWidgetType(r.Context(), newCreateWidgetTypeInput(request))
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusCreated, newCreateWidgetTypeResponse(created))
}

// PutWidgetTypesByID handles PUT /widget-types/{widget_type_id}
func (h WidgetTypesHandlers) PutWidgetTypesByID(w http.ResponseWriter, r *http.Request) {
	inID, ok := pathUUID(w, r, widgetTypeResource)
	if !ok {
		return
	}

	var request apiv0.UpdateWidgetTypeRequest
	if !decodeRequest(w, r, &request) {
		return
	}

	updated, err := h.service.UpdateWidgetType(r.Context(), inID, request.Version, newUpdateWidgetTypeInput(request))
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newUpdateWidgetTypeResponse(updated))
}

// DeleteWidgetTypesByID handles DELETE /widget-types/{widget_type_id}
func (h WidgetTypesHandlers) DeleteWidgetTypesByID(w http.ResponseWriter, r *http.Request) {
	inID, ok := pathUUID(w, r, widgetTypeResource)
	if !ok {
		return
	}

	deleted, err := h.service.DeleteWidgetTypeByID(r.Context(), inID)
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newWidgetTypeResponse(deleted))
}
