package restapi

import (
	"net/http"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
)

// DevicesHandlers handles HTTP requests related to the Devices of a project's
// Draft.
type DevicesHandlers struct {
	service application.DeviceService
}

// NewDevicesHandler creates a new handler for Devices.
func NewDevicesHandler(s application.DeviceService) *DevicesHandlers {
	return &DevicesHandlers{service: s}
}

// GetDevices handles GET /projects/{project_id}/devices
func (h DevicesHandlers) GetDevices(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	list, err := h.service.FindAllDevices(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetDevicesResponse(list))
}

// GetDevicesByID handles GET /projects/{project_id}/devices/{device_id}
func (h DevicesHandlers) GetDevicesByID(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	deviceID, ok := pathUUID(w, r, deviceResource)
	if !ok {
		return
	}
	found, err := h.service.FindDeviceByID(r.Context(), projectID, deviceID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newDeviceResponse(found))
}

// PostDevices handles POST /projects/{project_id}/devices
func (h DevicesHandlers) PostDevices(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	var request apiv0.CreateDeviceRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	created, err := h.service.CreateDevice(r.Context(), projectID, request.Version, newCreateDeviceInput(request))
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusCreated, newCreateDeviceResponse(created))
}

// PutDevicesByID handles PUT /projects/{project_id}/devices/{device_id}
func (h DevicesHandlers) PutDevicesByID(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	deviceID, ok := pathUUID(w, r, deviceResource)
	if !ok {
		return
	}
	var request apiv0.UpdateDeviceRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	updated, err := h.service.UpdateDevice(r.Context(), projectID, deviceID, request.Version, newUpdateDeviceInput(request))
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newUpdateDeviceResponse(updated))
}

// DeleteDevicesByID handles DELETE /projects/{project_id}/devices/{device_id}.
// Refused while a Tag is supplied by the Device or it stands in an
// Environment (409).
func (h DevicesHandlers) DeleteDevicesByID(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
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
	deleted, err := h.service.DeleteDeviceByID(r.Context(), projectID, deviceID, version)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newDeviceResponse(deleted))
}
