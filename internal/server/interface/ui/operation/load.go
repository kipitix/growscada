package operation

import (
	"context"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/tagstate"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// The lists of Operation belong to its Project client: a response of a project
// no longer shown is dropped. They are (re)loaded on mount, on revision_created
// and on the stream's resync.

func (o *Operation) projectContent() *apiclient.ProjectClient { return o.content }

// loadRevision (re)reads the Revision the project runs, the only configuration
// Operation shows, and with it the Tag states snapshot. A Project without a
// Revision is the empty state; nothing of the Draft is read.
func (o *Operation) loadRevision(ctx app.Context) {
	if o.content == nil {
		o.revision, o.deployed = apiv0.RevisionResponse{}, false
		o.scenes, o.widgets, o.tags = nil, nil, tagstate.Tags{}
		o.widgetTypes = map[uuid.UUID]apiv0.ReleasedWidgetTypeResponse{}
		return
	}
	uiutil.LoadOfProject(o.compoCtx, &o.revisionReloader, o.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) (apiclient.RevisionState, error) {
			return pc.Revision(c)
		},
		func(ctx app.Context, state apiclient.RevisionState) {
			if !state.Deployed {
				o.revision, o.deployed = apiv0.RevisionResponse{}, false
				o.scenes, o.widgets, o.tags = nil, nil, tagstate.Tags{}
				o.widgetTypes = map[uuid.UUID]apiv0.ReleasedWidgetTypeResponse{}
				return
			}
			o.revision, o.deployed = state.Revision, true
			o.applyRevision()
			o.loadTagStates(ctx)
		}, o.loadRevision)
}

// applyRevision takes the Scenes and the WidgetTypes of the Revision; the
// selected Scene falls back to the first when it is gone.
func (o *Operation) applyRevision() {
	content := o.revision.Content
	o.scenes = make([]apiv0.SceneResponse, len(content.Scenes))
	for i, scene := range content.Scenes {
		o.scenes[i] = revisionScene(scene)
	}
	o.widgetTypes = make(map[uuid.UUID]apiv0.ReleasedWidgetTypeResponse, len(content.WidgetTypes))
	for _, widgetType := range content.WidgetTypes {
		o.widgetTypes[widgetType.ID] = revisionWidgetType(widgetType)
	}
	o.selectedSceneID = selectScene(o.scenes, o.selectedSceneID)
	o.loadWidgets()
}

// loadWidgets picks the Widgets of the selected Scene out of the Revision.
func (o *Operation) loadWidgets() {
	o.widgets = nil
	for _, scene := range o.revision.Content.Scenes {
		if scene.ID != o.selectedSceneID {
			continue
		}
		o.widgets = make([]apiv0.WidgetResponse, len(scene.Widgets))
		for i, widget := range scene.Widgets {
			o.widgets[i] = revisionWidget(widget, scene.ID)
		}
		return
	}
}

// loadTagStates (re)reads the states of the Tags of the running Revision and
// merges them with the states tag_updated delivered meanwhile (the greater
// seq wins).
func (o *Operation) loadTagStates(ctx app.Context) {
	if o.content == nil || !o.deployed {
		return
	}
	mark := o.tags.Mark()
	uiutil.LoadOfProject(o.compoCtx, &o.tagStatesReloader, o.projectContent,
		func(c context.Context, pc *apiclient.ProjectClient) (map[string]apiv0.TagStateResponse, error) {
			return pc.TagStates(c)
		},
		func(ctx app.Context, states map[string]apiv0.TagStateResponse) {
			o.tags = o.tags.Merge(mark, revisionTags(o.revision.Content.Tags, states))
		}, o.loadTagStates)
}

// revisionScene is the Revision's Scene in the shape the view renders.
func revisionScene(scene projectv0.Scene) apiv0.SceneResponse {
	return apiv0.SceneResponse{
		ID:             scene.ID,
		Name:           scene.Name,
		Width:          scene.Width,
		Height:         scene.Height,
		BackgroundHTML: scene.BackgroundHTML,
	}
}

// revisionWidget is the Revision's Widget in the shape the view renders. The
// Revision has no matrix and no aggregate versions: the matrix is derived from
// the geometry, and the versions are not read.
func revisionWidget(widget projectv0.Widget, sceneID uuid.UUID) apiv0.WidgetResponse {
	bindings := make([]apiv0.PortBinding, len(widget.PortBindings))
	for i, binding := range widget.PortBindings {
		bindings[i] = apiv0.PortBinding{PortName: binding.PortName, TagID: binding.TagID}
	}
	return apiv0.WidgetResponse{
		ID:           widget.ID,
		Name:         widget.Name,
		Position:     apiv0.PositionResponse{X: widget.Position.X, Y: widget.Position.Y, Z: widget.Position.Z},
		Size:         apiv0.SizeResponse{Width: widget.Size.Width, Height: widget.Size.Height},
		Origin:       apiv0.OriginResponse{X: widget.Origin.X, Y: widget.Origin.Y},
		Rotation:     apiv0.RotationResponse{Degrees: widget.Rotation.Degrees},
		TypeID:       widget.TypeID,
		SceneID:      sceneID,
		Labels:       widget.Labels,
		PortBindings: bindings,
		TransformMatrix: apiv0.TransformMatrixResponse{
			CSS: revisionMatrixCSS(widget),
		},
	}
}

// revisionWidgetType is the Revision's copy of a WidgetType in the shape the
// view renders.
func revisionWidgetType(widgetType projectv0.WidgetType) apiv0.ReleasedWidgetTypeResponse {
	ports := make([]apiv0.InputPort, len(widgetType.InputPorts))
	for i, port := range widgetType.InputPorts {
		ports[i] = apiv0.InputPort{Name: port.Name, Description: port.Description, TypeHint: port.TypeHint}
	}
	return apiv0.ReleasedWidgetTypeResponse{
		ID:             widgetType.ID,
		LibraryID:      widgetType.LibraryID,
		Name:           widgetType.Name,
		HtmlTemplate:   widgetType.HTMLTemplate,
		Script:         widgetType.Script,
		ScriptLanguage: widgetType.ScriptLanguage,
		DefaultWidth:   widgetType.DefaultWidth,
		DefaultHeight:  widgetType.DefaultHeight,
		InputPorts:     ports,
	}
}

// revisionTags combines the Tag definitions of the Revision with their states
// snapshot: a Tag without a state shows its TagType's zero value with quality
// bad, as a Tag never written does.
func revisionTags(definitions []projectv0.TagDefinition, states map[string]apiv0.TagStateResponse) []apiv0.TagResponse {
	tags := make([]apiv0.TagResponse, len(definitions))
	for i, definition := range definitions {
		state := states[definition.ID.String()]
		tags[i] = apiv0.TagResponse{
			ID:      definition.ID,
			Name:    definition.Name,
			Type:    definition.Type,
			Value:   state.Value,
			Quality: state.Quality,
			Seq:     state.Seq,
		}
		if tags[i].Quality == "" {
			tags[i].Quality = "bad"
		}
	}
	return tags
}
