package restapi

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
)

// DeviceTokensHandlers handles the management of Device Tokens: issue,
// rotate, revoke and read the metadata of the one token a Device has. The
// tokens are the Runtime node's (ADR 0011): on a server without that role
// there is no management at all, and the routes answer 404.
//
// The Device is checked in the Draft through the Engineering node's service:
// the route names a Device of a project, and a token is never issued for one
// that does not exist.
type DeviceTokensHandlers struct {
	tokens  runtimeapp.DeviceTokenService
	devices application.DeviceService
}

// NewDeviceTokensHandler creates a new handler for Device Tokens.
func NewDeviceTokensHandler(tokens runtimeapp.DeviceTokenService, devices application.DeviceService) *DeviceTokensHandlers {
	return &DeviceTokensHandlers{tokens: tokens, devices: devices}
}

// PostDeviceToken handles POST
// /projects/{project_id}/devices/{device_id}/token: issues the Device's token
// (201). The secret is in the response alone; it is never shown again. A
// Device that already has a token gives 409.
func (h DeviceTokensHandlers) PostDeviceToken(w http.ResponseWriter, r *http.Request) {
	projectID, deviceID, ok := h.deviceOf(w, r)
	if !ok {
		return
	}
	issued, err := h.tokens.Issue(r.Context(), projectID, deviceID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusCreated, newDeviceTokenResponse(issued))
}

// PostDeviceTokenRotate handles POST
// /projects/{project_id}/devices/{device_id}/token/rotate: replaces the token,
// revoking the old one (201). A Device without a token gets one.
func (h DeviceTokensHandlers) PostDeviceTokenRotate(w http.ResponseWriter, r *http.Request) {
	projectID, deviceID, ok := h.deviceOf(w, r)
	if !ok {
		return
	}
	issued, err := h.tokens.Rotate(r.Context(), projectID, deviceID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusCreated, newDeviceTokenResponse(issued))
}

// DeleteDeviceToken handles DELETE
// /projects/{project_id}/devices/{device_id}/token: revokes the token and
// returns its metadata. A Device without a token gives 404.
func (h DeviceTokensHandlers) DeleteDeviceToken(w http.ResponseWriter, r *http.Request) {
	projectID, deviceID, ok := h.deviceOf(w, r)
	if !ok {
		return
	}
	revoked, err := h.tokens.Revoke(r.Context(), projectID, deviceID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newDeviceTokenMetadataResponse(revoked))
}

// GetDeviceToken handles GET
// /projects/{project_id}/devices/{device_id}/token: the token's metadata, the
// secret never included. A Device without a token gives 404.
func (h DeviceTokensHandlers) GetDeviceToken(w http.ResponseWriter, r *http.Request) {
	projectID, deviceID, ok := h.deviceOf(w, r)
	if !ok {
		return
	}
	found, err := h.tokens.FindToken(r.Context(), projectID, deviceID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newDeviceTokenMetadataResponse(found))
}

// deviceOf parses the path and checks that the Device is one of the Project's.
// Without a token service (a server with the Engineering role alone) there is
// no management: 404. On failure it has already sent the problem.
func (h DeviceTokensHandlers) deviceOf(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	deviceID, ok := pathUUID(w, r, deviceResource)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	if h.tokens == nil {
		sendProblem(w, NewNotFound(deviceTokenResource.name, deviceID.String(), r.URL.Path))
		return uuid.Nil, uuid.Nil, false
	}
	if _, err := h.devices.FindDeviceByID(r.Context(), projectID, deviceID); err != nil {
		sendError(w, r, err)
		return uuid.Nil, uuid.Nil, false
	}
	return projectID, deviceID, true
}
