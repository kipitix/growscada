package appdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// Project is the application-layer DTO for project data. Its Organization is
// left out: it is the server's own, the same for every project.
type Project struct {
	ID          uuid.UUID
	Name        string
	Description string
	Version     int
	// HasUnsavedChanges reports whether the Draft moved since its last Commit:
	// the read model of a Project's selection row (task 73; ADR 0016,
	// ADR 0021). False for a project without a Draft row or a Commit.
	HasUnsavedChanges bool
}

// ProjectDraftState is what a Project's Draft tells of it: the Version of the
// Draft and the one the last Commit fixed (ADR 0016, ADR 0021).
type ProjectDraftState struct {
	DraftVersion int
	// LastCommitDraftVersion is 0 for a Draft never saved.
	LastCommitDraftVersion int
}

// DraftState is the state of a Project's Draft after a Discard: the Version
// every content DTO of the Project now carries (ADR 0016).
type DraftState struct {
	Version int
}

// DraftChanges is the state of a Draft's DraftChange stack (ADR 0017): the
// Draft's Version and what an undo and a redo would apply. A description is
// empty exactly when its command has nothing to apply.
type DraftChanges struct {
	Version         int
	CanUndo         bool
	CanRedo         bool
	UndoDescription string
	RedoDescription string
}

// ProjectInput holds the fields of a project a client creates or updates.
// The project's ID and the version an update expects are passed separately.
// A nil Description leaves the current one as it is, so a client that sends
// only a name does not erase it; a pointer to an empty string clears it.
type ProjectInput struct {
	Name        string
	Description *string
}

// NewProject creates a Project DTO from the domain aggregate and the state of
// its Draft.
func NewProject(p project.Project, state ProjectDraftState) Project {
	return Project{
		ID:                p.ID().UUID(),
		Name:              p.Name().String(),
		Description:       p.Description().String(),
		Version:           p.Version().Number(),
		HasUnsavedChanges: state.DraftVersion > state.LastCommitDraftVersion,
	}
}

// NewProjectList creates a slice of Project DTOs from domain aggregates and
// their Draft states, in the same order.
func NewProjectList(list []project.Project, states []ProjectDraftState) []Project {
	result := make([]Project, len(list))
	for i, p := range list {
		result[i] = NewProject(p, states[i])
	}
	return result
}
