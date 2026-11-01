package device

import "fmt"

// DeviceProtocol - the protocol a Device speaks: what kind of thing it is, and
// how the configuration in its Device exists under it. It is a closed set: a
// Gateway must know how to read a Device, so a protocol is added here, with
// its Gateway, never taken from input as free text.
// The zero value is invalid (see docs/adr/0001-no-unknown-enum-sentinels.md).
// Value Object.
type DeviceProtocol struct {
	protocol string
}

var _ fmt.Stringer = DeviceProtocol{}

// ProtocolSimulation is the protocol of the simulator: the configuration
// holds the patterns of what to write.
var ProtocolSimulation = DeviceProtocol{protocol: "simulation"}

// NewDeviceProtocol parses a string into the enum.
// Returns an error for any string that is not a known protocol.
func NewDeviceProtocol(s string) (DeviceProtocol, error) {
	if s == ProtocolSimulation.protocol {
		return ProtocolSimulation, nil
	}
	return DeviceProtocol{}, fmt.Errorf("unknown device protocol: %q", s)
}

// IsValid reports whether the protocol is one of the known values (not the
// zero value).
func (p DeviceProtocol) IsValid() bool { return p != DeviceProtocol{} }

// String implements [fmt.Stringer].
func (p DeviceProtocol) String() string { return p.protocol }
