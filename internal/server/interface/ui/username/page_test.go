package username

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/uitest"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// mountWithName seeds the LocalStorage a previous session left and mounts the
// page (see uitest).
type mountWithName struct {
	*Page
	stored string
}

func (m mountWithName) OnPreRender(ctx app.Context) {
	if m.stored != "" {
		if err := ctx.LocalStorage().Set(uiutil.AuthorKey, m.stored); err != nil {
			panic(err)
		}
	}
	m.OnMount(ctx)
}

type fixture struct {
	t *testing.T
	p *Page
	e *uitest.Engine
}

func start(t *testing.T, stored string) *fixture {
	t.Helper()
	p := New()
	e := uitest.LoadWatching(t, mountWithName{Page: p, stored: stored}, ActionChanged)
	return &fixture{t: t, p: p, e: e}
}

func (f *fixture) html() string { return app.HTMLString(f.p.Render()) }

func (f *fixture) stored() string {
	f.t.Helper()
	var name string
	if err := f.p.compoCtx.LocalStorage().Get(uiutil.AuthorKey, &name); err != nil {
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
	f.p.save(f.p.compoCtx)
	f.e.ConsumeAll()

	if got := f.stored(); got != "Grace" {
		t.Errorf("the stored name is %q, want the trimmed one", got)
	}
	if f.p.name != "Grace" || f.p.error != "" {
		t.Errorf("the field after Save: %q %q", f.p.name, f.p.error)
	}
	toasts := f.e.TakeToasts()
	if len(toasts) != 1 || toasts[0].Title != "User name saved" || toasts[0].Detail != "Grace" {
		t.Errorf("the toasts: %+v", toasts)
	}
	actions := f.e.TakeActions()
	if len(actions) != 1 || actions[0].Name != ActionChanged || actions[0].Value != "Grace" {
		t.Errorf("the actions: %+v", actions)
	}
}

// TestPage_RefusesAnEmptyName: an empty field saves nothing and says why
// (task 90).
func TestPage_RefusesAnEmptyName(t *testing.T) {
	f := start(t, "Ada")
	f.p.name = "   "
	f.p.save(f.p.compoCtx)
	f.e.ConsumeAll()

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
