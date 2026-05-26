package restapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kipitix/growscada/internal/application"
	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/widget"
	"github.com/kipitix/growscada/internal/interface/restapi/restdto"
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
		sendInternalError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, restdto.NewGetWidgetTypesResponse(list))
}

// GetWidgetTypesByID handles GET /widget-types/{id}
func (h WidgetTypesHandlers) GetWidgetTypesByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")

	idOpt, err := id.IDWithString[widget.WidgetType](idStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	inID, err := id.NewID[widget.WidgetType](idOpt)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	found, err := h.service.FindWidgetTypeByID(r.Context(), inID)
	if err != nil {
		if errors.Is(err, widget.ErrWidgetTypeNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), idStr, r.URL.Path))
			return
		}
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, restdto.NewWidgetTypeResponse(found))
}

// PostWidgetTypes handles POST /widget-types
func (h WidgetTypesHandlers) PostWidgetTypes(w http.ResponseWriter, r *http.Request) {
	var request restdto.CreateWidgetTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	created, err := h.service.CreateWidgetType(r.Context(), restdto.NewCreateWidgetTypeInput(request))
	if err != nil {
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusCreated, restdto.NewCreateWidgetTypeResponse(created))
}

// PutWidgetTypesByID handles PUT /widget-types/{id}
func (h WidgetTypesHandlers) PutWidgetTypesByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")

	idOpt, err := id.IDWithString[widget.WidgetType](idStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	inID, err := id.NewID[widget.WidgetType](idOpt)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	var request restdto.UpdateWidgetTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	updated, err := h.service.UpdateWidgetType(r.Context(), restdto.NewUpdateWidgetTypeInput(request, inID))
	if err != nil {
		if errors.Is(err, widget.ErrWidgetTypeNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), idStr, r.URL.Path))
			return
		}
		if errors.Is(err, widget.ErrWidgetTypeConflict) {
			sendJSONResponse(w, http.StatusConflict, NewConflict("widget type", err.Error(), r.URL.Path))
			return
		}
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, restdto.NewUpdateWidgetTypeResponse(updated))
}

// DeleteWidgetTypesByID handles DELETE /widget-types/{id}
func (h WidgetTypesHandlers) DeleteWidgetTypesByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")

	idOpt, err := id.IDWithString[widget.WidgetType](idStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	inID, err := id.NewID[widget.WidgetType](idOpt)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	deleted, err := h.service.DeleteWidgetTypeByID(r.Context(), inID)
	if err != nil {
		if errors.Is(err, widget.ErrWidgetTypeNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), idStr, r.URL.Path))
			return
		}
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, restdto.NewWidgetTypeResponse(deleted))
}
