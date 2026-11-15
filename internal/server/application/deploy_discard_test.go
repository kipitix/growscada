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
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// mustSaveState fixes the Draft of the project as a saved state at the Version
// it shows, as Save does. A Draft unchanged since its last saved state has
// nothing to save: that saved state is the one the test means.
func mustSaveState(t *testing.T, projectID uuid.UUID, title string) appdto.SavedState {
	t.Helper()
	ctx := context.Background()
	scope := draft.ProjectScope(id.NewID(id.IDWithUUID[project.Project](projectID)))
	saved, err := newSavedStateService().Save(ctx, scope, draftVersionOf(t, projectID), title, "Ada")
	if errors.Is(err, draft.ErrNothingToSave) {
		list, listErr := newSavedStateService().List(ctx, scope, uuid.Nil)
		if listErr != nil || len(list) == 0 {
			t.Fatalf("no saved state to reuse: %v", listErr)
		}
		return list[0]
	}
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	return saved
}

// mustDeploySavedState makes the saved state savedStateNumber the running
// Deployment.
func mustDeploySavedState(t *testing.T, projectID uuid.UUID, savedStateNumber int) appdto.Deployment {
	t.Helper()
	deployment, err := newDeployService().Deploy(context.Background(), projectID, savedStateNumber)
	if err != nil {
		t.Fatalf("Deploy saved state %d: %v", savedStateNumber, err)
	}
	return deployment
}

// mustDeployTheDraft saves the Draft as a saved state and deploys it.
func mustDeployTheDraft(t *testing.T, projectID uuid.UUID) appdto.Deployment {
	t.Helper()
	saved := mustSaveState(t, projectID, "Deploy")
	return mustDeploySavedState(t, projectID, saved.Number)
}

// newDiscardService returns the Engineering node's DiscardService over the
// test database.
func newDiscardService() application.DiscardService {
	return application.NewDiscardService(contentOver(newScopes()), newRuntimeNode(),
		repositories.NewLibraryRepositoryPostgres(testDB))
}

// runDiscard discards the project through the service.
func runDiscard(t *testing.T, projectID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	if _, err := newDiscardService().Discard(ctx, projectID); err != nil {
		t.Fatalf("Discard: %v", err)
	}
}

