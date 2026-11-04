// Package operation is the Operator's runtime view: the Scenes of the
// Revision the Project runs, with live Tag values, read-only. It reads the
// Revision and the snapshot of the Tag states from the REST API and follows
// server events, applying tag_updated values directly; rendering is left to
// livescene.View. The Draft and Library changes are invisible here until a
// Deploy makes a new Revision, whose revision_created reloads it.
package operation

import (
	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
	"github.com/kipitix/growscada/internal/server/interface/ui/livescene"
	"github.com/kipitix/growscada/internal/server/interface/ui/tagstate"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

const sceneStorageKey = "operation:sceneID"

type Operation struct {
	app.Compo
	// compoCtx is the component's own context, captured in OnMount; async
	// results are dispatched through it (see project.Project.compoCtx).
	compoCtx  app.Context
	mounted   bool
	ThemeMode string // exported so go-app detects theme changes and re-renders
	client    *apiclient.Client
	// projectID is the project shown, uuid.Nil if none; content is the client
	// of its content, nil if none (SetProject). The Scene and the WidgetTypes
	// come from the Revision the project runs, never from the Draft.
	projectID uuid.UUID
	content   *apiclient.ProjectClient

	// deployed is false until the project's first Deploy: the view then says
	// so, and nothing of the Draft is shown in its place.
	deployed bool
	revision apiv0.RevisionResponse
	scenes   []apiv0.SceneResponse
	// widgetTypes is the copy of each WidgetType the Revision holds, by ID.
	widgetTypes map[uuid.UUID]apiv0.ReleasedWidgetTypeResponse
	// widgets are those of the selected Scene of the Revision.
	widgets []apiv0.WidgetResponse
	tags    tagstate.Tags

	selectedSceneID uuid.UUID
	disconnected    bool

	// Coalesce reloads of each server-backed list.
	revisionReloader  uiutil.Reloader
	tagStatesReloader uiutil.Reloader
}

func NewOperation(client *apiclient.Client) *Operation {
	return &Operation{client: client}
}

func (o *Operation) OnMount(ctx app.Context) {
	o.compoCtx, o.mounted = ctx, true
	o.widgetTypes = map[uuid.UUID]apiv0.ReleasedWidgetTypeResponse{}
	ctx.ObserveState("theme", &o.ThemeMode)
	ctx.ObserveState(eventlog.StateDisconnected, &o.disconnected)
	o.selectedSceneID = uiutil.LoadID(ctx, o.sceneKey())

	o.reload(ctx, eventlog.AllTargets)

	ctx.Handle(eventlog.ActionServerEvent, func(ctx app.Context, a app.Action) {
		ev, ok := a.Value.(apiv0.Event)
		if !ok || !eventlog.ForProject(a, o.projectID) { // another mode's project
			return
		}
		o.tags = eventlog.ApplyToTags(o.tags, ev)
		o.reload(ctx, eventlog.ReloadTargetsFor(ev))
	})
}

// OnDismount marks the Operation unmounted: a SetProject until the next
// OnMount only remembers the project, whose content that OnMount loads,
// rather than working through the context of an unmounted component.
func (o *Operation) OnDismount() {
	o.mounted = false
}

// SetProject shows the project id (uuid.Nil: none), dropping what was shown
// of another one and restoring the new one's selected Scene. Its content is
// loaded on the EventProjectResync of the Bar's stream of it, like
// Project's. Before the Operation is mounted it only remembers it.
func (o *Operation) SetProject(id uuid.UUID) {
	if id == o.projectID {
		return
	}
	o.projectID, o.content = id, nil
	if id != uuid.Nil {
		o.content = o.client.Project(id)
	}
	if !o.mounted {
		return
	}
	o.scenes, o.widgets = nil, nil
	o.widgetTypes = map[uuid.UUID]apiv0.ReleasedWidgetTypeResponse{}
	o.tags = tagstate.Tags{}
	o.revision, o.deployed = apiv0.RevisionResponse{}, false
	o.selectedSceneID = uiutil.LoadID(o.compoCtx, o.sceneKey())
	// Root calls it, so nothing marks this component for an update.
	o.compoCtx.Update()
}

// sceneKey is the LocalStorage key of the selected Scene of the project
// shown.
func (o *Operation) sceneKey() string { return sceneStorageKey + ":" + o.projectID.String() }

func (o *Operation) reload(ctx app.Context, targets eventlog.ReloadTargets) {
	// Only the running Revision is read here: a Draft or Library event changes
	// nothing until a Deploy makes a Revision (task 35, Q1/Q3).
	if targets.Revision {
		o.loadRevision(ctx)
	}
}

func (o *Operation) selectScene(ctx app.Context, sceneID uuid.UUID) {
	if sceneID == o.selectedSceneID {
		return
	}
	o.selectedSceneID = sceneID
	uiutil.StoreID(ctx, o.sceneKey(), sceneID)
	o.loadWidgets()
}

func (o *Operation) Render() app.UI {
	if o.content == nil {
		return uiutil.NoProject()
	}
	if !o.deployed {
		return renderNotDeployed()
	}
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex", "1").
		Style("min-width", "0").
		Style("min-height", "0").
		Style("border", "1px solid var(--border)").
		Body(
			o.renderSceneTabs(),
			o.renderScene(),
		)
}

func (o *Operation) renderSceneTabs() app.UI {
	tabs := make([]app.UI, 0, len(o.scenes))
	for _, sc := range o.scenes {
		sc := sc
		active := o.selectedSceneID == sc.ID

		bg, borderBottom, color, fontWeight := "var(--bg-hover)", "2px solid transparent", "var(--text-2)", "normal"
		if active {
			bg, borderBottom, color, fontWeight = "var(--bg)", "2px solid var(--accent)", "var(--accent)", "600"
		}

		tabs = append(tabs, app.Div().
			Attr("role", "tab").
			Attr("aria-selected", active).
			Style("padding", "6px 14px").
			Style("font-size", "13px").
			Style("cursor", "pointer").
			Style("background", bg).
			Style("border-right", "1px solid var(--border)").
			Style("border-bottom", borderBottom).
			Style("color", color).
			Style("font-weight", fontWeight).
			Style("white-space", "nowrap").
			Text(sc.Name).
			// Scoped by the scene: go-app keeps a handler of the same func
			// literal on a reused tab, which would select the scene it showed
			// before the list changed.
			OnClick(func(ctx app.Context, e app.Event) {
				o.selectScene(ctx, sc.ID)
			}, app.EventScope(sc.ID.String())))
	}

	return app.Div().
		Attr("role", "tablist").
		Style("display", "flex").
		Style("flex-direction", "row").
		Style("flex-shrink", "0").
		Style("overflow-x", "auto").
		Style("border-bottom", "1px solid var(--border)").
		Style("background", "var(--bg-elevated)").
		Body(tabs...)
}

func (o *Operation) renderScene() app.UI {
	sc, ok := o.findScene(o.selectedSceneID)
	if !ok {
		return app.Div().
			Style("flex", "1").
			Style("display", "flex").
			Style("align-items", "center").
			Style("justify-content", "center").
			Style("color", "var(--text-muted)").
			Style("font-size", "14px").
			Text("No scenes to show")
	}

	var widgets []apiv0.WidgetResponse
	if o.selectedSceneID == sc.ID {
		widgets = o.widgets
	}
	return &livescene.View{
		Scene:       sc,
		Widgets:     widgets,
		WidgetTypes: o.widgetTypes,
		Tags:        livescene.SceneTags(widgets, o.tags.Find),
		Connected:   !o.disconnected,
		Theme:       o.ThemeMode,
	}
}

func (o *Operation) findScene(id uuid.UUID) (apiv0.SceneResponse, bool) {
	return uiutil.FindByID(o.scenes, id, uiutil.SceneID)
}

// renderNotDeployed is Operation's empty state: the Project runs no Revision,
// so there is no configuration to show. The Draft is deliberately not shown in
// its place: it is not what the plant runs.
func renderNotDeployed() app.UI {
	return app.Div().
		Attr("data-operation-not-deployed", "true").
		Style("flex", "1").
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("align-items", "center").
		Style("justify-content", "center").
		Style("gap", "6px").
		Style("color", "var(--text-muted)").
		Body(
			app.Div().
				Style("font-size", "15px").
				Style("font-weight", "600").
				Text("Project is not deployed"),
			app.Div().
				Style("font-size", "13px").
				Text("Deploy its Draft from the Project mode to see its scenes here."),
		)
}
