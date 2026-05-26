package restapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/application"
	"github.com/kipitix/growscada/internal/domain/widget"
	"github.com/kipitix/growscada/internal/interface/restapi/restdto"
)

// WidgetsHandlers handles HTTP requests related to widget instances.
type WidgetsHandlers struct {
	service application.WidgetService
}

func NewWidgetsHandler(s application.WidgetService) *WidgetsHandlers {
	return &WidgetsHandlers{service: s}
}

// GetWidgets handles GET /widgets
func (h WidgetsHandlers) GetWidgets(w http.ResponseWriter, r *http.Request) {
	list, err := h.service.FindAllWidgets(r.Context())
	if err != nil {
		sendInternalError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, restdto.NewGetWidgetsResponse(list))
}

// GetWidgetsByID handles GET /widgets/{id}
func (h WidgetsHandlers) GetWidgetsByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	widgetID, err := uuid.Parse(idStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	found, err := h.service.FindWidgetByID(r.Context(), widgetID)
	if err != nil {
		if errors.Is(err, widget.ErrWidgetNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), idStr, r.URL.Path))
			return
		}
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, restdto.NewWidgetResponse(found))
}

// PostWidgets handles POST /widgets
func (h WidgetsHandlers) PostWidgets(w http.ResponseWriter, r *http.Request) {
	var request restdto.CreateWidgetRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	created, err := h.service.CreateWidget(r.Context(), restdto.NewCreateWidgetInput(request))
	if err != nil {
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusCreated, restdto.NewCreateWidgetResponse(created))
}

// PutWidgetsByID handles PUT /widgets/{id}
func (h WidgetsHandlers) PutWidgetsByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	widgetID, err := uuid.Parse(idStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	var request restdto.UpdateWidgetRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	updated, err := h.service.UpdateWidget(r.Context(), restdto.NewUpdateWidgetInput(request, widgetID))
	if err != nil {
		if errors.Is(err, widget.ErrWidgetNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), idStr, r.URL.Path))
			return
		}
		if errors.Is(err, widget.ErrWidgetConflict) {
			sendJSONResponse(w, http.StatusConflict, NewConflict("widget", err.Error(), r.URL.Path))
			return
		}
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, restdto.NewUpdateWidgetResponse(updated))
}

// DeleteWidgetsByID handles DELETE /widgets/{id}
func (h WidgetsHandlers) DeleteWidgetsByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	widgetID, err := uuid.Parse(idStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	deleted, err := h.service.DeleteWidgetByID(r.Context(), widgetID)
	if err != nil {
		if errors.Is(err, widget.ErrWidgetNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), idStr, r.URL.Path))
			return
		}
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, restdto.NewWidgetResponse(deleted))
}
