package restapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/domain/draft"
)

// ReleaseHandlers serves the Releases of a Project (task 72_3; ADR 0021): a
// Commit is published as an immutable Release, the Releases are listed and
// read, a note is changed, and a Release is downloaded as the ProjectFile it
// copied.
type ReleaseHandlers struct {
	releases application.ReleaseService
}

// NewReleaseHandler creates the handler over the Engineering node's Releases.
func NewReleaseHandler(releases application.ReleaseService) *ReleaseHandlers {
	return &ReleaseHandlers{releases: releases}
}

// PostRelease handles POST …/releases: the publication of a Commit. 201 with
// the Release; 409 commit_already_released; 404 for the Project or the Commit;
// 400 for a missing Commit number, an empty author or an over-long note.
func (h ReleaseHandlers) PostRelease(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	var request apiv0.CreateProjectReleaseRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	created, err := h.releases.Create(r.Context(), projectID, request.Commit, request.Note, request.Author)
	if err != nil {
		// The Commit is named by the body, so the path cannot say which one
		// was missing: answer with the number the request holds.
		if errors.Is(err, draft.ErrCommitNotFound) {
			sendProblem(w, NewNotFound(commitResource.name, strconv.Itoa(request.Commit), r.URL.Path))
			return
		}
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusCreated, newReleaseResponse(created))
}

// GetReleases handles GET …/releases: the Project's Releases, newest first,
// without content.
func (h ReleaseHandlers) GetReleases(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	releases, err := h.releases.List(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetReleasesResponse(releases))
}

// GetReleaseByNumber handles GET …/releases/{release_number}: one Release,
// with the counts of its snapshot. 404 for a number the Project does not
// have.
func (h ReleaseHandlers) GetReleaseByNumber(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	number, ok := pathInt(w, r, projectReleaseResource)
	if !ok {
		return
	}
	found, err := h.releases.Find(r.Context(), projectID, number)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newReleaseResponse(found))
}

// PatchRelease handles PATCH …/releases/{release_number}: a new note. The
// content of the Release never changes and no Draft Version is stated.
func (h ReleaseHandlers) PatchRelease(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	number, ok := pathInt(w, r, projectReleaseResource)
	if !ok {
		return
	}
	var request apiv0.UpdateReleaseNoteRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	updated, err := h.releases.UpdateNote(r.Context(), projectID, number, request.Note)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newReleaseResponse(updated))
}

// GetReleaseProjectFile handles GET …/releases/{release_number}/projectfile:
// the exact document the Release copied, as an attachment.
func (h ReleaseHandlers) GetReleaseProjectFile(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	number, ok := pathInt(w, r, projectReleaseResource)
	if !ok {
		return
	}
	download, err := h.releases.ProjectFile(r.Context(), projectID, number)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendProjectFile(w, download.File, projectFileName(download.ProjectName, projectID, fmt.Sprintf("release-%d", number)))
}
