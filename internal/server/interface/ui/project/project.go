package project

import (
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/savedstates"
	"github.com/kipitix/growscada/internal/server/interface/ui/tagstate"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Component ─────────────────────────────────────────────────────────────────

type projectSubTab string

const (
	subTabScenes projectSubTab = "scenes"
	subTabTags   projectSubTab = "tags"
)

type Project struct {
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
	// projectID is the project shown, uuid.Nil if none; content is the client
	// of its content, nil if none (SetProject). The WidgetTypes are those of
	// the saved states the project imports; the libraries are the server's,
	// shown whatever the project, for its imports.
	projectID uuid.UUID
	content   *apiclient.ProjectClient

	activeSubTab projectSubTab

	widgetTypes []apiv0.WidgetTypeCopyResponse
	libraries   []apiv0.LibraryResponse
	imports     []apiv0.LibraryImportResponse
	scenes      []apiv0.SceneResponse
	widgets     []apiv0.WidgetResponse
	tags        tagstate.Tags
	// deployment is the Deployment the project runs and deployed says whether
	// it has one; deploymentLoaded is false until the first answer, and
	// deploymentFailed tells a failed read (the running saved state is then
	// unknown, not "not deployed"). The saved state it runs is
	// deployment.SavedStateNumber (task 85_4).
	deployment       apiv0.DeploymentResponse
	deployed         bool
	deploymentLoaded bool
	deploymentFailed bool
	// deploying marks a Deploy in flight; loadingSavedState is
	// the number of the saved state being loaded into the Draft (0: none,
	// task 84).
	deploying         bool
	loadingSavedState int
	// deployments is the list the Deployments dialog shows, newest first,
	// loaded when it opens and reloaded on deployment_created while it is open.
	// selectedDeployment is the row whose document is shown (0: none) as
	// deploymentDetail, once deploymentDetailLoaded. The Project mode shows no
	// Rollback: it stays in the API and History (task 72_4).
	deployments            []apiv0.DeploymentSummaryResponse
	deploymentsLoaded      bool
	deploymentsDialog      bool
	selectedDeployment     int
	deploymentDetail       apiv0.DeploymentResponse
	deploymentDetailLoaded bool
	// ── Project file dialog (task 39, ADR 0018) ─────────────────────────────
	// projectFileDialog is true while the dialog is open; pendingProjectFile
	// is the picked file waiting for confirmation, projectFileIssues the
	// problems the server found in it. projectFileBusy marks a request in
	// flight.
	projectFileDialog  bool
	projectFileBusy    bool
	pendingProjectFile *pendingProjectFile
	projectFileIssues  []apiv0.FileIssue
	projectFileDetail  string
	// draftVersions holds the Version of the Draft: every write of its content
	// states it and every content response carries it (ADR 0016). Like the
	// other Versions it is forgotten when the server may have gone back (a
	// resync) and on a conflict, and a response sent before that does not
	// bring the old one back (uiutil.Versions). One number serves the whole
	// Draft — the Scenes, their Widgets, the Tags, the imports —, so it is also
	// what says whether a reread state is newer than the one shown.
	draftVersions uiutil.Versions

	// ── saved states (task 72_2; ADR 0021) ─────────────────────────────────
	// savedStates is the history of the Draft, newest first; savedStatesLoaded is
	// false until its first answer. saveDialog is true while the Save dialog
	// is open, saveTitle and saveAuthor its fields; savingSavedState marks the
	// request in flight.
	savedStates         []apiv0.SavedStateResponse
	savedStatesLoaded   bool
	savedStatesReloader uiutil.Reloader
	saveDialog          bool
	savingSavedState    bool
	saveTitle           string
	saveAuthor          string
	// draftUnsaved is the unsaved-changes flag of the project's Draft, as the
	// server answers it: its Version is above its Clean Version (ADR 0016,
	// amendment 2026-11-10; task 74). The editor draws it and never derives it.
	draftUnsaved bool
	// draftStateLoaded is false until the first answer of the Draft's state:
	// the header says nothing about saved changes before it.
	draftStateLoaded bool
	// contentReplacements counts the replacements of the whole Draft (a
	// Discard, a Revert, an opened project file): the first Scene list requested
	// after one is the truth whatever Version it states, so the Scenes shown and
	// the Widgets of the selected one are replaced by it. The replacement answers
	// the Version itself, which the list would then not advance. appliedReplacements
	// is the last one whose list was applied: a list requested after it is an
	// ordinary one again and is judged by its Version (task 72_6).
	contentReplacements uint64
	appliedReplacements uint64
	// historyScreen is true while the saved states are shown in place of the
	// content (task 84): History opens them and the way back closes them, and
	// history is the screen that draws them, shared with the Library mode.
	historyScreen bool
	history       *savedstates.Screen
	// historyLabels are the labels of the Organization, the menu of every
	// saved state of the history and the filter of the Deploy window;
	// historyLabelsReloader loads them when History or the Deploy window opens
	// and on a label event. historyLabelsLoaded is false until its first
	// answer, so the window never reports an empty list it has not read
	// (task 85_3, task 85_5; ADR 0025).
	historyLabels         []apiv0.LabelResponse
	historyLabelsLoaded   bool
	historyLabelsReloader uiutil.Reloader

	// OnBack and BackName are the panel's way back to the selection page and
	// the project it shows (task 84). Root owns both — the page and the name
	// the selection announced — and sets them when it builds the mode; a mode
	// nothing wired shows no way back.
	OnBack   func()
	BackName func() string

	// ── Import libraries (task 85_6) ───────────────────────────────────────
	// importDialog is true while the window is shown; importLibraryID is the
	// library whose saved states it lists (uuid.Nil: none) as importSavedStates,
	// newest first, narrowed by the label importLabel (uuid.Nil: all) —
	// importSavedStatesLoaded is false until its first answer.
	importDialog              bool
	importLibraryID           uuid.UUID
	importSavedStates         []apiv0.SavedStateResponse
	importSavedStatesLoaded   bool
	importSavedStatesReloader uiutil.Reloader
	importLabel               uuid.UUID
	// librariesLoaded is false until the server's libraries were read once:
	// the Import window says nothing of their list before it.
	librariesLoaded bool

	// ── Deploy (task 85_5; ADR 0021, ADR 0025) ─────────────────────────────
	// deployDialog is true while the window is shown; deploySavedStates is the
	// saved states it lists, newest first — all of them, or the ones the label
	// deployLabel marks (uuid.Nil: all labels) — and deploySavedStatesLoaded is
	// false until its first answer. A Deploy makes a saved state the running
	// Deployment (task 85_4).
	deployDialog              bool
	deploySavedStates         []apiv0.SavedStateResponse
	deploySavedStatesLoaded   bool
	deploySavedStatesReloader uiutil.Reloader
	deployLabel               uuid.UUID

	// ── DraftChange stack (ADR 0017) ────────────────────────────────────────
	// changes is the state of the Draft's undo/redo stack, read on mount, on a
	// resync, after the content events and after each undo or redo;
	// changesLoaded is false until the first answer, so the buttons stay
	// disabled before it. undoing and redoing mark a command in flight.
	changes       apiv0.DraftChangesResponse
	changesLoaded bool
	undoing       bool
	redoing       bool

	// Coalesce reloads of each server-backed list (see load.go).
	widgetTypesReloader uiutil.Reloader
	draftReloader       uiutil.Reloader
	unsavedReloader     uiutil.Reloader
	changesReloader     uiutil.Reloader
	librariesReloader   uiutil.Reloader
	importsReloader     uiutil.Reloader
	scenesReloader      uiutil.Reloader
	widgetsReloader     uiutil.Reloader
	tagsReloader        uiutil.Reloader
	deployReloader      uiutil.Reloader
	deploymentsReloader uiutil.Reloader
	// onKeyDown listens for Ctrl+Z and Ctrl+Shift+Z while the mode is shown;
	// it is released with the component (noKeyDown). onKeyUp and onBlur end a
	// series of arrow moves at the keyboard or when the window loses focus.
	onKeyDown app.Func
	onKeyUp   app.Func
	onBlur    app.Func
	// staleScenes are the Scenes to reread (loadScene) once the reread in
	// flight on staleScenesReloader is done.
	staleScenes         map[uuid.UUID]struct{}
	staleScenesReloader uiutil.Reloader
	// widgetsReloadDeferred is set when a widget reload arrives mid-drag; it
	// runs once the drag ends so the canvas does not jump under the cursor.
	widgetsReloadDeferred bool

	// ── Tags sub-tab state ──────────────────────────────────────────────────
	selectedTagID uuid.UUID
	// tagVersions marks the Tags known to exist, so a Tag list requested
	// before a creation does not drop the new Tag's selection.
	tagVersions uiutil.Versions

	creatingTag bool
	newTagName  string
	newTagType  string

	selectedSceneID uuid.UUID
	// selectedWidgetIDs is the ordered selection of the Scene's Widgets; the
	// primary — the one the properties panel edits and LocalStorage keeps — is
	// its last element (task 40). A marquee selects in the order of p.widgets,
	// so its primary is the last one it caught.
	selectedWidgetIDs []uuid.UUID

	editingSceneID   uuid.UUID
	editingSceneName string

	// ── Scene properties panel editing state ───────────────────────────────
	sceneEdit sceneEditFields

	draggedTypeID uuid.UUID
	dragJustEnded bool // suppresses the canvas OnClick that fires right after mouseup
	dragDidMove   bool // true if mouse moved during the drag; only then suppress the click
	// dragStartGeometry is the dragged widget's geometry before its first
	// move; a drag that ends where it started saves nothing.
	dragStartGeometry widgetGeometry

	// ── Drag: move widget body ──────────────────────────────────────────────
	// A body drag moves the whole selection: dragGroup is the selection as it
	// was when the drag began, dragStartPositions its positions then.
	draggingWidgetID   uuid.UUID
	dragSceneID        uuid.UUID
	dragGroup          []uuid.UUID
	dragStartPositions map[uuid.UUID]apiv0.PositionResponse
	dragStartCliX      float64
	dragStartCliY      float64
	// dragGuides are the alignment guides of the body drag in flight, and
	// dragAltHeld is Alt in it: held, the drag is free of snapping (task 41).
	// dragLastCliX/Y are the pointer's last client position, so pressing or
	// releasing Alt re-applies the drag without another mousemove.
	dragGuides   dragGuides
	dragAltHeld  bool
	dragLastCliX float64
	dragLastCliY float64

	// ── Arrow-key move series (task 40) ─────────────────────────────────────
	// arrowActive is true from the first arrow keydown until the series is
	// flushed: by an arrow keyup, 500 ms without a press, or any other write
	// of the Draft. arrowBase holds the positions the series started from;
	// one PATCH sends the positions it reached.
	arrowActive  bool
	arrowSceneID uuid.UUID
	arrowBase    map[uuid.UUID]apiv0.PositionResponse
	arrowTimer   *time.Timer
	// moveSending is true while a move PATCH is in flight; moveQueue holds the
	// writes that wait for it, so two writes never state the same Version, and
	// moveDraining is true while those writes run: they run now, so they are
	// not queued again.
	moveSending  bool
	moveDraining bool
	moveQueue    []func(app.Context)
	// marquee is the selection rectangle being dragged on empty canvas.
	marquee marqueeState

	// ── Drag: move origin anchor ────────────────────────────────────────────
	// The anchor is moved within the widget in local space; the widget
	// position is adjusted simultaneously to keep the visual bounding-box
	// stationary on screen.
	draggingOriginID uuid.UUID
	originCliX       float64 // client X when drag started
	originCliY       float64 // client Y when drag started
	originStartOX    float64 // origin.X before drag
	originStartOY    float64 // origin.Y before drag
	originStartPosX  float64 // position.X before drag
	originStartPosY  float64 // position.Y before drag
	originDragW      int     // widget width during drag
	originDragH      int     // widget height during drag
	originDragRotDeg float64 // widget rotation during drag

	// ── Drag: rotation handle ───────────────────────────────────────────────
	// Dragging a circle positioned above the origin in local widget space.
	// We store the origin's client-space position and the initial angle so we
	// can compute the angular delta on every mousemove.
	rotatingWidgetID uuid.UUID
	rotOriginCliX    float64 // origin screen position X (client coords)
	rotOriginCliY    float64 // origin screen position Y (client coords)
	rotStartAngle    float64 // atan2 from origin to handle when drag started
	rotStartDeg      float64 // rotation.Degrees before drag

	// ── Drag: resize SE handle ──────────────────────────────────────────────
	resizingWidgetID uuid.UUID
	resizeStartCliX  float64
	resizeStartCliY  float64
	resizeStartW     int
	resizeStartH     int
	resizeStartDeg   float64 // for inverse-rotation delta
	resizeStartE     float64 // canvas X of widget top-left at resize start
	resizeStartF     float64 // canvas Y of widget top-left at resize start
	resizeOriginX    float64 // origin.X at resize start (for position compensation)
	resizeOriginY    float64 // origin.Y at resize start

	// ── Properties panel editing state ─────────────────────────────────────
	widgetEdit widgetEditFields

	// ── Input simulation (in-memory only, not persisted) ───────────────────
	// simInputs[widgetID][portName] = test value string
	simInputs map[uuid.UUID]map[string]string

	// ── Panel resize state ──────────────────────────────────────────────────
	widgetTypeWidth       int
	propertiesWidth       int
	resizingPanelSide     string // "left" | "right" | ""
	panelResizeStartX     float64
	panelResizeStartWidth int
}

// LocalStorage keys of the selection, kept per project (selectionKey).
const (
	sceneIDKey  = "project:sceneID"
	widgetIDKey = "project:widgetID"
	tagIDKey    = "project:tagID"
)

func NewProject(client *apiclient.Client) *Project {
	return &Project{
		client:       client,
		activeSubTab: subTabScenes,
		simInputs:    make(map[uuid.UUID]map[string]string),
		history:      savedstates.New(),
	}
}

func (p *Project) OnMount(ctx app.Context) {
	p.compoCtx, p.mounted = ctx, true
	ctx.ObserveState("theme", &p.ThemeMode)
	p.widgetTypeWidth = 180
	p.propertiesWidth = 260
	ctx.LocalStorage().Get("project:widgetTypeWidth", &p.widgetTypeWidth)
	ctx.LocalStorage().Get("project:propertiesWidth", &p.propertiesWidth)

	ctx.LocalStorage().Get(uiutil.AuthorKey, &p.saveAuthor)
	var savedTab string
	ctx.LocalStorage().Get("project:subTab", &savedTab)
	if savedTab != "" {
		p.activeSubTab = projectSubTab(savedTab)
	}
	p.restoreSelection(ctx)
	p.loadLibraries(ctx)
	p.loadImports(ctx)
	p.loadWidgetTypes(ctx)
	p.loadScenes(ctx)
	if p.selectedSceneID != uuid.Nil {
		p.loadWidgets(ctx) // load widgets for the restored scene immediately
	}
	p.loadTags(ctx)
	p.loadDeploy(ctx)
	p.loadDraftState(ctx)
	p.loadDraftChanges(ctx)
	p.loadSavedStates(ctx)

	p.listenShortcuts()
	p.handleServerEvents(ctx)
}

// OnDismount marks the Project unmounted: a SetProject until the next
// OnMount only remembers the project, whose content that OnMount loads,
// rather than working through the context of an unmounted component.
func (p *Project) OnDismount() {
	p.mounted = false
	p.noKeyDown()
}

// SetProject shows the project id (uuid.Nil: none). What was shown of
// another project is dropped, its unsaved edits with it, and the new
// project's selection is restored. Its content is loaded on the
// EventProjectResync of the Bar's stream of it (Root has the Bar follow the
// project of the mode shown, or resync the one it follows already), so no
// event of it published before the stream opens is missed. Before the
// Project is mounted it only remembers the project, whose content OnMount
// loads.
func (p *Project) SetProject(id uuid.UUID) {
	if id == p.projectID {
		// The editor of the project shown is asked for again — the selection
		// page's Open after the way back —, so the history screen closes: it is
		// not the content the page opens (task 84).
		if p.historyScreen {
			p.closeHistory()
		}
		return
	}
	// A series of arrow moves belongs to the project left: it is sent before
	// its content is dropped, while the selection and the Version still say
	// what to send.
	if p.mounted {
		p.flushMovesBeforeProjectChange(p.compoCtx)
	}
	p.projectID, p.content = id, nil
	if id != uuid.Nil {
		p.content = p.client.Project(id)
	}
	if !p.mounted {
		return
	}
	ctx := p.compoCtx
	p.scenes, p.widgets, p.tags = nil, nil, tagstate.Tags{}
	p.widgetTypes, p.imports = nil, nil
	p.deployment, p.deployed, p.deploymentLoaded, p.deploymentFailed = apiv0.DeploymentResponse{}, false, false, false
	p.deploying, p.loadingSavedState = false, 0
	p.historyScreen = false
	p.deployments, p.deploymentsLoaded = nil, false
	p.deploymentsDialog, p.selectedDeployment = false, 0
	p.deploymentDetail, p.deploymentDetailLoaded = apiv0.DeploymentResponse{}, false
	p.projectFileDialog, p.projectFileBusy = false, false
	p.pendingProjectFile = nil
	p.projectFileIssues, p.projectFileDetail = nil, ""
	p.changes, p.changesLoaded = apiv0.DraftChangesResponse{}, false
	p.undoing, p.redoing = false, false
	p.savedStates, p.savedStatesLoaded = nil, false
	p.saveDialog, p.savingSavedState = false, false
	p.history.Reset()
	p.draftUnsaved, p.draftStateLoaded = false, false
	p.contentReplacements, p.appliedReplacements = 0, 0
	p.deploySavedStates, p.deploySavedStatesLoaded, p.deployLabel = nil, false, uuid.Nil
	p.importDialog, p.importLibraryID, p.importLabel = false, uuid.Nil, uuid.Nil
	p.importSavedStates, p.importSavedStatesLoaded = nil, false
	p.deployDialog = false
	p.draftVersions.Forget()
	p.tagVersions.Forget()
	clear(p.staleScenes)
	p.widgetsReloadDeferred = false
	p.dragGuides, p.dragAltHeld = dragGuides{}, false
	p.restoreSelection(ctx)
	p.widgetEdit = widgetEditFields{}
	p.sceneEdit = sceneEditFields{}
	p.editingSceneID, p.editingSceneName = uuid.Nil, ""
	p.creatingTag, p.newTagName, p.newTagType = false, "", ""
	p.simInputs = make(map[uuid.UUID]map[string]string)
	// Root calls it, so nothing marks this component for an update.
	ctx.Update()
}

// selectionKey is the LocalStorage key base of the project shown.
func (p *Project) selectionKey(base string) string { return base + ":" + p.projectID.String() }

// restoreSelection reads the selection of the project shown from
// LocalStorage.
func (p *Project) restoreSelection(ctx app.Context) {
	p.selectedSceneID = uiutil.LoadID(ctx, p.selectionKey(sceneIDKey))
	p.selectedWidgetIDs = nil
	// LocalStorage keeps the primary alone: after a reload one Widget is
	// selected (task 40).
	if id := uiutil.LoadID(ctx, p.selectionKey(widgetIDKey)); id != uuid.Nil {
		p.selectedWidgetIDs = []uuid.UUID{id}
	}
	p.selectedTagID = uiutil.LoadID(ctx, p.selectionKey(tagIDKey))
}

// ── Input simulation ──────────────────────────────────────────────────────────

// setSimInput sets the Simulate Inputs value of port on the widget wid.
func (p *Project) setSimInput(wid uuid.UUID, port, value string) {
	if p.simInputs[wid] == nil {
		p.simInputs[wid] = make(map[string]string)
	}
	p.simInputs[wid][port] = value
}

// ── Widget selection helpers ──────────────────────────────────────────────────
//
// The selection is an ordered set of IDs whose last element is primary (see
// Project.selectedWidgetIDs); selection.go holds its operations.

// ── Lookups ───────────────────────────────────────────────────────────────────

func (p *Project) sceneIdx(id uuid.UUID) int { return uiutil.IdxByID(p.scenes, id, uiutil.SceneID) }

func (p *Project) findScene(id uuid.UUID) (apiv0.SceneResponse, bool) {
	return uiutil.FindByID(p.scenes, id, uiutil.SceneID)
}

func (p *Project) selectedWidgetIdx() int {
	if id, ok := p.singleSelected(); ok {
		return uiutil.IdxByID(p.widgets, id, uiutil.WidgetID)
	}
	return -1
}

// findWidget returns the current live widget data by ID. Use it inside event
// handler closures instead of the closure-captured `w`, because go-app may
// not re-attach handlers when only the transform changes, leaving `w` stale
// across drags.
func (p *Project) findWidget(id uuid.UUID) (apiv0.WidgetResponse, bool) {
	return uiutil.FindByID(p.widgets, id, uiutil.WidgetID)
}

func (p *Project) widgetTypeByID(id uuid.UUID) (apiv0.WidgetTypeCopyResponse, bool) {
	return uiutil.FindByID(p.widgetTypes, id, uiutil.WidgetTypeCopyID)
}

func (p *Project) findTag(id uuid.UUID) (apiv0.TagResponse, bool) {
	return p.tags.Find(id)
}

// ── Render ────────────────────────────────────────────────────────────────────

func (p *Project) Render() app.UI {
	if p.content == nil {
		return uiutil.NoProject()
	}
	body := app.UI(app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex", "1").
		Style("min-height", "0").
		Body(
			p.renderSubTabs(),
			app.If(p.activeSubTab == subTabScenes, func() app.UI {
				return p.renderScenesContent()
			}).Else(func() app.UI {
				return p.renderTagsPanel()
			}),
		))
	if p.historyScreen {
		body = p.renderHistoryScreen()
	}
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex", "1").
		Style("min-height", "0").
		Style("overflow", "hidden").
		Body(p.renderDeployBar(), body)
}

func (p *Project) renderSubTabs() app.UI {
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "row").
		Style("border-bottom", "1px solid var(--border)").
		Style("background", "var(--bg-elevated)").
		Style("padding", "0 4px").
		Body(
			p.subTab("Scenes", subTabScenes),
			p.subTab("Tags", subTabTags),
		)
}

