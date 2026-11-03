package apiv0

import (
	"github.com/google/uuid"
)

// DeviceConfigQueryEnvironment is the parameter of GET /device/config naming
// the Environment the Device Gateway serves: the request answers with the
// connection parameters of the pair (Environment, Device) and with the Tags
// the Device supplies. A Gateway knows which Environment it stands in; the
// token alone names the Device.
const DeviceConfigQueryEnvironment = "environment"

// DeviceConfigResponse is the HTTP DTO of the configuration of a Device
// Gateway: the Device it acts for, the Environment it serves, the connection
// parameters of that pair and the Tags the Device supplies.
type DeviceConfigResponse struct {
	Device      DeviceConfigDeviceResponse      `json:"device"`
	Environment DeviceConfigEnvironmentResponse `json:"environment"`
	Connection  DeviceConfigConnectionResponse  `json:"connection"`
	Tags        []DeviceConfigTagResponse       `json:"tags"`
}

// DeviceConfigDeviceResponse is what a Device Gateway must execute: the
// Device's protocol and the configuration read under it.
type DeviceConfigDeviceResponse struct {
	ID       uuid.UUID  `json:"id"`
	Name     string     `json:"name"`
	Protocol string     `json:"protocol"`
	Config   JSONObject `json:"config"`
}

// DeviceConfigEnvironmentResponse names the Environment the Gateway serves.
type DeviceConfigEnvironmentResponse struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// DeviceConfigConnectionResponse is how the Device is reached in the
// Environment: the parameters of the pair.
type DeviceConfigConnectionResponse struct {
	Params JSONObject `json:"params"`
}

// DeviceConfigTagResponse is a Tag the Device supplies: its definition —
// the identity by which the Gateway writes its values.
type DeviceConfigTagResponse struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Type string    `json:"type"`
}
