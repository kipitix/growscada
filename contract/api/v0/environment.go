package apiv0

import (
	"github.com/google/uuid"
)

// EnvironmentResponse is the HTTP DTO of an Environment of a project's
// Draft: a named set of its Devices as they stand in one context of
// operation. Comment is descriptive metadata.
type EnvironmentResponse struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Comment string    `json:"comment"`
	Version int       `json:"version"`
}

// GetEnvironmentsResponse is the HTTP DTO for the Environments of a project,
// in creation order.
type GetEnvironmentsResponse struct {
	Environments []EnvironmentResponse `json:"environments"`
}

// CreateEnvironmentRequest is the HTTP DTO for creating an Environment.
type CreateEnvironmentRequest struct {
	Name    string `json:"name"`
	Comment string `json:"comment"`
}

// CreateEnvironmentResponse is the HTTP DTO for an Environment creation
// response. Version is the new Environment's version, the one its first
// update states.
type CreateEnvironmentResponse struct {
	ID      uuid.UUID `json:"id"`
	Version int       `json:"version"`
}

// UpdateEnvironmentRequest is the HTTP DTO for changing an Environment.
// Version must match the current persisted version for optimistic locking.
type UpdateEnvironmentRequest struct {
	Name    string `json:"name"`
	Comment string `json:"comment"`
	Version int    `json:"version"`
}

// UpdateEnvironmentResponse is the HTTP DTO for an Environment update
// response.
type UpdateEnvironmentResponse struct {
	Version int `json:"version"`
}

// EnvironmentDeviceResponse is the HTTP DTO of one Device as configured in
// one Environment: the pair (Environment, Device) and its connection
// parameters. The pair has no ID of its own: EnvironmentID and DeviceID name
// it.
type EnvironmentDeviceResponse struct {
	EnvironmentID uuid.UUID  `json:"environment_id"`
	DeviceID      uuid.UUID  `json:"device_id"`
	Params        JSONObject `json:"params"`
	Version       int        `json:"version"`
}

// GetEnvironmentDevicesResponse is the HTTP DTO for the Devices of an
// Environment, in creation order.
type GetEnvironmentDevicesResponse struct {
	Devices []EnvironmentDeviceResponse `json:"devices"`
}

// UpdateEnvironmentDeviceRequest is the HTTP DTO of PUT
// /projects/{id}/environments/{environment_id}/devices/{device_id}: the
// connection parameters of the pair. Version 0 adds the pair (the
// Environment must not hold the Device yet); any other changes it and must
// match the pair's current version.
type UpdateEnvironmentDeviceRequest struct {
	Params  JSONObject `json:"params"`
	Version int        `json:"version"`
}