func (p *Project) subTab(label string, tab projectSubTab) app.UI {
	active := p.activeSubTab == tab
	el := app.Div().
		Style("padding", "6px 16px").
		Style("cursor", "pointer").
		Style("font-size", "13px").
		Style("user-select", "none").
		Style("border-bottom", "2px solid transparent").
		Style("margin-bottom", "-1px").
		Text(label).
		OnClick(func(ctx app.Context, e app.Event) {
			p.activeSubTab = tab
			ctx.LocalStorage().Set("project:subTab", string(tab))
		})
	if active {
		return el.
			Style("border-bottom-color", "var(--accent)").
			Style("color", "var(--accent)").
			Style("font-weight", "600")
	}
	return el.Style("color", "var(--text-2)")
}

func (p *Project) renderPanelDivider(side string) app.UI {
	return app.Div().
		Style("width", "5px").
		Style("flex-shrink", "0").
		Style("cursor", "col-resize").
		Style("background", "var(--border)").
		Style("user-select", "none").
		OnMouseDown(func(ctx app.Context, e app.Event) {
			e.PreventDefault()
			p.resizingPanelSide = side
			p.panelResizeStartX = e.Get("clientX").Float()
			if side == "left" {
				p.panelResizeStartWidth = p.widgetTypeWidth
			} else {
				p.panelResizeStartWidth = p.propertiesWidth
			}
		})
}

