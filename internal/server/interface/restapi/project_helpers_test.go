package restapi_test

import (
	"context"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
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
	projects    application.ProjectService
	tags        application.TagService
	states      runtimeapp.TagStateService
	libraries   application.LibraryService
	widgetTypes application.WidgetTypeService
	pins        application.LibraryPinService
	scenes      application.SceneService
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
	orgID, err := repositories.FindSoleOrganization(context.Background(), db)
	if err != nil {
		panic("organization: " + err.Error())
	}
	projectRepo := repositories.NewProjectRepositoryPostgres(db, opts...)
	content := application.NewProjectContent(projectRepo, repositories.NewProjectScopes(db, opts...), orgID)
	libraries := repositories.NewLibraryRepositoryPostgres(db, opts...)
	releases := repositories.NewLibraryReleaseRepositoryPostgres(db, opts...)
	widgetTypes := repositories.NewWidgetTypeRepositories(db, opts...)
	states := runtimeapp.NewTagStateService(runtimepg.NewTagStateRepositoryPostgres(db), orgID, publish)
	return testServices{
		projects:    application.NewProjectService(projectRepo, orgID),
		tags:        application.NewTagService(content, inprocess.NewRuntimeNode(states)),
		states:      states,
		libraries:   application.NewLibraryService(libraries, releases, widgetTypes, orgID),
		widgetTypes: application.NewWidgetTypeService(libraries, widgetTypes, orgID),
		pins:        application.NewLibraryPinService(content, libraries, releases),
		scenes:      application.NewSceneService(content),
	}
}

// services returns the services as the router takes them.
func (s testServices) services() restapi.Services {
	return restapi.Services{
		Projects: s.projects, Tags: s.tags, TagStates: s.states,
		Libraries: s.libraries, WidgetTypes: s.widgetTypes, LibraryPins: s.pins, Scenes: s.scenes,
	}
}

// router returns the API router over the services.
func (s testServices) router() *restapi.APIRouter {
	return restapi.NewRouter(s.services(), eventbus.NewEventBus(), 100)
}

// anyProjects is a ProjectService that finds every project, for the event
// streams of projects that are not stored.
type anyProjects struct {
	application.ProjectService
}

func (anyProjects) FindProjectByID(_ context.Context, projectID uuid.UUID) (appdto.Project, error) {
	return appdto.Project{ID: projectID, Name: "any", Version: 1}, nil
}
