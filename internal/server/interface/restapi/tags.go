package restapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/tag"
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

// GetTags handles GET /tags request to retrieve the list of tags.
// At most one name filter may be given:
//   - ?name=<name> — exact unique name, the list holds 0 or 1 tags;
//   - ?name_pattern=<pattern> — wildcard over the whole name ("*" any sequence, "?" one character);
//   - ?name_regex=<regex> — Go (RE2) regular expression matched anywhere in the name.
func (h TagsHandlers) GetTags(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	filters := 0
	for _, param := range []string{apiv0.TagsQueryName, apiv0.TagsQueryNamePattern, apiv0.TagsQueryNameRegex} {
		if query.Has(param) {
			filters++
		}
	}
	if filters > 1 {
		sendJSONResponse(w, http.StatusBadRequest,
			NewBadRequest("use only one of name, name_pattern, name_regex", r.URL.Path))
		return
	}

	switch {
	case query.Has(apiv0.TagsQueryName):
		h.getTagsByName(w, r, query.Get(apiv0.TagsQueryName))
		return
	case query.Has(apiv0.TagsQueryNamePattern):
		h.sendMatchedTags(w, r, h.service.FindTagsByNamePattern, query.Get(apiv0.TagsQueryNamePattern))
		return
	case query.Has(apiv0.TagsQueryNameRegex):
		h.sendMatchedTags(w, r, h.service.FindTagsByNameRegex, query.Get(apiv0.TagsQueryNameRegex))
		return
	}

	tagList, err := h.service.FindAllTags(r.Context())
	if err != nil {
		sendInternalError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetTagsResponse(tagList))
}

// getTagsByName handles GET /tags?name=<name>
func (h TagsHandlers) getTagsByName(w http.ResponseWriter, r *http.Request, name string) {
	foundTag, err := h.service.FindTagByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, tag.ErrTagNotFound) {
			sendJSONResponse(w, http.StatusOK, newGetTagsResponse(nil))
			return
		}
		sendInternalError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetTagsResponse([]appdto.Tag{foundTag}))
}

// sendMatchedTags responds with the tags selected by a name pattern or regex;
// an invalid one is a 400.
func (h TagsHandlers) sendMatchedTags(w http.ResponseWriter, r *http.Request,
	find func(context.Context, string) ([]appdto.Tag, error), filter string) {
	tagList, err := find(r.Context(), filter)
	if err != nil {
		if errors.Is(err, tag.ErrInvalidTagNameMatcher) {
			sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
			return
		}
		sendInternalError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetTagsResponse(tagList))
}

// GetTagsByID handles GET /tags/{id}
func (h TagsHandlers) GetTagsByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	tagID, err := uuid.Parse(idStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	foundTag, err := h.service.FindTagByID(r.Context(), tagID)
	if err != nil {
		if errors.Is(err, tag.ErrTagNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), idStr, r.URL.Path))
			return
		}
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newTagResponse(foundTag))
}

// DeleteTagsByID handles DELETE /tags/{id} for removing a tag
func (h TagsHandlers) DeleteTagsByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	tagID, err := uuid.Parse(idStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	deletedTag, err := h.service.DeleteTagByID(r.Context(), tagID)
	if err != nil {
		if errors.Is(err, tag.ErrTagNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), idStr, r.URL.Path))
			return
		}
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newTagResponse(deletedTag))
}

// PatchTagsValue handles PATCH /tags/{id}/value for setting tag value and quality
func (h TagsHandlers) PatchTagsValue(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	tagID, err := uuid.Parse(idStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	var request apiv0.UpdateTagRequest
	if !decodeRequest(w, r, &request) {
		return
	}

	updatedTag, err := h.service.SetTagValueByID(r.Context(), newUpdateTagInput(request, tagID))
	if err != nil {
		if errors.Is(err, tag.ErrTagNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), idStr, r.URL.Path))
			return
		}
		if errors.Is(err, tag.ErrTagConflict) {
			sendJSONResponse(w, http.StatusConflict, NewConflict("tag", err.Error(), r.URL.Path))
			return
		}
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newUpdateTagResponse(updatedTag))
}

// PostTags handles POST /tags for creating a new tag
func (h TagsHandlers) PostTags(w http.ResponseWriter, r *http.Request) {
	var request apiv0.CreateTagRequest

	if !decodeRequest(w, r, &request) {
		return
	}

	createdTag, err := h.service.CreateTag(r.Context(), newCreateTagInput(request))
	if err != nil {
		if errors.Is(err, tag.ErrTagNameTaken) {
			sendJSONResponse(w, http.StatusConflict, NewConflict("tag", err.Error(), r.URL.Path))
			return
		}
		sendInternalError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusCreated, newCreateTagResponse(createdTag))
}