func (p *Project) renderScenesContent() app.UI {
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "row").
		Style("flex", "1").
		Style("min-height", "0").
		Style("overflow", "hidden").
		Style("gap", "0").
		// ── Drag handlers live here so they capture mouse events across the
		// entire Project panel regardless of which scene/canvas is active.
		// Previously these were on the canvas div, which caused drag to break
		// on smaller scenes (e.g. 1280×720) when the mouse left the canvas.
		OnMouseMove(func(ctx app.Context, e app.Event) {
			// A selection rectangle follows the pointer over the whole panel,
			// also outside the canvas; the released button ends it.
			if p.marquee.active {
				if e.Get("buttons").Int() == 0 {
					if p.finishMarquee(ctx) {
						p.dragJustEnded = true
					}
				} else {
					e.PreventDefault()
					p.updateMarquee(e)
				}
				return
			}
			anyDrag := p.isInteracting() || p.resizingPanelSide != ""
			if !anyDrag {
				return
			}
			// Mouse button released outside this element — cancel all drags.
			if e.Get("buttons").Int() == 0 {
				if p.resizingPanelSide == "left" {
					ctx.LocalStorage().Set("project:widgetTypeWidth", p.widgetTypeWidth)
				} else if p.resizingPanelSide == "right" {
					ctx.LocalStorage().Set("project:propertiesWidth", p.propertiesWidth)
				}
				p.resizingPanelSide = ""
				p.finalizeAllDrags(ctx)
				return
			}
			e.PreventDefault()

			clientX := e.Get("clientX").Float()
			clientY := e.Get("clientY").Float()

			// Resize left or right panel
			if p.resizingPanelSide != "" {
				dx := int(clientX - p.panelResizeStartX)
				if p.resizingPanelSide == "left" {
					newW := p.panelResizeStartWidth + dx
					if newW < 100 {
						newW = 100
					} else if newW > 480 {
						newW = 480
					}
					p.widgetTypeWidth = newW
				} else {
					newW := p.panelResizeStartWidth - dx
					if newW < 160 {
						newW = 160
					} else if newW > 600 {
						newW = 600
					}
					p.propertiesWidth = newW
				}
			}

			// Mark that a real drag occurred (not just a mousedown+mouseup click),
			// remembering the widget as it was before the first move.
			if id := p.draggedWidgetID(); id != uuid.Nil {
				if !p.dragDidMove {
					if w, ok := p.findWidget(id); ok {
						p.dragStartGeometry = geometryOf(w)
					}
				}
				p.dragDidMove = true
			}

			// Move widget body: the whole selection rides on the same delta,
			// each Widget from the position it had when the drag began. Alt
			// frees the drag of the alignment guides and their snapping.
			if p.draggingWidgetID != uuid.Nil {
				p.dragAltHeld = e.Get("altKey").Bool()
				p.moveWidgetDrag(clientX, clientY)
			}

			// Move origin anchor; compensate position to keep widget visually stationary.
			// Screen delta (dx,dy) is rotated into widget-local space to get the local
			// origin delta; position is then compensated so the matrix e/f stays fixed.
			if p.draggingOriginID != uuid.Nil {
				dx := clientX - p.originCliX
				dy := clientY - p.originCliY
				rad := p.originDragRotDeg * math.Pi / 180
				cosA := math.Cos(rad)
				sinA := math.Sin(rad)
				// Rotate screen delta into widget-local coordinate axes.
				dOxPx := cosA*dx + sinA*dy
				dOyPx := -sinA*dx + cosA*dy
				newOX := math.Max(0, math.Min(1, p.originStartOX+dOxPx/float64(p.originDragW)))
				newOY := math.Max(0, math.Min(1, p.originStartOY+dOyPx/float64(p.originDragH)))
				// Use clamped pixel deltas for position compensation.
				dOx := (newOX - p.originStartOX) * float64(p.originDragW)
				dOy := (newOY - p.originStartOY) * float64(p.originDragH)
				newPosX := p.originStartPosX - dOx*(1-cosA) - dOy*sinA
				newPosY := p.originStartPosY - dOy*(1-cosA) + dOx*sinA
				for i := range p.widgets {
					if p.widgets[i].ID == p.draggingOriginID {
						p.widgets[i].Origin.X = newOX
						p.widgets[i].Origin.Y = newOY
						p.widgets[i].Position.X = newPosX
						p.widgets[i].Position.Y = newPosY
						if id, single := p.singleSelected(); single && id == p.draggingOriginID {
							f := widgetEditFieldsOf(p.widgets[i])
							p.widgetEdit.originX, p.widgetEdit.originY = f.originX, f.originY
							p.widgetEdit.posX, p.widgetEdit.posY = f.posX, f.posY
						}
						break
					}
				}
			}

			// Rotate widget around origin
			if p.rotatingWidgetID != uuid.Nil {
				dx := clientX - p.rotOriginCliX
				dy := clientY - p.rotOriginCliY
				currentAngle := math.Atan2(dy, dx)
				delta := currentAngle - p.rotStartAngle
				newDeg := math.Mod(p.rotStartDeg+delta*180/math.Pi, 360)
				if newDeg < 0 {
					newDeg += 360
				}
				for i := range p.widgets {
					if p.widgets[i].ID == p.rotatingWidgetID {
						p.widgets[i].Rotation.Degrees = newDeg
						if id, single := p.singleSelected(); single && id == p.rotatingWidgetID {
							p.widgetEdit.rotation = widgetEditFieldsOf(p.widgets[i]).rotation
						}
						break
					}
				}
			}

			// Resize widget via SE handle (inverse-rotation corrected).
			// After computing the new size we also recompute posX/posY so
			// that the canvas-space top-left corner (e, f) stays fixed,
			// preventing the widget from shifting when it is rotated and the
			// origin is not at (0, 0).
			if p.resizingWidgetID != uuid.Nil {
				dx := clientX - p.resizeStartCliX
				dy := clientY - p.resizeStartCliY
				rad := p.resizeStartDeg * math.Pi / 180
				cosA := math.Cos(rad)
				sinA := math.Sin(rad)
				dxLocal := dx*cosA + dy*sinA
				dyLocal := -dx*sinA + dy*cosA
				const minSize = 20
				newW := p.resizeStartW + int(math.Round(dxLocal))
				newH := p.resizeStartH + int(math.Round(dyLocal))
				if newW < minSize {
					newW = minSize
				}
				if newH < minSize {
					newH = minSize
				}
				// Compensate position so the top-left canvas corner is unchanged.
				newOx := p.resizeOriginX * float64(newW)
				newOy := p.resizeOriginY * float64(newH)
				newPosX := p.resizeStartE - newOx*(1-cosA) - sinA*newOy
				newPosY := p.resizeStartF + sinA*newOx - newOy*(1-cosA)
				for i := range p.widgets {
					if p.widgets[i].ID == p.resizingWidgetID {
						p.widgets[i].Size.Width = newW
						p.widgets[i].Size.Height = newH
						p.widgets[i].Position.X = newPosX
						p.widgets[i].Position.Y = newPosY
						if id, single := p.singleSelected(); single && id == p.resizingWidgetID {
							f := widgetEditFieldsOf(p.widgets[i])
							p.widgetEdit.width, p.widgetEdit.height = f.width, f.height
							p.widgetEdit.posX, p.widgetEdit.posY = f.posX, f.posY
						}
						break
					}
				}
			}
		}).
		OnMouseUp(func(ctx app.Context, e app.Event) {
			if p.marquee.active {
				if p.finishMarquee(ctx) {
					p.dragJustEnded = true
				}
				return
			}
			if p.resizingPanelSide == "left" {
				ctx.LocalStorage().Set("project:widgetTypeWidth", p.widgetTypeWidth)
			} else if p.resizingPanelSide == "right" {
				ctx.LocalStorage().Set("project:propertiesWidth", p.propertiesWidth)
			}
			p.resizingPanelSide = ""
			p.finalizeAllDrags(ctx)
		}).
		Body(
			p.renderWidgetTypePanel(),
			p.renderPanelDivider("left"),
			p.renderScenePanel(),
			p.renderPanelDivider("right"),
			p.renderPropertiesPanel(),
		)
}
