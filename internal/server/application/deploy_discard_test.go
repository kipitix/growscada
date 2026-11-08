package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/release"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// mustSaveCommit fixes the Draft of the project as a Commit at the Version it
// shows, as Save does. A Draft unchanged since its last Commit has nothing to
// save: that Commit is the one the test means.
func mustSaveCommit(t *testing.T, projectID uuid.UUID, title string) appdto.Commit {
	t.Helper()
	ctx := context.Background()
	scope := draft.ProjectScope(id.NewID(id.IDWithUUID[project.Project](projectID)))
	saved, err := newCommitService().Save(ctx, scope, draftVersionOf(t, projectID), title, "Ada")
	if errors.Is(err, draft.ErrNothingToSave) {
		list, listErr := newCommitService().List(ctx, scope)
		if listErr != nil || len(list) == 0 {
			t.Fatalf("no commit to reuse: %v", listErr)
		}
		return list[0]
	}
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	return saved
}

// mustRelease publishes Commit number, or returns the Release that already
// published it.
func mustRelease(t *testing.T, projectID uuid.UUID, commitNumber int) appdto.Release {
	t.Helper()
	ctx := context.Background()
	created, err := newReleaseService().Create(ctx, projectID, commitNumber, "", "Ada")
	if err == nil {
		return created
	}
	list, listErr := newReleaseService().List(ctx, projectID)
	if listErr != nil {
		t.Fatalf("Create release: %v", err)
	}
	for _, existing := range list {
		if existing.CommitNumber == commitNumber {
			return existing
		}
	}
	t.Fatalf("Create release: %v", err)
	return appdto.Release{}
}

// mustReleaseAndDeploy publishes Commit commitNumber and makes it the running
// Deployment: the two steps a Deploy takes since task 72_4.
func mustReleaseAndDeploy(t *testing.T, projectID uuid.UUID, commitNumber int) appdto.Deployment {
	t.Helper()
	ctx := context.Background()
	released := mustRelease(t, projectID, commitNumber)
	deployment, err := newReleaseService().Deploy(ctx, projectID, released.Number, "Ada")
	if err != nil {
		t.Fatalf("Deploy release %d: %v", released.Number, err)
	}
	return deployment
}

// mustDeployTheDraft saves the Draft as a Commit and deploys its Release.
func mustDeployTheDraft(t *testing.T, projectID uuid.UUID) appdto.Deployment {
	t.Helper()
	saved := mustSaveCommit(t, projectID, "Deploy")
	return mustReleaseAndDeploy(t, projectID, saved.Number)
}

// newDiscardService returns the Engineering node's DiscardService over the
// test database.
func newDiscardService() application.DiscardService {
	return application.NewDiscardService(contentOver(newScopes()), newRuntimeNode(),
		repositories.NewLibraryRepositoryPostgres(testDB), repositories.NewLibraryReleaseRepositoryPostgres(testDB))
}

// runDiscard discards the project through the service.
func runDiscard(t *testing.T, projectID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	if _, err := newDiscardService().Discard(ctx, projectID); err != nil {
		t.Fatalf("Discard: %v", err)
	}
}

// cleanPins removes the Library Pins of mainProject: a test that counts them
// starts from none. The Libraries and their releases stay.
func cleanPins(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(),
		"DELETE FROM engineering.library_pins WHERE project_id = $1", mainProject); err != nil {
		t.Fatalf("cleanPins: %v", err)
	}
}

// mustCreateTag creates a Tag through the service.
func mustCreateTag(t *testing.T, name, tagType string) appdto.Tag {
	t.Helper()
	created, err := newService().CreateTag(context.Background(), mainProject, appdto.CreateTagInput{Version: draftVersionOf(t, mainProject), Name: name, Type: tagType})
	if err != nil {
		t.Fatalf("CreateTag(%s): %v", name, err)
	}
	return created
}

// --- Deploy Release ---