// cleanImports removes the Library Imports of mainProject: a test that counts
// them starts from none. The Libraries and their saved states stay.
func cleanImports(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if _, err := testDB.ExecContext(ctx,
		"DELETE FROM engineering.library_imports WHERE project_id = $1", mainProject); err != nil {
		t.Fatalf("cleanImports: %v", err)
	}
	// The content changed without a Save: the Draft's Clean Version is gone.
	if _, err := testDB.ExecContext(ctx,
		"UPDATE engineering.drafts SET clean_version = 0 WHERE project_id = $1", mainProject); err != nil {
		t.Fatalf("cleanImports: %v", err)
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

// --- Deploy a saved state ---

func TestDeploySavedState_CreatesADeployment(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	ctx := context.Background()
	created := mustCreateTag(t, "speed", "integer")
	saved := mustSaveState(t, mainProject, "First save")

	deployment, err := newDeployService().Deploy(ctx, mainProject, saved.Number)
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if deployment.Number < 1 || deployment.DeployedAt.IsZero() {
		t.Errorf("deployment %+v, want a number and a time", deployment)
	}
	if deployment.Cause != "deploy" || deployment.RolledBackFrom != nil {
		t.Errorf("the cause of the Deployment is %q from %v, want a deploy", deployment.Cause, deployment.RolledBackFrom)
	}
	if deployment.SavedStateNumber == nil || *deployment.SavedStateNumber != saved.Number {
		t.Errorf("the deployment names saved state %v, want %d", deployment.SavedStateNumber, saved.Number)
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

func TestDeploySavedState_Repeated_IsRefused(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	ctx := context.Background()
	mustCreateTag(t, "speed", "integer")
	saved := mustSaveState(t, mainProject, "First save")
	if _, err := newDeployService().Deploy(ctx, mainProject, saved.Number); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if _, err := newDeployService().Deploy(ctx, mainProject, saved.Number); !errors.Is(err, application.ErrNothingToDeploy) {
		t.Errorf("a second Deploy of the same saved state: %v, want ErrNothingToDeploy", err)
	}
}

func TestDeploySavedState_Missing_IsNotFound(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	if _, err := newDeployService().Deploy(context.Background(), mainProject, 9); !errors.Is(err, draft.ErrSavedStateNotFound) {
		t.Errorf("Deploy of a missing saved state: %v, want ErrSavedStateNotFound", err)
	}
	if _, err := newDeployService().Deploy(context.Background(), mainProject, 0); !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("Deploy of saved state 0: %v, want invalid input", err)
	}
}

func TestDeploySavedState_AChangedSavedState_IsANewDeployment(t *testing.T) {
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
	if first.SavedStateNumber == nil {
		t.Fatalf("the first deployment: %+v, want its saved state", first)
	}

	changed := testCreateWidgetInput
	changed.Name = "moved-gauge"
	if _, err := scenes.UpdateWidget(ctx, mainProject, scene.ID, widget.ID, widget.Version, changed); err != nil {
		t.Fatalf("UpdateWidget: %v", err)
	}
	second := mustDeployTheDraft(t, mainProject)
	if second.Number != first.Number+1 || second.SavedStateNumber == nil || *second.SavedStateNumber <= *first.SavedStateNumber {
		t.Errorf("the second deployment: %+v, want the next Deployment of a later saved state", second)
	}
}

// TestProjectFileService_Deploy_StatesNoSavedState: a file deployment is a
// Deployment with no saved state, and it publishes no Release — there is none
// (task 85_4, task 85_7).
func TestProjectFileService_Deploy_LeavesNoRelease(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	ctx := context.Background()
	mustCreateTag(t, "speed", "integer")
	file, err := application.NewProjectFileBuilder(contentOver(newScopes())).Build(ctx, mainProject)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	document, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	deployment, err := newProjectFileService().Deploy(ctx, mainProject, document)
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if deployment.Number < 1 || deployment.Cause != "file" || deployment.SavedStateNumber != nil {
		t.Errorf("the deployment of the file: %+v, want a file Deployment with no saved state", deployment)
	}
	current, ok, err := newRuntimeNode().FindCurrentDeployment(ctx, mainProject)
	if err != nil || !ok || current.SavedStateNumber != nil || current.Number != deployment.Number {
		t.Errorf("the running deployment: %+v, %v, %v", current, ok, err)
	}
}

// --- Discard to the last saved state and Revert to the Deployment ---

func TestDiscard_WithoutASavedState_IsRefused(t *testing.T) {
	cleanProjects(t)
	ctx := context.Background()
	created, err := application.NewProjectService(repositories.NewProjectRepositoryPostgres(testDB), repositories.NewDraftRepositoryPostgres(testDB), soleOrganization()).
		CreateProject(ctx, appdto.ProjectInput{Name: "discard-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	if _, err := newDiscardService().Discard(ctx, created.ID); !errors.Is(err, application.ErrNothingToDiscard) {
		t.Errorf("Discard of a project never saved: %v, want ErrNothingToDiscard", err)
	}
}

// TestDiscard_RestoresTheLastSavedState: the Draft is brought back to its last
// saved state — the removed Widget, Scene, Tag, Device and pin come back under
// their IDs, what was added goes, and the Tag states are the Runtime node's,
// kept.
func TestDiscard_RestoresTheLastSavedState(t *testing.T) {
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
// Environment added since the last saved state go, with the pair that joins them
// and the Tag the Device supplies.
func TestDiscard_RemovesADeviceAndAnEnvironmentOfTheDraft(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	cleanImports(t)
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

// TestDiscard_OfAnImportedLibraryRestoresTheImport: the import of a Library
// the Draft dropped comes back, at the saved state the last Save fixed.
func TestDiscard_OfAnImportedLibraryRestoresTheImport(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	cleanImports(t)
	ctx := context.Background()

	_, lib := importNewWidgetType(t, mainProject, appdto.WidgetTypeInput{Name: "gauge"})
	mustDeployTheDraft(t, mainProject)
	if _, err := newLibraryImportService(t).DeleteLibraryImport(ctx, mainProject, lib.ID, draftVersionOf(t, mainProject)); err != nil {
		t.Fatalf("DeleteLibraryImport: %v", err)
	}

	runDiscard(t, mainProject)

	imports, err := newLibraryImportService(t).FindLibraryImports(ctx, mainProject)
	if err != nil || len(imports) != 1 || imports[0].LibraryID != lib.ID || imports[0].SavedStateNumber != 1 {
		t.Errorf("the imports after the Discard: %+v, %v, want %s at saved state 1", imports, err, lib.ID)
	}
}

// TestUnsavedChanges_FollowTheDraftsCleanVersion: the unsaved-changes flag of a
// Project comes from the Draft's own Clean Version marker: a never written
// Draft is clean, a write is unsaved, and a Save, a Discard and a Revert to the
// last saved state's content clean it; a write above the marker, or a Revert to
// another content, is unsaved again (task 74; ADR 0016, amendment 2026-11-10).
func TestUnsavedChanges_FollowTheDraftsCleanVersion(t *testing.T) {
	cleanProjects(t)
	ctx := context.Background()
	projects := application.NewProjectService(repositories.NewProjectRepositoryPostgres(testDB),
		repositories.NewDraftRepositoryPostgres(testDB), soleOrganization())
	drafts := application.NewDraftService(contentOver(newScopes()))
	created, err := projects.CreateProject(ctx, appdto.ProjectInput{Name: "unsaved-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	check := func(step string, want bool) {
		t.Helper()
		found, err := projects.FindProjectByID(ctx, created.ID)
		if err != nil || found.HasUnsavedChanges != want {
			t.Errorf("%s: the project shows %+v, %v; want unsaved %t", step, found, err, want)
		}
		state, err := drafts.State(ctx, created.ID)
		if err != nil || state.HasUnsavedChanges != want {
			t.Errorf("%s: the Draft state is %+v, %v; want unsaved %t", step, state, err, want)
		}
	}
	write := func(name string) {
		t.Helper()
		if _, err := newService().CreateTag(ctx, created.ID, appdto.CreateTagInput{
			Version: draftVersionOf(t, created.ID), Name: name, Type: "integer",
		}); err != nil {
			t.Fatalf("CreateTag %s: %v", name, err)
		}
	}
	scope := draft.ProjectScope(id.NewID(id.IDWithUUID[project.Project](created.ID)))
	refused := func(step string) {
		t.Helper()
		if _, err := newSavedStateService().Save(ctx, scope, draftVersionOf(t, created.ID), step, "Ada"); !errors.Is(err, draft.ErrNothingToSave) {
			t.Errorf("%s: Save %v, want ErrNothingToSave", step, err)
		}
	}

	check("never written", false)
	write("speed")
	check("written", true)
	mustSaveState(t, created.ID, "First")
	check("saved", false)
	refused("saved")

	write("pressure")
	check("written after the save", true)
	if _, err := newDiscardService().Discard(ctx, created.ID); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	check("discarded", false)
	refused("discarded")

	write("flow")
	check("written after the discard", true)
	mustDeployTheDraft(t, created.ID)
	check("deployed", false)
	write("temperature")
	check("written after the deploy", true)
	if _, err := newDiscardService().Revert(ctx, created.ID); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	check("reverted to the last savedState's content", false)

	// A later saved state is not the running Deployment: a Revert to that
	// Deployment is another content, so the Draft stays unsaved.
	write("level")
	mustSaveState(t, created.ID, "Second")
	check("saved after the revert", false)
	if _, err := newDiscardService().Revert(ctx, created.ID); err != nil {
		t.Fatalf("Revert to the running deployment: %v", err)
	}
	check("reverted to another content", true)
}

// TestDiscard_WithAWarningStaysUnsaved: a Discard that could not restore a
// Library Release drops its pin with its Widgets — the content really differs
// from the saved state's — and the Draft stays unsaved (task 74; task 71).
func TestDiscard_WithAWarningStaysUnsaved(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	_, lib := importNewWidgetType(t, mainProject, appdto.WidgetTypeInput{Name: "gauge"})
	mustDeployTheDraft(t, mainProject)
	if found, err := r.projectSvc.FindProjectByID(ctx, mainProject); err != nil || found.HasUnsavedChanges {
		t.Fatalf("before the Discard: %+v, %v; want the deployed Draft clean", found, err)
	}

	// This server does not have the Library the saved state pins any more.
	if _, err := testDB.ExecContext(ctx,
		`UPDATE engineering.libraries SET deleted_at = now() WHERE id = $1`, lib.ID); err != nil {
		t.Fatalf("mark the library deleted: %v", err)
	}
	answer, err := newDiscardService().Discard(ctx, mainProject)
	if err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if len(answer.Warnings) != 1 || answer.Warnings[0].LibraryID != lib.ID {
		t.Fatalf("the Discard's warnings: %+v, want the missing library", answer.Warnings)
	}
	if found, err := r.projectSvc.FindProjectByID(ctx, mainProject); err != nil || !found.HasUnsavedChanges {
		t.Errorf("after a Discard that dropped a pin: %+v, %v; want unsaved changes", found, err)
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
// when a later saved state exists — that is the difference from Discard.
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
