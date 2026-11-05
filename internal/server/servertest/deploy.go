package servertest

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
	runtimepg "github.com/kipitix/growscada/internal/server/infrastructure/postgres/runtime"
	"github.com/kipitix/growscada/internal/server/inprocess"
)

// DeployProject deploys the Project's Draft as a Revision, through the
// Engineering node's DeployService, as the server does (task 35, part 4): the
// Tags with their suppliers, the Devices, the Environments with their pairs,
// the Scenes with their Widgets and the WidgetTypes their pins hold. A test
// that needs a Gateway's configuration or a Tag value of the running
// configuration deploys in its setup: nothing of the Draft reaches the Runtime
// node before a Deploy.
func DeployProject(t *testing.T, db *sql.DB, projectID uuid.UUID) {
	t.Helper()
	ctx := editlock.System(context.Background())
	orgID, err := repositories.FindSoleOrganization(ctx, db)
	if err != nil {
		t.Fatalf("organization: %v", err)
	}
	releases := repositories.NewLibraryReleaseRepositoryPostgres(db)
	drafts := repositories.NewDraftRepositoryPostgres(db, releases)
	content := application.NewProjectContent(
		repositories.NewProjectRepositoryPostgres(db), repositories.NewProjectScopes(db), drafts, orgID)
	states := runtimeapp.NewTagStateService(runtimepg.NewTagStateRepositoryPostgres(db), orgID, func(event.Event) {})
	tokens := runtimeapp.NewDeviceTokenService(runtimepg.NewDeviceTokenRepositoryPostgres(db))
	revisions := runtimeapp.NewRevisionService(runtimepg.NewRevisionRepositoryPostgres(db), func(event.Event) {})
	locks := application.NewEditLockService(content,
		application.NewLibraryContent(repositories.NewLibraryRepositoryPostgres(db), drafts, orgID),
		drafts)
	service := application.NewDeployService(
		content,
		application.NewProjectFileBuilder(content),
		inprocess.NewRuntimeNode(states, tokens, revisions),
		locks,
	)
	if _, err := service.Deploy(ctx, projectID); err != nil {
		// A test that deploys twice without changing the Draft asks for
		// nothing: the second Deploy is refused as unchanged, as the API does.
		if errors.Is(err, application.ErrNothingToDeploy) {
			return
		}
		t.Fatalf("deploy project %s: %v", projectID, err)
	}
}
