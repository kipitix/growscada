package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// newDeployService returns the Engineering node's DeployService over the test
// database, as the server wires it, and the RuntimeNode behind it.
func newDeployService() (application.DeployService, application.RuntimeNode) {
	node := newRuntimeNode()
	return application.NewDeployService(
		application.NewProjectFileBuilder(contentOver(newScopes())),
		repositories.NewProjectFileRepositoryPostgres(testDB),
		node,
	), node
}

// newDiscardService returns the Engineering node's DiscardService over the
// test database.
func newDiscardService() application.DiscardService {
	return application.NewDiscardService(contentOver(newScopes()), newRuntimeNode())
}

// newDiscardPolicy returns the policy carrying out Discards over the test
// database, as the server registers it.
func newDiscardPolicy() application.Policy {
	return application.NewDiscardProjectPolicy(
		repositories.NewProjectRepositoryPostgres(testDB),
		newScopes(),
		repositories.NewLibraryReleaseRepositoryPostgres(testDB),
		newRuntimeNode(),
	)
}

// runDiscard discards the project through the service and runs the policy, as
// the dispatcher and the runner would.
func runDiscard(t *testing.T, projectID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	if _, err := newDiscardService().Discard(ctx, projectID); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if err := newDiscardPolicy().Run(ctx, "project:"+projectID.String()); err != nil {
		t.Fatalf("the discard policy: %v", err)
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
	created, err := newService().CreateTag(context.Background(), mainProject, appdto.CreateTagInput{Name: name, Type: tagType})
	if err != nil {
		t.Fatalf("CreateTag(%s): %v", name, err)
	}
	return created
}

// --- Deploy ---

func TestDeploy_CreatesARevisionAndItsCopy(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	ctx := context.Background()
	created := mustCreateTag(t, "speed", "integer")

	deploy, node := newDeployService()
	revision, err := deploy.Deploy(ctx, mainProject)
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if revision.Number != 1 || revision.DeployedAt.IsZero() {
		t.Errorf("revision %+v, want number 1 and a time", revision)
	}
	current, ok, err := node.FindCurrentRevision(ctx, mainProject)
	if err != nil || !ok || current.Number != 1 {
		t.Fatalf("FindCurrentRevision: %+v, %v, %v", current, ok, err)
	}
	if len(current.Content.Tags) != 1 || current.Content.Tags[0].ID != created.ID {
		t.Errorf("the revision carries %+v, want the tag %s", current.Content.Tags, created.ID)
	}

	copied, ok, err := repositories.NewProjectFileRepositoryPostgres(testDB).FindByProject(ctx, mainProject)
	if err != nil || !ok || copied.Number != 1 {
		t.Fatalf("the copy at Engineering: %+v, %v, %v", copied, ok, err)
	}
	if len(copied.Content.Tags) != 1 || copied.Content.Tags[0].ID != created.ID {
		t.Errorf("the copy carries %+v, want the tag %s", copied.Content.Tags, created.ID)
	}
	if n := tagStateCount(t, mainProject); n != 1 {
		t.Errorf("tag states at the Runtime node: %d, want the deployed one", n)
	}
}

func TestDeploy_UnchangedDraft_IsRefused(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	ctx := context.Background()
	mustCreateTag(t, "speed", "integer")

	deploy, _ := newDeployService()
	if _, err := deploy.Deploy(ctx, mainProject); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if _, err := deploy.Deploy(ctx, mainProject); !errors.Is(err, application.ErrNothingToDeploy) {
		t.Errorf("a second Deploy of the same Draft: %v, want ErrNothingToDeploy", err)
	}
}

func TestDeploy_AChangedWidget_IsANewRevision(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	ctx := context.Background()
	scenes := newSceneService()
	scene := mustCreateScene(t, scenes)
	widget, err := scenes.CreateWidget(ctx, mainProject, scene.ID, scene.Version, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	deploy, _ := newDeployService()
	if _, err := deploy.Deploy(ctx, mainProject); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	state, err := deploy.State(ctx, mainProject)
	if err != nil || state.Deployed == nil || state.Differs {
		t.Fatalf("State after the Deploy: %+v, %v, want the copy and no difference", state, err)
	}

	changed := testCreateWidgetInput
	changed.Name = "moved-gauge"
	if _, err := scenes.UpdateWidget(ctx, mainProject, scene.ID, widget.ID, widget.SceneVersion, changed); err != nil {
		t.Fatalf("UpdateWidget: %v", err)
	}
	state, err = deploy.State(ctx, mainProject)
	if err != nil || !state.Differs {
		t.Fatalf("State after the change: %+v, %v, want a difference", state, err)
	}

	revision, err := deploy.Deploy(ctx, mainProject)
	if err != nil {
		t.Fatalf("the second Deploy: %v", err)
	}
	if revision.Number != 2 {
		t.Errorf("the second revision: %d, want 2", revision.Number)
	}
}

// failingCopies is the Engineering node's copies whose writes fail: the
// Revision is created and its copy is not.
type failingCopies struct {
	application.ProjectFileCopies
}

var errCopyDown = errors.New("the copies' table is down")

func (failingCopies) Save(context.Context, uuid.UUID, appdto.Revision) error { return errCopyDown }

func TestDeploy_CopyNotSaved_TheRevisionIsActive(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	ctx := context.Background()
	mustCreateTag(t, "speed", "integer")

	node := newRuntimeNode()
	deploy := application.NewDeployService(
		application.NewProjectFileBuilder(contentOver(newScopes())),
		failingCopies{ProjectFileCopies: repositories.NewProjectFileRepositoryPostgres(testDB)},
		node,
	)
	if _, err := deploy.Deploy(ctx, mainProject); !errors.Is(err, application.ErrRevisionCopyNotSaved) || !errors.Is(err, errCopyDown) {
		t.Fatalf("Deploy with the copy failing: %v, want ErrRevisionCopyNotSaved", err)
	}
	if current, ok, err := node.FindCurrentRevision(ctx, mainProject); err != nil || !ok || current.Number != 1 {
		t.Fatalf("the revision of the failed copy: %+v, %v, %v; want it active", current, ok, err)
	}

	// The copy is a cache: the Project mode sees nothing deployed, and the
	// next change deploys the Revision after it.
	state, err := deploy.State(ctx, mainProject)
	if err != nil || state.Deployed != nil || !state.Differs {
		t.Errorf("State without a copy: %+v, %v, want no Revision and a difference", state, err)
	}
	if _, err := newService().CreateTag(ctx, mainProject, appdto.CreateTagInput{Name: "flow", Type: "integer"}); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	healthy, _ := newDeployService()
	if _, err := healthy.Deploy(ctx, mainProject); err != nil {
		t.Fatalf("the Deploy after the failed copy: %v", err)
	}
}

func TestDeployPreview_ReportsTheCurrentRevisionAndTheDraft(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	ctx := context.Background()
	mustCreateTag(t, "speed", "integer")

	deploy, _ := newDeployService()
	preview, err := deploy.Preview(ctx, mainProject)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.CurrentRevision != nil || preview.Unchanged || len(preview.DroppedWidgets) != 0 {
		t.Errorf("the preview before a Deploy: %+v, want no Revision, changed, nothing dropped", preview)
	}

	if _, err := deploy.Deploy(ctx, mainProject); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	preview, err = deploy.Preview(ctx, mainProject)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.CurrentRevision == nil || preview.CurrentRevision.Number != 1 || !preview.Unchanged {
		t.Errorf("the preview after the Deploy: %+v, want revision 1 unchanged", preview)
	}
}

// --- Discard ---

func TestDiscard_WithoutARevision_IsRefused(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)

	if _, err := newDiscardService().Discard(context.Background(), mainProject); !errors.Is(err, application.ErrNoRevisionToDiscard) {
		t.Errorf("Discard of a project never deployed: %v, want ErrNoRevisionToDiscard", err)
	}
}

func TestDiscard_TheDraftIsReadOnlyWhileItRuns(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	ctx := context.Background()
	mustCreateTag(t, "speed", "integer")

	deploy, _ := newDeployService()
	if _, err := deploy.Deploy(ctx, mainProject); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	marked, err := newDiscardService().Discard(ctx, mainProject)
	if err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if marked.ID != mainProject {
		t.Errorf("the marked project: %s, want %s", marked.ID, mainProject)
	}

	if _, err := newService().CreateTag(ctx, mainProject, appdto.CreateTagInput{Name: "flow", Type: "integer"}); !errors.Is(err, project.ErrProjectDiscarding) {
		t.Errorf("a Tag created while the Discard runs: %v, want ErrProjectDiscarding", err)
	}
	if _, err := newSceneService().CreateScene(ctx, mainProject, testCreateSceneInput); !errors.Is(err, project.ErrProjectDiscarding) {
		t.Errorf("a Scene created while the Discard runs: %v, want ErrProjectDiscarding", err)
	}

	if err := newDiscardPolicy().Run(ctx, "project:"+mainProject.String()); err != nil {
		t.Fatalf("the discard policy: %v", err)
	}
	if _, err := newService().CreateTag(ctx, mainProject, appdto.CreateTagInput{Name: "flow", Type: "integer"}); err != nil {
		t.Errorf("a Tag created after the Discard: %v", err)
	}
}

// TestDiscard_RestoresTheDraft: the Draft is brought back to the Revision —
// the removed Widget, Scene, Tag, Device and pin come back under their IDs,
// what was added goes, and the Tag states are the Runtime node's, kept.
func TestDiscard_RestoresTheDraft(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	ctx := context.Background()

	tag := mustCreateTag(t, "speed", "integer")
	devices := newDeviceService()
	device, err := devices.CreateDevice(ctx, mainProject, appdto.DeviceInput{
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

	deploy, _ := newDeployService()
	if _, err := deploy.Deploy(ctx, mainProject); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	// A state written before the Discard is the Runtime node's to keep.
	states, _ := newRuntime()
	if _, err := states.SetTagValue(ctx, mainProject, runtimeapp.SetTagValueInput{TagID: tag.ID, Value: "7", Quality: "good"}); err != nil {
		t.Fatalf("SetTagValue: %v", err)
	}

	// Change the Draft: remove the Scene, the Tag and the Device (in the order
	// the Draft's references allow), and add a Tag and a Device.
	if _, err := scenes.DeleteSceneByID(ctx, mainProject, scene.ID); err != nil {
		t.Fatalf("DeleteSceneByID: %v", err)
	}
	if _, err := newService().DeleteTagByID(ctx, mainProject, tag.ID); err != nil {
		t.Fatalf("DeleteTagByID: %v", err)
	}
	if _, err := devices.DeleteDeviceByID(ctx, mainProject, device.ID); err != nil {
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
		t.Errorf("the Tag states after the Discard: %d, want the running revision's one kept", n)
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

// TestDiscard_OfAPinnedLibraryRestoresThePin: the pin of a Library the Draft
// dropped comes back, at the release the Revision names, without the check of
// the Widgets a switch runs.
func TestDiscard_OfAPinnedLibraryRestoresThePin(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	cleanPins(t)
	ctx := context.Background()

	_, lib := pinNewWidgetType(t, mainProject, appdto.WidgetTypeInput{Name: "gauge"})

	deploy, _ := newDeployService()
	if _, err := deploy.Deploy(ctx, mainProject); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if _, err := newLibraryPinService().DeleteLibraryPin(ctx, mainProject, lib.ID); err != nil {
		t.Fatalf("DeleteLibraryPin: %v", err)
	}

	runDiscard(t, mainProject)

	pins, err := newLibraryPinService().FindLibraryPins(ctx, mainProject)
	if err != nil || len(pins) != 1 || pins[0].LibraryID != lib.ID || pins[0].Release != 1 {
		t.Errorf("the pins after the Discard: %+v, %v, want %s at release 1", pins, err, lib.ID)
	}
}
