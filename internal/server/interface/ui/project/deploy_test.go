package project

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// deployDraft saves the Draft as a Commit and deploys it: a Deploy takes a
// saved state since task 85_4.
func deployDraft(t *testing.T, api *testAPI) apiv0.DeployResponse {
	t.Helper()
	ctx := context.Background()
	state, err := api.DraftState(ctx)
	if err != nil {
		t.Fatalf("draft state: %v", err)
	}
	commit, err := api.SaveSavedState(ctx, apiv0.CreateSavedStateRequest{Version: state.Version, Title: "Save", Author: "Ada"})
	if err != nil {
		t.Fatalf("save commit: %v", err)
	}
	deployed, err := api.DeploySavedState(ctx, commit.Number)
	if err != nil {
		t.Fatalf("deploy saved state: %v", err)
	}
	return deployed
}

// TestDeployBar_ShowsTheDeploymentAndItsSavedState: the bar starts at "Not
// deployed", and after a Deploy it names the saved state it runs.
func TestDeployBar_ShowsTheDeploymentAndItsSavedState(t *testing.T) {
	f := startProject(t)

	if !f.p.deploymentLoaded || f.p.deployed {
		t.Fatalf("deployment state: loaded %v, deployed %v", f.p.deploymentLoaded, f.p.deployed)
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, "Not deployed") {
		t.Errorf("the bar does not say the project is not deployed: %s", html)
	}

	deployDraft(t, f.api)
	f.p.loadDeploy(f.p.compoCtx)
	f.run()
	f.noToasts()
	if !f.p.deployed || f.p.deployment.Number != 1 || f.p.deployment.SavedStateNumber == nil || *f.p.deployment.SavedStateNumber != 1 {
		t.Fatalf("after Deploy: deployed %v, %+v", f.p.deployed, f.p.deployment)
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, "Saved state #1 · running") {
		t.Errorf("the bar does not show saved state 1 running: %s", html)
	}
}

// TestDeployWindow_DeploysASavedStateAndMovesTheMark: the window marks the
// running saved state and offers every other one, and a Deploy moves the mark
// (task 85_5).
func TestDeployWindow_DeploysASavedStateAndMovesTheMark(t *testing.T) {
	f := startProject(t)
	deployDraft(t, f.api) // saved state 1 is running
	createScene(t, f.api, "Second")
	state, err := f.api.DraftState(context.Background())
	if err != nil {
		t.Fatalf("draft state: %v", err)
	}
	if _, err := f.api.SaveSavedState(context.Background(), apiv0.CreateSavedStateRequest{
		Version: state.Version, Title: "Second save", Author: "Ada"}); err != nil {
		t.Fatalf("save commit: %v", err)
	}

	f.p.openDeployDialog(f.p.compoCtx)
	f.run()
	f.p.loadDeploy(f.p.compoCtx)
	f.run()
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-saved-state-running="1"`) {
		t.Errorf("saved state 1 is not marked running: %s", html)
	}
	if !strings.Contains(html, `data-saved-state-deploy="2"`) {
		t.Errorf("saved state 2 is not offered for Deploy: %s", html)
	}

	f.p.doDeploySavedState(f.p.compoCtx, 2)
	f.run()
	if got := f.oneToast(); !strings.Contains(got.Title, "Saved state 2 deployed") ||
		!strings.Contains(got.Detail, "Deployment 2 is running") {
		t.Errorf("the Deploy showed %+v, want its confirmation", got)
	}
	if f.p.deployment.SavedStateNumber == nil || *f.p.deployment.SavedStateNumber != 2 {
		t.Fatalf("the running saved state after the Deploy: %+v", f.p.deployment)
	}
	html = app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-saved-state-running="2"`) || strings.Contains(html, `data-saved-state-running="1"`) {
		t.Errorf("the running mark did not move to saved state 2: %s", html)
	}
}

