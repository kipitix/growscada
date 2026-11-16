package labels

import (
	"context"
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

// The Labels page runs against the real REST API over Postgres.

var postgres servertest.Postgres

func TestMain(m *testing.M) {
	code := m.Run()
	postgres.Close()
	os.Exit(code)
}

// mountOnHost lets go-app mount the page on the host (see uitest).
type mountOnHost struct{ *Page }

func (m mountOnHost) OnPreRender(ctx app.Context) { m.OnMount(ctx) }

type fixture struct {
	t   *testing.T
	d   *Page
	e   *uitest.Engine
	api *apiclient.Client
}

func start(t *testing.T) *fixture {
	t.Helper()
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	f := &fixture{t: t, api: apiclient.New(srv.URL, srv.Client())}
	f.d = New(apiclient.New(srv.URL, srv.Client()))
	f.e = uitest.Load(t, mountOnHost{f.d})
	return f
}

func (f *fixture) html() string { return app.HTMLString(f.d.Render()) }

func (f *fixture) labels() []apiv0.LabelResponse {
	f.t.Helper()
	list, err := f.api.Labels(context.Background())
	if err != nil {
		f.t.Fatalf("Labels: %v", err)
	}
	return list
}

// findLabel returns the label named name; the list also holds the two labels
// the migrations seed, so a test reads its own by name instead of by position
// (task 85_2; ADR 0025).
func findLabel(t *testing.T, list []apiv0.LabelResponse, name string) apiv0.LabelResponse {
	t.Helper()
	found := 0
	var label apiv0.LabelResponse
	for _, l := range list {
		if l.Name == name {
			found++
			label = l
		}
	}
	if found != 1 {
		t.Fatalf("the labels named %q: %d, want one (all: %+v)", name, found, list)
	}
	return label
}

// TestPage_ListsTheLabels: the page shows every label with its color chip
// and the number of saved states it marks (task 85_2).
func TestPage_ListsTheLabels(t *testing.T) {
	f := start(t)
	created, err := f.api.CreateLabel(context.Background(), "Ready", "#22c55e")
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	f.d.load()
	f.e.ConsumeAll()

	html := f.html()
	if !strings.Contains(html, `data-labels-page`) || !strings.Contains(html, "Ready") {
		t.Fatalf("the page does not show the label: %s", html)
	}
	if !strings.Contains(html, `data-panel-back`) || !strings.Contains(html, "Labels") {
		t.Errorf("the page lacks its header and way back: %s", html)
	}
	if !strings.Contains(html, `data-label-assigned`) {
		t.Errorf("the assigned count is not 0: %s", html)
	}
	if !strings.Contains(html, `data-label-row="`+created.ID.String()+`"`) {
		t.Errorf("the row of the label is missing: %s", html)
	}
}

// TestPage_Create: Add sends the name and the optional color, clears the
// row and rereads the list.
func TestPage_Create(t *testing.T) {
	f := start(t)
	f.d.newName, f.d.newColor = "Deployable", "#3b82f6"
	f.d.create(f.d.compoCtx)
	f.e.ConsumeAll()

	list := f.labels()
	if created := findLabel(t, list, "Deployable"); created.Color != "#3b82f6" {
		t.Fatalf("the created label: %+v", created)
	}
	if f.d.newName != "" || f.d.newColor != "" || f.d.newError != "" {
		t.Errorf("the new row after a create: %q %q %q", f.d.newName, f.d.newColor, f.d.newError)
	}
	if html := f.html(); !strings.Contains(html, "Deployable") {
		t.Errorf("the created label is not shown: %s", html)
	}
}

// TestPage_Create_TakenNameIsShownUnderTheFieldAndKeepsTheText: a 409 is the
// server's answer under the field, the typed name stays (task 85_2).
func TestPage_Create_TakenNameIsShownUnderTheFieldAndKeepsTheText(t *testing.T) {
	f := start(t)
	if _, err := f.api.CreateLabel(context.Background(), "Prod", ""); err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	f.d.newName = "prod"
	f.d.create(f.d.compoCtx)
	f.e.ConsumeAll()

	if f.d.newError == "" || !strings.Contains(f.d.newError, "already taken") {
		t.Errorf("the refusal is %q, want the taken name", f.d.newError)
	}
	if f.d.newName != "prod" {
		t.Errorf("the typed name was dropped: %q", f.d.newName)
	}
	if html := f.html(); !strings.Contains(html, `data-label-error`) {
		t.Errorf("the refusal is not shown under the field: %s", html)
	}
}

// TestPage_RenameAndRecolor: the name is edited in place and saved, and the
// palette recolors the label and clears its color.
func TestPage_RenameAndRecolor(t *testing.T) {
	f := start(t)
	created, err := f.api.CreateLabel(context.Background(), "Ready", "")
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	f.d.startRename(created)
	if f.d.editText != "Ready" {
		t.Errorf("the field opened with %q, want the name", f.d.editText)
	}
	f.d.editText = "Deployable"
	f.d.commitRename(f.d.compoCtx)
	f.e.ConsumeAll()

	list := f.labels()
	renamed := findLabel(t, list, "Deployable")
	f.d.openPalette(renamed)
	f.d.paletteText = "#22c55e"
	f.d.applyColor(f.d.compoCtx, f.d.paletteText)
	f.e.ConsumeAll()

	recolored := findLabel(t, f.labels(), "Deployable")
	if recolored.Color != "#22c55e" {
		t.Fatalf("the recolored label: %+v", recolored)
	}
	f.d.openPalette(recolored)
	f.d.applyColor(f.d.compoCtx, "")
	f.e.ConsumeAll()
	if cleared := findLabel(t, f.labels(), "Deployable"); cleared.Color != "" {
		t.Errorf("the cleared color: %+v", cleared)
	}
}

// TestPage_DeleteAsksFirst: the confirmation names the label and warns how
// many saved states lose the mark, and only then is it deleted.
func TestPage_DeleteAsksFirst(t *testing.T) {
	f := start(t)
	if _, err := f.api.CreateLabel(context.Background(), "Ready", ""); err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	f.d.load()
	f.e.ConsumeAll()
	row := findLabel(t, f.d.labels, "Ready")

	f.d.startDelete(row)
	html := f.html()
	if !strings.Contains(html, `data-label-delete-confirm-panel`) {
		t.Fatalf("the confirmation is not open: %s", html)
	}
	if !strings.Contains(html, "Ready") || !strings.Contains(html, "0 saved states") {
		t.Errorf("the confirmation does not name the label and its count: %s", html)
	}
	if !strings.Contains(f.html(), `data-label-row="`+row.ID.String()+`"`) {
		t.Fatalf("the label is gone before the confirmation")
	}

	f.d.commitDelete(f.d.compoCtx)
	f.e.ConsumeAll()
	for _, l := range f.labels() {
		if l.ID == row.ID {
			t.Errorf("the deleted label is still stored: %+v", l)
		}
	}
	if strings.Contains(f.html(), `data-label-row="`+row.ID.String()+`"`) {
		t.Errorf("the deleted label is still shown: %s", f.html())
	}
}

// TestPage_RereadsOnAnotherClientsEvent: a label changed in another tab is
// an event of the server's stream, and the page rereads its list.
func TestPage_RereadsOnAnotherClientsEvent(t *testing.T) {
	f := start(t)
	created, err := f.api.CreateLabel(context.Background(), "From another tab", "#a855f7")
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	f.d.compoCtx.NewActionWithValue(eventlog.ActionServerEvent,
		apiv0.Event{Type: apiv0.EventLabelCreated, ID: created.ID.String()})
	f.e.ConsumeAll()

	if html := f.html(); !strings.Contains(html, "From another tab") {
		t.Errorf("the page did not reread the label: %s", html)
	}
}

// TestPage_KeyboardOnlyWhileShown: the page takes Ctrl/Cmd+Enter and Esc only
// while it is the page the Misc section shows; a hidden page leaves the keys
// to the mode in front of it (task 90).
func TestPage_KeyboardOnlyWhileShown(t *testing.T) {
	p := &Page{newError: "x"}
	if got := p.keyAction("Enter", true, false); got != pageKeyNone {
		t.Errorf("a hidden page takes Ctrl+Enter: %v", got)
	}
	p.Shown = true
	if got := p.keyAction("Enter", true, false); got != pageKeyCreate {
		t.Errorf("Ctrl+Enter on the shown page: %v, want create", got)
	}
	if got := p.keyAction("Escape", false, false); got != pageKeyCancel {
		t.Errorf("Esc with something to drop: %v, want cancel", got)
	}
	p.newError = ""
	if got := p.keyAction("Escape", false, false); got != pageKeyNone {
		t.Errorf("Esc with nothing to drop: %v", got)
	}
}
