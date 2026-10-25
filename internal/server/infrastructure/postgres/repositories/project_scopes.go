package repositories

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// projectScopes opens the Postgres repositories of a Project's content
// (ADR 0006); each scope is a pair of repositories bound to its project_id.
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
		tags:   NewTagRepositoryPostgres(s.db, projectID, s.opts...),
		scenes: NewSceneRepositoryPostgres(s.db, projectID, s.opts...),
	}
}

func (s *projectScopes) FindProjectsUsingWidgetType(ctx context.Context, typeID id.ID[library.WidgetType]) ([]id.ID[project.Project], error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT project_id FROM widgets WHERE type_id = $1 GROUP BY project_id ORDER BY min(pk_id)`,
		typeID.UUID(),
	)
	if err != nil {
		return nil, fmt.Errorf("error querying projects using widget type: %w", err)
	}
	defer rows.Close()
	var projects []id.ID[project.Project]
	for rows.Next() {
		var raw uuid.UUID
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("error scanning project id: %w", err)
		}
		projects = append(projects, id.NewID(id.IDWithUUID[project.Project](raw)))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over projects using widget type: %w", err)
	}
	return projects, nil
}

type projectScope struct {
	tags   tag.TagRepository
	scenes scene.SceneRepository
}

func (s projectScope) Tags() tag.TagRepository       { return s.tags }
func (s projectScope) Scenes() scene.SceneRepository { return s.scenes }
