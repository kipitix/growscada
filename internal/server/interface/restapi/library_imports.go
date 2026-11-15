package restapi

import (
	"net/http"
	"strconv"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
)

// LibraryImportsHandlers handles HTTP requests related to the Library Imports
// of a Project's Draft (task 85_6): the saved state of a Library the Project
// uses, its impact preview and the WidgetTypes it gives the Project.
type LibraryImportsHandlers struct {
	service application.LibraryImportService
}

func NewLibraryImportsHandler(s application.LibraryImportService) *LibraryImportsHandlers {
	return &LibraryImportsHandlers{service: s}
}

// GetLibraryImports handles GET /projects/{project_id}/library-imports
func (h LibraryImportsHandlers) GetLibraryImports(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	list, err := h.service.FindLibraryImports(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetLibraryImportsResponse(list))
}

// PutLibraryImport handles PUT /projects/{project_id}/library-imports/{library_id}:
// adds the import (201) or switches it (200). A switch that would leave Widgets
// without their WidgetType is refused (409 naming them).
func (h LibraryImportsHandlers) PutLibraryImport(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	libraryID, ok := pathUUID(w, r, libraryImportResource)
	if !ok {
		return
	}
	var request apiv0.PutLibraryImportRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	saved, created, err := h.service.PutLibraryImport(r.Context(), projectID, libraryID, request.SavedState, request.Version)
	if err != nil {
		sendError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	sendJSONResponse(w, status, newLibraryImportResponse(saved))
}

// DeleteLibraryImport handles DELETE
// /projects/{project_id}/library-imports/{library_id}. Refused while Widgets of
// the project are of the imported WidgetTypes (409 naming them).
func (h LibraryImportsHandlers) DeleteLibraryImport(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	libraryID, ok := pathUUID(w, r, libraryImportResource)
	if !ok {
		return
	}
	version, ok := versionQuery(w, r)
	if !ok {
		return
	}

	deleted, err := h.service.DeleteLibraryImport(r.Context(), projectID, libraryID, version)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newLibraryImportResponse(deleted))
}

// GetLibraryImportImpact handles GET
// /projects/{project_id}/library-imports/{library_id}/impact?saved_state=<n>: the
// preview of importing the saved state n, or of removing the import when the
// parameter is absent.
func (h LibraryImportsHandlers) GetLibraryImportImpact(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	libraryID, ok := pathUUID(w, r, libraryImportResource)
	if !ok {
		return
	}
	var savedState *int
	if query := r.URL.Query(); query.Has(apiv0.LibraryImportImpactQuerySavedState) {
		raw := query.Get(apiv0.LibraryImportImpactQuerySavedState)
		number, err := strconv.Atoi(raw)
		if err != nil || number < 1 {
			sendProblem(w, NewBadRequest(apiv0.LibraryImportImpactQuerySavedState+": '"+raw+"' is not a saved state number", r.URL.Path))
			return
		}
		savedState = &number
	}
	impact, err := h.service.ImportImpact(r.Context(), projectID, libraryID, savedState)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newImportImpactResponse(impact))
}

// GetImportedWidgetTypes handles GET /projects/{project_id}/widget-types: the
// WidgetTypes of the saved states the project imports — the palette of its
// Scenes.
func (h LibraryImportsHandlers) GetImportedWidgetTypes(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	list, err := h.service.FindImportedWidgetTypes(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, apiv0.GetImportedWidgetTypesResponse{WidgetTypes: newWidgetTypeCopyResponses(list)})
}
