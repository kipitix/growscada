package application_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// mainProject is the project "Main" the migrations create: the tests' content
// lives in it unless they make projects of their own.
var (
	mainProject   = uuid.MustParse("b0000000-0000-4000-8000-000000000001")
	mainProjectID = id.NewID(id.IDWithUUID[project.Project](mainProject))
)

// wrappedScopes are the ProjectScopes over the test database whose
// repositories pass through tags and scenes (nil: as they are), so a test can
// put a wrapper around them. A project's scope is opened once and kept, so a
// wrapper keeps its state between the calls of a service.
type wrappedScopes struct {
	application.ProjectScopes
	tags   func(tag.TagRepository) tag.TagRepository
	scenes func(scene.SceneRepository) scene.SceneRepository

	mu     sync.Mutex
	opened map[id.ID[project.Project]]application.ProjectScope
}

func newScopes(opts ...repositories.Option) *wrappedScopes {
	return &wrappedScopes{ProjectScopes: repositories.NewProjectScopes(testDB, opts...)}
}

func (s *wrappedScopes) ForProject(projectID id.ID[project.Project]) application.ProjectScope {
	s.mu.Lock()
	defer s.mu.Unlock()
	if scope, ok := s.opened[projectID]; ok {
		return scope
	}
	inner := s.ProjectScopes.ForProject(projectID)
	scope := testScope{projectID: projectID, tags: inner.Tags(), scenes: inner.Scenes()}
	if s.tags != nil {
		scope.tags = s.tags(scope.tags)
	}
	if s.scenes != nil {
		scope.scenes = s.scenes(scope.scenes)
	}
	if s.opened == nil {
		s.opened = map[id.ID[project.Project]]application.ProjectScope{}
	}
	s.opened[projectID] = scope
	return scope
}

type testScope struct {
	projectID id.ID[project.Project]
	tags      tag.TagRepository
	scenes    scene.SceneRepository
}

func (s testScope) ProjectID() id.ID[project.Project] { return s.projectID }
func (s testScope) Tags() tag.TagRepository           { return s.tags }
func (s testScope) Scenes() scene.SceneRepository     { return s.scenes }

// soleOrganization is the organization the migrations create.
func soleOrganization() id.ID[organization.Organization] {
	orgID, err := repositories.FindSoleOrganization(context.Background(), testDB)
	if err != nil {
		panic("organization: " + err.Error())
	}
	return orgID
}

// cleanProjects removes every project but mainProject; their content must be
// gone already.
func cleanProjects(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM projects WHERE id <> $1", mainProject); err != nil {
		t.Fatalf("cleanProjects: %v", err)
	}
}

// contentOver returns the ProjectContent of the projects of the test
// database's organization, through scopes.
func contentOver(scopes application.ProjectScopes) application.ProjectContent {
	return application.NewProjectContent(repositories.NewProjectRepositoryPostgres(testDB), scopes, soleOrganization())
}

// sceneServiceOver returns the scene service over scopes and the WidgetType
// repository wtRepo.
func sceneServiceOver(scopes application.ProjectScopes, wtRepo library.WidgetTypeRepository) application.SceneService {
	return application.NewSceneService(contentOver(scopes), wtRepo)
}

// sceneRepository returns, for wrappedScopes.scenes, the wrapper repo in
// place of any repository.
func sceneRepository(repo scene.SceneRepository) func(scene.SceneRepository) scene.SceneRepository {
	return func(scene.SceneRepository) scene.SceneRepository { return repo }
}
