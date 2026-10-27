package repositories

import (
	"database/sql"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// projectScopes opens the Postgres repositories of a Project's content
// (ADR 0006); each scope holds repositories bound to its project_id.
type projectScopes struct {
	db   *sql.DB
	opts []Option
}

var _ application.ProjectScopes = (*projectScopes)(nil)

// NewProjectScopes returns the ProjectScopes over the database; the options
// apply to every repository it opens.
func NewProjectScopes(aDb *sql.DB, opts ...Option) application.ProjectScopes {
	return &projectScopes{db: aDb, opts: opts}
}

func (s *projectScopes) ForProject(projectID id.ID[project.Project]) application.ProjectScope {
	return projectScope{
		projectID: projectID,
		tags:      NewTagRepositoryPostgres(s.db, projectID, s.opts...),
		scenes:    NewSceneRepositoryPostgres(s.db, projectID, s.opts...),
		pins:      NewLibraryPinRepositoryPostgres(s.db, projectID, s.opts...),
	}
}

type projectScope struct {
	projectID id.ID[project.Project]
	tags      tag.TagRepository
	scenes    scene.SceneRepository
	pins      librarypin.Repository
}

func (s projectScope) ProjectID() id.ID[project.Project] { return s.projectID }
func (s projectScope) Tags() tag.TagRepository           { return s.tags }
func (s projectScope) Scenes() scene.SceneRepository     { return s.scenes }
func (s projectScope) LibraryPins() librarypin.Repository { return s.pins }
