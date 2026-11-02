// Package device holds the Draft's Devices, the Environments they stand in
// and the parameters of one Device in one Environment (ADR 0004, amendment
// 2026-11-01). A Device says what a thing is — its protocol and the
// configuration read under it; an Environment says where it is — the address,
// the port, the credentials; an EnvironmentDevice holds the parameters of one
// pair. All three are content of a Project's Draft, the Engineering node's,
// with names unique in the Project and a Version of their own.
package device

import (
	"fmt"
	"slices"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// Device - aggregate of a Project's Draft: a piece of equipment that supplies
// Tag values, described by its name (unique in the Project), its protocol and
// the configuration read under that protocol. Where it stands and how to
// reach it is an EnvironmentDevice's business, and a Tag names at most one
// Device as its supplier. A Device is descriptive metadata: it never affects
// a Tag's Quality.
//
// Device is immutable: every change returns a new Device that also records
// the change's event. A change keeps the Version; the repository raises it
// once when it saves the result.
type Device interface {
	ID() id.ID[Device]
	// ProjectID is the Project whose Draft holds the Device; it never
	// changes.
	ProjectID() id.ID[project.Project]
	Name() DeviceName
	// Comment is descriptive metadata of the Device — a note for people,
	// such as "this one is the simulator"; it never affects behavior.
	Comment() string
	Protocol() DeviceProtocol
	// Config is the configuration read under Protocol: for ProtocolSimulation,
	// the patterns of what to write.
	Config() DeviceConfig
	Version() version.Version[Device]

	// CheckVersion reports an edit conflict: the caller changes the Device
	// having seen it at a Version other than the current one.
	// Returns ErrDeviceConflict if expected is not the Version.
	CheckVersion(expected version.Version[Device]) error

	// Update changes the Device's name, comment, protocol and configuration
	// and records a DeviceUpdatedEvent.
	// Returns ErrDeviceConflict if expected is not the Version, and an error
	// if the name or the protocol is the invalid zero value.
	Update(expected version.Version[Device], aName DeviceName, aComment string, aProtocol DeviceProtocol, aConfig DeviceConfig) (Device, error)

	// Delete records a DeviceDeletedEvent; the repository's Delete removes
	// the Device, and is refused while the Device still supplies a Tag.
	Delete() Device

	event.Recorder
	fmt.Stringer
}

// deviceImpl - Device implementation struct.
type deviceImpl struct {
	id        id.ID[Device]
	projectID id.ID[project.Project]
	name      DeviceName
	comment   string
	protocol  DeviceProtocol
	config    DeviceConfig
	version   version.Version[Device]
	pending   []event.Event
}

var _ Device = (*deviceImpl)(nil)

// CreateDevice creates a new, not yet saved Device of the Project
// aProjectID and records a DeviceCreatedEvent.
// Returns an error if the name or the protocol is the invalid zero value.
func CreateDevice(anID id.ID[Device], aProjectID id.ID[project.Project], aName DeviceName, aComment string, aProtocol DeviceProtocol, aConfig DeviceConfig) (Device, error) {
	created, err := ReconstituteDevice(anID, aProjectID, aName, aComment, aProtocol, aConfig, version.Initial[Device]())
	if err != nil {
		return nil, err
	}
	impl := created.(*deviceImpl)
	impl.pending = []event.Event{NewDeviceCreatedEvent(anID, aProjectID)}
	return impl, nil
}

// ReconstituteDevice rebuilds a Device as stored, at the given version; it
// records no event. For repositories.
// Returns an error if the name or the protocol is the invalid zero value.
func ReconstituteDevice(anID id.ID[Device], aProjectID id.ID[project.Project], aName DeviceName, aComment string, aProtocol DeviceProtocol, aConfig DeviceConfig, aVersion version.Version[Device]) (Device, error) {
	if !aName.IsValid() {
		return nil, fmt.Errorf("cannot create device: invalid name")
	}
	if !aProtocol.IsValid() {
		return nil, fmt.Errorf("cannot create device: invalid protocol")
	}
	return &deviceImpl{
		id:        anID,
		projectID: aProjectID,
		name:      aName,
		comment:   aComment,
		protocol:  aProtocol,
		config:    aConfig,
		version:   aVersion,
	}, nil
}

func (d *deviceImpl) ID() id.ID[Device]                 { return d.id }
func (d *deviceImpl) ProjectID() id.ID[project.Project] { return d.projectID }
func (d *deviceImpl) Name() DeviceName                  { return d.name }
func (d *deviceImpl) Comment() string                   { return d.comment }
func (d *deviceImpl) Protocol() DeviceProtocol          { return d.protocol }
func (d *deviceImpl) Config() DeviceConfig              { return d.config }
func (d *deviceImpl) Version() version.Version[Device]  { return d.version }

func (d *deviceImpl) CheckVersion(expected version.Version[Device]) error {
	if expected != d.version {
		return fmt.Errorf("%w: expected version %s, current %s", ErrDeviceConflict, expected, d.version)
	}
	return nil
}

func (d *deviceImpl) Update(expected version.Version[Device], aName DeviceName, aComment string, aProtocol DeviceProtocol, aConfig DeviceConfig) (Device, error) {
	if err := d.CheckVersion(expected); err != nil {
		return nil, err
	}
	if !aName.IsValid() {
		return nil, fmt.Errorf("cannot update device: invalid name")
	}
	if !aProtocol.IsValid() {
		return nil, fmt.Errorf("cannot update device: invalid protocol")
	}
	updated := *d
	updated.name = aName
	updated.comment = aComment
	updated.protocol = aProtocol
	updated.config = aConfig
	updated.pending = append(slices.Clone(d.pending), NewDeviceUpdatedEvent(d.id, d.projectID))
	return &updated, nil
}

func (d *deviceImpl) Delete() Device {
	deleted := *d
	deleted.pending = append(slices.Clone(d.pending), NewDeviceDeletedEvent(d.id, d.projectID))
	return &deleted
}

func (d *deviceImpl) PendingEvents() []event.Event {
	return slices.Clone(d.pending)
}

// String implements [fmt.Stringer].
func (d *deviceImpl) String() string {
	return fmt.Sprintf("Device: %s, Protocol: %s, Project: %s, Version: %s", d.name, d.protocol, d.projectID, d.version)
}
