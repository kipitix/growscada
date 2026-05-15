package restapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kipitix/growscada/internal/application"
	"github.com/kipitix/growscada/internal/domain/tag"
	"github.com/kipitix/growscada/internal/interface/restapi/restdto"
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
		sendInternalError(w, r, err)
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
		sendInternalError(w, r, err)
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
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, restdto.NewTagResponse(deletedTag))
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

	updatedTag, err := h.service.SetTagValueByID(r.Context(), restdto.NewUpdateTagInput(request, tagID.UUID()))
	if err != nil {
		if errors.Is(err, tag.ErrTagNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), tagIDString, r.URL.Path))
			return
		}
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, restdto.NewUpdateTagResponse(updatedTag))
}

// PostTags handles POST /tags for creating a new tag
func (h TagsHandlers) PostTags(w http.ResponseWriter, r *http.Request) {
	var request restdto.CreateTagRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	createdTag, err := h.service.CreateTag(r.Context(), restdto.NewCreateTagInput(request))
	if err != nil {
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusCreated, restdto.NewCreateTagResponse(createdTag))
}
