package restapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/application"
	"github.com/kipitix/growscada/internal/domain/scene"
	"github.com/kipitix/growscada/internal/interface/restapi/restdto"
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
		sendInternalError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, restdto.NewGetScenesResponse(list))
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
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, restdto.NewSceneResponse(found))
}

// PostScenes handles POST /scenes
func (h ScenesHandlers) PostScenes(w http.ResponseWriter, r *http.Request) {
	var request restdto.CreateSceneRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	created, err := h.service.CreateScene(r.Context(), restdto.NewCreateSceneInput(request))
	if err != nil {
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusCreated, restdto.NewCreateSceneResponse(created))
}

// PutScenesByID handles PUT /scenes/{id}
func (h ScenesHandlers) PutScenesByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	sceneID, err := uuid.Parse(idStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	var request restdto.UpdateSceneRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	updated, err := h.service.UpdateScene(r.Context(), restdto.NewUpdateSceneInput(request, sceneID))
	if err != nil {
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

	sendJSONResponse(w, http.StatusOK, restdto.NewUpdateSceneResponse(updated))
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
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, restdto.NewSceneResponse(deleted))
}
