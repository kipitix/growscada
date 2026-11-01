package device

import "fmt"

// DeviceName - name of a Device, unique among the live Devices of its Project
// (ADR 0003).
// Value Object.
type DeviceName struct {
	name string
}

var _ fmt.Stringer = DeviceName{}

// NewDeviceName creates a DeviceName from a string.
func NewDeviceName(aName string) (DeviceName, error) {
	if aName == "" {
		return DeviceName{}, fmt.Errorf("device name must not be empty")
	}
	return DeviceName{name: aName}, nil
}

// String implements [fmt.Stringer].
func (n DeviceName) String() string { return n.name }
