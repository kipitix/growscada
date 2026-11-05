package restapi_test

import (
	"context"
	"net/http"
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
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

// TestRevisions_ListDetailRollback drives the three routes of task 36: the
// list (an empty array before the first Deploy), the detail with its document,
// and a Rollback that appends a copy of an earlier Revision — with the
// refusals a bad number, a missing one and the running document get.
func TestRevisions_ListDetailRollback(t *testing.T) {
	cleanProjectsRest(t)
	router := newServices().router()

	rec := serve(router, http.MethodGet, mainPath+"/revisions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET revisions before the Deploy: %d %s", rec.Code, rec.Body)
	}
	if list := decodeBody[apiv0.ListRevisionsResponse](t, rec); list.Revisions == nil || len(list.Revisions) != 0 {
		t.Errorf("the list of a project without a Revision: %+v", list.Revisions)
	}
	if rec := serve(router, http.MethodGet, mainPath+"/revisions/1", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET a Revision of a project without one: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/revisions/1/rollback", nil); rec.Code != http.StatusNotFound {
		t.Errorf("Rollback of a project without a Revision: %d, want 404", rec.Code)
	}

	rec = serve(router, http.MethodPost, mainPath+"/tags", apiv0.CreateTagRequest{Name: "speed", Type: "integer"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST tag: %d %s", rec.Code, rec.Body)
	}
	first := decodeBody[apiv0.CreateTagResponse](t, rec)
	if rec := serve(router, http.MethodPost, mainPath+"/deploy", nil); rec.Code != http.StatusCreated {
		t.Fatalf("POST deploy: %d %s", rec.Code, rec.Body)
	}

	rec = serve(router, http.MethodGet, mainPath+"/revisions", nil)
	list := decodeBody[apiv0.ListRevisionsResponse](t, rec)
	if len(list.Revisions) != 1 || list.Revisions[0].Number != 1 || list.Revisions[0].Cause != apiv0.RevisionCauseDeploy {
		t.Fatalf("the list after the first Deploy: %+v", list.Revisions)
	}
	if list.Revisions[0].RolledBackFrom != nil {
		t.Errorf("a Deploy names a Revision it copied: %v", list.Revisions[0].RolledBackFrom)
	}

	if rec := serve(router, http.MethodGet, mainPath+"/revisions/abc", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("GET a revision number that is not a number: %d, want 400", rec.Code)
	}

	// The second Revision differs from the first: another Tag.
	rec = serve(router, http.MethodPost, mainPath+"/tags", apiv0.CreateTagRequest{Name: "flow", Type: "integer"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST the second tag: %d %s", rec.Code, rec.Body)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/deploy", nil); rec.Code != http.StatusCreated {
		t.Fatalf("the second deploy: %d %s", rec.Code, rec.Body)
	}

	rec = serve(router, http.MethodGet, mainPath+"/revisions/1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET revision 1: %d %s", rec.Code, rec.Body)
	}
	firstRevision := decodeBody[apiv0.RevisionResponse](t, rec)
	if firstRevision.Number != 1 || firstRevision.Cause != apiv0.RevisionCauseDeploy {
		t.Errorf("revision 1: %+v", firstRevision)
	}
	if len(firstRevision.Content.Tags) != 1 || firstRevision.Content.Tags[0].ID != first.ID {
		t.Errorf("revision 1 carries %+v, want only the first tag", firstRevision.Content.Tags)
	}
	if rec := serve(router, http.MethodGet, mainPath+"/revisions/99", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET a missing revision: %d, want 404", rec.Code)
	}

	// The running Revision's own document is nothing to roll back to.
	if rec := serve(router, http.MethodPost, mainPath+"/revisions/2/rollback", nil); rec.Code != http.StatusConflict {
		t.Errorf("Rollback to the running revision: %d, want 409", rec.Code)
	}

	rec = serve(router, http.MethodPost, mainPath+"/revisions/1/rollback", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Rollback to revision 1: %d %s", rec.Code, rec.Body)
	}
	rolled := decodeBody[apiv0.RevisionSummaryResponse](t, rec)
	if rolled.Number != 3 || rolled.Cause != apiv0.RevisionCauseRollback {
		t.Errorf("the rolled-back revision: %+v", rolled)
	}
	if rolled.RolledBackFrom == nil || *rolled.RolledBackFrom != 1 {
		t.Errorf("the rollback names %v, want revision 1", rolled.RolledBackFrom)
	}

	rec = serve(router, http.MethodGet, mainPath+"/revision", nil)
	running := decodeBody[apiv0.RevisionResponse](t, rec)
	if rec.Code != http.StatusOK || running.Number != 3 || running.Cause != apiv0.RevisionCauseRollback {
		t.Fatalf("the running revision after the Rollback: %d %+v", rec.Code, running)
	}
	if len(running.Content.Tags) != 1 || running.Content.Tags[0].ID != first.ID {
		t.Errorf("the running document is %+v, want the copy of revision 1", running.Content.Tags)
	}

	rec = serve(router, http.MethodGet, mainPath+"/revisions", nil)
	list = decodeBody[apiv0.ListRevisionsResponse](t, rec)
	if len(list.Revisions) != 3 || list.Revisions[0].Number != 3 || list.Revisions[0].Cause != apiv0.RevisionCauseRollback {
		t.Errorf("the list after the Rollback: %+v", list.Revisions)
	}
}

// TestRevisions_OfADeletedProject_Are404: a deleted Project keeps its rows
// until its cleanup removes them, but they are not its Revisions to serve.
func TestRevisions_OfADeletedProject_Are404(t *testing.T) {
	cleanProjectsRest(t)
	router := newServices().router()
	if rec := serve(router, http.MethodPost, mainPath+"/deploy", nil); rec.Code != http.StatusCreated {
		t.Fatalf("POST deploy: %d %s", rec.Code, rec.Body)
	}
	if _, err := testDB().ExecContext(editlock.System(context.Background()),
		"UPDATE projects SET deleted_at = now() WHERE id = '"+mainProject.String()+"'"); err != nil {
		t.Fatalf("mark the project deleted: %v", err)
	}
	t.Cleanup(func() {
		if _, err := testDB().ExecContext(editlock.System(context.Background()),
			"UPDATE projects SET deleted_at = NULL WHERE id = '"+mainProject.String()+"'"); err != nil {
			t.Errorf("clear the deletion mark: %v", err)
		}
	})

	if rec := serve(router, http.MethodGet, mainPath+"/revisions", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET revisions of a deleted project: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodGet, mainPath+"/revisions/1", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET revision 1 of a deleted project: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/revisions/1/rollback", nil); rec.Code != http.StatusNotFound {
		t.Errorf("Rollback of a deleted project: %d, want 404", rec.Code)
	}
}

func TestDeploy_Discard(t *testing.T) {
	cleanProjectsRest(t)
	router := newServices().router()

	if rec := serve(router, http.MethodPost, mainPath+"/discard", nil); rec.Code != http.StatusConflict {
		t.Fatalf("POST discard without a Revision: %d, want 409", rec.Code)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/deploy", nil); rec.Code != http.StatusCreated {
		t.Fatalf("POST deploy: %d %s", rec.Code, rec.Body)
	}

	rec := serve(router, http.MethodPost, mainPath+"/discard", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST discard: %d %s", rec.Code, rec.Body)
	}
	discarded := decodeBody[apiv0.DiscardProjectResponse](t, rec)
	if discarded.Version == 0 {
		t.Errorf("POST discard states no draft version: %+v", discarded)
	}
}