// TestDeployWindow_DeploymentWithoutSavedState: a Deployment from a file, or
// one made before saved states were deployed, is shown as one with no saved
// state and a saved state is offered (task 85_4, task 85_5).
func TestDeployWindow_DeploymentWithoutSavedState(t *testing.T) {
	f := startProject(t)
	f.p.deployed = true
	f.p.deploymentLoaded = true
	f.p.deployment = apiv0.DeploymentResponse{Number: 4, DeployedAt: "2026-11-04T10:00:00Z"}
	f.p.deployCommits = []apiv0.SavedStateResponse{{Number: 1, Title: "First"}}
	f.p.deployCommitsLoaded = true
	f.p.deployDialog = true

	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, "Running Deployment 4 (no saved state)") {
		t.Errorf("the window does not say the Deployment has no saved state: %s", html)
	}
	if !strings.Contains(html, "Deployment 4 · no saved state") {
		t.Errorf("the bar does not say the running Deployment has no saved state: %s", html)
	}
	if !strings.Contains(html, `data-saved-state-deploy="1"`) {
		t.Errorf("the window does not offer the saved state for Deploy: %s", html)
	}
}

// TestDeploySavedState_ServerRefusalIsAToast: another user deploys the saved
// state the window shows as deployable. The server's 409 must reach the user as
// a toast (task 85_5).
func TestDeploySavedState_ServerRefusalIsAToast(t *testing.T) {
	f := startProject(t)
	deployDraft(t, f.api)
	f.p.loadDeploy(f.p.compoCtx)
	f.p.openDeployDialog(f.p.compoCtx)
	f.run()

	f.p.doDeploySavedState(f.p.compoCtx, 1)
	f.run()

	if got := f.oneToast(); !strings.Contains(got.Title, "running already") {
		t.Errorf("the refusal is not the running-already toast: %+v", got)
	}
}

// TestLoadSavedState_ResetsToTheLastSavedState: a saved state becomes the
// Draft, unsaved changes and all, and the indicator turns off when it is the
// last one; the panel carries the icons of Save, Undo, Redo and History, and
// the two removed resets are gone (task 72_6; task 84; ADR 0021).
func TestLoadSavedState_ResetsToTheLastSavedState(t *testing.T) {
	f := startProject(t)
	f.run()
	f.p.createScene(f.p.compoCtx)
	f.run()
	f.noToasts()
	f.p.saveAuthor = "Ada"
	f.p.saveTitle = "First save"
	f.p.saveCommit(f.p.compoCtx)
	f.run()
	f.e.TakeToasts() // the Save's own confirmation

	f.p.createScene(f.p.compoCtx)
	f.run()
	f.noToasts()
	if !f.p.hasUnsavedChanges() {
		t.Fatal("the change after the saved state is not unsaved")
	}
	html := app.HTMLString(f.p.Render())
	for _, icon := range []string{"save", "undo", "redo", "history", "deploy"} {
		if !strings.Contains(html, `data-icon="`+icon+`"`) {
			t.Errorf("the panel has no %s action: %s", icon, html)
		}
	}
	if strings.Contains(html, "Discard changes") || strings.Contains(html, "Revert to deployed") {
		t.Errorf("the panel still offers the removed resets: %s", html)
	}

	f.p.doLoadSavedState(f.p.compoCtx, 1)
	f.run()
	if got := f.oneToast(); !strings.Contains(got.Title, "Saved state 1 loaded") {
		t.Errorf("the load confirmation: %+v", got)
	}
	if f.p.hasUnsavedChanges() {
		t.Errorf("the Draft still reports unsaved changes after loading its last saved state")
	}
	scenes, err := f.api.ListScenes(context.Background())
	if err != nil {
		t.Fatalf("server scenes: %v", err)
	}
	if len(scenes) != 1 {
		t.Errorf("the Draft after the load has %d scenes, want the saved state's 1", len(scenes))
	}
	if len(f.p.scenes) != 1 {
		t.Errorf("the shown Draft has %d scenes after the load, want 1", len(f.p.scenes))
	}
}

