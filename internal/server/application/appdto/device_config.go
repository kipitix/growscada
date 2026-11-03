package appdto

import (
	"encoding/json"

	"github.com/google/uuid"
)

// DeviceConfig is what a Device Gateway needs to run one Device in one
// Environment: the Device itself, the Environment it stands in, the
// connection parameters of the pair and the Tags the Device supplies.
type DeviceConfig struct {
	Device      Device
	Environment Environment
	// Connection is the raw parameters of the pair (Environment, Device).
	Connection json.RawMessage
	Tags       []DeviceConfigTag
}

// DeviceConfigTag is a Tag the Gateway writes: its definition's identity,
// name and type.
type DeviceConfigTag struct {
	ID   uuid.UUID
	Name string
	Type string
}
