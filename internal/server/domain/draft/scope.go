package draft

import (
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// Scope is the Draft an operation belongs to: a Project's or a Library's.
// Exactly one ID is set.
type Scope struct {
	projectID id.ID[project.Project]
	libraryID id.ID[library.Library]
}

// ProjectScope is the Scope of the Draft of the Project projectID.
func ProjectScope(projectID id.ID[project.Project]) Scope {
	return Scope{projectID: projectID}
}

// LibraryScope is the Scope of the Draft of the Library libraryID.
func LibraryScope(libraryID id.ID[library.Library]) Scope {
	return Scope{libraryID: libraryID}
}

// ProjectID is the Project of the Scope, a zero ID for a Library's.
func (s Scope) ProjectID() id.ID[project.Project] { return s.projectID }

// LibraryID is the Library of the Scope, a zero ID for a Project's.
func (s Scope) LibraryID() id.ID[library.Library] { return s.libraryID }

// IsProject reports whether the Scope is a Project's Draft.
func (s Scope) IsProject() bool { return !s.projectID.IsZero() }

// IsLibrary reports whether the Scope is a Library's Draft.
func (s Scope) IsLibrary() bool { return !s.libraryID.IsZero() }

// String implements [fmt.Stringer].
func (s Scope) String() string {
	if s.IsLibrary() {
		return "library " + s.libraryID.String()
	}
	return "project " + s.projectID.String()
}
