package device

import (
	"fmt"
	"slices"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// EnvironmentDevice - aggregate of a Project's Draft, one per pair
// (Environment, Device): the parameters by which that Device is reached in
// that Environment. The pair is what identifies it, as a Library Pin is
// identified by its Library in its Project; it has no identifier of its own.
//
// EnvironmentDevice is immutable: every change returns a new
// EnvironmentDevice that also records the change's event. It is an entity of
// its Project's Draft, which carries the one Version of the Draft's content
// (ADR 0016).
type EnvironmentDevice interface {
	// ProjectID is the Project of the Environment and of the Device; it never
	// changes.
	ProjectID() id.ID[project.Project]
	// EnvironmentID is the Environment of the pair; it never changes.
	EnvironmentID() id.ID[Environment]
	// DeviceID is the Device of the pair; it never changes.
	DeviceID() id.ID[Device]
	// Params are the parameters by which the Device is reached in the
	// Environment, read under the Device's protocol.
	Params() EnvironmentDeviceParams

	// UpdateParams changes the parameters of the pair and records an
	// EnvironmentDeviceUpdatedEvent.
	UpdateParams(aParams EnvironmentDeviceParams) (EnvironmentDevice, error)

	// Delete records an EnvironmentDeviceDeletedEvent; the repository's
	// Delete removes the pair.
	Delete() EnvironmentDevice

	event.Recorder
	fmt.Stringer
}

// environmentDeviceImpl - EnvironmentDevice implementation struct.
type environmentDeviceImpl struct {
	projectID     id.ID[project.Project]
	environmentID id.ID[Environment]
	deviceID      id.ID[Device]
	params        EnvironmentDeviceParams
	pending       []event.Event
}

var _ EnvironmentDevice = (*environmentDeviceImpl)(nil)

// CreateEnvironmentDevice creates a new, not yet saved pair of the Device
// aDeviceID in the Environment anEnvironmentID of the Project aProjectID, and
// records an EnvironmentDeviceCreatedEvent.
func CreateEnvironmentDevice(aProjectID id.ID[project.Project], anEnvironmentID id.ID[Environment], aDeviceID id.ID[Device], aParams EnvironmentDeviceParams) EnvironmentDevice {
	return &environmentDeviceImpl{
		projectID:     aProjectID,
		environmentID: anEnvironmentID,
		deviceID:      aDeviceID,
		params:        aParams,
		pending:       []event.Event{NewEnvironmentDeviceCreatedEvent(anEnvironmentID, aDeviceID, aProjectID)},
	}
}

// ReconstituteEnvironmentDevice rebuilds a pair as stored; it records no
// event. For repositories, which read the pair within its Project and so name
// it.
func ReconstituteEnvironmentDevice(aProjectID id.ID[project.Project], anEnvironmentID id.ID[Environment], aDeviceID id.ID[Device], aParams EnvironmentDeviceParams) EnvironmentDevice {
	return &environmentDeviceImpl{
		projectID:     aProjectID,
		environmentID: anEnvironmentID,
		deviceID:      aDeviceID,
		params:        aParams,
	}
}

func (e *environmentDeviceImpl) ProjectID() id.ID[project.Project] { return e.projectID }
func (e *environmentDeviceImpl) EnvironmentID() id.ID[Environment] { return e.environmentID }
func (e *environmentDeviceImpl) DeviceID() id.ID[Device]           { return e.deviceID }
func (e *environmentDeviceImpl) Params() EnvironmentDeviceParams   { return e.params }
func (e *environmentDeviceImpl) UpdateParams(aParams EnvironmentDeviceParams) (EnvironmentDevice, error) {
	updated := *e
	updated.params = aParams
	updated.pending = append(slices.Clone(e.pending),
		NewEnvironmentDeviceUpdatedEvent(e.environmentID, e.deviceID, e.projectID))
	return &updated, nil
}

func (e *environmentDeviceImpl) Delete() EnvironmentDevice {
	deleted := *e
	deleted.pending = append(slices.Clone(e.pending),
		NewEnvironmentDeviceDeletedEvent(e.environmentID, e.deviceID, e.projectID))
	return &deleted
}

func (e *environmentDeviceImpl) PendingEvents() []event.Event {
	return slices.Clone(e.pending)
}

// String implements [fmt.Stringer].
func (e *environmentDeviceImpl) String() string {
	return fmt.Sprintf("EnvironmentDevice: environment %s, device %s", e.environmentID, e.deviceID)
}
