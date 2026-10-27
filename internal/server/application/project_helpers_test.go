package application_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
	runtimepg "github.com/kipitix/growscada/internal/server/infrastructure/postgres/runtime"
	"github.com/kipitix/growscada/internal/server/inprocess"
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
	pins   func(librarypin.Repository) librarypin.Repository

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
	scope := testScope{projectID: projectID, tags: inner.Tags(), scenes: inner.Scenes(), pins: inner.LibraryPins()}
	if s.tags != nil {
		scope.tags = s.tags(scope.tags)
	}
	if s.scenes != nil {
		scope.scenes = s.scenes(scope.scenes)
	}
	if s.pins != nil {
		scope.pins = s.pins(scope.pins)
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
	pins      librarypin.Repository
}

func (s testScope) ProjectID() id.ID[project.Project] { return s.projectID }
func (s testScope) Tags() tag.TagRepository           { return s.tags }
func (s testScope) Scenes() scene.SceneRepository     { return s.scenes }
func (s testScope) LibraryPins() librarypin.Repository { return s.pins }

// soleOrganization is the organization the migrations create.
func soleOrganization() id.ID[organization.Organization] {
	orgID, err := repositories.FindSoleOrganization(context.Background(), testDB)
	if err != nil {
		panic("organization: " + err.Error())
	}
	return orgID
}

// cleanProjects removes every project but mainProject, with their Library
// Pins; their other content must be gone already.
func cleanProjects(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(),
		"DELETE FROM engineering.library_pins WHERE project_id <> '"+mainProject.String()+"'; DELETE FROM projects WHERE id <> '"+mainProject.String()+"'"); err != nil {
		t.Fatalf("cleanProjects: %v", err)
	}
}

// contentOver returns the ProjectContent of the projects of the test
// database's organization, through scopes.
func contentOver(scopes application.ProjectScopes) application.ProjectContent {
	return application.NewProjectContent(repositories.NewProjectRepositoryPostgres(testDB), scopes, soleOrganization())
}

// sceneServiceOver returns the scene service over scopes.
func sceneServiceOver(scopes application.ProjectScopes) application.SceneService {
	return application.NewSceneService(contentOver(scopes))
}

// sceneRepository returns, for wrappedScopes.scenes, the wrapper repo in
// place of any repository.
func sceneRepository(repo scene.SceneRepository) func(scene.SceneRepository) scene.SceneRepository {
	return func(scene.SceneRepository) scene.SceneRepository { return repo }
}

// newRuntimeNode returns the Runtime node over the test database, in-process
// as the server wires it; its tag_updated go nowhere.
func newRuntimeNode() application.RuntimeNode {
	_, node := newRuntime()
	return node
}

// newRuntime returns the Runtime node's service of Tag states over the test
// database and the RuntimeNode port over it.
func newRuntime() (runtimeapp.TagStateService, application.RuntimeNode) {
	states := runtimeapp.NewTagStateService(runtimepg.NewTagStateRepositoryPostgres(testDB), soleOrganization(), func(event.Event) {})
	return states, inprocess.NewRuntimeNode(states)
}

// newServiceWithStates returns the tag service and the Runtime node's
// service of Tag states behind it.
func newServiceWithStates() (application.TagService, runtimeapp.TagStateService) {
	states, node := newRuntime()
	return application.NewTagService(contentOver(newScopes()), node), states
}

// errRuntimeDown is the error of failingRuntimeNode.
var errRuntimeDown = errors.New("runtime node down")

// failingRuntimeNode is a Runtime node whose upserts and removals fail; its
// reads go to the RuntimeNode it wraps.
type failingRuntimeNode struct {
	application.RuntimeNode
}

func (failingRuntimeNode) UpsertTagDefinitions(context.Context, uuid.UUID, []appdto.TagDefinition) error {
	return errRuntimeDown
}

func (failingRuntimeNode) RemoveTagStates(context.Context, uuid.UUID, []uuid.UUID) error {
	return errRuntimeDown
}

func (failingRuntimeNode) RemoveProjectTagStates(context.Context, uuid.UUID) error {
	return errRuntimeDown
}

// tagStateCount returns how many Tag states the Runtime node holds of the
// Project.
func tagStateCount(t *testing.T, projectID uuid.UUID) int {
	t.Helper()
	var n int
	if err := testDB.QueryRowContext(context.Background(), `SELECT count(*) FROM runtime.tags WHERE project_id = $1`, projectID).Scan(&n); err != nil {
		t.Fatalf("count tag states: %v", err)
	}
	return n
}
