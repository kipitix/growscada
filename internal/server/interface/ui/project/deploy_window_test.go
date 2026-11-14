package project

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// uriLog is a transport that records the requests sent through it with their
// query, which tells a server filter (GET …/commits?label=<id>) from one the
// client makes in memory.
type uriLog struct {
	mu   sync.Mutex
	reqs []string
}

func (l *uriLog) RoundTrip(r *http.Request) (*http.Response, error) {
	l.mu.Lock()
	l.reqs = append(l.reqs, r.Method+" "+r.URL.RequestURI())
	l.mu.Unlock()
	return http.DefaultTransport.RoundTrip(r)
}

// take returns the requests sent so far, and forgets them.
func (l *uriLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	taken := l.reqs
	l.reqs = nil
	return taken
}

// saveState saves the Draft as the next saved state after touching it: a Save
// of a Draft nothing was written to is the server's 409 (ADR 0021).
func saveState(t *testing.T, api *testAPI, title string) apiv0.CommitResponse {
	t.Helper()
	ctx := context.Background()
	createScene(t, api, title)
	state, err := api.DraftState(ctx)
	if err != nil {
		t.Fatalf("draft state: %v", err)
	}
	saved, err := api.SaveCommit(ctx, apiv0.CreateCommitRequest{
		Version: state.Version, Title: title, Author: "Ada"})
	if err != nil {
		t.Fatalf("save commit: %v", err)
	}
	return saved
}

// TestDeployWindow_ListsTheSavedStates: the window shows the saved states, not
// Releases: each row's number, title, author, time and counts, and the two
// actions — Deploy and the download of its ProjectFile (task 85_5).
func TestDeployWindow_ListsTheSavedStates(t *testing.T) {
	f := startProject(t)
	f.run()
	saveState(t, f.api, "First save")
	saveState(t, f.api, "Second save")

	f.p.openDeployDialog(f.p.compoCtx)
	f.run()
	f.noToasts()
	html := app.HTMLString(f.p.Render())
	for _, want := range []string{
		"Deploy a saved state",
		`data-saved-state-deploy-row="2"`,
		`data-saved-state-deploy-row="1"`,
		"Second save",
		"First save",
		"Ada",
		"scenes",
		`data-saved-state-deploy="1"`,
		`data-saved-state-download="1"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the window does not show %q: %s", want, html)
		}
	}
	if strings.Contains(html, "Release") {
		t.Errorf("the window still shows a Release: %s", html)
	}
}

// TestDeployWindow_EmptySaysWhatToDoFirst: a Draft never saved says that a
// saved state is what gets deployed (task 85_5).
func TestDeployWindow_EmptySaysWhatToDoFirst(t *testing.T) {
	f := startProject(t)
	f.run()
	f.p.openDeployDialog(f.p.compoCtx)
	f.run()
	f.noToasts()
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, "Nothing saved yet. Save the Draft first: a saved state is what gets deployed.") {
		t.Errorf("the empty window does not say what to do first: %s", html)
	}
	if !strings.Contains(html, "Save the Draft first: only a saved state is deployed") {
		t.Errorf("the Deploy button does not say what to do first: %s", html)
	}
}

// TestDeployWindow_FilterByLabel: the filter is the server's — GET
// …/commits?label=<id> —, All labels asks for every saved state, and a chosen
// label narrows the window to the ones it marks (task 85_3, task 85_5).
func TestDeployWindow_FilterByLabel(t *testing.T) {
	url, api := newServer(t)
	log := &uriLog{}
	f := mountWith(t, url, api, &http.Client{Transport: log})
	f.run()
	saveState(t, api, "First save")
	second := saveState(t, api, "Second save")
	label, err := api.CreateLabel(context.Background(), "deploy-"+uuid.NewString()[:8], "#22c55e")
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	if _, err := api.AssignLabel(context.Background(), second.Number, label.ID); err != nil {
		t.Fatalf("assign label: %v", err)
	}

	f.p.openDeployDialog(f.p.compoCtx)
	f.run()
	f.noToasts()
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-deploy-label`) || !strings.Contains(html, "All labels") {
		t.Errorf("the window has no label filter: %s", html)
	}
	if !strings.Contains(html, `data-saved-state-deploy-row="1"`) ||
		!strings.Contains(html, `data-saved-state-deploy-row="2"`) {
		t.Errorf("All labels does not list every saved state: %s", html)
	}

	log.take()
	f.p.setDeployLabel(f.p.compoCtx, label.ID)
	f.run()
	f.noToasts()
	html = app.HTMLString(f.p.Render())
	if strings.Contains(html, `data-saved-state-deploy-row="1"`) {
		t.Errorf("the label filter kept a saved state it does not mark: %s", html)
	}
	if !strings.Contains(html, `data-saved-state-deploy-row="2"`) {
		t.Errorf("the label filter dropped the saved state it marks: %s", html)
	}
	asked := false
	for _, req := range log.take() {
		if strings.Contains(req, apiv0.CommitPath+"?"+apiv0.CommitsQueryLabel+"="+label.ID.String()) {
			asked = true
		}
	}
	if !asked {
		t.Errorf("the filter is not the server's: no GET with ?label=%s", label.ID)
	}

	f.p.setDeployLabel(f.p.compoCtx, uuid.Nil)
	f.run()
	f.noToasts()
	html = app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-saved-state-deploy-row="1"`) ||
		!strings.Contains(html, `data-saved-state-deploy-row="2"`) {
		t.Errorf("All labels does not bring every saved state back: %s", html)
	}
}

// TestDeployWindow_DeletedLabelResetsTheFilter: a label deleted elsewhere takes
// a filter that named it back to All labels (task 85_5).
func TestDeployWindow_DeletedLabelResetsTheFilter(t *testing.T) {
	f := startProject(t)
	f.run()
	first := saveState(t, f.api, "First save")
	second := saveState(t, f.api, "Second save")
	label, err := f.api.CreateLabel(context.Background(), "deploy-"+uuid.NewString()[:8], "")
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	if _, err := f.api.AssignLabel(context.Background(), second.Number, label.ID); err != nil {
		t.Fatalf("assign label: %v", err)
	}

	f.p.openDeployDialog(f.p.compoCtx)
	f.run()
	f.p.setDeployLabel(f.p.compoCtx, label.ID)
	f.run()
	f.noToasts()
	if f.p.deployLabel != label.ID {
		t.Fatalf("the filter: %v, want %v", f.p.deployLabel, label.ID)
	}

	if err := f.api.DeleteLabel(context.Background(), label.ID); err != nil {
		t.Fatalf("delete label: %v", err)
	}
	f.event(apiv0.Event{Type: apiv0.EventLabelDeleted})

	if f.p.deployLabel != uuid.Nil {
		t.Errorf("the deleted label is still the filter: %v", f.p.deployLabel)
	}
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-saved-state-deploy-row="`+strconv.Itoa(first.Number)+`"`) ||
		!strings.Contains(html, `data-saved-state-deploy-row="`+strconv.Itoa(second.Number)+`"`) {
		t.Errorf("the window did not fall back to every saved state: %s", html)
	}
}

