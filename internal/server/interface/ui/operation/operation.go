// Package operation is the Operator's runtime view: the Scenes of the
// Deployment the Project runs, with live Tag values, read-only. The operational
// picture (the Deployment, the Tag states, the Journal position and the
// connection) comes from a player.Player over the Project's Journal stream;
// rendering is left to livescene.View. The Draft and Library changes are
// invisible here until a Deploy makes a new Deployment, which reaches the
// Player as a Journal entry (ADR 0020).
package operation

import (
	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/livescene"
	"github.com/kipitix/growscada/internal/server/interface/ui/player"
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
	// come from the Deployment the project runs, never from the Draft.
	projectID uuid.UUID
	content   *apiclient.ProjectClient
	// player is the operational model of the project shown: the Deployment it
	// runs, its Tag states and the Journal position. It is replaced on a
	// switch of Project and closed when the mode leaves.
	player *player.Player
	// newSource builds the Player's entry source; nil means the live Journal
	// stream (a test sets its own).
	newSource func(*apiclient.ProjectClient) player.Source

	selectedSceneID uuid.UUID
}

func NewOperation(client *apiclient.Client) *Operation {
	return &Operation{client: client}
}

func (o *Operation) OnMount(ctx app.Context) {
	o.compoCtx, o.mounted = ctx, true
	ctx.ObserveState("theme", &o.ThemeMode)
	o.selectedSceneID = uiutil.LoadID(ctx, o.sceneKey())
	o.startPlayer()
}

// OnDismount closes the Player's stream: the mode no longer shows anything
// until it is mounted again, which reads a fresh starting state.
func (o *Operation) OnDismount() {
	o.mounted = false
	o.stopPlayer()
}

// SetProject shows the project id (uuid.Nil: none), dropping what was shown of
// another one and restoring the new one's selected Scene. A new Player reads
// the new Project's starting state at once; before the Operation is mounted
// it only remembers the project, whose Player that OnMount creates.
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
	o.selectedSceneID = uiutil.LoadID(o.compoCtx, o.sceneKey())
	o.startPlayer()
	// Root calls it, so nothing marks this component for an update.
	o.compoCtx.Update()
}

// startPlayer replaces the Player of the project shown with a fresh one. The
// Player reads the starting state and opens the stream itself.
func (o *Operation) startPlayer() {
	o.stopPlayer()
	if !o.mounted || o.content == nil {
		return
	}
	var source player.Source
	if o.newSource != nil {
		source = o.newSource(o.content)
	} else {
		source = player.NewLiveSource(o.content)
	}
	o.player = player.New(o.compoCtx, o.content, source).WithOnChange(func() {
		o.compoCtx.Update()
	})
	o.player.Resync()
}

func (o *Operation) stopPlayer() {
	if o.player != nil {
		o.player.Close()
		o.player = nil
	}
}

// sceneKey is the LocalStorage key of the selected Scene of the project
// shown.
func (o *Operation) sceneKey() string { return sceneStorageKey + ":" + o.projectID.String() }

func (o *Operation) selectScene(ctx app.Context, sceneID uuid.UUID) {
	if sceneID == o.selectedSceneID {
		return
	}
	o.selectedSceneID = sceneID
	uiutil.StoreID(ctx, o.sceneKey(), sceneID)
}

func (o *Operation) Render() app.UI {
	if o.content == nil {
		return uiutil.NoProject()
	}
	var view player.View
	if o.player != nil {
		view = o.player.View()
	}
	if !view.Deployed() {
		return renderNotDeployed()
	}
	selected := selectScene(view.Scenes, o.selectedSceneID)
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex", "1").
		Style("min-width", "0").
		Style("min-height", "0").
		Style("border", "1px solid var(--border)").
		Body(
			o.renderSceneTabs(view, selected),
			o.renderScene(view, selected),
		)
}

func (o *Operation) renderSceneTabs(view player.View, selected uuid.UUID) app.UI {
	tabs := make([]app.UI, 0, len(view.Scenes))
	for _, sc := range view.Scenes {
		sc := sc
		active := selected == sc.ID

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

func (o *Operation) renderScene(view player.View, selected uuid.UUID) app.UI {
	sc, ok := uiutil.FindByID(view.Scenes, selected, uiutil.SceneID)
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

	widgets := view.Widgets(sc.ID)
	return &livescene.View{
		Scene:       sc,
		Widgets:     widgets,
		WidgetTypes: view.WidgetTypes,
		Tags: livescene.SceneTags(widgets, func(id uuid.UUID) (apiv0.TagResponse, bool) {
			tag, ok := view.Tags[id]
			return tag, ok
		}),
		Connected: view.Connected,
		Theme:     o.ThemeMode,
	}
}

// renderNotDeployed is Operation's empty state: the Project runs no Deployment,
// so there is no configuration to show. The Draft is deliberately not shown in
// its place: it is not what the plant runs. The first Deploy arrives as a
// Journal entry and switches the screen without a reload.
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
