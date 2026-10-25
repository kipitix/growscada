package project

import (
	"fmt"
	"slices"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// Project - aggregate representing a project: the unit of operational
// ownership and isolation (ADR 0006). Tags and Scenes with their Widgets
// belong to exactly one Project and carry its ID; the Project itself holds
// only its name and its owning Organization, both local to this server.
//
// Deleting a Project only marks it deleted: from then on it does not exist
// for anyone but the policy that empties it, which removes it last.
//
// Project is immutable: every change returns a new Project that also records
// the change's event. A change keeps the Version; the repository raises it
// once when it saves the result.
type Project interface {
	ID() id.ID[Project]
	OrganizationID() id.ID[organization.Organization]
	Name() ProjectName
	Version() version.Version[Project]
	// IsDeleted reports whether the project is marked deleted (Delete).
	IsDeleted() bool

	// CheckVersion reports an edit conflict: the caller changes the project
	// having seen it at a Version other than the current one.
	// Returns ErrProjectConflict if expected is not the Version.
	CheckVersion(expected version.Version[Project]) error

	// Rename changes the project's name and records a ProjectUpdatedEvent.
	// Returns ErrProjectConflict if expected is not the Version.
	Rename(expected version.Version[Project], aName ProjectName) (Project, error)

	// Delete marks the project deleted and records a ProjectDeletedEvent;
	// saving it frees its name. Like removing any aggregate, it expects no
	// Version.
	Delete() Project

	event.Recorder
	fmt.Stringer
}

// projectImpl - Project implementation struct.
type projectImpl struct {
	id             id.ID[Project]
	organizationID id.ID[organization.Organization]
	name           ProjectName
	deleted        bool
	version        version.Version[Project]
	pending        []event.Event
}

var _ Project = (*projectImpl)(nil)

// CreateProject creates a new, not yet saved project of the Organization
// anOrganizationID and records a ProjectCreatedEvent.
func CreateProject(anID id.ID[Project], anOrganizationID id.ID[organization.Organization], aName ProjectName) Project {
	return &projectImpl{
		id:             anID,
		organizationID: anOrganizationID,
		name:           aName,
		version:        version.Initial[Project](),
		pending:        []event.Event{NewProjectCreatedEvent(anID)},
	}
}

// ReconstituteProject rebuilds a project as stored, at the given version; it
// records no event. For repositories.
func ReconstituteProject(
	anID id.ID[Project],
	anOrganizationID id.ID[organization.Organization],
	aName ProjectName,
	deleted bool,
	aVersion version.Version[Project],
) Project {
	return &projectImpl{
		id:             anID,
		organizationID: anOrganizationID,
		name:           aName,
		deleted:        deleted,
		version:        aVersion,
	}
}

func (p *projectImpl) ID() id.ID[Project]                               { return p.id }
func (p *projectImpl) OrganizationID() id.ID[organization.Organization] { return p.organizationID }
func (p *projectImpl) Name() ProjectName                                { return p.name }
func (p *projectImpl) Version() version.Version[Project]                { return p.version }
func (p *projectImpl) IsDeleted() bool                                  { return p.deleted }

func (p *projectImpl) CheckVersion(expected version.Version[Project]) error {
	if expected != p.version {
		return fmt.Errorf("%w: expected version %s, current %s", ErrProjectConflict, expected, p.version)
	}
	return nil
}

func (p *projectImpl) Rename(expected version.Version[Project], aName ProjectName) (Project, error) {
	if err := p.CheckVersion(expected); err != nil {
		return nil, err
	}
	renamed := *p
	renamed.name = aName
	renamed.pending = append(slices.Clone(p.pending), NewProjectUpdatedEvent(p.id))
	return &renamed, nil
}

func (p *projectImpl) Delete() Project {
	deleted := *p
	deleted.deleted = true
	deleted.pending = append(slices.Clone(p.pending), NewProjectDeletedEvent(p.id))
	return &deleted
}

func (p *projectImpl) PendingEvents() []event.Event {
	return slices.Clone(p.pending)
}

// String implements [fmt.Stringer].
func (p *projectImpl) String() string {
	return fmt.Sprintf("Project: %s, Organization: %s, Deleted: %t, Version: %s", p.name, p.organizationID, p.deleted, p.version)
}
