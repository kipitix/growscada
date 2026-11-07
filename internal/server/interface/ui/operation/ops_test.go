package operation

import (
	"context"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	recordv0 "github.com/kipitix/growscada/contract/record/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/player"
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

// fakeSource is a player.Source a test drives: it reports the callbacks it is
// opened with, so the test feeds the entries the server's Journal holds.
type fakeSource struct {
	events player.Events
	from   int64
	closed bool
}

func (s *fakeSource) Open(_ app.Context, from int64, events player.Events) {
	s.from, s.events, s.closed = from, events, false
	if events.Opened != nil {
		events.Opened()
	}
}

func (s *fakeSource) Close() { s.closed = true }

func (s *fakeSource) feed(entries ...recordv0.Entry) {
	for _, entry := range entries {
		s.events.Entry(entry)
	}
}

// countingTransport counts the requests the Operation makes, by path suffix.
type countingTransport struct {
	base http.RoundTripper

	mu    sync.Mutex
	paths map[string]int
}

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.mu.Lock()
	if c.paths == nil {
		c.paths = make(map[string]int)
	}
	c.paths[r.URL.Path]++
	c.mu.Unlock()
	return c.base.RoundTrip(r)
}

func (c *countingTransport) count(suffix string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := 0
	for path, n := range c.paths {
		if strings.HasSuffix(path, suffix) {
			total += n
		}
	}
	return total
}

// countingClient makes an Operation's requests go through counter.
func countingClient(base *http.Client, counter *countingTransport) *http.Client {
	counter.base = base.Transport
	if counter.base == nil {
		counter.base = http.DefaultTransport
	}
	return &http.Client{Transport: counter}
}

type fixture struct {
	t      *testing.T
	o      *Operation
	e      *uitest.Engine
	source *fakeSource
}

func mount(t *testing.T, url string, httpClient *http.Client) *fixture {
	t.Helper()
	source := &fakeSource{}
	o := NewOperation(apiclient.New(url, httpClient))
	o.newSource = func(*apiclient.ProjectClient) player.Source { return source }
	o.SetProject(servertest.MainProject)
	f := &fixture{t: t, o: o, source: source}
	f.e = uitest.Load(t, mountOnHost{o})
	f.run()
	return f
}

func (f *fixture) run() { f.e.ConsumeAll() }

// feed delivers entries to the Player on the UI goroutine and lets it finish
// what they start (the Revision document read included).
func (f *fixture) feed(entries ...recordv0.Entry) {
	f.o.compoCtx.Dispatch(func(app.Context) { f.source.feed(entries...) })
	f.run()
}

// pump delivers the entries the Project's Journal holds after the position
// the Player has consumed.
func (f *fixture) pump(project *apiclient.ProjectClient) {
	f.t.Helper()
	entries, err := project.Journal(context.Background(), f.o.player.Position(), 100)
	if err != nil {
		f.t.Fatalf("read the journal: %v", err)
	}
	f.feed(entries...)
}

