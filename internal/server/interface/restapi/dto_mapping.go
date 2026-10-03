package restapi

import (
	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application/appdto"
)

// Conversions between the server API contract (apiv0) and the application
// layer's DTOs.

// newTagResponse converts an app-level Tag DTO to an apiv0.TagResponse DTO.
func newTagResponse(t appdto.Tag) apiv0.TagResponse {
	return apiv0.TagResponse{
		ID:      t.ID,
		Name:    t.Name,
		Type:    t.Type,
		Value:   t.Value,
		Quality: t.Quality,
		Version: t.Version,
	}
}

// newGetTagsResponse converts an app-level TagList DTO to an apiv0.GetTagsResponse DTO.
func newGetTagsResponse(list []appdto.Tag) apiv0.GetTagsResponse {
	tags := make([]apiv0.TagResponse, len(list))
	for i, t := range list {
		tags[i] = newTagResponse(t)
	}
	return apiv0.GetTagsResponse{Tags: tags}
}

// newCreateTagResponse converts an app-level Tag DTO to an apiv0.CreateTagResponse DTO.
func newCreateTagResponse(t appdto.Tag) apiv0.CreateTagResponse {
	return apiv0.CreateTagResponse{ID: t.ID}
}

// newUpdateTagResponse converts an app-level Tag DTO to an apiv0.UpdateTagResponse DTO.
func newUpdateTagResponse(t appdto.Tag) apiv0.UpdateTagResponse {
	return apiv0.UpdateTagResponse{Version: t.Version}
}

// newCreateTagInput converts an apiv0.CreateTagRequest DTO to an app-level CreateTagInput DTO.
func newCreateTagInput(r apiv0.CreateTagRequest) appdto.CreateTagInput {
	return appdto.CreateTagInput{
		Name:    r.Name,
		Type:    r.Type,
		Value:   r.Value,
		Quality: r.Quality,
	}
}

// newUpdateTagInput converts an apiv0.UpdateTagRequest DTO to an app-level UpdateTagInput DTO.
func newUpdateTagInput(r apiv0.UpdateTagRequest, tagID uuid.UUID) appdto.UpdateTagInput {
	return appdto.UpdateTagInput{
		ID:      tagID,
		Value:   r.Value,
		Quality: r.Quality,
		Version: r.Version,
	}
}

func newSceneResponse(s appdto.Scene) apiv0.SceneResponse {
	return apiv0.SceneResponse{
		ID:             s.ID,
		Name:           s.Name,
		Width:          s.Width,
		Height:         s.Height,
		BackgroundHTML: s.BackgroundHTML,
		Version:        s.Version,
	}
}

func newGetScenesResponse(list []appdto.Scene) apiv0.GetScenesResponse {
	items := make([]apiv0.SceneResponse, len(list))
	for i, s := range list {
		items[i] = newSceneResponse(s)
	}
	return apiv0.GetScenesResponse{Scenes: items}
}

func newCreateSceneResponse(s appdto.Scene) apiv0.CreateSceneResponse {
	return apiv0.CreateSceneResponse{ID: s.ID}
}

func newUpdateSceneResponse(s appdto.Scene) apiv0.UpdateSceneResponse {
	return apiv0.UpdateSceneResponse{Version: s.Version}
}

func newCreateSceneInput(r apiv0.CreateSceneRequest) appdto.CreateSceneInput {
	return appdto.CreateSceneInput{
		Name:           r.Name,
		Width:          r.Width,
		Height:         r.Height,
		BackgroundHTML: r.BackgroundHTML,
	}
}

func newUpdateSceneInput(r apiv0.UpdateSceneRequest, sceneID uuid.UUID) appdto.UpdateSceneInput {
	return appdto.UpdateSceneInput{
		ID:             sceneID,
		Name:           r.Name,
		Width:          r.Width,
		Height:         r.Height,
		BackgroundHTML: r.BackgroundHTML,
		Version:        r.Version,
	}
}

func portBindingDTOsToAppDTOs(bindings []apiv0.PortBinding) []appdto.PortBinding {
	result := make([]appdto.PortBinding, len(bindings))
	for i, b := range bindings {
		result[i] = appdto.PortBinding{PortName: b.PortName, TagID: b.TagID}
	}
	return result
}

func appPortBindingsToRest(bindings []appdto.PortBinding) []apiv0.PortBinding {
	result := make([]apiv0.PortBinding, len(bindings))
	for i, b := range bindings {
		result[i] = apiv0.PortBinding{PortName: b.PortName, TagID: b.TagID}
	}
	return result
}

func newWidgetResponse(w appdto.Widget) apiv0.WidgetResponse {
	labels := w.Labels
	if labels == nil {
		labels = []string{}
	}
	portBindings := appPortBindingsToRest(w.PortBindings)
	return apiv0.WidgetResponse{
		ID:   w.ID,
		Name: w.Name,
		Position: apiv0.PositionResponse{
			X: w.X,
			Y: w.Y,
			Z: w.Z,
		},
		Size: apiv0.SizeResponse{
			Width:  w.Width,
			Height: w.Height,
		},
		Origin: apiv0.OriginResponse{
			X: w.OriginX,
			Y: w.OriginY,
		},
		Rotation: apiv0.RotationResponse{
			Degrees: w.RotationDegrees,
		},
		TransformMatrix: apiv0.TransformMatrixResponse{
			A:   w.TransformMatrix.A,
			B:   w.TransformMatrix.B,
			C:   w.TransformMatrix.C,
			D:   w.TransformMatrix.D,
			E:   w.TransformMatrix.E,
			F:   w.TransformMatrix.F,
			CSS: w.TransformMatrix.CSS,
		},
		TypeID:       w.TypeID,
		SceneID:      w.SceneID,
		SceneVersion: w.SceneVersion,
		Labels:       labels,
		PortBindings: portBindings,
	}
}

