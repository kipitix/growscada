package operation

import (
	"github.com/maxence-charriere/go-app/v10/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/livescene"
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// Each server-backed list is (re)loaded through a uiutil.Reloader, on mount,
// on server events and after a reconnection.

func (o *Operation) loadWidgetTypes(ctx app.Context) {
	ctx = o.compoCtx
	if !o.widgetTypesReloader.Start() {
		return
	}
	url := o.apiServerURL + "/api/v1/widget-types"
	ctx.Async(func() {
		result, problem := uiutil.FetchJSON[getWidgetTypesResponse](url)
		ctx.Dispatch(func(ctx app.Context) {
			if problem != nil {
				ctx.NewActionWithValue(toast.ActionAdd, *problem)
			} else {
				byID := make(map[string]livescene.WidgetType, len(result.WidgetTypes))
				for _, wt := range result.WidgetTypes {
					byID[wt.ID] = wt
				}
				o.widgetTypes = byID
			}
			if o.widgetTypesReloader.Done() {
				o.loadWidgetTypes(ctx)
			}
		})
	})
}

func (o *Operation) loadScenes(ctx app.Context) {
	ctx = o.compoCtx
	if !o.scenesReloader.Start() {
		return
	}
	url := o.apiServerURL + "/api/v1/scenes"
	ctx.Async(func() {
		result, problem := uiutil.FetchJSON[getScenesResponse](url)
		ctx.Dispatch(func(ctx app.Context) {
			if problem != nil {
				ctx.NewActionWithValue(toast.ActionAdd, *problem)
			} else {
				o.scenes = result.Scenes
				o.selectScene(ctx, selectScene(o.scenes, o.selectedSceneID))
			}
			if o.scenesReloader.Done() {
				o.loadScenes(ctx)
			}
		})
	})
}

// loadWidgets (re)loads the widgets of the selected Scene.
func (o *Operation) loadWidgets(ctx app.Context) {
	ctx = o.compoCtx
	if o.selectedSceneID == "" {
		o.widgets, o.widgetsSceneID = nil, ""
		return
	}
	if !o.widgetsReloader.Start() {
		return
	}
	sceneID := o.selectedSceneID
	url := o.apiServerURL + "/api/v1/scenes/" + sceneID + "/widgets"
	ctx.Async(func() {
		result, problem := uiutil.FetchJSON[getWidgetsResponse](url)
		ctx.Dispatch(func(ctx app.Context) {
			rerun := o.widgetsReloader.Done()
			switch {
			case o.selectedSceneID != sceneID:
				rerun = true // the selection changed meanwhile: load the new Scene
			case problem != nil:
				ctx.NewActionWithValue(toast.ActionAdd, *problem)
			default:
				o.widgets, o.widgetsSceneID = result.Widgets, sceneID
			}
			if rerun {
				o.loadWidgets(ctx)
			}
		})
	})
}

func (o *Operation) loadTags(ctx app.Context) {
	ctx = o.compoCtx
	if !o.tagsReloader.Start() {
		return
	}
	url := o.apiServerURL + "/api/v1/tags"
	ctx.Async(func() {
		result, problem := uiutil.FetchJSON[getTagsResponse](url)
		ctx.Dispatch(func(ctx app.Context) {
			if problem != nil {
				ctx.NewActionWithValue(toast.ActionAdd, *problem)
			} else {
				o.tags = mergeTags(o.tags, result.Tags)
			}
			if o.tagsReloader.Done() {
				o.loadTags(ctx)
			}
		})
	})
}
