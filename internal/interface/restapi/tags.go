package restapi

import (
	"encoding/json"
	"net/http"

	"github.com/kipitix/growscada/internal/application"
	"github.com/kipitix/growscada/internal/application/dto"
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonData, err := json.Marshal(tagList)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(jsonData); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// GetTagsByID handles GET /tags/{id}
func (h TagsHandlers) GetTagsByID(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	w.WriteHeader(http.StatusNotAcceptable)
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
		errorResponse := NewInternalError(err.Error(), r.URL.Path)
		sendJSONResponse(w, http.StatusInternalServerError, errorResponse)
	}

	sendJSONResponse(w, http.StatusCreated, response)
}