func newGetWidgetsResponse(list []appdto.Widget) apiv0.GetWidgetsResponse {
	items := make([]apiv0.WidgetResponse, len(list))
	for i, w := range list {
		items[i] = newWidgetResponse(w)
	}
	return apiv0.GetWidgetsResponse{Widgets: items}
}

func newCreateWidgetResponse(w appdto.Widget) apiv0.CreateWidgetResponse {
	return apiv0.CreateWidgetResponse{ID: w.ID, SceneVersion: w.SceneVersion}
}

func newUpdateWidgetResponse(w appdto.Widget) apiv0.UpdateWidgetResponse {
	return apiv0.UpdateWidgetResponse{SceneVersion: w.SceneVersion}
}

func newCreateWidgetInput(r apiv0.CreateWidgetRequest) appdto.CreateWidgetInput {
	return appdto.CreateWidgetInput{
		Name:            r.Name,
		X:               r.Position.X,
		Y:               r.Position.Y,
		Z:               r.Position.Z,
		Width:           r.Size.Width,
		Height:          r.Size.Height,
		OriginX:         r.Origin.X,
		OriginY:         r.Origin.Y,
		RotationDegrees: r.Rotation.Degrees,
		TypeID:          r.TypeID,
		SceneVersion:    r.SceneVersion,
		Labels:          r.Labels,
		PortBindings:    portBindingDTOsToAppDTOs(r.PortBindings),
	}
}

func newUpdateWidgetInput(r apiv0.UpdateWidgetRequest, widgetID uuid.UUID) appdto.UpdateWidgetInput {
	return appdto.UpdateWidgetInput{
		ID:              widgetID,
		Name:            r.Name,
		X:               r.Position.X,
		Y:               r.Position.Y,
		Z:               r.Position.Z,
		Width:           r.Size.Width,
		Height:          r.Size.Height,
		OriginX:         r.Origin.X,
		OriginY:         r.Origin.Y,
		RotationDegrees: r.Rotation.Degrees,
		TypeID:          r.TypeID,
		SceneVersion:    r.SceneVersion,
		Labels:          r.Labels,
		PortBindings:    portBindingDTOsToAppDTOs(r.PortBindings),
	}
}

func inputPortDTOsToAppDTOs(ports []apiv0.InputPort) []appdto.InputPort {
	result := make([]appdto.InputPort, len(ports))
	for i, p := range ports {
		result[i] = appdto.InputPort{
			Name:        p.Name,
			Description: p.Description,
			TypeHint:    p.TypeHint,
		}
	}
	return result
}

func appInputPortsToRest(ports []appdto.InputPort) []apiv0.InputPort {
	result := make([]apiv0.InputPort, len(ports))
	for i, p := range ports {
		result[i] = apiv0.InputPort{
			Name:        p.Name,
			Description: p.Description,
			TypeHint:    p.TypeHint,
		}
	}
	return result
}

func newWidgetTypeResponse(wt appdto.WidgetType) apiv0.WidgetTypeResponse {
	return apiv0.WidgetTypeResponse{
		ID:             wt.ID,
		Name:           wt.Name,
		HtmlTemplate:   wt.HtmlTemplate,
		Script:         wt.Script,
		ScriptLanguage: wt.ScriptLanguage,
		DefaultWidth:   wt.DefaultWidth,
		DefaultHeight:  wt.DefaultHeight,
		InputPorts:     appInputPortsToRest(wt.InputPorts),
		Version:        wt.Version,
	}
}

func newGetWidgetTypesResponse(list []appdto.WidgetType) apiv0.GetWidgetTypesResponse {
	items := make([]apiv0.WidgetTypeResponse, len(list))
	for i, wt := range list {
		items[i] = newWidgetTypeResponse(wt)
	}
	return apiv0.GetWidgetTypesResponse{WidgetTypes: items}
}

func newCreateWidgetTypeResponse(wt appdto.WidgetType) apiv0.CreateWidgetTypeResponse {
	return apiv0.CreateWidgetTypeResponse{ID: wt.ID}
}

func newUpdateWidgetTypeResponse(wt appdto.WidgetType) apiv0.UpdateWidgetTypeResponse {
	return apiv0.UpdateWidgetTypeResponse{Version: wt.Version}
}

func newCreateWidgetTypeInput(r apiv0.CreateWidgetTypeRequest) appdto.CreateWidgetTypeInput {
	return appdto.CreateWidgetTypeInput{
		Name:           r.Name,
		HtmlTemplate:   r.HtmlTemplate,
		Script:         r.Script,
		ScriptLanguage: r.ScriptLanguage,
		DefaultWidth:   r.DefaultWidth,
		DefaultHeight:  r.DefaultHeight,
		InputPorts:     inputPortDTOsToAppDTOs(r.InputPorts),
	}
}

func newUpdateWidgetTypeInput(r apiv0.UpdateWidgetTypeRequest, anID uuid.UUID) appdto.UpdateWidgetTypeInput {
	return appdto.UpdateWidgetTypeInput{
		ID:             anID,
		Name:           r.Name,
		HtmlTemplate:   r.HtmlTemplate,
		Script:         r.Script,
		ScriptLanguage: r.ScriptLanguage,
		DefaultWidth:   r.DefaultWidth,
		DefaultHeight:  r.DefaultHeight,
		InputPorts:     inputPortDTOsToAppDTOs(r.InputPorts),
		Version:        r.Version,
	}
}