// TestLoadSavedState_RestoresTheContentShown: a load replaces the whole Draft,
// so the Scenes and the Widgets of the selected one are read again — the
// Version of the Draft was answered by the load itself and says nothing of the
// content it restored (task 72_6; task 84; ADR 0021).
func TestLoadSavedState_RestoresTheContentShown(t *testing.T) {
	f := startProject(t)
	f.run()
	sceneID := createScene(t, f.api, "First")
	typeID := createWidgetType(t, f.api, "Lamp")
	widgetID := createWidget(t, f.api, sceneID, typeID, "W")
	f.p.loadScenes(f.p.compoCtx)
	f.p.loadWidgets(f.p.compoCtx)
	f.run()
	f.noToasts()
	f.p.saveAuthor, f.p.saveTitle = "Ada", "First save"
	f.p.saveCommit(f.p.compoCtx)
	f.run()
	f.e.TakeToasts() // the Save's own confirmation

	// Another tab renames the Scene and drops its Widget: the Draft this tab
	// shows no longer matches its Commit.
	sc := f.scene(sceneID)
	if _, err := f.api.UpdateScene(context.Background(), sceneID, apiv0.UpdateSceneRequest{
		Name: "Second", Width: sc.Width, Height: sc.Height, Version: sc.Version,
	}); err != nil {
		t.Fatalf("rename scene: %v", err)
	}
	if _, err := f.api.DeleteWidget(context.Background(), sceneID, widgetID, 0); err != nil {
		t.Fatalf("delete widget: %v", err)
	}
	f.p.loadScenes(f.p.compoCtx)
	f.p.loadWidgets(f.p.compoCtx)
	f.run()
	if len(f.p.scenes) != 1 || f.p.scenes[0].Name != "Second" || len(f.p.widgets) != 0 {
		t.Fatalf("the shown Draft before the Discard: %+v, widgets %+v", f.p.scenes, f.p.widgets)
	}

	f.p.doLoadSavedState(f.p.compoCtx, 1)
	f.run()
	if got := f.oneToast(); !strings.Contains(got.Title, "Saved state 1 loaded") {
		t.Errorf("the load confirmation: %+v", got)
	}
	if sc := f.scene(sceneID); sc.Name != "First" {
		t.Fatalf("server scene after the Discard: %+v, want the Commit's", sc)
	}
	if len(f.p.scenes) != 1 || f.p.scenes[0].Name != "First" {
		t.Errorf("the shown scenes after the Discard: %+v, want the restored one", f.p.scenes)
	}
	widgets, err := f.api.ListWidgets(context.Background(), sceneID)
	if err != nil {
		t.Fatalf("server widgets: %v", err)
	}
	if len(widgets) != 1 {
		t.Fatalf("server widgets after the Discard: %+v, want the Commit's one", widgets)
	}
	if len(f.p.widgets) != 1 {
		t.Errorf("the shown widgets after the Discard: %+v, want the restored one", f.p.widgets)
	}
}

// TestLoadSavedState_LaterSceneListFollowsTheVersion: only the list the
// replacement itself asked for is the truth whatever Version it states; a list
// requested after it is an ordinary one again, so a stale answer does not put
// the Scenes shown back (task 72_6; task 84; ADR 0016).
func TestLoadSavedState_LaterSceneListFollowsTheVersion(t *testing.T) {
	f := startProject(t)
	f.run()
	sceneID := createScene(t, f.api, "First")
	f.p.loadScenes(f.p.compoCtx)
	f.run()
	f.noToasts()
	f.p.saveAuthor, f.p.saveTitle = "Ada", "First save"
	f.p.saveCommit(f.p.compoCtx)
	f.run()
	f.e.TakeToasts() // the Save's own confirmation

	f.p.doLoadSavedState(f.p.compoCtx, 1)
	f.run()
	if got := f.oneToast(); !strings.Contains(got.Title, "Saved state 1 loaded") {
		t.Errorf("the load confirmation: %+v", got)
	}
	if len(f.p.scenes) != 1 || f.p.scenes[0].Name != "First" {
		t.Fatalf("the shown Draft after the load: %+v, want the restored one", f.p.scenes)
	}

	// A list read now but answered after a rename that follows: it is stale.
	stale, mark := f.scene(sceneID), f.p.markDraft()
	f.p.startEditingScene(sceneID, "Renamed")
	f.p.commitSceneEdit(f.p.compoCtx)
	f.run()
	f.noToasts()
	f.p.setScenes(f.p.compoCtx, mark, f.p.contentReplacements, []apiv0.SceneResponse{stale})
	f.run()

	if sc, _ := f.p.findScene(sceneID); sc.Name != "Renamed" {
		t.Errorf("the shown scene is %q, want the saved rename: a stale list replaced it", sc.Name)
	}
}

