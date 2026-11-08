package restapi_test

import (
	"context"
	"net/http"
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// TestReleases_Project covers the Release routes of a Project (task 72_3;
// ADR 0021): the publication of a Commit, its refusals, the list, one Release,
// the note change and the ProjectFile download.
func TestReleases_Project(t *testing.T) {
	cleanProjectsRest(t)
	if _, err := testDB().ExecContext(context.Background(),
		"DELETE FROM engineering.drafts"); err != nil {
		t.Fatalf("clean drafts: %v", err)
	}
	services := newServices()
	router := services.router()

	if rec := serve(router, http.MethodGet, mainPath+"/releases", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET releases: %d %s", rec.Code, rec.Body)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/releases", apiv0.CreateProjectReleaseRequest{
		Commit: 9, Author: "Grace",
	}); rec.Code != http.StatusNotFound {
		t.Errorf("a release of a missing commit: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/releases", apiv0.CreateProjectReleaseRequest{
		Commit: 0, Author: "Grace",
	}); rec.Code != http.StatusBadRequest {
		t.Errorf("a release of commit 0: %d, want 400", rec.Code)
	}

	if rec := serve(router, http.MethodPost, mainPath+"/commits", apiv0.CreateCommitRequest{
		Version: 1, Title: "First save", Author: "Ada",
	}); rec.Code != http.StatusCreated {
		t.Fatalf("POST commit: %d %s", rec.Code, rec.Body)
	}
	rec := serve(router, http.MethodPost, mainPath+"/releases", apiv0.CreateProjectReleaseRequest{
		Commit: 1, Note: " for the demo ", Author: "Grace",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST release: %d %s", rec.Code, rec.Body)
	}
	created := decodeBody[apiv0.ReleaseResponse](t, rec)
	if created.Number != 1 || created.CommitNumber != 1 || created.Title != "First save" ||
		created.Note != "for the demo" || created.Author != "Grace" || created.Origin != apiv0.ReleaseOriginCommit {
		t.Errorf("the created release: %+v", created)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/releases", apiv0.CreateProjectReleaseRequest{
		Commit: 1, Author: "Grace",
	}); rec.Code != http.StatusConflict {
		t.Errorf("a second release of a commit: %d, want 409", rec.Code)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/releases", apiv0.CreateProjectReleaseRequest{
		Commit: 1, Author: "",
	}); rec.Code != http.StatusBadRequest {
		t.Errorf("a release without an author: %d, want 400", rec.Code)
	}

	list := decodeBody[apiv0.GetReleasesResponse](t, serve(router, http.MethodGet, mainPath+"/releases", nil))
	if len(list.Releases) != 1 || list.Releases[0].Number != 1 {
		t.Errorf("releases: %+v", list)
	}
	found := decodeBody[apiv0.ReleaseResponse](t, serve(router, http.MethodGet, mainPath+"/releases/1", nil))
	if found.Note != "for the demo" {
		t.Errorf("the read release: %+v", found)
	}
	if rec := serve(router, http.MethodGet, mainPath+"/releases/9", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET a missing release: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodGet, mainPath+"/releases/not-a-number", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("GET a release by a non-number: %d, want 400", rec.Code)
	}

	rec = serve(router, http.MethodPatch, mainPath+"/releases/1", apiv0.UpdateReleaseNoteRequest{Note: "after the demo"})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH release: %d %s", rec.Code, rec.Body)
	}
	if updated := decodeBody[apiv0.ReleaseResponse](t, rec); updated.Note != "after the demo" {
		t.Errorf("the updated release: %+v", updated)
	}
	if rec := serve(router, http.MethodPatch, mainPath+"/releases/9", apiv0.UpdateReleaseNoteRequest{Note: "x"}); rec.Code != http.StatusNotFound {
		t.Errorf("PATCH a missing release: %d, want 404", rec.Code)
	}

	rec = serve(router, http.MethodGet, mainPath+"/releases/1/projectfile", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET the release's project file: %d %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("project file content type %q", ct)
	}
	if rec := serve(router, http.MethodGet, mainPath+"/releases/9/projectfile", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET a missing release's project file: %d, want 404", rec.Code)
	}

	commits := decodeBody[apiv0.GetCommitsResponse](t, serve(router, http.MethodGet, mainPath+"/commits", nil))
	if len(commits.Commits) != 1 || !commits.Commits[0].Released {
		t.Errorf("the history does not mark the published commit: %+v", commits)
	}
}
