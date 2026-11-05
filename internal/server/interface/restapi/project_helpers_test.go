package restapi_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
	runtimepg "github.com/kipitix/growscada/internal/server/infrastructure/postgres/runtime"
	"github.com/kipitix/growscada/internal/server/inprocess"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
	"github.com/kipitix/growscada/internal/server/interface/restapi"
)

// mainProject is the project "Main" the migrations create, mainPath its
// path: the tests' content lives in it unless they make projects of their own.
var (
	mainProject   = uuid.MustParse("b0000000-0000-4000-8000-000000000001")
	mainProjectID = id.NewID(id.IDWithUUID[project.Project](mainProject))
)

const mainPath = apiv0.PathPrefix + "/projects/b0000000-0000-4000-8000-000000000001"

// testServices are the application services over the test database, as
// main.go wires them.
type testServices struct {
	projects     application.ProjectService
	tags         application.TagService
	states       runtimeapp.TagStateService
	revisions    runtimeapp.RevisionService
	deploy       application.DeployService
	editLocks    application.EditLockService
	discard      application.DiscardService
	libraries    application.LibraryService
	widgetTypes  application.WidgetTypeService
	pins         application.LibraryPinService
	scenes       application.SceneService
	devices      application.DeviceService
	environments application.EnvironmentService
	deviceConfig runtimeapp.DeviceConfigService
	deviceTokens runtimeapp.DeviceTokenService
	draft        application.DraftService
}

// newServices returns the services over the test database; the Runtime
// node's tag_updated go nowhere.
func newServices(opts ...repositories.Option) testServices {
	return newServicesPublishingTo(func(event.Event) {}, opts...)
}

// newServicesPublishingTo returns the services over the test database, the
// Runtime node publishing its tag_updated to publish.
func newServicesPublishingTo(publish func(event.Event), opts ...repositories.Option) testServices {
	db := testDB()
	orgID, err := repositories.FindSoleOrganization(editlock.System(context.Background()), db)
	if err != nil {
		panic("organization: " + err.Error())
	}
	projectRepo := repositories.NewProjectRepositoryPostgres(db, opts...)
	releases := repositories.NewLibraryReleaseRepositoryPostgres(db, opts...)
	drafts := repositories.NewDraftRepositoryPostgres(db, releases, opts...)
	content := application.NewProjectContent(projectRepo, repositories.NewProjectScopes(db, opts...), drafts, orgID)
	editLocks := application.NewEditLockService(content,
		application.NewLibraryContent(repositories.NewLibraryRepositoryPostgres(db, opts...), orgID),
		drafts)
	libraries := repositories.NewLibraryRepositoryPostgres(db, opts...)
	widgetTypes := repositories.NewWidgetTypeRepositories(db, opts...)
	states := runtimeapp.NewTagStateService(runtimepg.NewTagStateRepositoryPostgres(db), orgID, publish)
	revisions := runtimeapp.NewRevisionService(runtimepg.NewRevisionRepositoryPostgres(db), publish)
	deviceTokens := runtimeapp.NewDeviceTokenService(runtimepg.NewDeviceTokenRepositoryPostgres(db))
	runtimeNode := inprocess.NewRuntimeNode(states, deviceTokens, revisions)
	return testServices{
		projects:     application.NewProjectService(projectRepo, orgID),
		tags:         application.NewTagService(content, runtimeNode),
		states:       states,
		revisions:    revisions,
		deploy:       application.NewDeployService(content, application.NewProjectFileBuilder(content), runtimeNode, editLocks),
		editLocks:    editLocks,
		discard:      application.NewDiscardService(content, runtimeNode),
		draft:        application.NewDraftService(content),
		libraries:    application.NewLibraryService(libraries, releases, widgetTypes, orgID),
		widgetTypes:  application.NewWidgetTypeService(libraries, widgetTypes, orgID),
		pins:         application.NewLibraryPinService(content, libraries, releases),
		scenes:       application.NewSceneService(content),
		devices:      application.NewDeviceService(content),
		environments: application.NewEnvironmentService(content),
		deviceConfig: runtimeapp.NewDeviceConfigService(runtimepg.NewDeviceConfigRepositoryPostgres(db)),
		deviceTokens: deviceTokens,
	}
}

// services returns the services as the router takes them.
func (s testServices) services() restapi.Services {
	return restapi.Services{
		Projects: s.projects, Tags: s.tags, TagStates: s.states,
		Libraries: s.libraries, WidgetTypes: s.widgetTypes, LibraryPins: s.pins, Scenes: s.scenes,
		Devices: s.devices, Environments: s.environments,
		Draft: s.draft, Deploy: s.deploy, Discard: s.discard, Revisions: s.revisions,
		DeviceConfig: s.deviceConfig, DeviceTokens: s.deviceTokens, EditLocks: s.editLocks,
	}
}

// router returns the API router over the services, with the test client's key
// on every request: the tests write the Draft, which needs the EditLock.
func (s testServices) router() testRouter {
	return testRouter{APIRouter: restapi.NewRouter(s.services(), eventbus.NewEventBus(), 100)}
}

// deployRevision deploys file to mainProject through the Runtime node's
// service, as the Engineering node's DeployService will (task 35, part 4).
func deployRevision(t *testing.T, service runtimeapp.RevisionService, file projectv0.ProjectFile) {
	t.Helper()
	if _, err := service.Deploy(editlock.System(context.Background()), mainProject, file); err != nil {
		t.Fatalf("deploy: %v", err)
	}
}

// anyProjects is a ProjectService that finds every project, for the event
// streams of projects that are not stored.
type anyProjects struct {
	application.ProjectService
}

func (anyProjects) FindProjectByID(_ context.Context, projectID uuid.UUID) (appdto.Project, error) {
	return appdto.Project{ID: projectID, Name: "any", Version: 1}, nil
}

// draftVersion returns the Version of mainProject's Draft: what a write of its
// content states (ADR 0016).
func (s testServices) draftVersion(t *testing.T) int {
	t.Helper()
	state, err := s.draft.State(editlock.System(context.Background()), mainProject)
	if err != nil {
		t.Fatalf("draft state: %v", err)
	}
	return state.Version
}

// testDraftVersion returns the Version of the Draft of projectID: what a
// write of its content states (ADR 0016).
func testDraftVersion(t *testing.T, projectID uuid.UUID) int {
	t.Helper()
	drafts := repositories.NewDraftRepositoryPostgres(testDB(), nil)
	found, ok, err := drafts.Read(editlock.System(context.Background()),
		editlock.ProjectScope(id.NewID(id.IDWithUUID[project.Project](projectID))))
	if err != nil {
		t.Fatalf("read the draft of %s: %v", projectID, err)
	}
	if !ok {
		return 1
	}
	return found.Version().Number()
}