func TestDeployRelease_CreatesADeployment(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	ctx := context.Background()
	created := mustCreateTag(t, "speed", "integer")
	saved := mustSaveCommit(t, mainProject, "First save")
	released, err := newReleaseService().Create(ctx, mainProject, saved.Number, "", "Ada")
	if err != nil {
		t.Fatalf("Create release: %v", err)
	}

	deployment, err := newReleaseService().Deploy(ctx, mainProject, released.Number, "Ada")
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if deployment.Number < 1 || deployment.DeployedAt.IsZero() {
		t.Errorf("deployment %+v, want a number and a time", deployment)
	}
	if deployment.Cause != "deploy" || deployment.RolledBackFrom != nil {
		t.Errorf("the cause of the Deployment is %q from %v, want a deploy", deployment.Cause, deployment.RolledBackFrom)
	}
	if deployment.ReleaseNumber == nil || *deployment.ReleaseNumber != released.Number {
		t.Errorf("the deployment names release %v, want %d", deployment.ReleaseNumber, released.Number)
	}
	current, ok, err := newRuntimeNode().FindCurrentDeployment(ctx, mainProject)
	if err != nil || !ok || current.Number != deployment.Number {
		t.Fatalf("FindCurrentDeployment: %+v, %v, %v", current, ok, err)
	}
	if len(current.Content.Tags) != 1 || current.Content.Tags[0].ID != created.ID {
		t.Errorf("the deployment carries %+v, want the tag %s", current.Content.Tags, created.ID)
	}
	if n := tagStateCount(t, mainProject); n != 1 {
		t.Errorf("tag states at the Runtime node: %d, want the deployed one", n)
	}
}

func TestDeployRelease_Repeated_IsRefused(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	ctx := context.Background()
	mustCreateTag(t, "speed", "integer")
	saved := mustSaveCommit(t, mainProject, "First save")
	released, err := newReleaseService().Create(ctx, mainProject, saved.Number, "", "Ada")
	if err != nil {
		t.Fatalf("Create release: %v", err)
	}
	if _, err := newReleaseService().Deploy(ctx, mainProject, released.Number, "Ada"); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if _, err := newReleaseService().Deploy(ctx, mainProject, released.Number, "Ada"); !errors.Is(err, application.ErrNothingToDeploy) {
		t.Errorf("a second Deploy of the same Release: %v, want ErrNothingToDeploy", err)
	}
}

func TestDeployRelease_Missing_IsNotFound(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	if _, err := newReleaseService().Deploy(context.Background(), mainProject, 9, "Ada"); !errors.Is(err, release.ErrReleaseNotFound) {
		t.Errorf("Deploy of a missing release: %v, want ErrReleaseNotFound", err)
	}
}

func TestDeployRelease_AChangedCommit_IsANewDeployment(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	ctx := context.Background()
	scenes := newSceneService()
	scene := mustCreateScene(t, scenes)
	widget, err := scenes.CreateWidget(ctx, mainProject, scene.ID, scene.Version, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}
	first := mustDeployTheDraft(t, mainProject)
	if first.ReleaseNumber == nil {
		t.Fatalf("the first deployment: %+v, want its release", first)
	}

	changed := testCreateWidgetInput
	changed.Name = "moved-gauge"
	if _, err := scenes.UpdateWidget(ctx, mainProject, scene.ID, widget.ID, widget.Version, changed); err != nil {
		t.Fatalf("UpdateWidget: %v", err)
	}
	second := mustDeployTheDraft(t, mainProject)
	if second.Number != first.Number+1 || second.ReleaseNumber == nil || *second.ReleaseNumber <= *first.ReleaseNumber {
		t.Errorf("the second deployment: %+v, want the next Deployment of a later Release", second)
	}
}

// TestCreateReleaseFromFile_DeploysTheFile: a file deployment publishes the
// document as a Release with origin file and makes it the running Deployment
// (task 72_4).
func TestCreateReleaseFromFile_DeploysTheFile(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	ctx := context.Background()
	mustCreateTag(t, "speed", "integer")
	content := contentOver(newScopes())
	file, err := application.NewProjectFileBuilder(content).Build(ctx, mainProject)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	document, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	released, deployment, err := newReleaseService().CreateFromFile(ctx, mainProject, document, "", "")
	if err != nil {
		t.Fatalf("CreateFromFile: %v", err)
	}
	if released.Origin != "file" || released.CommitNumber != 0 || released.Title != "File" || released.Author != "file" {
		t.Errorf("the release of the file: %+v", released)
	}
	if deployment.Number < 1 || deployment.ReleaseNumber == nil || *deployment.ReleaseNumber != released.Number {
		t.Errorf("the deployment of the file: %+v, want a Deployment of release %d", deployment, released.Number)
	}
	current, ok, err := newRuntimeNode().FindCurrentDeployment(ctx, mainProject)
	if err != nil || !ok || current.ReleaseNumber == nil || *current.ReleaseNumber != released.Number {
		t.Errorf("the running deployment: %+v, %v, %v", current, ok, err)
	}
}

