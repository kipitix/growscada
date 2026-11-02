package tag

import (
	"fmt"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// Supplier - the Device that supplies a Tag's values: part of the Tag
// definition, written to the Runtime node with it. A Tag people write has no
// supplier; at most one Device supplies a Tag (a second one is a Draft
// validation error).
// Value Object.
type Supplier struct {
	deviceID id.ID[device.Device]
}

var _ fmt.Stringer = Supplier{}

// NoSupplier is the supplier of a Tag people write: it has no Device.
func NoSupplier() Supplier {
	return Supplier{}
}

// SuppliedByDevice is the supplier of a Tag whose values the Device writes.
func SuppliedByDevice(aDeviceID id.ID[device.Device]) Supplier {
	return Supplier{deviceID: aDeviceID}
}

// DeviceID is the ID of the Device that supplies the Tag; the zero ID when
// none does (HasDevice tells which).
func (s Supplier) DeviceID() id.ID[device.Device] {
	return s.deviceID
}

// HasDevice reports whether a Device supplies the Tag.
func (s Supplier) HasDevice() bool {
	return !s.deviceID.IsZero()
}

// String implements [fmt.Stringer].
func (s Supplier) String() string {
	if !s.HasDevice() {
		return "no supplier"
	}
	return s.deviceID.String()
}
