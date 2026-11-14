package library

import (
	"context"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/savedstates"
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
	mounted   bool
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
	widgetTypes   []apiv0.WidgetTypeResponse
	// releases are the library's, read for the history screen: a saved state a
	// Release published shows that Release's number and note (task 84).
	releases         []apiv0.LibraryReleaseResponse
	releasesReloader uiutil.Reloader
	// history is the screen that lists the saved states, shared with the
	// Project mode (savedstates.Screen).
	history *savedstates.Screen
	// draftVersions holds the Version of the selected library's Draft: every
	// write of its content states it and every content DTO carries it
	// (ADR 0016). One number serves the whole Draft, so it is also what says
	// whether a reread list is newer than the one shown (uiutil.Versions).
	draftVersions uiutil.Versions
	reloader      uiutil.Reloader
	// draftStateReloader loads the Draft's Version when the WidgetTypes carry
	// none: the Draft of a library without one.
	draftStateReloader uiutil.Reloader
	// unsavedReloader loads the Draft's unsaved-changes flag on the events of
	// its content alone, never raising the known Version (task 74).
	unsavedReloader uiutil.Reloader
	// ── Commits (task 72_2; ADR 0021) ──────────────────────────────────────
	// commits is the history of the selected library's Draft, newest first;
	// commitsLoaded is false until its first answer. saveDialog is true while
	// the Save dialog is open, saveTitle and saveAuthor its fields;
	// savingCommit marks the request in flight.
	commits         []apiv0.CommitResponse
	commitsLoaded   bool
	commitsReloader uiutil.Reloader
	saveDialog      bool
	savingCommit    bool
	saveTitle       string
	saveAuthor      string
	// historyScreen is true while the saved states are shown in place of the
	// editor (task 84): History opens them and the way back closes them.
	// loadingSavedState is the number of the saved state being loaded into the
	// Draft (0: none).
	historyScreen     bool
	loadingSavedState int
	// contentReplacements counts the replacements of the whole Draft (a load of
	// a saved state): the first list of WidgetTypes requested after one
	// replaces the editor instead of merging it, so a text typed but never
	// applied does not survive the load. appliedReplacements is the last one
	// whose list was applied; every later list is an ordinary one again
	// (task 84).
	contentReplacements uint64
	appliedReplacements uint64
	// draftUnsaved is the unsaved-changes flag of the selected library's
	// Draft, as the server answers it: its Version is above its Clean Version
	// (ADR 0016, amendment 2026-11-10; task 74). The editor draws it and never
	// derives it.
	draftUnsaved bool
	// draftStateLoaded is false until the first answer of the Draft's state:
	// the header says nothing about saved changes before it.
	draftStateLoaded bool
	// ── Release from a Commit (task 72_7; ADR 0021) ────────────────────────
	// releaseDialog is true while a Commit is being published, releaseCommit
	// the Commit of the dialog, releaseNote and releaseAuthor its fields;
	// releasing marks the request in flight.
	releaseDialog bool
	releasing     bool
	releaseCommit int
	releaseNote   string
	releaseAuthor string

	// ── DraftChange stack (ADR 0017) ───────────────────────────────────────
	// changes is the state of the Draft's undo/redo stack, read on mount, on a
	// resync, after the content events and after each undo or redo;
	// changesLoaded is false until the first answer, so the buttons stay
	// disabled before it. undoing and redoing mark a command in flight.
	changes         apiv0.DraftChangesResponse
	changesLoaded   bool
	changesReloader uiutil.Reloader
	undoing         bool
	redoing         bool
	selectedID      uuid.UUID
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
	// onKeyDown listens for Ctrl+Z and Ctrl+Shift+Z while the mode is shown;
	// it is released with the component (noKeyDown).
	onKeyDown app.Func

	// OnBack and BackName are the panel's way back to the selection page and
	// the library it shows (task 84). Root owns both — the page and the name
	// the selection announced — and sets them when it builds the mode; a mode
	// nothing wired shows no way back.
	OnBack   func()
	BackName func() string
}

// selectedIDKey is the LocalStorage key of the selected WidgetType,
// libraryIDKey that of the selected library.
const (
	selectedIDKey = "library:selectedID"
	libraryIDKey  = "library:selectedLibrary"
)

func NewLibrary(client *apiclient.Client) *Library {
	return &Library{
		client:      client,
		inputValues: make(map[uuid.UUID]map[string]string),
		history:     savedstates.New(),
	}
}

