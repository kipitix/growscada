package apiv0

import (
	"testing"

	"github.com/kipitix/growscada/contract/internal/jsonschema"
)

func TestSchemaIsCommitted(t *testing.T) {
	jsonschema.CheckCommitted(t, "../../../schemas/api/"+SchemaVersion().String()+".json", jsonschema.Document{
		ID:    "https://github.com/kipitix/growscada/schemas/api/" + SchemaVersion().String() + ".json",
		Title: "GrowSCADA server API " + SchemaVersion().String(),
		Tag:   "json",
		Roots: []any{
			TagResponse{}, GetTagsResponse{}, CreateTagRequest{}, CreateTagResponse{}, UpdateTagRequest{}, UpdateTagResponse{},
			SceneResponse{}, GetScenesResponse{}, CreateSceneRequest{}, CreateSceneResponse{}, UpdateSceneRequest{}, UpdateSceneResponse{},
			WidgetResponse{}, GetWidgetsResponse{}, CreateWidgetRequest{}, CreateWidgetResponse{}, UpdateWidgetRequest{}, UpdateWidgetResponse{},
			WidgetTypeResponse{}, GetWidgetTypesResponse{}, CreateWidgetTypeRequest{}, CreateWidgetTypeResponse{}, UpdateWidgetTypeRequest{}, UpdateWidgetTypeResponse{},
			ProjectResponse{}, GetProjectsResponse{}, CreateProjectRequest{}, CreateProjectResponse{}, UpdateProjectRequest{}, UpdateProjectResponse{},
			DeviceResponse{}, GetDevicesResponse{}, CreateDeviceRequest{}, CreateDeviceResponse{}, UpdateDeviceRequest{}, UpdateDeviceResponse{},
			EnvironmentResponse{}, GetEnvironmentsResponse{}, CreateEnvironmentRequest{}, CreateEnvironmentResponse{}, UpdateEnvironmentRequest{}, UpdateEnvironmentResponse{},
			EnvironmentDeviceResponse{}, GetEnvironmentDevicesResponse{}, UpdateEnvironmentDeviceRequest{},
			DeviceTokenResponse{}, DeviceTokenMetadataResponse{},
			DeviceConfigResponse{}, DeviceConfigDeviceResponse{}, DeviceConfigEnvironmentResponse{}, DeviceConfigConnectionResponse{}, DeviceConfigTagResponse{},
			DeviceIdentityResponse{},
			RevisionResponse{}, RevisionSummaryResponse{}, ListRevisionsResponse{}, DroppedWidgetResponse{}, DeployPreviewResponse{},
			DeployResponse{}, DiscardProjectResponse{}, TagStateResponse{}, GetTagStatesResponse{},
			DraftChangesResponse{}, DraftChangeCommandRequest{},
			EditLockResponse{}, EditLockHolderResponse{}, CheckOutEditLockRequest{}, TakeOverEditLockRequest{},
			Event{}, Problem{},
		},
	})
}
