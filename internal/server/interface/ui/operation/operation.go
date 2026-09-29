// Package operation is the Operator's runtime view: the Scenes with live Tag
// values, read-only. It loads the configuration and Tags from the REST API,
// follows server events and applies tag_updated values directly; rendering is
// left to livescene.View.
//
// For now it follows the current configuration live, like Project; once
// Deploy exists it will follow the running Revision only.
package operation

import (
	"github.com/maxence-charriere/go-app/v10/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
	"github.com/kipitix/growscada/internal/server/interface/ui/livescene"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

const sceneStorageKey = "operation:sceneID"

type Operation struct {
	app.Compo
	// compoCtx is the component's own context, captured in OnMount; async
	// results are dispatched through it (see project.Project.compoCtx).
	compoCtx     app.Context
	ThemeMode    string // exported so go-app detects theme changes and re-renders
	apiServerURL string

	scenes      []livescene.Scene
	widgetTypes map[string]livescene.WidgetType
	widgets     []livescene.Widget
	tags        map[string]livescene.Tag

	selectedSceneID string
	// widgetsSceneID is the Scene p.widgets belong to.
	widgetsSceneID string
	disconnected   bool

	// Coalesce reloads of each server-backed list.
	widgetTypesReloader uiutil.Reloader
	scenesReloader      uiutil.Reloader
	widgetsReloader     uiutil.Reloader
	tagsReloader        uiutil.Reloader
}

func NewOperation(apiServerURL string) *Operation {
	return &Operation{apiServerURL: apiServerURL}
}

func (o *Operation) OnMount(ctx app.Context) {
	o.compoCtx = ctx
	o.widgetTypes = map[string]livescene.WidgetType{}
	o.tags = map[string]livescene.Tag{}
	ctx.ObserveState("theme", &o.ThemeMode)
	ctx.ObserveState(eventlog.StateDisconnected, &o.disconnected).OnChange(func() {
		if !o.disconnected {
			o.reload(o.compoCtx, allTargets)
		}
	})
	ctx.LocalStorage().Get(sceneStorageKey, &o.selectedSceneID)

	o.reload(ctx, allTargets)

	ctx.Handle(eventlog.ActionServerEvent, func(ctx app.Context, a app.Action) {
		ev, ok := a.Value.(eventlog.ServerEvent)
		if !ok {
			return
		}
		if ev.Type == "tag_updated" {
			if t, ok := decodeTagUpdate(ev.Tag); ok {
				o.tags = applyTagUpdate(o.tags, t)
			} else {
				o.loadTags(ctx)
			}
			return
		}
		o.reload(ctx, reloadTargetsFor(ev.Type))
	})
}

func (o *Operation) reload(ctx app.Context, targets reloadTargets) {
	if targets.widgetTypes {
		o.loadWidgetTypes(ctx)
	}
	if targets.scenes {
		o.loadScenes(ctx)
	}
	if targets.widgets {
		o.loadWidgets(ctx)
	}
	if targets.tags {
		o.loadTags(ctx)
	}
}

func (o *Operation) selectScene(ctx app.Context, sceneID string) {
	if sceneID == o.selectedSceneID {
		return
	}
	o.selectedSceneID = sceneID
	ctx.LocalStorage().Set(sceneStorageKey, sceneID)
	o.loadWidgets(ctx)
}

func (o *Operation) Render() app.UI {
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
			OnClick(func(ctx app.Context, e app.Event) {
				o.selectScene(ctx, sc.ID)
			}))
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

	var widgets []livescene.Widget
	if o.widgetsSceneID == sc.ID {
		widgets = o.widgets
	}
	return &livescene.View{
		Scene:       sc,
		Widgets:     widgets,
		WidgetTypes: o.widgetTypes,
		Tags:        o.tags,
		Connected:   !o.disconnected,
	}
}

func (o *Operation) findScene(id string) (livescene.Scene, bool) {
	for _, sc := range o.scenes {
		if sc.ID == id {
			return sc, true
		}
	}
	return livescene.Scene{}, false
}
