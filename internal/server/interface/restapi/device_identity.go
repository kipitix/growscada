package restapi

import (
	"net/http"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
)

// DeviceIdentityHandlers answers a Device Gateway which Device and Project
// its token acts for. The token names both; nothing else is needed, and
// nothing of the request does.
type DeviceIdentityHandlers struct {
	tokens runtimeapp.DeviceTokenService
}

// NewDeviceIdentityHandler creates a new handler for the Gateway's identity.
func NewDeviceIdentityHandler(tokens runtimeapp.DeviceTokenService) *DeviceIdentityHandlers {
	return &DeviceIdentityHandlers{tokens: tokens}
}

// GetDeviceIdentity handles GET /api/v0/device: the Project and the Device the
// token acts for. A Gateway reads it first: the Project is part of every
// write's path, and a token is the only thing that names it. An unknown or
// revoked token is 401, as everywhere on the Gateway's routes.
func (h DeviceIdentityHandlers) GetDeviceIdentity(w http.ResponseWriter, r *http.Request) {
	token, ok := authenticateDevice(w, r, h.tokens)
	if !ok {
		return
	}
	sendJSONResponse(w, http.StatusOK, apiv0.DeviceIdentityResponse{
		ProjectID: token.ProjectID,
		DeviceID:  token.DeviceID,
	})
}
