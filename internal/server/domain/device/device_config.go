package device

import (
	"encoding/json"
	"slices"
)

// DeviceConfig - the configuration of a Device read under its protocol: what
// the protocol says about the Device itself (for ProtocolSimulation, the
// patterns of what to write). An empty configuration is the empty JSON
// object.
// Value Object: immutable, its JSON copied in and out.
type DeviceConfig struct {
	raw json.RawMessage
}

// NewDeviceConfig creates a DeviceConfig from its JSON representation; no
// input at all is the empty object.
// Returns an error if raw is not a JSON object.
func NewDeviceConfig(raw []byte) (DeviceConfig, error) {
	object, err := newJSONObject(raw, "device config")
	if err != nil {
		return DeviceConfig{}, err
	}
	return DeviceConfig{raw: object}, nil
}

// EmptyDeviceConfig is the configuration of a Device whose protocol has
// nothing to configure yet.
func EmptyDeviceConfig() DeviceConfig {
	return DeviceConfig{raw: json.RawMessage("{}")}
}

// Bytes returns the configuration's JSON: a copy, so that changing it changes
// no Device. The zero value is the empty object.
func (c DeviceConfig) Bytes() []byte {
	if len(c.raw) == 0 {
		return []byte("{}")
	}
	return slices.Clone(c.raw)
}

// String implements [fmt.Stringer]. The zero value is the empty object.
func (c DeviceConfig) String() string {
	if len(c.raw) == 0 {
		return "{}"
	}
	return string(c.raw)
}