// TestOperation_FirstDeployArrivesFromTheJournal: a Project without a
// Revision shows the empty state, and its first Deploy arrives as a Journal
// entry — no reload of /revision or /tag-states, the values then follow
// tag_state entries.
func TestOperation_FirstDeployArrivesFromTheJournal(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	api := apiclient.New(srv.URL, srv.Client()).WithClientKey(servertest.ClientKey.String())
	servertest.EnsureEditLock(t, srv, servertest.MainProject)
	project := api.Project(servertest.MainProject)
	ctx := context.Background()

	tag, err := project.CreateTag(ctx, apiv0.CreateTagRequest{Name: "speed", Type: "integer"})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	scene, err := project.CreateScene(ctx, apiv0.CreateSceneRequest{Name: "Overview", Width: 800, Height: 600})
	if err != nil {
		t.Fatalf("create scene: %v", err)
	}

	counter := &countingTransport{}
	f := mount(t, srv.URL, countingClient(srv.Client(), counter))
	if f.o.player.Deployed() {
		t.Fatal("the project runs no Revision, but Operation is deployed")
	}
	html := app.HTMLString(f.o.Render())
	if !strings.Contains(html, "Project is not deployed") || !strings.Contains(html, "data-operation-not-deployed") {
		t.Errorf("Operation does not show the empty state: %s", html)
	}

	if _, err := project.Deploy(ctx); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	f.pump(project)
	view := f.o.player.View()
	if !view.Deployed() || len(view.Scenes) != 1 || view.Scenes[0].ID != scene.ID {
		t.Fatalf("the revision entry did not switch the configuration: %+v", view)
	}
	if tagView := view.Tags[tag.ID]; tagView.Value != "0" || tagView.Quality != "bad" || tagView.Seq != 0 {
		t.Errorf("an unwritten Tag = %+v, want its type's zero value at bad", tagView)
	}
	html = app.HTMLString(f.o.Render())
	if !strings.Contains(html, "Overview") {
		t.Errorf("Operation does not render the Revision's Scene: %s", html)
	}

	if _, err := project.SetTagValue(ctx, tag.ID, "42", "good"); err != nil {
		t.Fatalf("set tag value: %v", err)
	}
	f.pump(project)
	if tagView := f.o.player.View().Tags[tag.ID]; tagView.Value != "42" || tagView.Quality != "good" {
		t.Errorf("the tag_state entry did not move the value: %+v", tagView)
	}

	// The Player reads the starting state and the Revision documents alone:
	// Operation no longer reads /revision or /tag-states (ADR 0020).
	if got := counter.count("/revision"); got != 0 {
		t.Errorf("Operation read /revision %d times", got)
	}
	if got := counter.count("/tag-states"); got != 0 {
		t.Errorf("Operation read /tag-states %d times", got)
	}
	if got := counter.count("/operation"); got == 0 {
		t.Error("Operation never read the starting state")
	}
}

// TestOperation_SecondDeployAndRollbackSwitchTheConfiguration: a Draft change
// alone is invisible; a Deploy makes a new Revision entry, which swaps the
// configuration, and a Rollback does the same with a new number.
func TestOperation_SecondDeployAndRollbackSwitchTheConfiguration(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	api := apiclient.New(srv.URL, srv.Client()).WithClientKey(servertest.ClientKey.String())
	servertest.EnsureEditLock(t, srv, servertest.MainProject)
	project := api.Project(servertest.MainProject)
	ctx := context.Background()

	if _, err := project.CreateScene(ctx, apiv0.CreateSceneRequest{Name: "First", Width: 800, Height: 600}); err != nil {
		t.Fatalf("create scene: %v", err)
	}
	if _, err := project.Deploy(ctx); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	f := mount(t, srv.URL, srv.Client())
	if view := f.o.player.View(); len(view.Scenes) != 1 || view.Scenes[0].Name != "First" {
		t.Fatalf("the starting state = %+v", view.Scenes)
	}

	// A Draft change reaches Operation only through a Deploy.
	if _, err := project.CreateScene(ctx, apiv0.CreateSceneRequest{Name: "Second", Width: 800, Height: 600}); err != nil {
		t.Fatalf("create second scene: %v", err)
	}
	if view := f.o.player.View(); len(view.Scenes) != 1 {
		t.Fatalf("a Draft Scene reached Operation: %+v", view.Scenes)
	}

	if _, err := project.Deploy(ctx); err != nil {
		t.Fatalf("second deploy: %v", err)
	}
	f.pump(project)
	view := f.o.player.View()
	if len(view.Scenes) != 2 || view.Revision.Number != 2 {
		t.Fatalf("the second Deploy did not swap the configuration: #%d, %+v", view.Revision.Number, view.Scenes)
	}

	if _, err := project.Rollback(ctx, 1); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	f.pump(project)
	view = f.o.player.View()
	if view.Revision.Number != 3 || view.Revision.Cause != apiv0.RevisionCauseRollback {
		t.Fatalf("revision = #%d %q, want #3 rolled back", view.Revision.Number, view.Revision.Cause)
	}
	if len(view.Scenes) != 1 || view.Scenes[0].Name != "First" {
		t.Errorf("the Rollback did not restore the first configuration: %+v", view.Scenes)
	}
}
