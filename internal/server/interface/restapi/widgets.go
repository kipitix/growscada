package restapi

import (
	"net/http"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
)

// WidgetsHandlers handles HTTP requests related to widget instances. Widget is
// an entity of the Scene aggregate, so every route here is nested under a scene.
type WidgetsHandlers struct {
	service application.SceneService
}

func NewWidgetsHandler(s application.SceneService) *WidgetsHandlers {
	return &WidgetsHandlers{service: s}
}

// GetWidgetsBySceneID handles GET /projects/{project_id}/scenes/{scene_id}/widgets
func (h WidgetsHandlers) GetWidgetsBySceneID(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	sceneID, ok := pathUUID(w, r, sceneResource)
	if !ok {
		return
	}

	list, err := h.service.FindWidgetsBySceneID(r.Context(), projectID, sceneID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetWidgetsResponse(list))
}

// GetWidgetsByID handles GET /projects/{project_id}/scenes/{scene_id}/widgets/{widget_id}
func (h WidgetsHandlers) GetWidgetsByID(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	sceneID, ok := pathUUID(w, r, sceneResource)
	if !ok {
		return
	}
	widgetID, ok := pathUUID(w, r, widgetResource)
	if !ok {
		return
	}

	found, err := h.service.FindWidgetByID(r.Context(), projectID, sceneID, widgetID)
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newWidgetResponse(found))
}

// PostWidgets handles POST /projects/{project_id}/scenes/{scene_id}/widgets
func (h WidgetsHandlers) PostWidgets(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	sceneID, ok := pathUUID(w, r, sceneResource)
	if !ok {
		return
	}

	var request apiv0.CreateWidgetRequest
	if !decodeRequest(w, r, &request) {
		return
	}

	created, err := h.service.CreateWidget(r.Context(), projectID, sceneID, request.SceneVersion, newCreateWidgetInput(request))
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusCreated, newCreateWidgetResponse(created))
}

// PutWidgetsByID handles PUT /projects/{project_id}/scenes/{scene_id}/widgets/{widget_id}
func (h WidgetsHandlers) PutWidgetsByID(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	sceneID, ok := pathUUID(w, r, sceneResource)
	if !ok {
		return
	}
	widgetID, ok := pathUUID(w, r, widgetResource)
	if !ok {
		return
	}

	var request apiv0.UpdateWidgetRequest
	if !decodeRequest(w, r, &request) {
		return
	}

	updated, err := h.service.UpdateWidget(r.Context(), projectID, sceneID, widgetID, request.SceneVersion, newUpdateWidgetInput(request))
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newUpdateWidgetResponse(updated))
}

// DeleteWidgetsByID handles DELETE /projects/{project_id}/scenes/{scene_id}/widgets/{widget_id}
func (h WidgetsHandlers) DeleteWidgetsByID(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	sceneID, ok := pathUUID(w, r, sceneResource)
	if !ok {
		return
	}
	widgetID, ok := pathUUID(w, r, widgetResource)
	if !ok {
		return
	}

	deleted, err := h.service.DeleteWidgetByID(r.Context(), projectID, sceneID, widgetID)
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newWidgetResponse(deleted))
}