// TestLoadSavedState_WithoutThatSavedStateIsAToast: a number the Draft does
// not have is the server's 404 as a toast (task 84).
func TestLoadSavedState_WithoutThatSavedStateIsAToast(t *testing.T) {
	f := startProject(t)
	f.run()
	f.p.doLoadSavedState(f.p.compoCtx, 1)
	f.run()
	if got := f.oneToast(); got.Status != http.StatusNotFound {
		t.Errorf("loading a saved state the project does not have: toast %+v, want a 404", got)
	}
}

// failDeploymentRead fails the GET of the Deployment alone: the rest of the
// mode works, so a test sees what the Deploy window says when the running one
// cannot be checked.
type failDeploymentRead struct{}

func (failDeploymentRead) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/deployment") {
		return nil, errors.New("the deployment cannot be read")
	}
	return http.DefaultTransport.RoundTrip(r)
}

// TestDeployWindow_UnknownDeployment: a failed read of the Deployment is not
// "not deployed": the window says the running saved state cannot be checked,
// and the list itself stays readable because it is the Engineering node's
// (task 72_4; task 85_5).
func TestDeployWindow_UnknownDeployment(t *testing.T) {
	url, api := newServer(t)
	deployDraft(t, api)
	f := mountWith(t, url, api, &http.Client{Transport: failDeploymentRead{}})
	f.run()
	f.e.TakeToasts() // the failed read of the Deployment is the toast of its load

	if !f.p.deploymentFailed || f.p.deploymentLoaded {
		t.Fatalf("the failed read: failed %v, loaded %v", f.p.deploymentFailed, f.p.deploymentLoaded)
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, "Deployment unknown") {
		t.Errorf("the bar passes an unknown Deployment off as not deployed: %s", html)
	}

	f.p.openDeployDialog(f.p.compoCtx)
	f.run()
	f.e.TakeToasts()
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, "data-saved-state-running-unknown") {
		t.Errorf("the window does not say the running saved state cannot be checked: %s", html)
	}
	if !strings.Contains(html, `data-saved-state-deploy-row="1"`) {
		t.Errorf("the list is not shown without the Deployment: %s", html)
	}
}

// TestDiscard_AnotherTabsDiscardClearsTheBadge: a Discard performed elsewhere
// arrives as the content events it wrote, and the badge follows the Draft's own
// state — the flag the server answers —, not anything this tab remembers
// (task 74; ADR 0016, amendment 2026-11-10).
func TestDiscard_AnotherTabsDiscardClearsTheBadge(t *testing.T) {
	f := startProject(t)
	f.run()
	f.p.saveAuthor, f.p.saveTitle = "Ada", "First save"
	f.p.saveCommit(f.p.compoCtx)
	f.run()
	f.e.TakeToasts() // the Save's own confirmation

	f.p.createScene(f.p.compoCtx)
	f.run()
	if !f.p.hasUnsavedChanges() {
		t.Fatal("the change after the Commit is not unsaved")
	}

	if _, err := f.api.Discard(context.Background()); err != nil {
		t.Fatalf("another tab's Discard: %v", err)
	}
	// The events of the Discard reach this tab: the Scene it removed is one.
	f.event(apiv0.Event{Type: apiv0.EventSceneDeleted, ID: uuid.NewString()})
	if f.p.hasUnsavedChanges() {
		t.Errorf("the badge stays after another tab's Discard")
	}
	if html := app.HTMLString(f.p.Render()); strings.Contains(html, "data-commit-unsaved") {
		t.Errorf("the header still marks unsaved changes: %s", html)
	}
}
