package operation

import (
	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// Each server-backed list is (re)loaded through a uiutil.Reloader, on mount,
// on server events and after a reconnection.

func (o *Operation) loadWidgetTypes(ctx app.Context) {
	uiutil.Load(o.compoCtx, &o.widgetTypesReloader, o.client.ListWidgetTypes, func(ctx app.Context, list []apiv0.WidgetTypeResponse) {
		byID := make(map[uuid.UUID]apiv0.WidgetTypeResponse, len(list))
		for _, wt := range list {
			byID[wt.ID] = wt
		}
		o.widgetTypes = byID
	}, o.loadWidgetTypes)
}

func (o *Operation) loadScenes(ctx app.Context) {
	uiutil.Load(o.compoCtx, &o.scenesReloader, o.client.ListScenes, func(ctx app.Context, scenes []apiv0.SceneResponse) {
		o.scenes = scenes
		o.selectScene(ctx, selectScene(o.scenes, o.selectedSceneID))
	}, o.loadScenes)
}

// loadWidgets (re)loads the widgets of the selected Scene.
func (o *Operation) loadWidgets(ctx app.Context) {
	ctx = o.compoCtx
	if o.selectedSceneID == uuid.Nil {
		o.widgets, o.widgetsSceneID = nil, uuid.Nil
		return
	}
	uiutil.LoadFor(ctx, &o.widgetsReloader, func() uuid.UUID { return o.selectedSceneID }, o.client.ListWidgets,
		func(ctx app.Context, sceneID uuid.UUID, widgets []apiv0.WidgetResponse) {
			o.widgets, o.widgetsSceneID = widgets, sceneID
		}, o.loadWidgets)
}

func (o *Operation) loadTags(ctx app.Context) {
	mark := o.tags.Mark()
	uiutil.Load(o.compoCtx, &o.tagsReloader, o.client.ListTags, func(ctx app.Context, tags []apiv0.TagResponse) {
		o.tags = o.tags.Merge(mark, tags)
	}, o.loadTags)
}
