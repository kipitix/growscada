package project

import (
	"context"
	"errors"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
)

var (
	// ErrProjectNotFound is returned when there is no live project with the
	// given ID or name: a project marked deleted is not found either.
	ErrProjectNotFound = errors.New("project not found")
	// ErrProjectConflict is returned by the Project's change methods when the
	// expected version is not the current one (an edit conflict), and by
	// Save when the stored version changed after the project was read (a
	// write race).
	ErrProjectConflict = errors.New("project version conflict")
	// ErrProjectNameTaken is returned by Save when another live project of
	// the same Organization already has the name.
	ErrProjectNameTaken = errors.New("project name already taken")
	// ErrProjectNotEmpty is returned by Delete while Tags or Scenes still
	// belong to the project.
	ErrProjectNotEmpty = errors.New("project still has content")
)

// ProjectRepository - repository interface for storing and managing projects.
// Every read but FindDeleted sees only live projects: one marked deleted
// does not exist for them.
type ProjectRepository interface {
	// NextID returns a new unique identifier for a project.
	NextID() id.ID[Project]

	// Save stores a project and the events it recorded, in one transaction:
	// a project of the initial version is inserted, any other replaces the
	// stored live one if the stored version is still the project's version;
	// a project marked deleted (Project.Delete) is stored so. The version is
	// raised once per Save; the returned project carries the new one and no
	// events.
	// Returns ErrProjectNotFound if there is no such live project on update,
	// ErrProjectConflict if the stored version does not match,
	// ErrProjectNameTaken if another live project of its Organization has the
	// name, or an error on failure.
	Save(context.Context, Project) (Project, error)

	// FindByID returns the live project with the given ID.
	// Returns ErrProjectNotFound if there is none.
	FindByID(context.Context, id.ID[Project]) (Project, error)

	// FindByName returns the live project of the Organization with the given
	// name. Returns ErrProjectNotFound if there is none.
	FindByName(context.Context, id.ID[organization.Organization], ProjectName) (Project, error)

	// FindAll returns the live projects of the Organization in creation
	// order, so the order is stable while projects change.
	FindAll(context.Context, id.ID[organization.Organization]) ([]Project, error)

	// FindDeleted returns the project with the given ID if it is marked
	// deleted. Returns ErrProjectNotFound if there is no such project, or
	// it is live.
	FindDeleted(context.Context, id.ID[Project]) (Project, error)

	// Delete removes a project marked deleted, once its content is gone. It
	// stores no events: the project recorded its ProjectDeletedEvent when it
	// was marked.
	// Returns ErrProjectNotFound if there is no such project marked deleted,
	// ErrProjectNotEmpty while Tags or Scenes still belong to it.
	Delete(context.Context, Project) error
}
