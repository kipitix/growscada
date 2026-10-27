package operation

import (
	"context"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/tagstate"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// Each server-backed list is (re)loaded through a uiutil.Reloader, on mount,
// on server events and after a reconnection.

// The lists of the project's content belong to its client (Operation.content):
// a response of a project no longer shown is dropped.

func (o *Operation) projectContent() *apiclient.ProjectClient { return o.content }

// loadWidgetTypes (re)loads the WidgetTypes of the releases the project pins:
// those its Widgets are of (ADR 0007).
func (o *Operation) loadWidgetTypes(ctx app.Context) {
	if o.content == nil {
		o.widgetTypes = map[uuid.UUID]apiv0.ReleasedWidgetTypeResponse{}
		return
	}
	uiutil.LoadOfProject(o.compoCtx, &o.widgetTypesReloader, o.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) ([]apiv0.ReleasedWidgetTypeResponse, error) {
			return pc.ListWidgetTypes(c)
		},
		func(ctx app.Context, list []apiv0.ReleasedWidgetTypeResponse) {
			byID := make(map[uuid.UUID]apiv0.ReleasedWidgetTypeResponse, len(list))
			for _, wt := range list {
				byID[wt.ID] = wt
			}
			o.widgetTypes = byID
		}, o.loadWidgetTypes)
}

func (o *Operation) loadScenes(ctx app.Context) {
	if o.content == nil {
		o.scenes = nil
		return
	}
	uiutil.LoadOfProject(o.compoCtx, &o.scenesReloader, o.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) ([]apiv0.SceneResponse, error) {
			return pc.ListScenes(c)
		},
		func(ctx app.Context, scenes []apiv0.SceneResponse) {
			o.scenes = scenes
			o.selectScene(ctx, selectScene(o.scenes, o.selectedSceneID))
		}, o.loadScenes)
}

// loadWidgets (re)loads the widgets of the selected Scene.
func (o *Operation) loadWidgets(ctx app.Context) {
	ctx = o.compoCtx
	if o.selectedSceneID == uuid.Nil || o.content == nil {
		o.widgets, o.widgetsSceneID = nil, uuid.Nil
		return
	}
	uiutil.LoadFor(ctx, &o.widgetsReloader,
		func() uiutil.SceneOfProject {
			return uiutil.SceneOfProject{Content: o.content, SceneID: o.selectedSceneID}
		},
		uiutil.ListWidgets,
		func(ctx app.Context, k uiutil.SceneOfProject, widgets []apiv0.WidgetResponse) {
			o.widgets, o.widgetsSceneID = widgets, k.SceneID
		}, o.loadWidgets)
}

func (o *Operation) loadTags(ctx app.Context) {
	if o.content == nil {
		o.tags = tagstate.Tags{}
		return
	}
	mark := o.tags.Mark()
	uiutil.LoadOfProject(o.compoCtx, &o.tagsReloader, o.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) ([]apiv0.TagResponse, error) {
			return pc.ListTags(c)
		},
		func(ctx app.Context, tags []apiv0.TagResponse) {
			o.tags = o.tags.Merge(mark, tags)
		}, o.loadTags)
}
