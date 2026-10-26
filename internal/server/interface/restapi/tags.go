package restapi

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
)

// TagsHandlers handles HTTP requests related to tags.
// It depends on the application layer abstraction: the Engineering node's
// tag service for the definitions (read with their states), the Runtime
// node's for the writes of states (ADR 0011).
type TagsHandlers struct {
	service application.TagService
	states  runtimeapp.TagStateService
}

// NewTagsHandler creates a new handler for tags.
// Accepts the tag services and returns a pointer to TagsHandlers.
func NewTagsHandler(s application.TagService, states runtimeapp.TagStateService) *TagsHandlers {
	return &TagsHandlers{service: s, states: states}
}

// GetTags handles GET /projects/{project_id}/tags request to retrieve the
// list of the project's tags. At most one name filter may be given:
//   - ?name=<name> — exact name, unique in the project; the list holds 0 or 1 tags;
//   - ?name_pattern=<pattern> — wildcard over the whole name ("*" any sequence, "?" one character);
//   - ?name_regex=<regex> — Go (RE2) regular expression matched anywhere in the name.
func (h TagsHandlers) GetTags(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	query := r.URL.Query()
	filters := 0
	for _, param := range []string{apiv0.TagsQueryName, apiv0.TagsQueryNamePattern, apiv0.TagsQueryNameRegex} {
		if query.Has(param) {
			filters++
		}
	}
	if filters > 1 {
		sendProblem(w, NewBadRequest("use only one of name, name_pattern, name_regex", r.URL.Path))
		return
	}

	switch {
	case query.Has(apiv0.TagsQueryName):
		h.getTagsByName(w, r, projectID, query.Get(apiv0.TagsQueryName))
		return
	case query.Has(apiv0.TagsQueryNamePattern):
		h.sendMatchedTags(w, r, projectID, h.service.FindTagsByNamePattern, query.Get(apiv0.TagsQueryNamePattern))
		return
	case query.Has(apiv0.TagsQueryNameRegex):
		h.sendMatchedTags(w, r, projectID, h.service.FindTagsByNameRegex, query.Get(apiv0.TagsQueryNameRegex))
		return
	}

	tagList, err := h.service.FindAllTags(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetTagsResponse(tagList))
}

// getTagsByName handles GET /projects/{project_id}/tags?name=<name>
func (h TagsHandlers) getTagsByName(w http.ResponseWriter, r *http.Request, projectID uuid.UUID, name string) {
	foundTag, err := h.service.FindTagByName(r.Context(), projectID, name)
	if err != nil {
		if isNotFound(err, tagResource) {
			sendJSONResponse(w, http.StatusOK, newGetTagsResponse(nil))
			return
		}
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetTagsResponse([]appdto.Tag{foundTag}))
}

// sendMatchedTags responds with the tags selected by a name pattern or regex;
// an invalid one is a 400.
func (h TagsHandlers) sendMatchedTags(w http.ResponseWriter, r *http.Request, projectID uuid.UUID,
	find func(context.Context, uuid.UUID, string) ([]appdto.Tag, error), filter string) {
	tagList, err := find(r.Context(), projectID, filter)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetTagsResponse(tagList))
}

// GetTagsByID handles GET /projects/{project_id}/tags/{tag_id}
func (h TagsHandlers) GetTagsByID(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	tagID, ok := pathUUID(w, r, tagResource)
	if !ok {
		return
	}

	foundTag, err := h.service.FindTagByID(r.Context(), projectID, tagID)
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newTagResponse(foundTag))
}

// DeleteTagsByID handles DELETE /projects/{project_id}/tags/{tag_id} for removing a tag
func (h TagsHandlers) DeleteTagsByID(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	tagID, ok := pathUUID(w, r, tagResource)
	if !ok {
		return
	}

	deletedTag, err := h.service.DeleteTagByID(r.Context(), projectID, tagID)
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newTagResponse(deletedTag))
}

// PatchTagsValue handles PATCH /projects/{project_id}/tags/{tag_id}/value for
// setting tag value and quality: a write of the tag's state on the Runtime
// node, with no expected version.
func (h TagsHandlers) PatchTagsValue(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	tagID, ok := pathUUID(w, r, tagResource)
	if !ok {
		return
	}

	var request apiv0.UpdateTagRequest
	if !decodeRequest(w, r, &request) {
		return
	}

	written, err := h.states.SetTagValue(r.Context(), projectID, newSetTagValueInput(request, tagID))
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newUpdateTagResponse(written))
}

// PostTags handles POST /projects/{project_id}/tags for creating a new tag
func (h TagsHandlers) PostTags(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	var request apiv0.CreateTagRequest

	if !decodeRequest(w, r, &request) {
		return
	}

	createdTag, err := h.service.CreateTag(r.Context(), projectID, newCreateTagInput(request))
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusCreated, newCreateTagResponse(createdTag))
}
