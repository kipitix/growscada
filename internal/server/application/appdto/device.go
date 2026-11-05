package appdto

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/device"
)

// Device is the application-layer DTO for Device data: what a thing is — its
// protocol and the configuration read under it — of one Project's Draft. The
// connection parameters are the EnvironmentDevice's.
type Device struct {
	ID       uuid.UUID
	Name     string
	Comment  string
	Protocol string
	Config   json.RawMessage
	Version  int
}

// DeviceInput holds the fields of a Device a client creates or changes. The
// Device's ID and the Version a change expects are passed separately.
type DeviceInput struct {
	Name     string
	Comment  string
	Protocol string
	Config   json.RawMessage
}

// Environment is the application-layer DTO for Environment data: a named set
// of the Project's Devices as they stand in one context of operation.
type Environment struct {
	ID      uuid.UUID
	Name    string
	Comment string
	Version int
}

// EnvironmentInput holds the fields of an Environment a client creates or
// changes. The Environment's ID and the Version a change expects are passed
// separately.
type EnvironmentInput struct {
	Name    string
	Comment string
}

// EnvironmentDevice is the application-layer DTO for a pair of an Environment
// and a Device, with the parameters by which the Device is reached in the
// Environment. The pair has no ID of its own: its Environment and its Device
// name it.
type EnvironmentDevice struct {
	EnvironmentID uuid.UUID
	DeviceID      uuid.UUID
	Params        json.RawMessage
	Version       int
}

// EnvironmentDeviceInput holds the parameters of a pair a client adds or
// changes. The pair's IDs and the Version a change expects are passed
// separately.
type EnvironmentDeviceInput struct {
	Params json.RawMessage
}

// NewDevice creates a Device DTO from the domain aggregate.
func NewDevice(aDevice device.Device, version int) Device {
	return Device{
		ID:       aDevice.ID().UUID(),
		Name:     aDevice.Name().String(),
		Comment:  aDevice.Comment(),
		Protocol: aDevice.Protocol().String(),
		Config:   aDevice.Config().Bytes(),
		Version:  version,
	}
}

// NewDeviceList creates a slice of Device DTOs from domain aggregates.
func NewDeviceList(list []device.Device, version int) []Device {
	result := make([]Device, len(list))
	for i, d := range list {
		result[i] = NewDevice(d, version)
	}
	return result
}

// NewEnvironment creates an Environment DTO from the domain aggregate.
func NewEnvironment(anEnvironment device.Environment, version int) Environment {
	return Environment{
		ID:      anEnvironment.ID().UUID(),
		Name:    anEnvironment.Name().String(),
		Comment: anEnvironment.Comment(),
		Version: version,
	}
}

// NewEnvironmentList creates a slice of Environment DTOs from domain
// aggregates.
func NewEnvironmentList(list []device.Environment, version int) []Environment {
	result := make([]Environment, len(list))
	for i, e := range list {
		result[i] = NewEnvironment(e, version)
	}
	return result
}

// NewEnvironmentDevice creates an EnvironmentDevice DTO from the domain
// aggregate.
func NewEnvironmentDevice(aPair device.EnvironmentDevice, version int) EnvironmentDevice {
	return EnvironmentDevice{
		EnvironmentID: aPair.EnvironmentID().UUID(),
		DeviceID:      aPair.DeviceID().UUID(),
		Params:        aPair.Params().Bytes(),
		Version:       version,
	}
}

// NewEnvironmentDeviceList creates a slice of EnvironmentDevice DTOs from
// domain aggregates.
func NewEnvironmentDeviceList(list []device.EnvironmentDevice, version int) []EnvironmentDevice {
	result := make([]EnvironmentDevice, len(list))
	for i, p := range list {
		result[i] = NewEnvironmentDevice(p, version)
	}
	return result
}
