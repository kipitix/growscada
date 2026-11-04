package restapi_test

import (
	"context"
	"net/http"
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

func TestDeploy_RevisionPreviewAndTagStates(t *testing.T) {
	cleanProjectsRest(t)
	router := newServices().router()

	if rec := serve(router, http.MethodGet, mainPath+"/revision", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("GET revision before Deploy: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodGet, mainPath+"/tag-states", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("GET tag-states before Deploy: %d, want 404", rec.Code)
	}

	rec := serve(router, http.MethodGet, mainPath+"/deploy/preview", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET preview before Deploy: %d %s", rec.Code, rec.Body)
	}
	preview := decodeBody[apiv0.DeployPreviewResponse](t, rec)
	if preview.CurrentRevision != nil || preview.Unchanged || preview.DroppedWidgets == nil || len(preview.DroppedWidgets) != 0 {
		t.Errorf("preview before Deploy: %+v", preview)
	}

	rec = serve(router, http.MethodPost, mainPath+"/tags", apiv0.CreateTagRequest{Name: "speed", Type: "integer"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST tag: %d %s", rec.Code, rec.Body)
	}
	createdTag := decodeBody[apiv0.CreateTagResponse](t, rec)

	rec = serve(router, http.MethodPost, mainPath+"/deploy", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST deploy: %d %s", rec.Code, rec.Body)
	}
	deployed := decodeBody[apiv0.DeployResponse](t, rec)
	if deployed.Number != 1 || deployed.DeployedAt == "" {
		t.Errorf("the first Deploy: %+v", deployed)
	}

	rec = serve(router, http.MethodGet, mainPath+"/revision", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET revision: %d %s", rec.Code, rec.Body)
	}
	revision := decodeBody[apiv0.RevisionResponse](t, rec)
	if revision.Number != 1 || revision.DeployedAt == "" || revision.Content.ProjectID != mainProject {
		t.Errorf("GET revision: %+v", revision)
	}
	if len(revision.Content.Tags) == 0 {
		t.Errorf("the Revision holds no Tag definitions: %+v", revision.Content)
	}

	rec = serve(router, http.MethodGet, mainPath+"/tag-states", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET tag-states: %d %s", rec.Code, rec.Body)
	}
	states := decodeBody[apiv0.GetTagStatesResponse](t, rec)
	if len(states.TagStates) == 0 {
		t.Errorf("the snapshot holds no Tag states: %+v", states)
	}
	if state, ok := states.TagStates[createdTag.ID.String()]; !ok || state.Quality != "bad" || state.Seq != 0 {
		t.Errorf("the initial state of the new Tag: %+v, %v", state, ok)
	}

	rec = serve(router, http.MethodGet, mainPath+"/deploy/preview", nil)
	preview = decodeBody[apiv0.DeployPreviewResponse](t, rec)
	if preview.CurrentRevision == nil || preview.CurrentRevision.Number != 1 || !preview.Unchanged {
		t.Errorf("preview after Deploy: %+v", preview)
	}

	if rec := serve(router, http.MethodPost, mainPath+"/deploy", nil); rec.Code != http.StatusConflict {
		t.Errorf("a second Deploy of the same Draft: %d, want 409", rec.Code)
	}
}

func TestDeploy_Discard(t *testing.T) {
	cleanProjectsRest(t)
	t.Cleanup(func() {
		if _, err := testDB().ExecContext(context.Background(),
			"UPDATE projects SET discarding = FALSE WHERE id = '"+mainProject.String()+"'"); err != nil {
			t.Errorf("clear the Discard mark: %v", err)
		}
	})
	router := newServices().router()

	if rec := serve(router, http.MethodPost, mainPath+"/discard", nil); rec.Code != http.StatusConflict {
		t.Fatalf("POST discard without a Revision: %d, want 409", rec.Code)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/deploy", nil); rec.Code != http.StatusCreated {
		t.Fatalf("POST deploy: %d %s", rec.Code, rec.Body)
	}

	rec := serve(router, http.MethodPost, mainPath+"/discard", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST discard: %d %s", rec.Code, rec.Body)
	}
	discarded := decodeBody[apiv0.DiscardProjectResponse](t, rec)
	if !discarded.Discarding {
		t.Errorf("POST discard: %+v", discarded)
	}
}
