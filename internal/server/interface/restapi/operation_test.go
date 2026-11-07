package restapi_test

import (
	"context"
	"net/http"
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
)

// TestOperation_StateReadsTheSnapshot walks the Operation read model through
// its states: a live Project before its first Deploy answers 200 with no
// Revision, a Deploy brings the document and the initial states of its Tags,
// a value write moves both the state and the Journal position, and a deleted
// Project is 404 (task 43, ADR 0020).
func TestOperation_StateReadsTheSnapshot(t *testing.T) {
	cleanProjectsRest(t)
	router := newServices().router()

	rec := serve(router, http.MethodGet, mainPath+"/operation", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET operation before Deploy: %d %s", rec.Code, rec.Body)
	}
	before := decodeBody[apiv0.OperationStateResponse](t, rec)
	if before.Revision != nil {
		t.Errorf("GET operation before Deploy carries a Revision: %+v", before.Revision)
	}
	if before.TagStates == nil || len(before.TagStates) != 0 {
		t.Errorf("GET operation before Deploy has tag_states %+v, want an empty object", before.TagStates)
	}

	rec = serve(router, http.MethodPost, mainPath+"/tags", apiv0.CreateTagRequest{Name: "speed", Type: "integer"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST tag: %d %s", rec.Code, rec.Body)
	}
	created := decodeBody[apiv0.CreateTagResponse](t, rec)
	if rec := serve(router, http.MethodPost, mainPath+"/deploy", nil); rec.Code != http.StatusCreated {
		t.Fatalf("POST deploy: %d %s", rec.Code, rec.Body)
	}

	rec = serve(router, http.MethodGet, mainPath+"/operation", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET operation after Deploy: %d %s", rec.Code, rec.Body)
	}
	after := decodeBody[apiv0.OperationStateResponse](t, rec)
	if after.Revision == nil || after.Revision.Number != 1 || after.Revision.Content.ProjectID != mainProject {
		t.Fatalf("GET operation after Deploy: %+v", after.Revision)
	}
	if len(after.Revision.Content.Tags) != 1 || after.Revision.Content.Tags[0].ID != created.ID {
		t.Errorf("the Revision of the snapshot carries %+v", after.Revision.Content.Tags)
	}
	initial, ok := after.TagStates[created.ID.String()]
	if !ok || initial.Value != "0" || initial.Quality != "bad" || initial.Seq != 0 || initial.WrittenAt != nil {
		t.Errorf("the initial state of the new Tag: %+v, %v", initial, ok)
	}
	if after.Position <= 0 {
		t.Errorf("the position after Deploy is %d, want the revision entry's", after.Position)
	}

	rec = serve(router, http.MethodPatch, mainPath+"/tags/"+created.ID.String()+"/value",
		apiv0.UpdateTagRequest{Value: "42", Quality: "good"})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH tag value: %d %s", rec.Code, rec.Body)
	}
	written := decodeBody[apiv0.UpdateTagResponse](t, rec)

	rec = serve(router, http.MethodGet, mainPath+"/operation", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET operation after the write: %d %s", rec.Code, rec.Body)
	}
	afterWrite := decodeBody[apiv0.OperationStateResponse](t, rec)
	state := afterWrite.TagStates[created.ID.String()]
	if state.Value != "42" || state.Quality != "good" || state.Seq != written.Seq {
		t.Errorf("the state after the write: %+v, want the written one", state)
	}
	if afterWrite.Position != written.Seq {
		t.Errorf("the position after the write is %d, want the state's %d", afterWrite.Position, written.Seq)
	}
}

// TestOperation_OfADeletedProject_Is404: a deleted Project keeps its rows until
// its cleanup removes them, but its Operation state is not served.
func TestOperation_OfADeletedProject_Is404(t *testing.T) {
	cleanProjectsRest(t)
	router := newServices().router()
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

	if rec := serve(router, http.MethodGet, mainPath+"/operation", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET operation of a deleted project: %d, want 404", rec.Code)
	}
}
