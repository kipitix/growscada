package operation

import (
	"context"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v10/pkg/app"

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
	if !o.widgetsReloader.Start() {
		return
	}
	sceneID := o.selectedSceneID
	uiutil.Do(ctx, func(c context.Context) ([]apiv0.WidgetResponse, error) {
		return o.client.ListWidgets(c, sceneID)
	}, func(ctx app.Context, widgets []apiv0.WidgetResponse, err error) {
		rerun := o.widgetsReloader.Done()
		switch {
		case o.selectedSceneID != sceneID:
			rerun = true // the selection changed meanwhile: load the new Scene
		case err != nil:
			uiutil.ShowError(ctx, err)
		default:
			o.widgets, o.widgetsSceneID = widgets, sceneID
		}
		if rerun {
			o.loadWidgets(ctx)
		}
	})
}

func (o *Operation) loadTags(ctx app.Context) {
	uiutil.Load(o.compoCtx, &o.tagsReloader, o.client.ListTags, func(ctx app.Context, tags []apiv0.TagResponse) {
		o.tags = mergeTags(o.tags, tags)
	}, o.loadTags)
}
