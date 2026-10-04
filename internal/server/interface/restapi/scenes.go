package restapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/domain/scene"
)

// ScenesHandlers handles HTTP requests related to scenes.
type ScenesHandlers struct {
	service application.SceneService
}

func NewScenesHandler(s application.SceneService) *ScenesHandlers {
	return &ScenesHandlers{service: s}
}

// GetScenes handles GET /scenes
func (h ScenesHandlers) GetScenes(w http.ResponseWriter, r *http.Request) {
	list, err := h.service.FindAllScenes(r.Context())
	if err != nil {
		sendServiceError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetScenesResponse(list))
}

// GetScenesByID handles GET /scenes/{id}
func (h ScenesHandlers) GetScenesByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	sceneID, err := uuid.Parse(idStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	found, err := h.service.FindSceneByID(r.Context(), sceneID)
	if err != nil {
		if errors.Is(err, scene.ErrSceneNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), idStr, r.URL.Path))
			return
		}
		sendServiceError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newSceneResponse(found))
}

// PostScenes handles POST /scenes
func (h ScenesHandlers) PostScenes(w http.ResponseWriter, r *http.Request) {
	var request apiv0.CreateSceneRequest
	if !decodeRequest(w, r, &request) {
		return
	}

	created, err := h.service.CreateScene(r.Context(), newCreateSceneInput(request))
	if err != nil {
		sendServiceError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusCreated, newCreateSceneResponse(created))
}

// PutScenesByID handles PUT /scenes/{id}
func (h ScenesHandlers) PutScenesByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	sceneID, err := uuid.Parse(idStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	var request apiv0.UpdateSceneRequest
	if !decodeRequest(w, r, &request) {
		return
	}

	updated, err := h.service.UpdateScene(r.Context(), newUpdateSceneInput(request, sceneID))
	if err != nil {
		if errors.Is(err, scene.ErrSceneNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), idStr, r.URL.Path))
			return
		}
		if errors.Is(err, scene.ErrSceneConflict) {
			sendJSONResponse(w, http.StatusConflict, NewConflict("scene", err.Error(), r.URL.Path))
			return
		}
		sendServiceError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newUpdateSceneResponse(updated))
}

// DeleteScenesByID handles DELETE /scenes/{id}
func (h ScenesHandlers) DeleteScenesByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	sceneID, err := uuid.Parse(idStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	deleted, err := h.service.DeleteSceneByID(r.Context(), sceneID)
	if err != nil {
		if errors.Is(err, scene.ErrSceneNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), idStr, r.URL.Path))
			return
		}
		sendServiceError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newSceneResponse(deleted))
}
