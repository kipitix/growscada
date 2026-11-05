package restapi

import (
	"net/http"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
)

// EnvironmentsHandlers handles HTTP requests related to the Environments of a
// project's Draft and the parameters of its Devices in them.
type EnvironmentsHandlers struct {
	service application.EnvironmentService
}

// NewEnvironmentsHandler creates a new handler for Environments.
func NewEnvironmentsHandler(s application.EnvironmentService) *EnvironmentsHandlers {
	return &EnvironmentsHandlers{service: s}
}

// GetEnvironments handles GET /projects/{project_id}/environments
func (h EnvironmentsHandlers) GetEnvironments(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	list, err := h.service.FindAllEnvironments(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetEnvironmentsResponse(list))
}

// GetEnvironmentsByID handles GET
// /projects/{project_id}/environments/{environment_id}
func (h EnvironmentsHandlers) GetEnvironmentsByID(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	environmentID, ok := pathUUID(w, r, environmentResource)
	if !ok {
		return
	}
	found, err := h.service.FindEnvironmentByID(r.Context(), projectID, environmentID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newEnvironmentResponse(found))
}

// PostEnvironments handles POST /projects/{project_id}/environments
func (h EnvironmentsHandlers) PostEnvironments(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	var request apiv0.CreateEnvironmentRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	created, err := h.service.CreateEnvironment(r.Context(), projectID, request.Version, newCreateEnvironmentInput(request))
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusCreated, newCreateEnvironmentResponse(created))
}

// PutEnvironmentsByID handles PUT
// /projects/{project_id}/environments/{environment_id}
func (h EnvironmentsHandlers) PutEnvironmentsByID(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	environmentID, ok := pathUUID(w, r, environmentResource)
	if !ok {
		return
	}
	var request apiv0.UpdateEnvironmentRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	updated, err := h.service.UpdateEnvironment(r.Context(), projectID, environmentID, request.Version, newUpdateEnvironmentInput(request))
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newUpdateEnvironmentResponse(updated))
}

// DeleteEnvironmentsByID handles DELETE
// /projects/{project_id}/environments/{environment_id}. Refused while the
// Environment still holds the pairs of its Devices (409).
func (h EnvironmentsHandlers) DeleteEnvironmentsByID(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	environmentID, ok := pathUUID(w, r, environmentResource)
	if !ok {
		return
	}
	version, ok := versionQuery(w, r)
	if !ok {
		return
	}
	deleted, err := h.service.DeleteEnvironmentByID(r.Context(), projectID, environmentID, version)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newEnvironmentResponse(deleted))
}

// GetEnvironmentDevices handles GET
// /projects/{project_id}/environments/{environment_id}/devices
func (h EnvironmentsHandlers) GetEnvironmentDevices(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	environmentID, ok := pathUUID(w, r, environmentResource)
	if !ok {
		return
	}
	list, err := h.service.FindEnvironmentDevices(r.Context(), projectID, environmentID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetEnvironmentDevicesResponse(list))
}

// PutEnvironmentDevice handles PUT
// /projects/{project_id}/environments/{environment_id}/devices/{device_id}:
// adds the pair (201) or changes its parameters (200).
func (h EnvironmentsHandlers) PutEnvironmentDevice(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	environmentID, ok := pathUUID(w, r, environmentResource)
	if !ok {
		return
	}
	deviceID, ok := pathUUID(w, r, deviceResource)
	if !ok {
		return
	}
	var request apiv0.UpdateEnvironmentDeviceRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	saved, created, err := h.service.PutEnvironmentDevice(r.Context(), projectID, environmentID, deviceID, request.Version, newUpdateEnvironmentDeviceInput(request))
	if err != nil {
		sendError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	sendJSONResponse(w, status, newEnvironmentDeviceResponse(saved))
}

// DeleteEnvironmentDevice handles DELETE
// /projects/{project_id}/environments/{environment_id}/devices/{device_id}
func (h EnvironmentsHandlers) DeleteEnvironmentDevice(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	environmentID, ok := pathUUID(w, r, environmentResource)
	if !ok {
		return
	}
	deviceID, ok := pathUUID(w, r, deviceResource)
	if !ok {
		return
	}
	version, ok := versionQuery(w, r)
	if !ok {
		return
	}
	deleted, err := h.service.DeleteEnvironmentDevice(r.Context(), projectID, environmentID, deviceID, version)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newEnvironmentDeviceResponse(deleted))
}
