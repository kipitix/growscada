package library

import (
	"slices"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
	"github.com/kipitix/growscada/internal/server/interface/ui/uidto"
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
	compoCtx     app.Context
	ThemeMode    string // exported so go-app detects theme changes and re-renders
	apiServerURL string
	widgetTypes  []widgetTypeItem
	loading      bool
	reloader     uiutil.Reloader
	selectedID   string
	editedName   string
	editedHTML   string
	editedScript string
	// inputValues[widgetTypeID][portName] = Input Data value, kept in the
	// app state inputValuesState
	inputValues      map[string]map[string]string
	editedScriptLang string
	editedInputPorts []uidto.InputPortDTO
	editingID        string
	editingName      string
	// add-port form state
	newPortName string
	newPortDesc string
	newPortType string
}

func NewLibrary(apiServerURL string) *Library {
	return &Library{apiServerURL: apiServerURL}
}

func (l *Library) OnMount(ctx app.Context) {
	l.compoCtx = ctx
	ctx.ObserveState("theme", &l.ThemeMode)
	l.loadInputValues(ctx)
	ctx.LocalStorage().Get("library:selectedID", &l.selectedID)
	l.loadList(ctx)

	ctx.Handle(eventlog.ActionServerEvent, func(ctx app.Context, a app.Action) {
		if ev, ok := a.Value.(eventlog.ServerEvent); ok && eventlog.ReloadTargetsFor(ev.Type).WidgetTypes {
			l.loadList(ctx)
		}
	})
}

// ── Input Data ────────────────────────────────────────────────────────────────

// inputValuesState is the app state holding the Input Data values of the
// preview, per WidgetType. They live in the page, not in the component: Root
// makes a new Library each time the Library tab is opened, and the values
// must survive a visit to another tab. Not persisted: a reload starts empty.
const inputValuesState = "library:inputValues"

func (l *Library) loadInputValues(ctx app.Context) {
	l.inputValues = nil
	ctx.GetState(inputValuesState, &l.inputValues)
	if l.inputValues == nil {
		l.inputValues = make(map[string]map[string]string)
	}
}

// selectedInputValues returns the Input Data values of the selected
// WidgetType, keyed by port name; nil if none was entered.
func (l *Library) selectedInputValues() map[string]string {
	return l.inputValues[l.selectedID]
}

// setInputValue sets the Input Data value of port for the selected
// WidgetType.
func (l *Library) setInputValue(ctx app.Context, port, value string) {
	if l.selectedID == "" {
		return
	}
	if l.inputValues[l.selectedID] == nil {
		l.inputValues[l.selectedID] = make(map[string]string)
	}
	l.inputValues[l.selectedID][port] = value
	ctx.SetState(inputValuesState, l.inputValues)
}

// ── Data loading ──────────────────────────────────────────────────────────────

// loadList (re)loads the widget type list; concurrent requests are coalesced.
func (l *Library) loadList(ctx app.Context) {
	ctx = l.compoCtx
	if !l.reloader.Start() {
		return
	}
	l.loading = true
	url := l.apiServerURL + apiv0.PathPrefix + "/widget-types"
	ctx.Async(func() {
		result, problem := uiutil.FetchJSON[getWidgetTypesResponse](url)
		ctx.Dispatch(func(ctx app.Context) {
			l.loading = false
			if problem != nil {
				ctx.NewActionWithValue(toast.ActionAdd, *problem)
			} else {
				l.setWidgetTypes(ctx, result.WidgetTypes)
			}
			if l.reloader.Done() {
				l.loadList(ctx)
			}
		})
	})
}

// setWidgetTypes replaces the list and brings the editor of the selected type
// up to date: a type seen for the first time fills the editor, a known one is
// merged field by field so unsaved edits survive changes made elsewhere, and a
// deleted one drops the selection.
func (l *Library) setWidgetTypes(ctx app.Context, items []widgetTypeItem) {
	old, known := l.findItem(l.selectedID)
	l.widgetTypes = items
	if l.selectedID == "" {
		return
	}
	current, exists := l.findItem(l.selectedID)
	switch {
	case !exists:
		l.clearSelection()
		ctx.LocalStorage().Set("library:selectedID", "")
	case !known:
		l.selectItem(l.selectedID)
	default:
		l.mergeSelected(old, current)
	}
}

// clearSelection deselects the widget type and empties its editor.
func (l *Library) clearSelection() {
	l.selectedID = ""
	l.editedHTML = ""
	l.editedScript = ""
	l.editedInputPorts = nil
}

func (l *Library) findItem(id string) (widgetTypeItem, bool) {
	for _, it := range l.widgetTypes {
		if it.ID == id {
			return it, true
		}
	}
	return widgetTypeItem{}, false
}

// mergeSelected updates each editor field that still holds the old server value.
// Preview input values and the add-port form are editor-only state and kept.
func (l *Library) mergeSelected(old, current widgetTypeItem) {
	l.editedName = uiutil.MergeField(l.editedName, old.Name, current.Name)
	l.editedHTML = uiutil.MergeField(l.editedHTML, old.HtmlTemplate, current.HtmlTemplate)
	l.editedScript = uiutil.MergeField(l.editedScript, old.Script, current.Script)
	l.editedScriptLang = uiutil.MergeField(l.editedScriptLang, old.ScriptLanguage, current.ScriptLanguage)
	if slices.Equal(l.editedInputPorts, old.InputPorts) {
		l.editedInputPorts = slices.Clone(current.InputPorts)
	}
}

// ── Selection helpers ─────────────────────────────────────────────────────────

func (l *Library) selectItem(id string) {
	for _, it := range l.widgetTypes {
		if it.ID == id {
			l.selectedID = id
			l.editedName = it.Name
			l.editedHTML = it.HtmlTemplate
			l.editedScript = it.Script
			l.editedScriptLang = it.ScriptLanguage
			ports := make([]uidto.InputPortDTO, len(it.InputPorts))
			copy(ports, it.InputPorts)
			l.editedInputPorts = ports
			l.newPortName = ""
			l.newPortDesc = ""
			l.newPortType = ""
			return
		}
	}
}

func (l *Library) startEditing(id, currentName string) {
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
			l.renderEditorColumn("HTML Template", "html-template", l.editedHTML, func(ctx app.Context, e app.Event) {
				l.editedHTML = ctx.JSSrc().Get("value").String()
			}, true),
			l.renderEditorColumn("Script", "script-editor", l.editedScript, func(ctx app.Context, e app.Event) {
				l.editedScript = ctx.JSSrc().Get("value").String()
			}, true),
			l.renderInputPortsColumn(),
			l.renderInputDataColumn(),
			l.renderPreviewColumn(),
		)
}
