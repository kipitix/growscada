package apiv0

import (
	"github.com/google/uuid"
)

// DeviceResponse is the HTTP DTO of a Device of a project's Draft: what a
// thing is — its protocol and the configuration read under it. Where it
// stands and how it is reached is an EnvironmentDevice's.
// Comment is descriptive metadata of the Device; it never affects behavior.
type DeviceResponse struct {
	ID       uuid.UUID  `json:"id"`
	Name     string     `json:"name"`
	Comment  string     `json:"comment"`
	Protocol string     `json:"protocol"`
	Config   JSONObject `json:"config"`
	Version  int        `json:"version"`
}

// GetDevicesResponse is the HTTP DTO for the Devices of a project, in
// creation order.
type GetDevicesResponse struct {
	Devices []DeviceResponse `json:"devices"`
}

// CreateDeviceRequest is the HTTP DTO for creating a Device. Config is the
// configuration read under Protocol; an omitted one is the empty object.
type CreateDeviceRequest struct {
	Name     string     `json:"name"`
	Comment  string     `json:"comment"`
	Protocol string     `json:"protocol"`
	Config   JSONObject `json:"config"`
}

// CreateDeviceResponse is the HTTP DTO for a Device creation response.
// Version is the new Device's version, the one its first update states.
type CreateDeviceResponse struct {
	ID      uuid.UUID `json:"id"`
	Version int       `json:"version"`
}

// UpdateDeviceRequest is the HTTP DTO for changing a Device.
// Version must match the current persisted version for optimistic locking.
type UpdateDeviceRequest struct {
	Name     string     `json:"name"`
	Comment  string     `json:"comment"`
	Protocol string     `json:"protocol"`
	Config   JSONObject `json:"config"`
	Version  int        `json:"version"`
}

// UpdateDeviceResponse is the HTTP DTO for a Device update response.
type UpdateDeviceResponse struct {
	Version int `json:"version"`
}
