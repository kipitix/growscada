package restapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/application"
)

// maxProjectFileBytes caps the document a POST of a project file may carry.
const maxProjectFileBytes = 64 << 20

// ProjectFileHandlers serves the project files of a Project (task 39,
// ADR 0018): its Draft and any Deployment as downloads, a file opened as the
// Draft, and a file deployed as a Release and a Deployment (task 72_4).
type ProjectFileHandlers struct {
	files    application.ProjectFileService
	releases application.ReleaseService
}

// NewProjectFileHandler creates the handler over the Engineering node's
// project files and Releases.
func NewProjectFileHandler(files application.ProjectFileService, releases application.ReleaseService) *ProjectFileHandlers {
	return &ProjectFileHandlers{files: files, releases: releases}
}

// GetDraftProjectFile handles GET /projects/{project_id}/projectfile: the
// Draft as a ProjectFile, assembled by the builder a Deploy uses. 409
// draft_not_deployable when the Draft cannot be built.
func (h ProjectFileHandlers) GetDraftProjectFile(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	download, err := h.files.Draft(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendProjectFile(w, download.File, projectFileName(download.ProjectName, projectID, "draft"))
}

// GetDeploymentProjectFile handles GET /projects/{project_id}/deployments/{deployment_number}/projectfile:
// the exact document the Runtime node stored for Deployment n.
func (h ProjectFileHandlers) GetDeploymentProjectFile(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	number, ok := pathInt(w, r, deploymentResource)
	if !ok {
		return
	}
	download, err := h.files.Deployment(r.Context(), projectID, number)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendProjectFile(w, download.File, projectFileName(download.ProjectName, projectID, fmt.Sprintf("deployment-%d", number)))
}

// PostProjectFile handles POST /projects/{project_id}/projectfile?version=V:
// it opens the document as the whole Draft, in one DraftChange. 200 with the
// Draft's new Version; 409 nothing_to_open when the file is the Draft
// already; 400 with file_issues when it does not validate.
func (h ProjectFileHandlers) PostProjectFile(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	version, ok := versionQuery(w, r)
	if !ok {
		return
	}
	document, ok := readDocument(w, r)
	if !ok {
		return
	}
	state, err := h.files.Open(r.Context(), projectID, version, document)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, apiv0.DraftStateResponse{Version: state.Version})
}

// PostProjectFileDeploy handles POST /projects/{project_id}/projectfile/deploy:
// it publishes the document as a Release with origin file and makes it the
// Project's new Deployment without touching the Draft (task 72_4). ?author= and
// ?title= are optional; the file and the plain title stand for them. 201 with
// the Deployment; 409 nothing_to_deploy when it is the running one; 400 with
// file_issues when it does not validate.
func (h ProjectFileHandlers) PostProjectFileDeploy(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	document, ok := readDocument(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	_, deployment, err := h.releases.CreateFromFile(r.Context(), projectID, document,
		query.Get("author"), query.Get("title"))
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusCreated, newDeployResponse(deployment))
}

// readDocument reads the ProjectFile a POST carries. It has already sent a 400
// on failure and returns false.
func readDocument(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	document, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxProjectFileBytes))
	if err != nil {
		sendProblem(w, NewBadRequest("the project file cannot be read: "+err.Error(), r.URL.Path))
		return nil, false
	}
	return document, true
}

// sendProjectFile writes a ProjectFile as an attachment, the document as it
// is and without a wrapper (ADR 0018). The API's schema version stays a
// header; the document carries the project format's own version.
func sendProjectFile(w http.ResponseWriter, document projectv0.ProjectFile, filename string) {
	body, err := json.Marshal(document)
	if err != nil {
		sendProblem(w, NewInternalError("the project file cannot be encoded", ""))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// projectFileName names a download "<project>-<suffix>.json": the project's
// name reduced to safe characters, the Project's ID when nothing is left of it
// (ADR 0018).
func projectFileName(projectName string, projectID uuid.UUID, suffix string) string {
	safe := safeFileStem(projectName)
	if safe == "" {
		safe = projectID.String()
	}
	return safe + "-" + suffix + ".json"
}

// safeFileStem keeps ASCII letters, digits, dots, dashes and underscores of a
// name, turns every other run of characters into one dash and trims the dashes
// and dots around it. A name of nothing but unsafe characters has no stem.
func safeFileStem(name string) string {
	var builder strings.Builder
	dash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			builder.WriteRune(r)
			dash = false
		case !dash && builder.Len() > 0:
			builder.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(builder.String(), "-.")
}
