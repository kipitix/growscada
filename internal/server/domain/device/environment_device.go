package device

import (
	"fmt"
	"slices"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// EnvironmentDevice - aggregate of a Project's Draft, one per pair
// (Environment, Device): the parameters by which that Device is reached in
// that Environment. The pair is what identifies it; its ID is its own, so a
// client can name the row a change happened to.
//
// EnvironmentDevice is immutable: every change returns a new
// EnvironmentDevice that also records the change's event. A change keeps the
// Version; the repository raises it once when it saves the result.
type EnvironmentDevice interface {
	ID() id.ID[EnvironmentDevice]
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
	Version() version.Version[EnvironmentDevice]

	// CheckVersion reports an edit conflict: the caller changes the pair
	// having seen it at a Version other than the current one.
	// Returns ErrEnvironmentDeviceConflict if expected is not the Version.
	CheckVersion(expected version.Version[EnvironmentDevice]) error

	// UpdateParams changes the parameters of the pair and records an
	// EnvironmentDeviceUpdatedEvent.
	// Returns ErrEnvironmentDeviceConflict if expected is not the Version.
	UpdateParams(expected version.Version[EnvironmentDevice], aParams EnvironmentDeviceParams) (EnvironmentDevice, error)

	// Delete records an EnvironmentDeviceDeletedEvent; the repository's
	// Delete removes the pair.
	Delete() EnvironmentDevice

	event.Recorder
	fmt.Stringer
}

// environmentDeviceImpl - EnvironmentDevice implementation struct.
type environmentDeviceImpl struct {
	id            id.ID[EnvironmentDevice]
	projectID     id.ID[project.Project]
	environmentID id.ID[Environment]
	deviceID      id.ID[Device]
	params        EnvironmentDeviceParams
	version       version.Version[EnvironmentDevice]
	pending       []event.Event
}

var _ EnvironmentDevice = (*environmentDeviceImpl)(nil)

// CreateEnvironmentDevice creates a new, not yet saved pair of the Device
// aDeviceID in the Environment anEnvironmentID of the Project aProjectID, and
// records an EnvironmentDeviceCreatedEvent.
func CreateEnvironmentDevice(anID id.ID[EnvironmentDevice], aProjectID id.ID[project.Project], anEnvironmentID id.ID[Environment], aDeviceID id.ID[Device], aParams EnvironmentDeviceParams) EnvironmentDevice {
	return &environmentDeviceImpl{
		id:            anID,
		projectID:     aProjectID,
		environmentID: anEnvironmentID,
		deviceID:      aDeviceID,
		params:        aParams,
		version:       version.Initial[EnvironmentDevice](),
		pending:       []event.Event{NewEnvironmentDeviceCreatedEvent(anID, anEnvironmentID, aDeviceID, aProjectID)},
	}
}

// ReconstituteEnvironmentDevice rebuilds a pair as stored, at the given
// version; it records no event. For repositories, which read the pair within
// its Project and so name it.
func ReconstituteEnvironmentDevice(anID id.ID[EnvironmentDevice], aProjectID id.ID[project.Project], anEnvironmentID id.ID[Environment], aDeviceID id.ID[Device], aParams EnvironmentDeviceParams, aVersion version.Version[EnvironmentDevice]) EnvironmentDevice {
	return &environmentDeviceImpl{
		id:            anID,
		projectID:     aProjectID,
		environmentID: anEnvironmentID,
		deviceID:      aDeviceID,
		params:        aParams,
		version:       aVersion,
	}
}

func (e *environmentDeviceImpl) ID() id.ID[EnvironmentDevice]                { return e.id }
func (e *environmentDeviceImpl) ProjectID() id.ID[project.Project]           { return e.projectID }
func (e *environmentDeviceImpl) EnvironmentID() id.ID[Environment]           { return e.environmentID }
func (e *environmentDeviceImpl) DeviceID() id.ID[Device]                     { return e.deviceID }
func (e *environmentDeviceImpl) Params() EnvironmentDeviceParams             { return e.params }
func (e *environmentDeviceImpl) Version() version.Version[EnvironmentDevice] { return e.version }

func (e *environmentDeviceImpl) CheckVersion(expected version.Version[EnvironmentDevice]) error {
	if expected != e.version {
		return fmt.Errorf("%w: expected version %s, current %s", ErrEnvironmentDeviceConflict, expected, e.version)
	}
	return nil
}

func (e *environmentDeviceImpl) UpdateParams(expected version.Version[EnvironmentDevice], aParams EnvironmentDeviceParams) (EnvironmentDevice, error) {
	if err := e.CheckVersion(expected); err != nil {
		return nil, err
	}
	updated := *e
	updated.params = aParams
	updated.pending = append(slices.Clone(e.pending),
		NewEnvironmentDeviceUpdatedEvent(e.id, e.environmentID, e.deviceID, e.projectID))
	return &updated, nil
}

func (e *environmentDeviceImpl) Delete() EnvironmentDevice {
	deleted := *e
	deleted.pending = append(slices.Clone(e.pending),
		NewEnvironmentDeviceDeletedEvent(e.id, e.environmentID, e.deviceID, e.projectID))
	return &deleted
}

func (e *environmentDeviceImpl) PendingEvents() []event.Event {
	return slices.Clone(e.pending)
}

// String implements [fmt.Stringer].
func (e *environmentDeviceImpl) String() string {
	return fmt.Sprintf("EnvironmentDevice: environment %s, device %s, Version: %s", e.environmentID, e.deviceID, e.version)
}
