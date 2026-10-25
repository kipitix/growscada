package restapi_test

import (
	"context"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
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
	widgetTypes application.WidgetTypeService
	scenes      application.SceneService
}

func newServices(opts ...repositories.Option) testServices {
	db := testDB()
	orgID, err := repositories.FindSoleOrganization(context.Background(), db)
	if err != nil {
		panic("organization: " + err.Error())
	}
	projectRepo := repositories.NewProjectRepositoryPostgres(db, opts...)
	content := application.NewProjectContent(projectRepo, repositories.NewProjectScopes(db, opts...), orgID)
	wtRepo := repositories.NewWidgetTypeRepositoryPostgres(db, opts...)
	return testServices{
		projects:    application.NewProjectService(projectRepo, orgID),
		tags:        application.NewTagService(content),
		widgetTypes: application.NewWidgetTypeService(wtRepo),
		scenes:      application.NewSceneService(content, wtRepo),
	}
}

// router returns the API router over the services.
func (s testServices) router() *restapi.APIRouter {
	return restapi.NewRouter(s.projects, s.tags, s.widgetTypes, s.scenes, eventbus.NewEventBus(), 100)
}

// anyProjects is a ProjectService that finds every project, for the event
// streams of projects that are not stored.
type anyProjects struct {
	application.ProjectService
}

func (anyProjects) FindProjectByID(_ context.Context, projectID uuid.UUID) (appdto.Project, error) {
	return appdto.Project{ID: projectID, Name: "any", Version: 1}, nil
}
