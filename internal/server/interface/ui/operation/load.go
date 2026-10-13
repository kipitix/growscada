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
	ctx = o.compoCtx
	if !o.widgetTypesReloader.Start() {
		return
	}
	uiutil.Call(ctx, o.client.ListWidgetTypes, func(ctx app.Context, list []apiv0.WidgetTypeResponse) {
		byID := make(map[uuid.UUID]apiv0.WidgetTypeResponse, len(list))
		for _, wt := range list {
			byID[wt.ID] = wt
		}
		o.widgetTypes = byID
		o.widgetTypesLoaded(ctx)
	}, func(ctx app.Context, err error) {
		uiutil.ShowError(ctx, err)
		o.widgetTypesLoaded(ctx)
	})
}

func (o *Operation) widgetTypesLoaded(ctx app.Context) {
	if o.widgetTypesReloader.Done() {
		o.loadWidgetTypes(ctx)
	}
}

func (o *Operation) loadScenes(ctx app.Context) {
	ctx = o.compoCtx
	if !o.scenesReloader.Start() {
		return
	}
	uiutil.Call(ctx, o.client.ListScenes, func(ctx app.Context, scenes []apiv0.SceneResponse) {
		o.scenes = scenes
		o.selectScene(ctx, selectScene(o.scenes, o.selectedSceneID))
		o.scenesLoaded(ctx)
	}, func(ctx app.Context, err error) {
		uiutil.ShowError(ctx, err)
		o.scenesLoaded(ctx)
	})
}

func (o *Operation) scenesLoaded(ctx app.Context) {
	if o.scenesReloader.Done() {
		o.loadScenes(ctx)
	}
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
	loaded := func(ctx app.Context, widgets []apiv0.WidgetResponse, err error) {
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
	}
	uiutil.Call(ctx, func(c context.Context) ([]apiv0.WidgetResponse, error) {
		return o.client.ListWidgets(c, sceneID)
	}, func(ctx app.Context, widgets []apiv0.WidgetResponse) {
		loaded(ctx, widgets, nil)
	}, func(ctx app.Context, err error) {
		loaded(ctx, nil, err)
	})
}

func (o *Operation) loadTags(ctx app.Context) {
	ctx = o.compoCtx
	if !o.tagsReloader.Start() {
		return
	}
	uiutil.Call(ctx, o.client.ListTags, func(ctx app.Context, tags []apiv0.TagResponse) {
		o.tags = mergeTags(o.tags, tags)
		o.tagsLoaded(ctx)
	}, func(ctx app.Context, err error) {
		uiutil.ShowError(ctx, err)
		o.tagsLoaded(ctx)
	})
}

func (o *Operation) tagsLoaded(ctx app.Context) {
	if o.tagsReloader.Done() {
		o.loadTags(ctx)
	}
}
