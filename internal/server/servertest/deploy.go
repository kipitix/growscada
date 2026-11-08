package servertest

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
	runtimepg "github.com/kipitix/growscada/internal/server/infrastructure/postgres/runtime"
	"github.com/kipitix/growscada/internal/server/inprocess"
)

// DeployProject publishes the Project's Draft as a Release and deploys it, as
// the server does since task 72_4: the Tags with their suppliers, the Devices,
// the Environments with their pairs, the Scenes with their Widgets and the
// WidgetTypes their pins hold. A test that needs a Gateway's configuration or a
// Tag value of the running configuration deploys in its setup: nothing of the
// Draft reaches the Runtime node before a Deploy.
func DeployProject(t *testing.T, db *sql.DB, projectID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	orgID, err := repositories.FindSoleOrganization(ctx, db)
	if err != nil {
		t.Fatalf("organization: %v", err)
	}
	libraryReleases := repositories.NewLibraryReleaseRepositoryPostgres(db)
	drafts := repositories.NewDraftRepositoryPostgres(db, libraryReleases)
	content := application.NewProjectContent(
		repositories.NewProjectRepositoryPostgres(db), repositories.NewProjectScopes(db), drafts, orgID)
	states := runtimeapp.NewTagStateService(runtimepg.NewTagStateRepositoryPostgres(db), orgID, func(event.Event) {})
	tokens := runtimeapp.NewDeviceTokenService(runtimepg.NewDeviceTokenRepositoryPostgres(db))
	deployments := runtimeapp.NewDeploymentService(runtimepg.NewDeploymentRepositoryPostgres(db), func(event.Event) {})
	builder := application.NewProjectFileBuilder(content)
	releasesService := application.NewReleaseService(content, repositories.NewProjectReleaseRepositoryPostgres(db),
		inprocess.NewRuntimeNode(states, tokens, deployments),
		repositories.NewLibraryRepositoryPostgres(db), libraryReleases)
	file, err := builder.Build(ctx, projectID)
	if err != nil {
		t.Fatalf("build project %s: %v", projectID, err)
	}
	document, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("encode project %s: %v", projectID, err)
	}
	// A test that deploys twice without changing the Draft asks for nothing:
	// an unchanged document is not deployed again, as the API refuses it.
	running, ok, err := deployments.FindCurrent(ctx, projectID)
	if err != nil {
		t.Fatalf("read the running deployment of project %s: %v", projectID, err)
	}
	if ok {
		runningDocument, err := json.Marshal(running.Content)
		if err != nil {
			t.Fatalf("encode the running deployment of project %s: %v", projectID, err)
		}
		if string(runningDocument) == string(document) {
			return
		}
	}
	if _, _, err := releasesService.CreateFromFile(ctx, projectID, document, "test", "Deploy"); err != nil {
		if errors.Is(err, application.ErrNothingToDeploy) {
			return
		}
		t.Fatalf("deploy project %s: %v", projectID, err)
	}
}
