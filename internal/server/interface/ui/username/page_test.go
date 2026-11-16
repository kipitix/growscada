package username

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// storageProbe holds the engine's LocalStorage: mounted before the page, it
// lets a test set the name a previous session left and read the one this one
// writes.
type storageProbe struct {
	app.Compo
	ctx app.Context
}

func (p *storageProbe) OnPreRender(ctx app.Context) { p.ctx = ctx }
func (p *storageProbe) Render() app.UI              { return app.Div() }

// mountOnHost lets go-app mount the page on the host (see uitest).
type mountOnHost struct{ *Page }

func (m mountOnHost) OnPreRender(ctx app.Context) { m.OnMount(ctx) }

type fixture struct {
	t      *testing.T
	p      *Page
	engine app.TestEngine
	probe  *storageProbe
}

func start(t *testing.T, stored string) *fixture {
	t.Helper()
	engine := app.NewTestEngine()
	probe := &storageProbe{}
	if err := engine.Load(probe); err != nil {
		t.Fatalf("load the storage probe: %v", err)
	}
	engine.ConsumeAll()
	if stored != "" {
		if err := probe.ctx.LocalStorage().Set(uiutil.AuthorKey, stored); err != nil {
			t.Fatalf("store the name: %v", err)
		}
	}
	p := New()
	if err := engine.Load(mountOnHost{p}); err != nil {
		t.Fatalf("load the page: %v", err)
	}
	engine.ConsumeAll()
	return &fixture{t: t, p: p, engine: engine, probe: probe}
}

func (f *fixture) html() string { return app.HTMLString(f.p.Render()) }

func (f *fixture) stored() string {
	f.t.Helper()
	var name string
	if err := f.probe.ctx.LocalStorage().Get(uiutil.AuthorKey, &name); err != nil {
		f.t.Fatalf("read the stored name: %v", err)
	}
	return name
}

// TestPage_ReadsTheStoredName: the page opens on the name a previous session
// left (task 90).
func TestPage_ReadsTheStoredName(t *testing.T) {
	f := start(t, "Ada")
	if f.p.name != "Ada" {
		t.Errorf("the field holds %q, want the stored name", f.p.name)
	}
	if html := f.html(); !strings.Contains(html, `data-username-page`) ||
		!strings.Contains(html, `data-panel-back`) || !strings.Contains(html, "User name") {
		t.Errorf("the page lacks its title: %s", html)
	}
}

// TestPage_SavesTheTrimmedName: Save writes the trimmed name, shows its toast
// and tells the modes through ActionChanged (task 90).
func TestPage_SavesTheTrimmedName(t *testing.T) {
	f := start(t, "Ada")
	f.p.name = "  Grace  "
	f.p.save(f.probe.ctx)
	f.engine.ConsumeAll()

	if got := f.stored(); got != "Grace" {
		t.Errorf("the stored name is %q, want the trimmed one", got)
	}
	if f.p.name != "Grace" || f.p.error != "" {
		t.Errorf("the field after Save: %q %q", f.p.name, f.p.error)
	}
}

// TestPage_RefusesAnEmptyName: an empty field saves nothing and says why
// (task 90).
func TestPage_RefusesAnEmptyName(t *testing.T) {
	f := start(t, "Ada")
	f.p.name = "   "
	f.p.save(f.probe.ctx)
	f.engine.ConsumeAll()

	if f.p.error == "" || !strings.Contains(f.p.error, "required") {
		t.Errorf("the refusal is %q", f.p.error)
	}
	if got := f.stored(); got != "Ada" {
		t.Errorf("the stored name moved to %q", got)
	}
	if html := f.html(); !strings.Contains(html, `data-username-error`) {
		t.Errorf("the refusal is not shown: %s", html)
	}
}
