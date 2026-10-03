package restapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/widget"
)

// WidgetsHandlers handles HTTP requests related to widget instances. Widget is
// an entity of the Scene aggregate, so every route here is nested under a scene.
type WidgetsHandlers struct {
	service application.SceneService
}

func NewWidgetsHandler(s application.SceneService) *WidgetsHandlers {
	return &WidgetsHandlers{service: s}
}

// GetWidgetsBySceneID handles GET /scenes/{id}/widgets
func (h WidgetsHandlers) GetWidgetsBySceneID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	sceneID, err := uuid.Parse(idStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	list, err := h.service.FindWidgetsBySceneID(r.Context(), sceneID)
	if err != nil {
		if errors.Is(err, scene.ErrSceneNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), idStr, r.URL.Path))
			return
		}
		sendInternalError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetWidgetsResponse(list))
}

// GetWidgetsByID handles GET /scenes/{sceneId}/widgets/{widgetId}
func (h WidgetsHandlers) GetWidgetsByID(w http.ResponseWriter, r *http.Request) {
	sceneIDStr := r.PathValue("sceneId")
	sceneID, err := uuid.Parse(sceneIDStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}
	widgetIDStr := r.PathValue("widgetId")
	widgetID, err := uuid.Parse(widgetIDStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	found, err := h.service.FindWidgetByID(r.Context(), sceneID, widgetID)
	if err != nil {
		if errors.Is(err, scene.ErrSceneNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), sceneIDStr, r.URL.Path))
			return
		}
		if errors.Is(err, widget.ErrWidgetNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), widgetIDStr, r.URL.Path))
			return
		}
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newWidgetResponse(found))
}

// PostWidgets handles POST /scenes/{id}/widgets
func (h WidgetsHandlers) PostWidgets(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	sceneID, err := uuid.Parse(idStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	var request apiv0.CreateWidgetRequest
	if !decodeRequest(w, r, &request) {
		return
	}

	created, err := h.service.CreateWidget(r.Context(), sceneID, newCreateWidgetInput(request))
	if err != nil {
		if errors.Is(err, widget.ErrWidgetInvalidInput) {
			sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
			return
		}
		if errors.Is(err, scene.ErrSceneNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), idStr, r.URL.Path))
			return
		}
		if errors.Is(err, scene.ErrSceneConflict) {
			sendJSONResponse(w, http.StatusConflict, NewConflict("scene", err.Error(), r.URL.Path))
			return
		}
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusCreated, newCreateWidgetResponse(created))
}

// PutWidgetsByID handles PUT /scenes/{sceneId}/widgets/{widgetId}
func (h WidgetsHandlers) PutWidgetsByID(w http.ResponseWriter, r *http.Request) {
	sceneIDStr := r.PathValue("sceneId")
	sceneID, err := uuid.Parse(sceneIDStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}
	widgetIDStr := r.PathValue("widgetId")
	widgetID, err := uuid.Parse(widgetIDStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	var request apiv0.UpdateWidgetRequest
	if !decodeRequest(w, r, &request) {
		return
	}

	updated, err := h.service.UpdateWidget(r.Context(), sceneID, widgetID, newUpdateWidgetInput(request, widgetID))
	if err != nil {
		if errors.Is(err, widget.ErrWidgetInvalidInput) {
			sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
			return
		}
		if errors.Is(err, scene.ErrSceneNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), sceneIDStr, r.URL.Path))
			return
		}
		if errors.Is(err, widget.ErrWidgetNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), widgetIDStr, r.URL.Path))
			return
		}
		if errors.Is(err, scene.ErrSceneConflict) {
			sendJSONResponse(w, http.StatusConflict, NewConflict("scene", err.Error(), r.URL.Path))
			return
		}
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newUpdateWidgetResponse(updated))
}

// DeleteWidgetsByID handles DELETE /scenes/{sceneId}/widgets/{widgetId}
func (h WidgetsHandlers) DeleteWidgetsByID(w http.ResponseWriter, r *http.Request) {
	sceneIDStr := r.PathValue("sceneId")
	sceneID, err := uuid.Parse(sceneIDStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}
	widgetIDStr := r.PathValue("widgetId")
	widgetID, err := uuid.Parse(widgetIDStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	deleted, err := h.service.DeleteWidgetByID(r.Context(), sceneID, widgetID)
	if err != nil {
		if errors.Is(err, scene.ErrSceneNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), sceneIDStr, r.URL.Path))
			return
		}
		if errors.Is(err, widget.ErrWidgetNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), widgetIDStr, r.URL.Path))
			return
		}
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newWidgetResponse(deleted))
}
