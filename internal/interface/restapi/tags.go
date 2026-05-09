package restapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kipitix/growscada/internal/application"
	appdto "github.com/kipitix/growscada/internal/application/dto"
	"github.com/kipitix/growscada/internal/domain/tag"
	restdto "github.com/kipitix/growscada/internal/interface/restapi/dto"
)

// TagsHandlers handles HTTP requests related to tags.
// It depends on the application layer abstraction.
type TagsHandlers struct {
	service application.TagService
}

// NewTagsHandler creates a new handler for tags.
// Accepts a tag service and returns a pointer to TagsHandlers.
func NewTagsHandler(s application.TagService) *TagsHandlers {
	return &TagsHandlers{service: s}
}

// GetTags handles GET /tags request to retrieve the list of tags
func (h TagsHandlers) GetTags(w http.ResponseWriter, r *http.Request) {
	tagList, err := h.service.FindAllTags(r.Context())
	if err != nil {
		sendJSONResponse(w, http.StatusInternalServerError, NewInternalError(err.Error(), r.URL.Path))
		return
	}
	sendJSONResponse(w, http.StatusOK, restdto.NewGetTagsResponse(tagList))
}

// GetTagsByID handles GET /tags/{id}
func (h TagsHandlers) GetTagsByID(w http.ResponseWriter, r *http.Request) {
	tagIDString := r.PathValue("id")
	tagID, err := tag.ParseTagID(tagIDString)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	foundTag, err := h.service.FindTagByID(r.Context(), tagID)
	if err != nil {
		if errors.Is(err, tag.ErrTagNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), tagIDString, r.URL.Path))
			return
		}
		sendJSONResponse(w, http.StatusInternalServerError, NewInternalError(err.Error(), r.URL.Path))
		return
	}

	sendJSONResponse(w, http.StatusOK, restdto.NewTagResponse(foundTag))
}

// DeleteTagsByID handles DELETE /tags/{id} for removing a tag
func (h TagsHandlers) DeleteTagsByID(w http.ResponseWriter, r *http.Request) {
	tagIDString := r.PathValue("id")
	tagID, err := tag.ParseTagID(tagIDString)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	deletedTag, err := h.service.DeleteTagByID(r.Context(), tagID)
	if err != nil {
		if errors.Is(err, tag.ErrTagNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), tagIDString, r.URL.Path))
			return
		}
		sendJSONResponse(w, http.StatusInternalServerError, NewInternalError(err.Error(), r.URL.Path))
		return
	}

	sendJSONResponse(w, http.StatusOK, restdto.DeleteTagResponse{Tag: restdto.NewTagResponse(deletedTag)})
}

// PatchTagsValue handles PATCH /tags/{id}/value for setting tag value and quality
func (h TagsHandlers) PatchTagsValue(w http.ResponseWriter, r *http.Request) {
	tagIDString := r.PathValue("id")
	tagID, err := tag.ParseTagID(tagIDString)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	var request restdto.UpdateTagRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	updatedTag, err := h.service.SetTagValueByID(r.Context(), appdto.UpdateTagRequest{
		ID:      tagID.UUID(),
		Value:   request.Value,
		Quality: request.Quality,
	})
	if err != nil {
		if errors.Is(err, tag.ErrTagNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), tagIDString, r.URL.Path))
			return
		}
		sendJSONResponse(w, http.StatusInternalServerError, NewInternalError(err.Error(), r.URL.Path))
		return
	}

	sendJSONResponse(w, http.StatusOK, restdto.UpdateTagResponse{Version: updatedTag.Version})
}

// PostTags handles POST /tags for creating a new tag
func (h TagsHandlers) PostTags(w http.ResponseWriter, r *http.Request) {
	var request restdto.CreateTagRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	createdTag, err := h.service.CreateTag(r.Context(), appdto.CreateTagRequest{
		Name:    request.Name,
		Type:    request.Type,
		Value:   request.Value,
		Quality: request.Quality,
	})
	if err != nil {
		sendJSONResponse(w, http.StatusInternalServerError, NewInternalError(err.Error(), r.URL.Path))
		return
	}

	sendJSONResponse(w, http.StatusCreated, restdto.CreateTagResponse{ID: createdTag.ID})
}
