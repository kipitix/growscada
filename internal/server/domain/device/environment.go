package device

import (
	"fmt"
	"slices"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// Environment - aggregate of a Project's Draft: a named set of the Project's
// Devices as they stand in one context of operation — a test bench, a plant, a
// simulator. It holds the parameters by which each of its Devices is reached
// (EnvironmentDevice); a Deploy does not choose an Environment, and a
// Device's Tag values are supplied whatever Environment it is described in.
//
// Environment is immutable: every change returns a new Environment that also
// records the change's event. A change keeps the Version; the repository
// raises it once when it saves the result.
type Environment interface {
	ID() id.ID[Environment]
	// ProjectID is the Project whose Draft holds the Environment; it never
	// changes.
	ProjectID() id.ID[project.Project]
	Name() EnvironmentName
	// Comment is descriptive metadata of the Environment — a note for people.
	Comment() string
	Version() version.Version[Environment]

	// CheckVersion reports an edit conflict: the caller changes the
	// Environment having seen it at a Version other than the current one.
	// Returns ErrEnvironmentConflict if expected is not the Version.
	CheckVersion(expected version.Version[Environment]) error

	// Rename changes the Environment's name and records an
	// EnvironmentUpdatedEvent.
	// Returns ErrEnvironmentConflict if expected is not the Version, and an
	// error if the name is the invalid zero value.
	Rename(expected version.Version[Environment], aName EnvironmentName) (Environment, error)

	// Delete records an EnvironmentDeletedEvent; the repository's Delete
	// removes the Environment. What that does to the pairs of its Devices is
	// a policy's business, not the Environment's (ADR 0008).
	Delete() Environment

	event.Recorder
	fmt.Stringer
}

// environmentImpl - Environment implementation struct.
type environmentImpl struct {
	id        id.ID[Environment]
	projectID id.ID[project.Project]
	name      EnvironmentName
	comment   string
	version   version.Version[Environment]
	pending   []event.Event
}

var _ Environment = (*environmentImpl)(nil)

// CreateEnvironment creates a new, not yet saved Environment of the Project
// aProjectID and records an EnvironmentCreatedEvent.
// Returns an error if the name is the invalid zero value.
func CreateEnvironment(anID id.ID[Environment], aProjectID id.ID[project.Project], aName EnvironmentName, aComment string) (Environment, error) {
	created, err := ReconstituteEnvironment(anID, aProjectID, aName, aComment, version.Initial[Environment]())
	if err != nil {
		return nil, err
	}
	impl := created.(*environmentImpl)
	impl.pending = []event.Event{NewEnvironmentCreatedEvent(anID, aProjectID)}
	return impl, nil
}

// ReconstituteEnvironment rebuilds an Environment as stored, at the given
// version; it records no event. For repositories.
// Returns an error if the name is the invalid zero value.
func ReconstituteEnvironment(anID id.ID[Environment], aProjectID id.ID[project.Project], aName EnvironmentName, aComment string, aVersion version.Version[Environment]) (Environment, error) {
	if !aName.IsValid() {
		return nil, fmt.Errorf("cannot create environment: invalid name")
	}
	return &environmentImpl{
		id:        anID,
		projectID: aProjectID,
		name:      aName,
		comment:   aComment,
		version:   aVersion,
	}, nil
}

func (e *environmentImpl) ID() id.ID[Environment]                { return e.id }
func (e *environmentImpl) ProjectID() id.ID[project.Project]     { return e.projectID }
func (e *environmentImpl) Name() EnvironmentName                 { return e.name }
func (e *environmentImpl) Comment() string                       { return e.comment }
func (e *environmentImpl) Version() version.Version[Environment] { return e.version }

func (e *environmentImpl) CheckVersion(expected version.Version[Environment]) error {
	if expected != e.version {
		return fmt.Errorf("%w: expected version %s, current %s", ErrEnvironmentConflict, expected, e.version)
	}
	return nil
}

func (e *environmentImpl) Rename(expected version.Version[Environment], aName EnvironmentName) (Environment, error) {
	if err := e.CheckVersion(expected); err != nil {
		return nil, err
	}
	if !aName.IsValid() {
		return nil, fmt.Errorf("cannot rename environment: invalid name")
	}
	renamed := *e
	renamed.name = aName
	renamed.pending = append(slices.Clone(e.pending), NewEnvironmentUpdatedEvent(e.id, e.projectID))
	return &renamed, nil
}

func (e *environmentImpl) Delete() Environment {
	deleted := *e
	deleted.pending = append(slices.Clone(e.pending), NewEnvironmentDeletedEvent(e.id, e.projectID))
	return &deleted
}

func (e *environmentImpl) PendingEvents() []event.Event {
	return slices.Clone(e.pending)
}

// String implements [fmt.Stringer].
func (e *environmentImpl) String() string {
	return fmt.Sprintf("Environment: %s, Project: %s, Version: %s", e.name, e.projectID, e.version)
}
