package restapi

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
)

// Conversions between the server API contract (apiv0) and the application
// layer's DTOs.

func newProjectResponse(p appdto.Project) apiv0.ProjectResponse {
	return apiv0.ProjectResponse{ID: p.ID, Name: p.Name, Version: p.Version}
}

func newGetProjectsResponse(list []appdto.Project) apiv0.GetProjectsResponse {
	items := make([]apiv0.ProjectResponse, len(list))
	for i, p := range list {
		items[i] = newProjectResponse(p)
	}
	return apiv0.GetProjectsResponse{Projects: items}
}

// newTagResponse converts an app-level Tag DTO to an apiv0.TagResponse DTO.
func newTagResponse(t appdto.Tag) apiv0.TagResponse {
	return apiv0.TagResponse{
		ID:      t.ID,
		Name:    t.Name,
		Type:    t.Type,
		Value:   t.Value,
		Quality: t.Quality,
		Version: t.Version,
		Seq:     t.Seq,
	}
}

// newTagStateResponse converts the Runtime node's Tag state DTO, its
// definition's copy included, to an apiv0.TagResponse DTO.
func newTagStateResponse(s runtimeapp.TagState) apiv0.TagResponse {
	return apiv0.TagResponse{
		ID:      s.ID,
		Name:    s.Name,
		Type:    s.Type,
		Value:   s.Value,
		Quality: s.Quality,
		Version: s.Version,
		Seq:     s.Seq,
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

// newUpdateTagResponse converts the written Tag state to an apiv0.UpdateTagResponse DTO.
func newUpdateTagResponse(s runtimeapp.TagState) apiv0.UpdateTagResponse {
	return apiv0.UpdateTagResponse{Seq: s.Seq}
}

// newCreateTagInput converts an apiv0.CreateTagRequest DTO to an app-level CreateTagInput DTO.
func newCreateTagInput(r apiv0.CreateTagRequest) appdto.CreateTagInput {
	return appdto.CreateTagInput{Name: r.Name, Type: r.Type}
}

// newSetTagValueInput converts an apiv0.UpdateTagRequest DTO to the Runtime
// node's input; deviceID is the writer's Device, the zero UUID when a person
// writes.
func newSetTagValueInput(r apiv0.UpdateTagRequest, tagID, deviceID uuid.UUID) runtimeapp.SetTagValueInput {
	return runtimeapp.SetTagValueInput{TagID: tagID, Value: r.Value, Quality: r.Quality, DeviceID: deviceID}
}

func newDeviceResponse(d appdto.Device) apiv0.DeviceResponse {
	return apiv0.DeviceResponse{
		ID:       d.ID,
		Name:     d.Name,
		Comment:  d.Comment,
		Protocol: d.Protocol,
		Config:   apiv0.JSONObject(d.Config),
		Version:  d.Version,
	}
}

func newGetDevicesResponse(list []appdto.Device) apiv0.GetDevicesResponse {
	items := make([]apiv0.DeviceResponse, len(list))
	for i, d := range list {
		items[i] = newDeviceResponse(d)
	}
	return apiv0.GetDevicesResponse{Devices: items}
}

func newCreateDeviceResponse(d appdto.Device) apiv0.CreateDeviceResponse {
	return apiv0.CreateDeviceResponse{ID: d.ID, Version: d.Version}
}

func newUpdateDeviceResponse(d appdto.Device) apiv0.UpdateDeviceResponse {
	return apiv0.UpdateDeviceResponse{Version: d.Version}
}

func newCreateDeviceInput(r apiv0.CreateDeviceRequest) appdto.DeviceInput {
	return appdto.DeviceInput{Name: r.Name, Comment: r.Comment, Protocol: r.Protocol, Config: json.RawMessage(r.Config)}
}

func newUpdateDeviceInput(r apiv0.UpdateDeviceRequest) appdto.DeviceInput {
	return appdto.DeviceInput{Name: r.Name, Comment: r.Comment, Protocol: r.Protocol, Config: json.RawMessage(r.Config)}
}

func newEnvironmentResponse(e appdto.Environment) apiv0.EnvironmentResponse {
	return apiv0.EnvironmentResponse{ID: e.ID, Name: e.Name, Comment: e.Comment, Version: e.Version}
}

func newGetEnvironmentsResponse(list []appdto.Environment) apiv0.GetEnvironmentsResponse {
	items := make([]apiv0.EnvironmentResponse, len(list))
	for i, e := range list {
		items[i] = newEnvironmentResponse(e)
	}
	return apiv0.GetEnvironmentsResponse{Environments: items}
}

func newCreateEnvironmentResponse(e appdto.Environment) apiv0.CreateEnvironmentResponse {
	return apiv0.CreateEnvironmentResponse{ID: e.ID, Version: e.Version}
}

func newUpdateEnvironmentResponse(e appdto.Environment) apiv0.UpdateEnvironmentResponse {
	return apiv0.UpdateEnvironmentResponse{Version: e.Version}
}

func newCreateEnvironmentInput(r apiv0.CreateEnvironmentRequest) appdto.EnvironmentInput {
	return appdto.EnvironmentInput{Name: r.Name, Comment: r.Comment}
}

func newUpdateEnvironmentInput(r apiv0.UpdateEnvironmentRequest) appdto.EnvironmentInput {
	return appdto.EnvironmentInput{Name: r.Name, Comment: r.Comment}
}

func newEnvironmentDeviceResponse(p appdto.EnvironmentDevice) apiv0.EnvironmentDeviceResponse {
	return apiv0.EnvironmentDeviceResponse{
		EnvironmentID: p.EnvironmentID,
		DeviceID:      p.DeviceID,
		Params:        apiv0.JSONObject(p.Params),
		Version:       p.Version,
	}
}

func newGetEnvironmentDevicesResponse(list []appdto.EnvironmentDevice) apiv0.GetEnvironmentDevicesResponse {
	items := make([]apiv0.EnvironmentDeviceResponse, len(list))
	for i, p := range list {
		items[i] = newEnvironmentDeviceResponse(p)
	}
	return apiv0.GetEnvironmentDevicesResponse{Devices: items}
}

func newUpdateEnvironmentDeviceInput(r apiv0.UpdateEnvironmentDeviceRequest) appdto.EnvironmentDeviceInput {
	return appdto.EnvironmentDeviceInput{Params: json.RawMessage(r.Params)}
}

func newDeviceTokenResponse(issued runtimeapp.IssuedDeviceToken) apiv0.DeviceTokenResponse {
	return apiv0.DeviceTokenResponse{Token: issued.Secret}
}

func newDeviceTokenMetadataResponse(t runtimeapp.DeviceToken) apiv0.DeviceTokenMetadataResponse {
	return apiv0.DeviceTokenMetadataResponse{
		DeviceID:    t.DeviceID,
		Fingerprint: t.Fingerprint,
		IssuedAt:    t.IssuedAt.UTC(),
	}
}

func newDeviceConfigResponse(c runtimeapp.DeviceConfig) apiv0.DeviceConfigResponse {
	tags := make([]apiv0.DeviceConfigTagResponse, len(c.Tags))
	for i, t := range c.Tags {
		tags[i] = apiv0.DeviceConfigTagResponse{ID: t.ID, Name: t.Name, Type: t.Type}
	}
	return apiv0.DeviceConfigResponse{
		Device: apiv0.DeviceConfigDeviceResponse{
			ID:       c.Device.ID,
			Name:     c.Device.Name,
			Protocol: c.Device.Protocol,
			Config:   apiv0.JSONObject(c.Device.Config),
		},
		Environment: apiv0.DeviceConfigEnvironmentResponse{ID: c.Environment.ID, Name: c.Environment.Name},
		Connection:  apiv0.DeviceConfigConnectionResponse{Params: apiv0.JSONObject(c.Connection)},
		Tags:        tags,
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
	return apiv0.CreateSceneResponse{ID: s.ID, Version: s.Version}
}

func newUpdateSceneResponse(s appdto.Scene) apiv0.UpdateSceneResponse {
	return apiv0.UpdateSceneResponse{Version: s.Version}
}

func newCreateSceneInput(r apiv0.CreateSceneRequest) appdto.SceneInput {
	return appdto.SceneInput{
		Name:           r.Name,
		Width:          r.Width,
		Height:         r.Height,
		BackgroundHTML: r.BackgroundHTML,
	}
}

func newUpdateSceneInput(r apiv0.UpdateSceneRequest) appdto.SceneInput {
	return appdto.SceneInput{
		Name:           r.Name,
		Width:          r.Width,
		Height:         r.Height,
		BackgroundHTML: r.BackgroundHTML,
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

func newCreateWidgetInput(r apiv0.CreateWidgetRequest) appdto.WidgetInput {
	return appdto.WidgetInput{
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
		Labels:          r.Labels,
		PortBindings:    portBindingDTOsToAppDTOs(r.PortBindings),
	}
}

func newUpdateWidgetInput(r apiv0.UpdateWidgetRequest) appdto.WidgetInput {
	return appdto.WidgetInput{
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
		LibraryID:      wt.LibraryID,
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
	return apiv0.CreateWidgetTypeResponse{ID: wt.ID, Version: wt.Version}
}

func newUpdateWidgetTypeResponse(wt appdto.WidgetType) apiv0.UpdateWidgetTypeResponse {
	return apiv0.UpdateWidgetTypeResponse{Version: wt.Version}
}

func newCreateWidgetTypeInput(r apiv0.CreateWidgetTypeRequest) appdto.WidgetTypeInput {
	return appdto.WidgetTypeInput{
		Name:           r.Name,
		HtmlTemplate:   r.HtmlTemplate,
		Script:         r.Script,
		ScriptLanguage: r.ScriptLanguage,
		DefaultWidth:   r.DefaultWidth,
		DefaultHeight:  r.DefaultHeight,
		InputPorts:     inputPortDTOsToAppDTOs(r.InputPorts),
	}
}

func newUpdateWidgetTypeInput(r apiv0.UpdateWidgetTypeRequest) appdto.WidgetTypeInput {
	return appdto.WidgetTypeInput{
		Name:           r.Name,
		HtmlTemplate:   r.HtmlTemplate,
		Script:         r.Script,
		ScriptLanguage: r.ScriptLanguage,
		DefaultWidth:   r.DefaultWidth,
		DefaultHeight:  r.DefaultHeight,
		InputPorts:     inputPortDTOsToAppDTOs(r.InputPorts),
	}
}

func newReleasedWidgetTypeResponse(wt appdto.ReleasedWidgetType) apiv0.ReleasedWidgetTypeResponse {
	return apiv0.ReleasedWidgetTypeResponse{
		ID:             wt.ID,
		LibraryID:      wt.LibraryID,
		Name:           wt.Name,
		HtmlTemplate:   wt.HtmlTemplate,
		Script:         wt.Script,
		ScriptLanguage: wt.ScriptLanguage,
		DefaultWidth:   wt.DefaultWidth,
		DefaultHeight:  wt.DefaultHeight,
		InputPorts:     appInputPortsToRest(wt.InputPorts),
	}
}

func newReleasedWidgetTypeResponses(list []appdto.ReleasedWidgetType) []apiv0.ReleasedWidgetTypeResponse {
	items := make([]apiv0.ReleasedWidgetTypeResponse, len(list))
	for i, wt := range list {
		items[i] = newReleasedWidgetTypeResponse(wt)
	}
	return items
}

func newLibraryResponse(l appdto.Library) apiv0.LibraryResponse {
	return apiv0.LibraryResponse{
		ID:                   l.ID,
		Name:                 l.Name,
		Version:              l.Version,
		LatestRelease:        l.LatestRelease,
		DiscardingTo:         l.DiscardingTo,
		HasUnreleasedChanges: l.HasUnreleasedChanges,
	}
}

func newGetLibrariesResponse(list []appdto.Library) apiv0.GetLibrariesResponse {
	items := make([]apiv0.LibraryResponse, len(list))
	for i, l := range list {
		items[i] = newLibraryResponse(l)
	}
	return apiv0.GetLibrariesResponse{Libraries: items}
}

func newLibraryReleaseResponse(r appdto.LibraryRelease) apiv0.LibraryReleaseResponse {
	return apiv0.LibraryReleaseResponse{
		LibraryID:   r.LibraryID,
		Number:      r.Number,
		Note:        r.Note,
		CreatedAt:   r.CreatedAt.UTC(),
		WidgetTypes: newReleasedWidgetTypeResponses(r.WidgetTypes),
	}
}

func newGetLibraryReleasesResponse(list []appdto.LibraryRelease) apiv0.GetLibraryReleasesResponse {
	items := make([]apiv0.LibraryReleaseResponse, len(list))
	for i, r := range list {
		items[i] = newLibraryReleaseResponse(r)
	}
	return apiv0.GetLibraryReleasesResponse{Releases: items}
}

func newLibraryPinResponse(p appdto.LibraryPin) apiv0.LibraryPinResponse {
	return apiv0.LibraryPinResponse{LibraryID: p.LibraryID, Release: p.Release, Version: p.Version}
}

func newGetLibraryPinsResponse(list []appdto.LibraryPin) apiv0.GetLibraryPinsResponse {
	items := make([]apiv0.LibraryPinResponse, len(list))
	for i, p := range list {
		items[i] = newLibraryPinResponse(p)
	}
	return apiv0.GetLibraryPinsResponse{LibraryPins: items}
}

func newPinnedWidgetResponse(w appdto.PinnedWidget) apiv0.PinnedWidgetResponse {
	return apiv0.PinnedWidgetResponse{SceneID: w.SceneID, WidgetID: w.WidgetID, Name: w.Name, TypeID: w.TypeID}
}

func newPinImpactResponse(i appdto.PinImpact) apiv0.PinImpactResponse {
	resp := apiv0.PinImpactResponse{
		OrphanedWidgets: make([]apiv0.PinnedWidgetResponse, len(i.Orphaned)),
		BrokenBindings:  make([]apiv0.BrokenBindingResponse, len(i.Broken)),
	}
	for n, w := range i.Orphaned {
		resp.OrphanedWidgets[n] = newPinnedWidgetResponse(w)
	}
	for n, b := range i.Broken {
		resp.BrokenBindings[n] = apiv0.BrokenBindingResponse{
			PinnedWidgetResponse: newPinnedWidgetResponse(b.PinnedWidget),
			PortName:             b.PortName,
			TagID:                b.TagID,
		}
	}
	return resp
}

// deployedAt formats a time as the contract states it: RFC 3339 in UTC.
func deployedAt(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func newRevisionResponse(revision runtimeapp.Revision) apiv0.RevisionResponse {
	return apiv0.RevisionResponse{
		Number:         revision.Number,
		DeployedAt:     deployedAt(revision.DeployedAt),
		Cause:          apiv0.RevisionCause(revision.Cause),
		RolledBackFrom: revision.RolledBackFrom,
		Content:        revision.Content,
	}
}

func newDeployResponse(revision appdto.Revision) apiv0.DeployResponse {
	return apiv0.DeployResponse{Number: revision.Number, DeployedAt: deployedAt(revision.DeployedAt)}
}

func newRevisionSummaryResponse(revision appdto.Revision) apiv0.RevisionSummaryResponse {
	return newRevisionSummary(runtimeapp.RevisionSummary{
		Number:         revision.Number,
		DeployedAt:     revision.DeployedAt,
		Cause:          runtimeapp.RevisionCause(revision.Cause),
		RolledBackFrom: revision.RolledBackFrom,
	})
}

func newRollbackResponse(revision runtimeapp.Revision) apiv0.RevisionSummaryResponse {
	return newRevisionSummary(revision.Summary())
}

func newRevisionSummary(summary runtimeapp.RevisionSummary) apiv0.RevisionSummaryResponse {
	return apiv0.RevisionSummaryResponse{
		Number:         summary.Number,
		DeployedAt:     deployedAt(summary.DeployedAt),
		Cause:          apiv0.RevisionCause(summary.Cause),
		RolledBackFrom: summary.RolledBackFrom,
	}
}

func newListRevisionsResponse(summaries []runtimeapp.RevisionSummary) apiv0.ListRevisionsResponse {
	resp := apiv0.ListRevisionsResponse{Revisions: make([]apiv0.RevisionSummaryResponse, len(summaries))}
	for i, summary := range summaries {
		resp.Revisions[i] = newRevisionSummary(summary)
	}
	return resp
}

func newDeployPreviewResponse(preview appdto.DeployPreview) apiv0.DeployPreviewResponse {
	resp := apiv0.DeployPreviewResponse{
		Unchanged:      preview.Unchanged,
		DroppedWidgets: make([]apiv0.DroppedWidgetResponse, len(preview.DroppedWidgets)),
	}
	if preview.CurrentRevision != nil {
		summary := newRevisionSummaryResponse(*preview.CurrentRevision)
		resp.CurrentRevision = &summary
	}
	for i, dropped := range preview.DroppedWidgets {
		resp.DroppedWidgets[i] = apiv0.DroppedWidgetResponse{
			SceneID: dropped.SceneID, WidgetID: dropped.WidgetID, Name: dropped.Name,
		}
	}
	return resp
}

func newGetTagStatesResponse(states []runtimeapp.TagState) apiv0.GetTagStatesResponse {
	resp := apiv0.GetTagStatesResponse{TagStates: make(map[string]apiv0.TagStateResponse, len(states))}
	for _, state := range states {
		resp.TagStates[state.ID.String()] = apiv0.TagStateResponse{
			Value: state.Value, Quality: state.Quality, Seq: state.Seq,
		}
	}
	return resp
}
