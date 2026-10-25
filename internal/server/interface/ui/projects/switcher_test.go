package projects

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
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

// mountOnHost lets go-app mount a Switcher on the host (see uitest).
type mountOnHost struct{ *Switcher }

func (m mountOnHost) OnPreRender(ctx app.Context) { m.OnMount(ctx) }

// fixture is a mounted Switcher, the selections it announced, and a second
// client of the same server.
type fixture struct {
	t         *testing.T
	s         *Switcher
	e         *uitest.Engine
	api       *apiclient.Client
	announced []uuid.UUID
}

func start(t *testing.T, arrange func(api *apiclient.Client)) *fixture {
	t.Helper()
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	api := apiclient.New(srv.URL, srv.Client())
	if arrange != nil {
		arrange(api)
	}
	f := &fixture{t: t, api: api}
	f.s = NewSwitcher(apiclient.New(srv.URL, http.DefaultClient), func(id uuid.UUID) { f.announced = append(f.announced, id) })
	f.e = uitest.Load(t, mountOnHost{f.s})
	return f
}

func (f *fixture) run() { f.e.ConsumeAll() }

func (f *fixture) createProject(name string) uuid.UUID {
	f.t.Helper()
	created, err := f.api.CreateProject(context.Background(), apiv0.CreateProjectRequest{Name: name})
	if err != nil {
		f.t.Fatalf("create project: %v", err)
	}
	return created.ID
}

// lastAnnounced fails unless the switcher announced want last.
func (f *fixture) lastAnnounced(want uuid.UUID) {
	f.t.Helper()
	if len(f.announced) == 0 || f.announced[len(f.announced)-1] != want || f.s.Selected() != want {
		f.t.Errorf("announced %v, selected %v; want %v", f.announced, f.s.Selected(), want)
	}
}

func TestSwitcher_SelectsTheFirstProjectOnceListed(t *testing.T) {
	f := start(t, nil)

	f.lastAnnounced(servertest.MainProject)
	if len(f.announced) != 1 || len(f.s.Projects()) != 1 {
		t.Errorf("announced %v of %+v: want Main once", f.announced, f.s.Projects())
	}
}

func TestSwitcher_SelectAndCreate(t *testing.T) {
	f := start(t, nil)
	line := f.createProject("Line 1")
	f.s.load(f.s.compoCtx)
	f.run()

	f.s.Select(f.s.compoCtx, line)
	f.lastAnnounced(line)

	f.s.Create(f.s.compoCtx, "Line 2")
	f.run()
	created, ok, err := f.api.FindProjectByName(context.Background(), "Line 2")
	if err != nil || !ok {
		t.Fatalf("Line 2 on the server: %v, %v", ok, err)
	}
	f.lastAnnounced(created.ID)
	if n := len(f.s.Projects()); n != 3 {
		t.Errorf("listed %d projects, want 3", n)
	}

	f.s.Create(f.s.compoCtx, "Line 2") // taken
	f.run()
	if toasts := f.e.TakeToasts(); len(toasts) != 1 || toasts[0].Status != http.StatusConflict {
		t.Errorf("toasts: %+v, want one 409", toasts)
	}
}

func TestSwitcher_Rename(t *testing.T) {
	f := start(t, nil)

	f.s.Rename(f.s.compoCtx, "Plant")
	f.run()

	got, err := f.api.GetProject(context.Background(), servertest.MainProject)
	if err != nil || got.Name != "Plant" {
		t.Fatalf("server project: %+v, %v", got, err)
	}
	if p := f.s.Projects(); len(p) != 1 || p[0].Name != "Plant" {
		t.Errorf("listed %+v, want the new name", p)
	}
	if toasts := f.e.TakeToasts(); len(toasts) != 0 {
		t.Errorf("toasts: %+v", toasts)
	}
}

func TestSwitcher_DeleteSelected_SelectsTheFirstLeft(t *testing.T) {
	var line uuid.UUID
	f := start(t, func(api *apiclient.Client) {
		created, err := api.CreateProject(context.Background(), apiv0.CreateProjectRequest{Name: "Line 1"})
		if err != nil {
			t.Fatalf("create project: %v", err)
		}
		line = created.ID
	})
	f.s.Select(f.s.compoCtx, line)

	f.s.Delete(f.s.compoCtx)
	f.run()

	f.lastAnnounced(servertest.MainProject)
	if _, err := f.api.GetProject(context.Background(), line); err == nil {
		t.Error("the project is still on the server")
	}
}

func TestSwitcher_SelectedDeletedElsewhere_SelectsTheFirstLeft(t *testing.T) {
	f := start(t, nil)
	line := f.createProject("Line 1")
	f.s.load(f.s.compoCtx)
	f.run()
	f.s.Select(f.s.compoCtx, line)

	if _, err := f.api.DeleteProject(context.Background(), line); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	f.s.compoCtx.NewActionWithValue(eventlog.ActionServerEvent, apiv0.Event{Type: apiv0.EventProjectDeleted, ID: line.String()})
	f.run()

	f.lastAnnounced(servertest.MainProject)
}

func TestSwitcher_NoProjectLeft_SelectsNone(t *testing.T) {
	f := start(t, nil)

	f.s.Delete(f.s.compoCtx)
	f.run()

	f.lastAnnounced(uuid.Nil)
	if html := app.HTMLString(f.s.Render()); !strings.Contains(html, "— no project —") {
		t.Errorf("the switcher does not say there is no project: %s", html)
	}
}
