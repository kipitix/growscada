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

// The Labels window runs against the real REST API over Postgres.

var postgres servertest.Postgres

func TestMain(m *testing.M) {
	code := m.Run()
	postgres.Close()
	os.Exit(code)
}

// mountOnHost lets go-app mount the dialog on the host (see uitest).
type mountOnHost struct{ *Dialog }

func (m mountOnHost) OnPreRender(ctx app.Context) { m.OnMount(ctx) }

type fixture struct {
	t   *testing.T
	d   *Dialog
	e   *uitest.Engine
	api *apiclient.Client
}

func start(t *testing.T) *fixture {
	t.Helper()
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	f := &fixture{t: t, api: apiclient.New(srv.URL, srv.Client())}
	f.d = New(apiclient.New(srv.URL, srv.Client()))
	f.d.Open = true
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

// TestDialog_ListsTheLabels: the window shows every label with its color chip
// and the number of saved states it marks (task 85_2).
func TestDialog_ListsTheLabels(t *testing.T) {
	f := start(t)
	created, err := f.api.CreateLabel(context.Background(), "Ready", "#22c55e")
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	f.d.load(f.d.compoCtx)
	f.e.ConsumeAll()

	html := f.html()
	if !strings.Contains(html, `data-labels-dialog`) || !strings.Contains(html, "Ready") {
		t.Fatalf("the window does not show the label: %s", html)
	}
	if !strings.Contains(html, `data-label-assigned`) {
		t.Errorf("the assigned count is not 0: %s", html)
	}
	if !strings.Contains(html, `data-label-row="`+created.ID.String()+`"`) {
		t.Errorf("the row of the label is missing: %s", html)
	}
}

// TestDialog_Create: Add sends the name and the optional color, clears the
// row and rereads the list.
func TestDialog_Create(t *testing.T) {
	f := start(t)
	f.d.newName, f.d.newColor = "Deployable", "#3b82f6"
	f.d.create(f.d.compoCtx)
	f.e.ConsumeAll()

	list := f.labels()
	if len(list) != 1 || list[0].Name != "Deployable" || list[0].Color != "#3b82f6" {
		t.Fatalf("the created labels: %+v", list)
	}
	if f.d.newName != "" || f.d.newColor != "" || f.d.newError != "" {
		t.Errorf("the new row after a create: %q %q %q", f.d.newName, f.d.newColor, f.d.newError)
	}
	if html := f.html(); !strings.Contains(html, "Deployable") {
		t.Errorf("the created label is not shown: %s", html)
	}
}

// TestDialog_Create_TakenNameIsShownUnderTheFieldAndKeepsTheText: a 409 is the
// server's answer under the field, the typed name stays (task 85_2).
func TestDialog_Create_TakenNameIsShownUnderTheFieldAndKeepsTheText(t *testing.T) {
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

// TestDialog_RenameAndRecolor: the name is edited in place and saved, and the
// palette recolors the label and clears its color.
func TestDialog_RenameAndRecolor(t *testing.T) {
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
	if len(list) != 1 || list[0].Name != "Deployable" {
		t.Fatalf("the renamed labels: %+v", list)
	}
	f.d.openPalette(list[0])
	f.d.paletteText = "#22c55e"
	f.d.applyColor(f.d.compoCtx, f.d.paletteText)
	f.e.ConsumeAll()

	list = f.labels()
	if len(list) != 1 || list[0].Color != "#22c55e" {
		t.Fatalf("the recolored label: %+v", list)
	}
	f.d.openPalette(list[0])
	f.d.applyColor(f.d.compoCtx, "")
	f.e.ConsumeAll()
	if list = f.labels(); len(list) != 1 || list[0].Color != "" {
		t.Errorf("the cleared color: %+v", list)
	}
}

// TestDialog_DeleteAsksFirst: the confirmation names the label and warns how
// many saved states lose the mark, and only then is it deleted.
func TestDialog_DeleteAsksFirst(t *testing.T) {
	f := start(t)
	if _, err := f.api.CreateLabel(context.Background(), "Ready", ""); err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	f.d.load(f.d.compoCtx)
	f.e.ConsumeAll()
	row := f.d.labels[0]

	f.d.startDelete(row)
	html := f.html()
	if !strings.Contains(html, `data-label-delete-confirm-panel`) {
		t.Fatalf("the confirmation is not open: %s", html)
	}
	if !strings.Contains(html, "Ready") || !strings.Contains(html, "0 saved states") {
		t.Errorf("the confirmation does not name the label and its count: %s", html)
	}
	if len(f.labels()) != 1 {
		t.Fatalf("the label is gone before the confirmation")
	}

	f.d.commitDelete(f.d.compoCtx)
	f.e.ConsumeAll()
	if list := f.labels(); len(list) != 0 {
		t.Errorf("the deleted label is still stored: %+v", list)
	}
	if strings.Contains(f.html(), "Ready") {
		t.Errorf("the deleted label is still shown: %s", f.html())
	}
}

// TestDialog_RereadsOnAnotherClientsEvent: a label changed in another tab is
// an event of the server's stream, and the window rereads its list.
func TestDialog_RereadsOnAnotherClientsEvent(t *testing.T) {
	f := start(t)
	created, err := f.api.CreateLabel(context.Background(), "From another tab", "#a855f7")
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	f.d.compoCtx.NewActionWithValue(eventlog.ActionServerEvent,
		apiv0.Event{Type: apiv0.EventLabelCreated, ID: created.ID.String()})
	f.e.ConsumeAll()

	if html := f.html(); !strings.Contains(html, "From another tab") {
		t.Errorf("the window did not reread the label: %s", html)
	}
}
