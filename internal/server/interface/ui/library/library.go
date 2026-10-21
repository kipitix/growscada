package library

import (
	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v10/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Component ─────────────────────────────────────────────────────────────────

type Library struct {
	app.Compo
	// compoCtx is the component's own context, captured in OnMount. Async work
	// dispatches its result through it: an event handler's ctx belongs to the
	// element that fired the event, and go-app silently drops Dispatch once that
	// element is unmounted (e.g. a tab that turns into its rename input), which
	// would lose the result and leave a Reloader loading forever.
	compoCtx    app.Context
	ThemeMode   string // exported so go-app detects theme changes and re-renders
	client      *apiclient.Client
	widgetTypes []apiv0.WidgetTypeResponse
	// versions holds the version each write of a WidgetType states; the
	// Version of widgetTypes is not read.
	versions   uiutil.Versions
	reloader   uiutil.Reloader
	selectedID uuid.UUID
	// edit is the editor's state that comes from the server.
	edit editFields
	// inputValues[widgetTypeID][portName] = Input Data value of the preview.
	// Not persisted: a reload starts empty.
	inputValues map[uuid.UUID]map[string]string
	editingID   uuid.UUID
	editingName string
	// add-port form state
	newPortName string
	newPortDesc string
	newPortType string
}

// selectedIDKey is the LocalStorage key of the selected WidgetType.
const selectedIDKey = "library:selectedID"

func NewLibrary(client *apiclient.Client) *Library {
	return &Library{client: client, inputValues: make(map[uuid.UUID]map[string]string)}
}

func (l *Library) OnMount(ctx app.Context) {
	l.compoCtx = ctx
	ctx.ObserveState("theme", &l.ThemeMode)
	l.selectedID = uiutil.LoadID(ctx, selectedIDKey)
	l.loadList(ctx)
	l.handleServerEvents(ctx)
}

// ── Input Data ────────────────────────────────────────────────────────────────

// selectedInputValues returns the Input Data values of the selected
// WidgetType, keyed by port name; nil if none was entered.
func (l *Library) selectedInputValues() map[string]string {
	return l.inputValues[l.selectedID]
}

// setInputValue sets the Input Data value of port for the selected
// WidgetType.
func (l *Library) setInputValue(port, value string) {
	if l.selectedID == uuid.Nil {
		return
	}
	if l.inputValues[l.selectedID] == nil {
		l.inputValues[l.selectedID] = make(map[string]string)
	}
	l.inputValues[l.selectedID][port] = value
}

// ── Selection helpers ─────────────────────────────────────────────────────────

// clearSelection deselects the widget type and empties its editor.
func (l *Library) clearSelection() {
	l.selectedID = uuid.Nil
	l.edit = editFields{}
	l.clearPortForm()
}

func (l *Library) findItem(id uuid.UUID) (apiv0.WidgetTypeResponse, bool) {
	return uiutil.FindByID(l.widgetTypes, id, uiutil.WidgetTypeID)
}

// itemIdx returns the index of the WidgetType id in the list; -1 if absent.
func (l *Library) itemIdx(id uuid.UUID) int {
	return uiutil.IdxByID(l.widgetTypes, id, uiutil.WidgetTypeID)
}

func (l *Library) startEditing(id uuid.UUID, currentName string) {
	l.editingID = id
	l.editingName = currentName
}

// ── Render ────────────────────────────────────────────────────────────────────

func (l *Library) Render() app.UI {
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "row").
		Style("flex", "1").
		Style("min-height", "0").
		Style("gap", "8px").
		Body(
			l.renderListColumn(),
			l.renderEditorColumn("HTML Template", "html-template", l.edit.html, func(ctx app.Context, e app.Event) {
				l.edit.html = ctx.JSSrc().Get("value").String()
			}, true),
			l.renderEditorColumn("Script", "script-editor", l.edit.script, func(ctx app.Context, e app.Event) {
				l.edit.script = ctx.JSSrc().Get("value").String()
			}, true),
			l.renderInputPortsColumn(),
			l.renderInputDataColumn(),
			l.renderPreviewColumn(),
		)
}
