package operation

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
	"github.com/kipitix/growscada/internal/server/interface/ui/uitest"
	"github.com/kipitix/growscada/internal/server/servertest"
)

var postgres servertest.Postgres

func TestMain(m *testing.M) {
	code := m.Run()
	postgres.Close()
	os.Exit(code)
}

// mountOnHost lets go-app mount an Operation on the host (see uitest).
type mountOnHost struct{ *Operation }

func (m mountOnHost) OnPreRender(ctx app.Context) { m.OnMount(ctx) }

type fixture struct {
	t *testing.T
	o *Operation
	e *uitest.Engine
}

func mount(t *testing.T, url string, httpClient *http.Client) *fixture {
	t.Helper()
	o := NewOperation(apiclient.New(url, httpClient))
	o.SetProject(servertest.MainProject)
	f := &fixture{t: t, o: o}
	f.e = uitest.Load(t, mountOnHost{o})
	f.run()
	return f
}

func (f *fixture) run() { f.e.ConsumeAll() }

func (f *fixture) event(ev apiv0.Event) {
	f.o.compoCtx.NewActionWithValue(eventlog.ActionServerEvent, ev)
	f.run()
}

// TestNotDeployed_ShowsTheEmptyState: without a Revision Operation shows that
// the Project is not deployed, not the Draft.
func TestNotDeployed_ShowsTheEmptyState(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	f := mount(t, srv.URL, srv.Client())

	if f.o.deployed {
		t.Fatalf("the project runs no Revision, but Operation is deployed")
	}
	html := app.HTMLString(f.o.Render())
	if !strings.Contains(html, "Project is not deployed") || !strings.Contains(html, "data-operation-not-deployed") {
		t.Errorf("Operation does not show the empty state: %s", html)
	}
}

// TestRevision_ShowsScenesAndTagStates: Operation renders the Revision's
// Scenes, its WidgetTypes and the Tag states snapshot.
func TestRevision_ShowsScenesAndTagStates(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	api := apiclient.New(srv.URL, srv.Client())
	project := api.Project(servertest.MainProject)
	ctx := context.Background()

	tag, err := project.CreateTag(ctx, apiv0.CreateTagRequest{Name: "speed", Type: "integer"})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if _, err := project.CreateScene(ctx, apiv0.CreateSceneRequest{Name: "Overview", Width: 800, Height: 600}); err != nil {
		t.Fatalf("create scene: %v", err)
	}
	if _, err := project.Deploy(ctx); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if _, err := project.SetTagValue(ctx, tag.ID, "42", "good"); err != nil {
		t.Fatalf("set tag value: %v", err)
	}

	f := mount(t, srv.URL, srv.Client())
	if !f.o.deployed || len(f.o.scenes) != 1 || f.o.scenes[0].Name != "Overview" {
		t.Fatalf("deployed %v, scenes %+v", f.o.deployed, f.o.scenes)
	}
	state, ok := f.o.tags.Find(tag.ID)
	if !ok || state.Value != "42" || state.Quality != "good" {
		t.Errorf("tag state: %+v, %v; want the snapshot's 42 good", state, ok)
	}
	html := app.HTMLString(f.o.Render())
	if !strings.Contains(html, "Overview") {
		t.Errorf("Operation does not render the Revision's Scene: %s", html)
	}
}

// TestRevision_DraftChangesAreInvisible: a content event of the Draft changes
// nothing; only revision_created reloads the running Revision.
func TestRevision_DraftChangesAreInvisible(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	api := apiclient.New(srv.URL, srv.Client())
	project := api.Project(servertest.MainProject)
	ctx := context.Background()

	if _, err := project.CreateScene(ctx, apiv0.CreateSceneRequest{Name: "First", Width: 800, Height: 600}); err != nil {
		t.Fatalf("create scene: %v", err)
	}
	deployed, err := project.Deploy(ctx)
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}

	f := mount(t, srv.URL, srv.Client())
	added, err := project.CreateScene(ctx, apiv0.CreateSceneRequest{Name: "Second", Width: 800, Height: 600})
	if err != nil {
		t.Fatalf("create second scene: %v", err)
	}

	f.event(apiv0.Event{Type: apiv0.EventSceneCreated, ID: added.ID.String()})
	if len(f.o.scenes) != 1 {
		t.Fatalf("a Draft Scene reached Operation: %+v", f.o.scenes)
	}

	revision, err := project.Deploy(ctx)
	if err != nil {
		t.Fatalf("second deploy: %v", err)
	}
	if revision.Number != deployed.Number+1 {
		t.Fatalf("second Deploy numbered %d, want %d", revision.Number, deployed.Number+1)
	}
	f.event(apiv0.Event{Type: apiv0.EventRevisionCreated, Revision: revision.Number, DeployedAt: revision.DeployedAt})
	if len(f.o.scenes) != 2 {
		t.Errorf("revision_created did not reload the Revision: %+v", f.o.scenes)
	}
}
