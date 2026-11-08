package restapi_test

import (
	"context"
	"net/http"
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application/appdto"
)

// TestCommits_ProjectSaveListFindRename covers the Commit routes of a
// Project (task 72_2; ADR 0021): a Save, its refusals, the history, one
// Commit, the rename and the ProjectFile download.
func TestCommits_ProjectSaveListFindRename(t *testing.T) {
	cleanProjectsRest(t)
	if _, err := testDB().ExecContext(context.Background(),
		"DELETE FROM engineering.drafts"); err != nil {
		t.Fatalf("clean drafts: %v", err)
	}
	services := newServices()
	router := services.router()

	if rec := serve(router, http.MethodGet, mainPath+"/commits", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET commits: %d %s", rec.Code, rec.Body)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/commits", apiv0.CreateCommitRequest{
		Version: 1, Title: "  First save ", Author: " Ada ",
	}); rec.Code != http.StatusCreated {
		t.Fatalf("POST commit: %d %s", rec.Code, rec.Body)
	}
	created := decodeBody[apiv0.CommitResponse](t, serve(router, http.MethodGet, mainPath+"/commits/1", nil))
	if created.Number != 1 || created.Title != "First save" || created.Author != "Ada" || created.Released {
		t.Errorf("the saved commit: %+v", created)
	}

	if rec := serve(router, http.MethodPost, mainPath+"/commits", apiv0.CreateCommitRequest{
		Version: 1, Title: "Again", Author: "Ada",
	}); rec.Code != http.StatusConflict {
		t.Errorf("a Save without changes: %d, want 409", rec.Code)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/commits", apiv0.CreateCommitRequest{
		Version: 1, Title: "", Author: "Ada",
	}); rec.Code != http.StatusBadRequest {
		t.Errorf("a Save without a title: %d, want 400", rec.Code)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/commits", apiv0.CreateCommitRequest{
		Version: 0, Title: "Stale", Author: "Ada",
	}); rec.Code != http.StatusConflict {
		t.Errorf("a Save at a stale version: %d, want 409", rec.Code)
	}

	rec := serve(router, http.MethodGet, mainPath+"/commits", nil)
	list := decodeBody[apiv0.GetCommitsResponse](t, rec)
	if len(list.Commits) != 1 || list.Commits[0].Number != 1 {
		t.Errorf("history: %+v", list)
	}
	if rec := serve(router, http.MethodGet, mainPath+"/commits/9", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET a missing commit: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodGet, mainPath+"/commits/not-a-number", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("GET a commit by a non-number: %d, want 400", rec.Code)
	}

	rec = serve(router, http.MethodPatch, mainPath+"/commits/1", apiv0.RenameCommitRequest{Title: "Before the demo"})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH commit: %d %s", rec.Code, rec.Body)
	}
	if renamed := decodeBody[apiv0.CommitResponse](t, rec); renamed.Title != "Before the demo" {
		t.Errorf("renamed commit: %+v", renamed)
	}
	if rec := serve(router, http.MethodPatch, mainPath+"/commits/1", apiv0.RenameCommitRequest{Title: " "}); rec.Code != http.StatusBadRequest {
		t.Errorf("PATCH with an empty title: %d, want 400", rec.Code)
	}

	rec = serve(router, http.MethodGet, mainPath+"/commits/1/projectfile", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET the commit's project file: %d %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("project file content type %q", ct)
	}
}

// TestCommits_LibrarySaveAndRead covers the Commit routes of a Library
// (task 72_2; ADR 0021).
func TestCommits_LibrarySaveAndRead(t *testing.T) {
	services := newServices()
	router := services.router()
	created, err := services.libraries.CreateLibrary(context.Background(),
		appdto.LibraryInput{Name: "commits-rest-" + t.Name()})
	if err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}
	path := apiv0.PathPrefix + apiv0.LibraryPath(created.ID.String())

	if rec := serve(router, http.MethodGet, path+"/commits", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET library commits: %d %s", rec.Code, rec.Body)
	}
	if _, err := services.widgetTypes.CreateWidgetType(context.Background(), created.ID, 1, appdto.WidgetTypeInput{
		Name: "gauge", HtmlTemplate: "<div></div>", Script: "function update(){}",
		ScriptLanguage: "javascript", DefaultWidth: 100, DefaultHeight: 100,
	}); err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	if rec := serve(router, http.MethodPost, path+"/commits", apiv0.CreateCommitRequest{
		Version: 2, Title: "Widgets", Author: "Ada",
	}); rec.Code != http.StatusCreated {
		t.Fatalf("POST library commit: %d %s", rec.Code, rec.Body)
	}
	rec := serve(router, http.MethodGet, path+"/commits/1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET library commit: %d %s", rec.Code, rec.Body)
	}
	if commit := decodeBody[apiv0.CommitResponse](t, rec); commit.WidgetTypes == nil {
		t.Errorf("a library's commit carries no widget types: %+v", commit)
	}
	if rec := serve(router, http.MethodPatch, path+"/commits/1", apiv0.RenameCommitRequest{Title: "For the demo"}); rec.Code != http.StatusOK {
		t.Errorf("PATCH library commit: %d, want 200", rec.Code)
	}
	if rec := serve(router, http.MethodGet, path+"/commits/1/projectfile", nil); rec.Code != http.StatusNotFound {
		t.Errorf("a library has no commit project file: %d, want 404", rec.Code)
	}
}
