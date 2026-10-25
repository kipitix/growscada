package restapi

import (
	"net/http"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
)

// ScenesHandlers handles HTTP requests related to scenes.
type ScenesHandlers struct {
	service application.SceneService
}

func NewScenesHandler(s application.SceneService) *ScenesHandlers {
	return &ScenesHandlers{service: s}
}

// GetScenes handles GET /projects/{project_id}/scenes
func (h ScenesHandlers) GetScenes(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	list, err := h.service.FindAllScenes(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetScenesResponse(list))
}

// GetScenesByID handles GET /projects/{project_id}/scenes/{scene_id}
func (h ScenesHandlers) GetScenesByID(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	sceneID, ok := pathUUID(w, r, sceneResource)
	if !ok {
		return
	}

	found, err := h.service.FindSceneByID(r.Context(), projectID, sceneID)
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newSceneResponse(found))
}

// PostScenes handles POST /projects/{project_id}/scenes
func (h ScenesHandlers) PostScenes(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	var request apiv0.CreateSceneRequest
	if !decodeRequest(w, r, &request) {
		return
	}

	created, err := h.service.CreateScene(r.Context(), projectID, newCreateSceneInput(request))
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusCreated, newCreateSceneResponse(created))
}

// PutScenesByID handles PUT /projects/{project_id}/scenes/{scene_id}
func (h ScenesHandlers) PutScenesByID(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	sceneID, ok := pathUUID(w, r, sceneResource)
	if !ok {
		return
	}

	var request apiv0.UpdateSceneRequest
	if !decodeRequest(w, r, &request) {
		return
	}

	updated, err := h.service.UpdateScene(r.Context(), projectID, sceneID, request.Version, newUpdateSceneInput(request))
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newUpdateSceneResponse(updated))
}

// DeleteScenesByID handles DELETE /projects/{project_id}/scenes/{scene_id}
func (h ScenesHandlers) DeleteScenesByID(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	sceneID, ok := pathUUID(w, r, sceneResource)
	if !ok {
		return
	}

	deleted, err := h.service.DeleteSceneByID(r.Context(), projectID, sceneID)
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newSceneResponse(deleted))
}