// --- Discard to the last Commit and Revert to the Deployment ---

func TestDiscard_WithoutACommit_IsRefused(t *testing.T) {
	cleanProjects(t)
	ctx := context.Background()
	created, err := application.NewProjectService(repositories.NewProjectRepositoryPostgres(testDB), soleOrganization()).
		CreateProject(ctx, appdto.ProjectInput{Name: "discard-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	if _, err := newDiscardService().Discard(ctx, created.ID); !errors.Is(err, application.ErrNothingToDiscard) {
		t.Errorf("Discard of a project never saved: %v, want ErrNothingToDiscard", err)
	}
}

// TestDiscard_RestoresTheLastCommit: the Draft is brought back to its last
// Commit — the removed Widget, Scene, Tag, Device and pin come back under
// their IDs, what was added goes, and the Tag states are the Runtime node's,
// kept.
func TestDiscard_RestoresTheLastCommit(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	ctx := context.Background()

	tag := mustCreateTag(t, "speed", "integer")
	devices := newDeviceService()
	device, err := devices.CreateDevice(ctx, mainProject, draftVersionOf(t, mainProject), appdto.DeviceInput{
		Name: "pump", Comment: "the pump", Protocol: "simulation", Config: []byte(`{"pattern":"ramp"}`),
	})
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	scenes := newSceneService()
	scene := mustCreateScene(t, scenes)
	widget, err := scenes.CreateWidget(ctx, mainProject, scene.ID, scene.Version, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	mustDeployTheDraft(t, mainProject)
	// A state written before the Discard is the Runtime node's to keep.
	states, _ := newRuntime()
	if _, err := states.SetTagValue(ctx, mainProject, runtimeapp.SetTagValueInput{TagID: tag.ID, Value: "7", Quality: "good"}); err != nil {
		t.Fatalf("SetTagValue: %v", err)
	}

	// Change the Draft: remove the Scene, the Tag and the Device (in the order
	// the Draft's references allow), and add a Tag and a Device.
	if _, err := scenes.DeleteSceneByID(ctx, mainProject, scene.ID, draftVersionOf(t, mainProject)); err != nil {
		t.Fatalf("DeleteSceneByID: %v", err)
	}
	if _, err := newService().DeleteTagByID(ctx, mainProject, tag.ID, draftVersionOf(t, mainProject)); err != nil {
		t.Fatalf("DeleteTagByID: %v", err)
	}
	if _, err := devices.DeleteDeviceByID(ctx, mainProject, device.ID, draftVersionOf(t, mainProject)); err != nil {
		t.Fatalf("DeleteDeviceByID: %v", err)
	}
	added := mustCreateTag(t, "added", "boolean")

	runDiscard(t, mainProject)

	allTags, err := newService().FindAllTags(ctx, mainProject)
	if err != nil {
		t.Fatalf("FindAllTags: %v", err)
	}
	if len(allTags) != 1 || allTags[0].ID != tag.ID || allTags[0].Name != "speed" {
		t.Errorf("the tags after the Discard: %+v, want only %s", allTags, tag.Name)
	}
	for _, found := range allTags {
		if found.ID == added.ID {
			t.Errorf("the tag added before the Discard is still there")
		}
	}
	if n := tagStateCount(t, mainProject); n != 1 {
		t.Errorf("the Tag states after the Discard: %d, want the running deployment's one kept", n)
	}
	if found, err := newService().FindTagByID(ctx, mainProject, tag.ID); err != nil || found.Value != "7" {
		t.Errorf("the state of %s after the Discard: %+v, %v, want the value 7 kept", tag.ID, found, err)
	}

	allDevices, err := devices.FindAllDevices(ctx, mainProject)
	if err != nil || len(allDevices) != 1 || allDevices[0].ID != device.ID {
		t.Errorf("the devices after the Discard: %+v, %v, want %s back", allDevices, err, device.ID)
	}

	allScenes, err := scenes.FindAllScenes(ctx, mainProject)
	if err != nil || len(allScenes) != 1 || allScenes[0].ID != scene.ID {
		t.Fatalf("the scenes after the Discard: %+v, %v, want %s back", allScenes, err, scene.ID)
	}
	widgets, err := scenes.FindWidgetsBySceneID(ctx, mainProject, scene.ID)
	if err != nil || len(widgets) != 1 || widgets[0].ID != widget.ID {
		t.Errorf("the widgets after the Discard: %+v, %v, want %s back", widgets, err, widget.ID)
	}
}

// TestDiscard_RemovesADeviceAndAnEnvironmentOfTheDraft: a Device and an
// Environment added since the last Commit go, with the pair that joins them and
// the Tag the Device supplies.
func TestDiscard_RemovesADeviceAndAnEnvironmentOfTheDraft(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	cleanPins(t)
	ctx := context.Background()

	mustDeployTheDraft(t, mainProject)

	devices := newDeviceService()
	added, err := devices.CreateDevice(ctx, mainProject, draftVersionOf(t, mainProject), appdto.DeviceInput{
		Name: "added-" + uuid.NewString(), Protocol: "simulation", Config: []byte(`{"pattern":"ramp"}`),
	})
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	environments := newEnvironmentService()
	environment, err := environments.CreateEnvironment(ctx, mainProject, draftVersionOf(t, mainProject), appdto.EnvironmentInput{Name: "added-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	if _, _, err := environments.PutEnvironmentDevice(ctx, mainProject, environment.ID, added.ID, draftVersionOf(t, mainProject),
		appdto.EnvironmentDeviceInput{Params: []byte(`{"timeout":1}`)}); err != nil {
		t.Fatalf("PutEnvironmentDevice: %v", err)
	}
	supplied := mustCreateTag(t, "supplied", "integer")
	if _, err := testDB.ExecContext(ctx,
		"UPDATE engineering.tags SET device_id = $1 WHERE id = $2", added.ID, supplied.ID); err != nil {
		t.Fatalf("supply tag %s: %v", supplied.ID, err)
	}

	runDiscard(t, mainProject)

	if all, err := devices.FindAllDevices(ctx, mainProject); err != nil || len(all) != 0 {
		t.Errorf("the devices after the Discard: %+v, %v, want none", all, err)
	}
	if all, err := environments.FindAllEnvironments(ctx, mainProject); err != nil || len(all) != 0 {
		t.Errorf("the environments after the Discard: %+v, %v, want none", all, err)
	}
	if all, err := newService().FindAllTags(ctx, mainProject); err != nil || len(all) != 0 {
		t.Errorf("the tags after the Discard: %+v, %v, want none", all, err)
	}
}

// TestDiscard_OfAPinnedLibraryRestoresThePin: the pin of a Library the Draft
// dropped comes back, at the release the last Commit names.
func TestDiscard_OfAPinnedLibraryRestoresThePin(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	cleanPins(t)
	ctx := context.Background()

	_, lib := pinNewWidgetType(t, mainProject, appdto.WidgetTypeInput{Name: "gauge"})
	mustDeployTheDraft(t, mainProject)
	if _, err := newLibraryPinService().DeleteLibraryPin(ctx, mainProject, lib.ID, draftVersionOf(t, mainProject)); err != nil {
		t.Fatalf("DeleteLibraryPin: %v", err)
	}

	runDiscard(t, mainProject)

	pins, err := newLibraryPinService().FindLibraryPins(ctx, mainProject)
	if err != nil || len(pins) != 1 || pins[0].LibraryID != lib.ID || pins[0].Release != 1 {
		t.Errorf("the pins after the Discard: %+v, %v, want %s at release 1", pins, err, lib.ID)
	}
}

func TestRevert_WithoutADeployment_IsRefused(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)

	if _, err := newDiscardService().Revert(context.Background(), mainProject); !errors.Is(err, application.ErrNoDeploymentToDiscard) {
		t.Errorf("Revert of a project never deployed: %v, want ErrNoDeploymentToDiscard", err)
	}
}

// TestRevert_RestoresTheDeployment: the Draft goes back to what runs, even
// when a later Commit exists — that is the difference from Discard.
func TestRevert_RestoresTheDeployment(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	ctx := context.Background()

	tag := mustCreateTag(t, "speed", "integer")
	mustDeployTheDraft(t, mainProject)
	if _, err := newService().DeleteTagByID(ctx, mainProject, tag.ID, draftVersionOf(t, mainProject)); err != nil {
		t.Fatalf("DeleteTagByID: %v", err)
	}

	if _, err := newDiscardService().Revert(ctx, mainProject); err != nil {
		t.Fatalf("Revert: %v", err)
	}

	allTags, err := newService().FindAllTags(ctx, mainProject)
	if err != nil || len(allTags) != 1 || allTags[0].ID != tag.ID {
		t.Errorf("the tags after the Revert: %+v, %v, want %s back", allTags, err, tag.ID)
	}
}
