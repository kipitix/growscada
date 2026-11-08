package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/draft"
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
	drafts draft.Repository
}

func newScopes(opts ...repositories.Option) *wrappedScopes {
	return &wrappedScopes{
		ProjectScopes: repositories.NewProjectScopes(testDB, opts...),
		drafts:        repositories.NewDraftRepositoryPostgres(testDB, repositories.NewLibraryReleaseRepositoryPostgres(testDB, opts...), opts...),
	}
}

// Drafts is the repository of the Drafts the scopes' services write through.
func (s *wrappedScopes) Drafts() draft.Repository { return s.drafts }

func (s *wrappedScopes) ForProject(projectID id.ID[project.Project]) application.ProjectScope {
	s.mu.Lock()
	defer s.mu.Unlock()
	if scope, ok := s.opened[projectID]; ok {
		return scope
	}
	inner := s.ProjectScopes.ForProject(projectID)
	scope := testScope{
		projectID: projectID, tags: inner.Tags(), scenes: inner.Scenes(), pins: inner.LibraryPins(),
		devices: inner.Devices(), environments: inner.Environments(), environmentDevices: inner.EnvironmentDevices(),
	}
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
	projectID          id.ID[project.Project]
	tags               tag.TagRepository
	scenes             scene.SceneRepository
	pins               librarypin.Repository
	devices            device.DeviceRepository
	environments       device.EnvironmentRepository
	environmentDevices device.EnvironmentDeviceRepository
}

func (s testScope) ProjectID() id.ID[project.Project]  { return s.projectID }
func (s testScope) Tags() tag.TagRepository            { return s.tags }
func (s testScope) Scenes() scene.SceneRepository      { return s.scenes }
func (s testScope) LibraryPins() librarypin.Repository { return s.pins }
func (s testScope) Devices() device.DeviceRepository   { return s.devices }
func (s testScope) Environments() device.EnvironmentRepository {
	return s.environments
}
func (s testScope) EnvironmentDevices() device.EnvironmentDeviceRepository {
	return s.environmentDevices
}

// soleOrganization is the organization the migrations create.
func soleOrganization() id.ID[organization.Organization] {
	orgID, err := repositories.FindSoleOrganization(context.Background(), testDB)
	if err != nil {
		panic("organization: " + err.Error())
	}
	return orgID
}

// cleanProjects removes every project but mainProject, with its Library
// Pins, Devices and Environments; their other content must be gone already.
// cleanDrafts removes every Draft row and its DraftChanges: the tests start
// from a fresh Draft.
func cleanDrafts(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(),
		"DELETE FROM engineering.draft_changes; DELETE FROM engineering.drafts"); err != nil {
		t.Fatalf("clean drafts: %v", err)
	}
}

func cleanProjects(t *testing.T) {
	t.Helper()
	main := "'" + mainProject.String() + "'"
	if _, err := testDB.ExecContext(context.Background(),
		`DELETE FROM engineering.environment_devices ed USING engineering.environments e
		 WHERE ed.environment_id = e.id AND e.project_id <> `+main+`;
		 DELETE FROM engineering.devices WHERE project_id <> `+main+`;
		 DELETE FROM engineering.environments WHERE project_id <> `+main+`;
		 DELETE FROM engineering.library_pins WHERE project_id <> `+main+`;
		 DELETE FROM projects WHERE id <> `+main); err != nil {
		t.Fatalf("cleanProjects: %v", err)
	}
}

// contentOver returns the ProjectContent of the projects of the test
// database's organization, through scopes.
func contentOver(scopes application.ProjectScopes) application.ProjectContent {
	drafts, ok := scopes.(interface{ Drafts() draft.Repository })
	draftRepo := repositories.NewDraftRepositoryPostgres(testDB, repositories.NewLibraryReleaseRepositoryPostgres(testDB))
	if ok {
		draftRepo = drafts.Drafts()
	}
	return application.NewProjectContent(repositories.NewProjectRepositoryPostgres(testDB), scopes, draftRepo, soleOrganization())
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

// newRuntime returns the Runtime node's services over the test database and
// the RuntimeNode port over them.
func newRuntime() (runtimeapp.TagStateService, application.RuntimeNode) {
	states := runtimeapp.NewTagStateService(runtimepg.NewTagStateRepositoryPostgres(testDB), soleOrganization(), func(event.Event) {})
	tokens := runtimeapp.NewDeviceTokenService(runtimepg.NewDeviceTokenRepositoryPostgres(testDB))
	deployments := runtimeapp.NewDeploymentService(runtimepg.NewDeploymentRepositoryPostgres(testDB), func(event.Event) {})
	return states, inprocess.NewRuntimeNode(states, tokens, deployments)
}

// deployProject publishes the Draft of the project as a Release and deploys it,
// as the server does since task 72_4: a test that writes a Tag value deploys
// first, or the Tag has no state to write.
func deployProject(t *testing.T, projectID uuid.UUID) {
	t.Helper()
	content := contentOver(newScopes())
	file, err := application.NewProjectFileBuilder(content).Build(context.Background(), projectID)
	if err != nil {
		t.Fatalf("build project %s: %v", projectID, err)
	}
	document, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("encode project %s: %v", projectID, err)
	}
	service := application.NewReleaseService(content, repositories.NewProjectReleaseRepositoryPostgres(testDB),
		newRuntimeNode(), repositories.NewLibraryRepositoryPostgres(testDB),
		repositories.NewLibraryReleaseRepositoryPostgres(testDB))
	if _, _, err := service.CreateFromFile(context.Background(), projectID, document, "test", "Deploy"); err != nil {
		t.Fatalf("deploy project %s: %v", projectID, err)
	}
}

// newServiceWithStates returns the tag service and the Runtime node's service
// of Tag states behind it.
func newServiceWithStates() (application.TagService, runtimeapp.TagStateService) {
	states, node := newRuntime()
	return application.NewTagService(contentOver(newScopes()), node), states
}

// errRuntimeDown is the error of unreadableRuntimeNode.
var errRuntimeDown = errors.New("runtime node down")

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

// draftVersionOf returns the Version of the Project's Draft: what a write of
// its content states (ADR 0016).
func draftVersionOf(t *testing.T, projectID uuid.UUID) int {
	t.Helper()
	drafts := repositories.NewDraftRepositoryPostgres(testDB, nil)
	found, ok, err := drafts.Read(context.Background(),
		draft.ProjectScope(id.NewID(id.IDWithUUID[project.Project](projectID))))
	if err != nil {
		t.Fatalf("read the draft of %s: %v", projectID, err)
	}
	if !ok {
		return 1
	}
	return found.Version().Number()
}
