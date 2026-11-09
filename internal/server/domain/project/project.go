package project

import (
	"fmt"
	"slices"

	"github.com/kipitix/growscada/internal/server/domain/description"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// Project - aggregate representing a project: the unit of operational
// ownership and isolation (ADR 0006). Tags and Scenes with their Widgets
// belong to exactly one Project and carry its ID; the Project itself holds
// only its name, its description and its owning Organization, all local to
// this server.
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
	// Description is the project's optional, free-text description; empty
	// when it has none. It is an attribute of the aggregate, never content.
	Description() description.Description
	Version() version.Version[Project]
	// IsDeleted reports whether the project is marked deleted (Delete).
	IsDeleted() bool

	// CheckVersion reports an edit conflict: the caller changes the project
	// having seen it at a Version other than the current one.
	// Returns ErrProjectConflict if expected is not the Version.
	CheckVersion(expected version.Version[Project]) error

	// Update changes the project's name and description and records a
	// ProjectUpdatedEvent. Returns ErrProjectConflict if expected is not the
	// Version.
	Update(expected version.Version[Project], aName ProjectName, aDescription description.Description) (Project, error)

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
	description    description.Description
	deleted        bool
	version        version.Version[Project]
	pending        []event.Event
}

var _ Project = (*projectImpl)(nil)

// CreateProject creates a new, not yet saved project of the Organization
// anOrganizationID and records a ProjectCreatedEvent.
func CreateProject(anID id.ID[Project], anOrganizationID id.ID[organization.Organization], aName ProjectName, aDescription description.Description) Project {
	return &projectImpl{
		id:             anID,
		organizationID: anOrganizationID,
		name:           aName,
		description:    aDescription,
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
	aDescription description.Description,
	deleted bool,
	aVersion version.Version[Project],
) Project {
	return &projectImpl{
		id:             anID,
		organizationID: anOrganizationID,
		name:           aName,
		description:    aDescription,
		deleted:        deleted,
		version:        aVersion,
	}
}

func (p *projectImpl) ID() id.ID[Project]                               { return p.id }
func (p *projectImpl) OrganizationID() id.ID[organization.Organization] { return p.organizationID }
func (p *projectImpl) Name() ProjectName                                { return p.name }
func (p *projectImpl) Description() description.Description             { return p.description }
func (p *projectImpl) Version() version.Version[Project]                { return p.version }
func (p *projectImpl) IsDeleted() bool                                  { return p.deleted }

func (p *projectImpl) CheckVersion(expected version.Version[Project]) error {
	if expected != p.version {
		return fmt.Errorf("%w: expected version %s, current %s", ErrProjectConflict, expected, p.version)
	}
	return nil
}

func (p *projectImpl) Update(expected version.Version[Project], aName ProjectName, aDescription description.Description) (Project, error) {
	if err := p.CheckVersion(expected); err != nil {
		return nil, err
	}
	updated := *p
	updated.name = aName
	updated.description = aDescription
	updated.pending = append(slices.Clone(p.pending), NewProjectUpdatedEvent(p.id))
	return &updated, nil
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
	return fmt.Sprintf("Project: %s, Description: %q, Organization: %s, Deleted: %t, Version: %s", p.name, p.description, p.organizationID, p.deleted, p.version)
}