func (l *Library) OnMount(ctx app.Context) {
	l.compoCtx, l.mounted = ctx, true
	ctx.ObserveState("theme", &l.ThemeMode)
	l.selectedID = uiutil.LoadID(ctx, selectedIDKey)
	// Root announces the library the selection page shows (SetLibrary); the
	// key read here is the same one, for the tab that opens it first.
	l.libraryID = uiutil.LoadID(ctx, libraryIDKey)
	l.loadLibraries(ctx)
	l.loadList(ctx)
	l.loadReleases(ctx)
	l.loadDraftState(ctx)
	l.loadDraftChanges(ctx)
	ctx.LocalStorage().Get(uiutil.AuthorKey, &l.saveAuthor)
	l.loadCommits(ctx)
	l.listenShortcuts()
	l.handleServerEvents(ctx)
}

// OnDismount releases the keyboard shortcuts of the Draft left.
func (l *Library) OnDismount() {
	l.mounted = false
	l.noKeyDown()
}

// ── Input Data ────────────────────────────────────────────────────────────────

// callDraft runs call against the Draft of the library shown, if there is one,
// and hands its answer to onOK only while that Draft is still the one shown: a
// response for the library left is dropped. onErr nil shows the error as a
// toast.
func callDraft[T any](l *Library, call func(context.Context, *apiclient.LibraryClient) (T, error), onOK func(app.Context, T), onErr func(app.Context, error)) {
	draft := l.draft()
	if draft == nil {
		return // no library shown
	}
	uiutil.Call(l.compoCtx, func(c context.Context) (T, error) {
		return call(c, draft)
	}, func(ctx app.Context, v T) {
		if draft == l.draft() && onOK != nil {
			onOK(ctx, v)
		}
	}, func(ctx app.Context, err error) {
		if draft != l.draft() {
			return // the Draft left: its failure is not this one's
		}
		if onErr != nil {
			onErr(ctx, err)
			return
		}
		uiutil.ShowError(ctx, err)
	})
}

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

// shownTypes returns the WidgetTypes of the Draft.
func (l *Library) shownTypes() []apiv0.WidgetTypeResponse {
	return l.widgetTypes
}

// SetLibrary shows the library id (uuid.Nil: none); Root calls it with what
// its selection page announced. The WidgetTypes and releases of the library
// are loaded, and what was shown of another one is dropped (task 73).
func (l *Library) SetLibrary(id uuid.UUID) {
	if id == l.libraryID {
		// The editor of the library shown is asked for again — the selection
		// page's Open after the way back —, so the history screen closes: it is
		// not the Draft the page opens (task 84).
		if l.historyScreen {
			l.closeHistory()
		}
		return
	}
	if !l.mounted {
		// Not mounted yet: OnMount loads what this remembers.
		l.libraryID = id
		return
	}
	l.selectLibrary(l.compoCtx, id)
}

// selectLibrary shows the library id's Draft; its WidgetTypes and releases
// are loaded.
func (l *Library) selectLibrary(ctx app.Context, id uuid.UUID) {
	if id == l.libraryID {
		return
	}
	l.libraryID = id
	l.widgetTypes, l.releases = nil, nil
	l.draftVersions.Forget()
	l.clearSelection()
	l.changes, l.changesLoaded = apiv0.DraftChangesResponse{}, false
	l.undoing, l.redoing = false, false
	l.commits, l.commitsLoaded = nil, false
	l.saveDialog, l.savingCommit = false, false
	l.historyScreen, l.loadingSavedState = false, 0
	l.contentReplacements, l.appliedReplacements = 0, 0
	l.history.ResetRename()
	l.draftUnsaved, l.draftStateLoaded = false, false
	l.releaseDialog, l.releasing, l.releaseCommit = false, false, 0
	uiutil.StoreID(ctx, libraryIDKey, id)
	uiutil.StoreID(ctx, selectedIDKey, uuid.Nil)
	l.loadList(ctx)
	l.loadReleases(ctx)
	l.loadDraftState(ctx)
	l.loadDraftChanges(ctx)
	l.loadCommits(ctx)
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
	body := l.renderEditor()
	if l.historyScreen {
		body = l.renderHistoryScreen()
	}
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex", "1").
		Style("min-height", "0").
		Body(l.renderHeader(), body)
}

// renderEditor is the Draft: the WidgetTypes, their editor and the preview.
func (l *Library) renderEditor() app.UI {
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "row").
		Style("flex", "1").
		Style("min-height", "0").
		Style("gap", "8px").
		Body(
			l.renderListColumn(),
			l.renderEditorColumn("HTML Template", "html-template", l.edit.html, l.editorInput(func(v string) { l.edit.html = v }), true),
			l.renderEditorColumn("Script", "script-editor", l.edit.script, l.editorInput(func(v string) { l.edit.script = v }), true),
			l.renderInputPortsColumn(),
			l.renderInputDataColumn(),
			l.renderPreviewColumn(),
		)
}

// editorInput returns the OnInput of an editor setting its field by set.
func (l *Library) editorInput(set func(string)) func(app.Context, app.Event) {
	return func(ctx app.Context, e app.Event) {
		set(ctx.JSSrc().Get("value").String())
	}
}