// TestDeployWindow_NoLabelsHint: an Organization without labels has no filter,
// and the window says how to make one (task 85_5).
func TestDeployWindow_NoLabelsHint(t *testing.T) {
	f := startProject(t)
	f.run()
	saveState(t, f.api, "First save")
	f.p.openDeployDialog(f.p.compoCtx)
	f.run()
	f.noToasts()
	f.p.historyLabels = nil

	html := app.HTMLString(f.p.Render())
	if strings.Contains(html, `data-deploy-label`) {
		t.Errorf("a window without labels offers a filter: %s", html)
	}
	for _, want := range []string{`data-deploy-no-labels`, "No labels yet", `data-deploy-manage-labels`} {
		if !strings.Contains(html, want) {
			t.Errorf("the window does not hold %q: %s", want, html)
		}
	}
}

// failDeployCall fails the request that deploys a saved state: it never reaches
// the server, so the window has to show a network error (task 85_5).
type failDeployCall struct{}

func (failDeployCall) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/deploy") {
		return nil, errors.New("the deploy did not reach the server")
	}
	return http.DefaultTransport.RoundTrip(r)
}

// TestDeployWindow_NetworkErrorIsAToast: a Deploy that never reaches the server
// is the error toast, and the window stops being busy (task 85_5).
func TestDeployWindow_NetworkErrorIsAToast(t *testing.T) {
	url, api := newServer(t)
	saveState(t, api, "First save")
	saveState(t, api, "Second save")
	f := mountWith(t, url, api, &http.Client{Transport: failDeployCall{}})
	f.run()
	f.e.TakeToasts() // the loads of the mount

	f.p.doDeploySavedState(f.p.compoCtx, 2)
	f.run()

	if got := f.oneToast(); got.Title != "Network Error" {
		t.Errorf("the failed Deploy showed %+v, want a network error", got)
	}
	if f.p.deploying {
		t.Errorf("the window stayed busy after the failed Deploy")
	}
}

// TestDeployWindow_ProjectFileDownload: a saved state is downloaded as the
// ProjectFile it fixed (task 85_5).
func TestDeployWindow_ProjectFileDownload(t *testing.T) {
	f := startProject(t)
	f.run()
	saved := saveState(t, f.api, "First save")
	f.p.openDeployDialog(f.p.compoCtx)
	f.run()

	f.p.downloadCommitProjectFile(f.p.compoCtx, saved.Number)
	f.run()
	f.noToasts()
}

// TestDeployWindow_DeploymentCreatedMovesTheMark: a Deploy in another tab moves
// the running mark of the open window without a reload of the page (task 85_5).
func TestDeployWindow_DeploymentCreatedMovesTheMark(t *testing.T) {
	f := startProject(t)
	f.run()
	saveState(t, f.api, "First save")
	second := saveState(t, f.api, "Second save")
	f.p.openDeployDialog(f.p.compoCtx)
	f.run()
	f.noToasts()

	if _, err := f.api.DeployCommit(context.Background(), second.Number); err != nil {
		t.Fatalf("deploy saved state: %v", err)
	}
	f.event(apiv0.Event{Type: apiv0.EventDeploymentCreated})

	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-saved-state-running="`+strconv.Itoa(second.Number)+`"`) {
		t.Errorf("the window did not follow the Deployment: %s", html)
	}
}
