package restapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kipitix/growscada/internal/application"
	"github.com/kipitix/growscada/internal/application/dto"
	"github.com/kipitix/growscada/internal/domain/tag"
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
	sendJSONResponse(w, http.StatusOK, tagList)
}

// GetTagsByID handles GET /tags/{id}
func (h TagsHandlers) GetTagsByID(w http.ResponseWriter, r *http.Request) {
	tagIDString := r.PathValue("id")
	tagID, err := tag.ParseTagID(tagIDString)
	if err != nil {
		errorResponse := NewBadRequest(err.Error(), r.URL.Path)
		sendJSONResponse(w, http.StatusBadRequest, errorResponse)
		return
	}

	foundTag, err := h.service.FindTagByID(r.Context(), tagID)
	if err != nil {
		// Specific error when not found
		if errors.Is(err, tag.ErrTagNotFound) {
			errorResponse := NewNotFound(err.Error(), tagIDString, r.URL.Path)
			sendJSONResponse(w, http.StatusNotFound, errorResponse)
			return
		}
		// Generic error
		errorResponse := NewInternalError(err.Error(), r.URL.Path)
		sendJSONResponse(w, http.StatusInternalServerError, errorResponse)
		return
	}

	sendJSONResponse(w, http.StatusOK, foundTag)
}

// DeleteTagByID handles DELETE /tags/{id} for removing a tag
func (h TagsHandlers) DeleteTagByID(w http.ResponseWriter, r *http.Request) {
	tagIDString := r.PathValue("id")
	tagID, err := tag.ParseTagID(tagIDString)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	response, err := h.service.DeleteTag(r.Context(), tagID)
	if err != nil {
		if errors.Is(err, tag.ErrTagNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), tagIDString, r.URL.Path))
			return
		}
		sendJSONResponse(w, http.StatusInternalServerError, NewInternalError(err.Error(), r.URL.Path))
		return
	}

	sendJSONResponse(w, http.StatusOK, response)
}

// PatchTagValue handles PATCH /tags/{id}/value for setting tag value and quality
func (h TagsHandlers) PatchTagValue(w http.ResponseWriter, r *http.Request) {
	tagIDString := r.PathValue("id")
	tagID, err := tag.ParseTagID(tagIDString)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	var request dto.UpdateTagRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}
	request.ID = tagID.UUID()

	response, err := h.service.SetTagValueByID(r.Context(), request)
	if err != nil {
		if errors.Is(err, tag.ErrTagNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), tagIDString, r.URL.Path))
			return
		}
		sendJSONResponse(w, http.StatusInternalServerError, NewInternalError(err.Error(), r.URL.Path))
		return
	}

	sendJSONResponse(w, http.StatusOK, response)
}

// PostTags handles POST /tags for creating a new tag
func (h TagsHandlers) PostTags(w http.ResponseWriter, r *http.Request) {
	var request dto.CreateTagRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		errorResponse := NewBadRequest(err.Error(), r.URL.Path)
		sendJSONResponse(w, http.StatusBadRequest, errorResponse)
		return
	}

	response, err := h.service.CreateTag(r.Context(), request)
	if err != nil {
		sendJSONResponse(w, http.StatusInternalServerError, NewInternalError(err.Error(), r.URL.Path))
		return
	}

	sendJSONResponse(w, http.StatusCreated, response)
}
