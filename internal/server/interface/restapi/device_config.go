package restapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// DeviceConfigHandlers serves a Device Gateway the configuration of its
// Device. The Device is named by the token the request carries, the
// Environment by the query; the Project is the token's.
type DeviceConfigHandlers struct {
	service runtimeapp.DeviceConfigService
	tokens  runtimeapp.DeviceTokenService
}

// NewDeviceConfigHandler creates a new handler for the Gateways'
// configuration.
func NewDeviceConfigHandler(s runtimeapp.DeviceConfigService, tokens runtimeapp.DeviceTokenService) *DeviceConfigHandlers {
	return &DeviceConfigHandlers{service: s, tokens: tokens}
}

// GetDeviceConfig handles GET /device/config?environment=<id>: the Device the
// token acts for, the Environment it serves, the connection parameters of
// the pair and the Tags the Device supplies. The Device must be in the
// project's configuration: a token whose Device is not — a dormant token that
// outlived a Deploy — is refused with 403, so the Gateway waits instead of
// failing for good.
func (h DeviceConfigHandlers) GetDeviceConfig(w http.ResponseWriter, r *http.Request) {
	// Authenticated first: an unauthenticated request is a 401, whatever its
	// query says.
	token, ok := authenticateDevice(w, r, h.tokens)
	if !ok {
		return
	}
	query := r.URL.Query()
	rawEnvironmentID := query.Get(apiv0.DeviceConfigQueryEnvironment)
	if rawEnvironmentID == "" {
		sendProblem(w, NewBadRequest(apiv0.DeviceConfigQueryEnvironment+": the environment is required", r.URL.Path))
		return
	}
	environmentID, err := uuid.Parse(rawEnvironmentID)
	if err != nil {
		sendProblem(w, NewBadRequest(apiv0.DeviceConfigQueryEnvironment+": '"+rawEnvironmentID+"' is not a valid UUID", r.URL.Path))
		return
	}
	config, err := h.service.Config(r.Context(), token.ProjectID, token.DeviceID, environmentID)
	switch {
	case err == nil:
		sendJSONResponse(w, http.StatusOK, newDeviceConfigResponse(config))
	case errors.Is(err, device.ErrDeviceNotFound):
		sendProblem(w, NewForbidden("the Device is not in the project's running configuration", r.URL.Path))
	case errors.Is(err, project.ErrProjectNotFound):
		sendProblem(w, NewNotFound(projectResource.name, token.ProjectID.String(), r.URL.Path))
	case errors.Is(err, device.ErrEnvironmentNotFound):
		sendProblem(w, NewNotFound(environmentResource.name, environmentID.String(), r.URL.Path))
	case errors.Is(err, device.ErrEnvironmentDeviceNotFound):
		pair := environmentID.String() + "/" + token.DeviceID.String()
		sendProblem(w, NewNotFound(environmentDeviceResource.name, pair, r.URL.Path))
	default:
		sendError(w, r, err)
	}
}
