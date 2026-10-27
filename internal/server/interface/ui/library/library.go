package library

import (
	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

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
	compoCtx  app.Context
	ThemeMode string // exported so go-app detects theme changes and re-renders
	client    *apiclient.Client
	// libraries are the server's; libraryID is the one selected, whose Draft
	// (widgetTypes) and releases are shown, uuid.Nil if none.
	libraries         []apiv0.LibraryResponse
	librariesReloader uiutil.Reloader
	libraryID         uuid.UUID
	// libraryClient is the client of the library libraryID, nil if none: the
	// key of its lists' loads.
	libraryClient *apiclient.LibraryClient
	releases          []apiv0.LibraryReleaseResponse
	releasesReloader  uiutil.Reloader
	// viewing is the number of the release shown, read-only, in place of the
	// Draft; 0 for the Draft.
	viewing     int
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

// selectedIDKey is the LocalStorage key of the selected WidgetType,
// libraryIDKey that of the selected library.
const (
	selectedIDKey = "library:selectedID"
	libraryIDKey  = "library:selectedLibrary"
)

func NewLibrary(client *apiclient.Client) *Library {
	return &Library{client: client, inputValues: make(map[uuid.UUID]map[string]string)}
}

func (l *Library) OnMount(ctx app.Context) {
	l.compoCtx = ctx
	ctx.ObserveState("theme", &l.ThemeMode)
	l.selectedID = uiutil.LoadID(ctx, selectedIDKey)
	l.libraryID = uiutil.LoadID(ctx, libraryIDKey)
	l.loadLibraries(ctx)
	l.loadList(ctx)
	l.loadReleases(ctx)
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
	return uiutil.FindByID(l.shownTypes(), id, uiutil.WidgetTypeID)
}

// ── Library helpers ───────────────────────────────────────────────────────────

// draft returns the client of the selected library, nil if none.
func (l *Library) draft() *apiclient.LibraryClient {
	if l.libraryClient == nil || l.libraryClient.ID() != l.libraryID {
		l.libraryClient = nil
		if l.libraryID != uuid.Nil {
			l.libraryClient = l.client.Library(l.libraryID)
		}
	}
	return l.libraryClient
}

// selectedLibrary returns the selected library as last loaded.
func (l *Library) selectedLibrary() (apiv0.LibraryResponse, bool) {
	return uiutil.FindByID(l.libraries, l.libraryID, uiutil.LibraryID)
}

// discarding reports whether a Discard of the selected library is under way:
// its Draft is read-only until it ends (ADR 0012).
func (l *Library) discarding() bool {
	lib, ok := l.selectedLibrary()
	return ok && lib.DiscardingTo != 0
}

// readOnly reports whether the WidgetTypes shown cannot be edited: those of a
// release, or of a Draft being discarded.
func (l *Library) readOnly() bool {
	return l.viewing != 0 || l.discarding()
}

// shownTypes returns the WidgetTypes shown: those of the release viewed, or
// of the Draft.
func (l *Library) shownTypes() []apiv0.WidgetTypeResponse {
	if l.viewing == 0 {
		return l.widgetTypes
	}
	for _, r := range l.releases {
		if r.Number != l.viewing {
			continue
		}
		types := make([]apiv0.WidgetTypeResponse, len(r.WidgetTypes))
		for i, wt := range r.WidgetTypes {
			types[i] = apiv0.WidgetTypeResponse{
				ID: wt.ID, LibraryID: wt.LibraryID, Name: wt.Name, HtmlTemplate: wt.HtmlTemplate, Script: wt.Script,
				ScriptLanguage: wt.ScriptLanguage, DefaultWidth: wt.DefaultWidth, DefaultHeight: wt.DefaultHeight,
				InputPorts: wt.InputPorts,
			}
		}
		return types
	}
	return nil
}

// selectLibrary shows the library id's Draft; its WidgetTypes and releases
// are loaded.
func (l *Library) selectLibrary(ctx app.Context, id uuid.UUID) {
	if id == l.libraryID {
		return
	}
	l.libraryID = id
	l.viewing = 0
	l.widgetTypes, l.releases = nil, nil
	l.versions.Forget()
	l.clearSelection()
	uiutil.StoreID(ctx, libraryIDKey, id)
	uiutil.StoreID(ctx, selectedIDKey, uuid.Nil)
	l.loadList(ctx)
	l.loadReleases(ctx)
}

// view shows release n read-only (0: back to the Draft).
func (l *Library) view(n int) {
	if n == l.viewing {
		return
	}
	l.viewing = n
	l.clearSelection()
}

// itemIdx returns the index of the WidgetType id in the list; -1 if absent.
func (l *Library) itemIdx(id uuid.UUID) int {
	if l.viewing != 0 {
		return -1
	}
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
			l.renderLibrariesColumn(),
			l.renderListColumn(),
			l.renderEditorColumn("HTML Template", "html-template", l.edit.html, l.editorInput(func(v string) { l.edit.html = v }), !l.readOnly()),
			l.renderEditorColumn("Script", "script-editor", l.edit.script, l.editorInput(func(v string) { l.edit.script = v }), !l.readOnly()),
			l.renderInputPortsColumn(),
			l.renderInputDataColumn(),
			l.renderPreviewColumn(),
		)
}

// editorInput returns the OnInput of an editor setting its field by set, nil
// while the WidgetTypes shown are read-only.
func (l *Library) editorInput(set func(string)) func(app.Context, app.Event) {
	if l.readOnly() {
		return nil
	}
	return func(ctx app.Context, e app.Event) {
		set(ctx.JSSrc().Get("value").String())
	}
}
