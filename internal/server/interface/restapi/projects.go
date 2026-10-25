package restapi

import (
	"net/http"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
)

// ProjectsHandlers handles HTTP requests related to projects. The content of
// a project is served under its path by the handlers of that content.
type ProjectsHandlers struct {
	service application.ProjectService
}

func NewProjectsHandler(s application.ProjectService) *ProjectsHandlers {
	return &ProjectsHandlers{service: s}
}

// GetProjects handles GET /projects, the live projects in creation order.
// ?name=<name> selects the project with that exact name: the list holds 0
// or 1 projects.
func (h ProjectsHandlers) GetProjects(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if query.Has(apiv0.ProjectsQueryName) {
		found, err := h.service.FindProjectByName(r.Context(), query.Get(apiv0.ProjectsQueryName))
		if err != nil {
			if isNotFound(err, projectResource) {
				sendJSONResponse(w, http.StatusOK, newGetProjectsResponse(nil))
				return
			}
			sendError(w, r, err)
			return
		}
		sendJSONResponse(w, http.StatusOK, newGetProjectsResponse([]appdto.Project{found}))
		return
	}

	list, err := h.service.FindAllProjects(r.Context())
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetProjectsResponse(list))
}

// GetProjectsByID handles GET /projects/{project_id}
func (h ProjectsHandlers) GetProjectsByID(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}

	found, err := h.service.FindProjectByID(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newProjectResponse(found))
}

// PostProjects handles POST /projects
func (h ProjectsHandlers) PostProjects(w http.ResponseWriter, r *http.Request) {
	var request apiv0.CreateProjectRequest
	if !decodeRequest(w, r, &request) {
		return
	}

	created, err := h.service.CreateProject(r.Context(), appdto.ProjectInput{Name: request.Name})
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusCreated, apiv0.CreateProjectResponse{ID: created.ID, Version: created.Version})
}

// PutProjectsByID handles PUT /projects/{project_id}: a rename.
func (h ProjectsHandlers) PutProjectsByID(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}

	var request apiv0.UpdateProjectRequest
	if !decodeRequest(w, r, &request) {
		return
	}

	updated, err := h.service.UpdateProject(r.Context(), projectID, request.Version, appdto.ProjectInput{Name: request.Name})
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, apiv0.UpdateProjectResponse{Version: updated.Version})
}

// DeleteProjectsByID handles DELETE /projects/{project_id}. The project is
// gone for the API at once; its content is removed after the response.
func (h ProjectsHandlers) DeleteProjectsByID(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}

	deleted, err := h.service.DeleteProjectByID(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}

	sendJSONResponse(w, http.StatusOK, newProjectResponse(deleted))
}
