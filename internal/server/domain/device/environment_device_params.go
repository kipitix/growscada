package device

import (
	"encoding/json"
	"slices"
)

// EnvironmentDeviceParams - the parameters by which a Device is reached in an
// Environment: the address, the port, the credentials — read under the
// Device's protocol. An empty set of parameters is the empty JSON object.
// Value Object: immutable, its JSON copied in and out.
type EnvironmentDeviceParams struct {
	raw json.RawMessage
}

// NewEnvironmentDeviceParams creates EnvironmentDeviceParams from their JSON
// representation; no input at all is the empty object.
// Returns an error if raw is not a JSON object.
func NewEnvironmentDeviceParams(raw []byte) (EnvironmentDeviceParams, error) {
	object, err := newJSONObject(raw, "environment device params")
	if err != nil {
		return EnvironmentDeviceParams{}, err
	}
	return EnvironmentDeviceParams{raw: object}, nil
}

// EmptyEnvironmentDeviceParams are the parameters of a Device in an
// Environment that needs none yet.
func EmptyEnvironmentDeviceParams() EnvironmentDeviceParams {
	return EnvironmentDeviceParams{raw: json.RawMessage("{}")}
}

// Bytes returns the parameters' JSON: a copy, so that changing it changes no
// EnvironmentDevice.
func (p EnvironmentDeviceParams) Bytes() []byte { return slices.Clone(p.raw) }

// String implements [fmt.Stringer].
func (p EnvironmentDeviceParams) String() string { return string(p.raw) }
