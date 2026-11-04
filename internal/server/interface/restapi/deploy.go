package restapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
)

// DeployHandlers handles the deployment of a Project: the Revision it runs,
// the snapshot of its Tag states, the preview and the act of Deploy, and
// Discard (task 35, part 5). The Revision and the Tag states are the Runtime
// node's; Deploy and Discard are the Engineering node's.
type DeployHandlers struct {
	deploy    application.DeployService
	discard   application.DiscardService
	revisions runtimeapp.RevisionService
	states    runtimeapp.TagStateService
}

// NewDeployHandler creates the handler over the Engineering node's Deploy and
// Discard services and the Runtime node's Revisions and Tag states.
func NewDeployHandler(deploy application.DeployService, discard application.DiscardService,
	revisions runtimeapp.RevisionService, states runtimeapp.TagStateService) *DeployHandlers {
	return &DeployHandlers{deploy: deploy, discard: discard, revisions: revisions, states: states}
}

// GetRevision handles GET /projects/{project_id}/revision: the Revision the
// Project runs, with its ProjectFile. 404 until the Project's first Deploy.
func (h DeployHandlers) GetRevision(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	revision, deployed, err := h.revisions.FindCurrent(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	if !deployed {
		sendProblem(w, NewNotFound("Revision of project", projectID.String(), r.URL.Path))
		return
	}
	sendJSONResponse(w, http.StatusOK, newRevisionResponse(revision))
}

// GetTagStates handles GET /projects/{project_id}/tag-states: the states of
// the Tags of the running Revision, by Tag ID — the snapshot Operation renders
// before tag_updated arrives. 404 until the Project's first Deploy.
func (h DeployHandlers) GetTagStates(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	revision, deployed, err := h.revisions.FindCurrent(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	if !deployed {
		sendProblem(w, NewNotFound("Revision of project", projectID.String(), r.URL.Path))
		return
	}
	tagIDs := make([]uuid.UUID, len(revision.Content.Tags))
	for i, definition := range revision.Content.Tags {
		tagIDs[i] = definition.ID
	}
	found, err := h.states.FindTagStates(r.Context(), projectID, tagIDs)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetTagStatesResponse(found))
}

// GetDeployPreview handles GET /projects/{project_id}/deploy/preview: the
// Revision the Project runs, whether its Draft is that Revision unchanged, and
// the runtime Widgets a Deploy would drop.
func (h DeployHandlers) GetDeployPreview(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	preview, err := h.deploy.Preview(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newDeployPreviewResponse(preview))
}

// PostDeploy handles POST /projects/{project_id}/deploy: it makes the Draft
// the new Revision of the Project. 201 with the new Revision; 409
// nothing_to_deploy when the Draft is the Revision the Project runs; 500 when
// the Revision was made but the Engineering node's copy of it was not saved.
func (h DeployHandlers) PostDeploy(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	revision, err := h.deploy.Deploy(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusCreated, newDeployResponse(revision))
}

// PostDiscard handles POST /projects/{project_id}/discard: it marks the Project
// as being reset to the Revision it runs and answers at once (202); the Draft
// is read-only until the Discard ends. 409 without a Revision.
func (h DeployHandlers) PostDiscard(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	if _, err := h.discard.Discard(r.Context(), projectID); err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusAccepted, apiv0.DiscardProjectResponse{Discarding: true})
}

// deployedAt formats a time as the contract states it: RFC 3339 in UTC.
func deployedAt(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func newRevisionResponse(revision runtimeapp.Revision) apiv0.RevisionResponse {
	return apiv0.RevisionResponse{
		Number:     revision.Number,
		DeployedAt: deployedAt(revision.DeployedAt),
		Content:    revision.Content,
	}
}

func newDeployResponse(revision appdto.Revision) apiv0.DeployResponse {
	return apiv0.DeployResponse{Number: revision.Number, DeployedAt: deployedAt(revision.DeployedAt)}
}

func newRevisionSummaryResponse(revision appdto.Revision) apiv0.RevisionSummaryResponse {
	return apiv0.RevisionSummaryResponse{Number: revision.Number, DeployedAt: deployedAt(revision.DeployedAt)}
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
