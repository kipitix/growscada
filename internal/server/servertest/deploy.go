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
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
	runtimepg "github.com/kipitix/growscada/internal/server/infrastructure/postgres/runtime"
	"github.com/kipitix/growscada/internal/server/inprocess"
)

// DeployProject saves the Project's Draft as a saved state and deploys it, as
// the server does since task 85_4: the Tags with their suppliers, the Devices,
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
	// The Deploy makes a saved state, as the order is Save → Deploy: a Draft
	// already saved at this content keeps its last Commit.
	scope := draft.ProjectScope(id.NewID(id.IDWithUUID[project.Project](projectID)))
	commit, err := saveDraft(ctx, drafts, scope, draft.NewProjectCommitContent(file))
	if err != nil {
		t.Fatalf("save the draft of project %s: %v", projectID, err)
	}
	node := inprocess.NewRuntimeNode(states, tokens, deployments)
	if _, err := application.NewDeployService(content, node).Deploy(ctx, projectID, commit.Number()); err != nil {
		if errors.Is(err, application.ErrNothingToDeploy) {
			return
		}
		t.Fatalf("deploy project %s: %v", projectID, err)
	}
}

// saveDraft fixes the Draft as a Commit, or returns its last Commit when the
// Draft is at its Clean Version already.
func saveDraft(ctx context.Context, drafts draft.Repository, scope draft.Scope, content draft.CommitContent) (draft.Commit, error) {
	current, ok, err := drafts.Read(ctx, scope)
	if err != nil {
		return nil, err
	}
	version := version.Committed[draft.Draft]()
	if ok {
		version = current.Version()
	}
	title, err := draft.NewCommitTitle("Deploy")
	if err != nil {
		return nil, err
	}
	author, err := draft.NewAuthor("test")
	if err != nil {
		return nil, err
	}
	commit, err := drafts.SaveCommit(ctx, scope, version, title, author, content)
	if !errors.Is(err, draft.ErrNothingToSave) {
		return commit, err
	}
	commit, ok, err = drafts.FindLastCommit(ctx, scope)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, draft.ErrCommitNotFound
	}
	return commit, nil
}
