package restapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// TestRemovedDeploymentRoutes_AreGoneWithHint: the routes that served a
// Revision before task 72_9 answer 410 with a hint to the Deployment routes.
func TestRemovedDeploymentRoutes_AreGoneWithHint(t *testing.T) {
	router := newRouter()
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, mainPath + "/revision"},
		{http.MethodGet, mainPath + "/revisions"},
		{http.MethodGet, mainPath + "/revisions/1"},
		{http.MethodGet, mainPath + "/revisions/1/projectfile"},
		{http.MethodPost, mainPath + "/revisions/1/rollback"},
	} {
		rec := httptest.NewRecorder()
		router.ServeMux().ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))

		if rec.Code != http.StatusGone {
			t.Errorf("%s %s: status %d, want 410", tc.method, tc.path, rec.Code)
		}
		var prob apiv0.Problem
		if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
			t.Fatalf("%s %s: decode problem: %v", tc.method, tc.path, err)
		}
		if !strings.Contains(prob.Detail, "/deployment") {
			t.Errorf("%s %s: detail %q does not point to the Deployment route", tc.method, tc.path, prob.Detail)
		}
	}
}

// TestRemovedReleaseRoutes_AreNotFound: the routes that served a Project's and
// a Library's Releases are gone with task 85_7, and no stub answers for them
// (task 85_7).
func TestRemovedReleaseRoutes_AreNotFound(t *testing.T) {
	router := newRouter()
	libraryPath := apiv0.PathPrefix + "/libraries/d0000000-0000-4000-8000-000000000001"
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, mainPath + "/releases"},
		{http.MethodPost, mainPath + "/releases"},
		{http.MethodGet, mainPath + "/releases/1"},
		{http.MethodPatch, mainPath + "/releases/1"},
		{http.MethodGet, mainPath + "/releases/1/projectfile"},
		{http.MethodPost, mainPath + "/releases/1/deploy"},
		{http.MethodGet, libraryPath + "/releases"},
		{http.MethodPost, libraryPath + "/releases"},
		{http.MethodGet, libraryPath + "/releases/1"},
		{http.MethodPatch, libraryPath + "/releases/1"},
		{http.MethodPost, libraryPath + "/revert-to-release"},
	} {
		rec := httptest.NewRecorder()
		router.ServeMux().ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))

		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: status %d, want 404", tc.method, tc.path, rec.Code)
		}
	}
}

// TestRemovedSavedStateRoutes_AreGoneWithHint: the routes that served a Commit
// before task 88_2 answer 410 with a hint to the saved state routes (ADR 0026).
func TestRemovedSavedStateRoutes_AreGoneWithHint(t *testing.T) {
	router := newRouter()
	libraryPath := apiv0.PathPrefix + "/libraries/d0000000-0000-4000-8000-000000000001"
	labelID := "f1000000-0000-4000-8000-0000000000aa"
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, mainPath + "/commits"},
		{http.MethodPost, mainPath + "/commits"},
		{http.MethodGet, mainPath + "/commits/1"},
		{http.MethodPatch, mainPath + "/commits/1"},
		{http.MethodPost, mainPath + "/commits/1/load"},
		{http.MethodPost, mainPath + "/commits/1/deploy"},
		{http.MethodGet, mainPath + "/commits/1/projectfile"},
		{http.MethodPut, mainPath + "/commits/1/labels/" + labelID},
		{http.MethodDelete, mainPath + "/commits/1/labels/" + labelID},
		{http.MethodGet, libraryPath + "/commits"},
		{http.MethodPost, libraryPath + "/commits"},
		{http.MethodGet, libraryPath + "/commits/1"},
		{http.MethodPatch, libraryPath + "/commits/1"},
		{http.MethodPost, libraryPath + "/commits/1/load"},
		{http.MethodPut, libraryPath + "/commits/1/labels/" + labelID},
		{http.MethodDelete, libraryPath + "/commits/1/labels/" + labelID},
	} {
		rec := httptest.NewRecorder()
		router.ServeMux().ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))

		if rec.Code != http.StatusGone {
			t.Errorf("%s %s: status %d, want 410", tc.method, tc.path, rec.Code)
		}
		if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
			t.Errorf("%s %s: content type %q, want application/problem+json", tc.method, tc.path, got)
		}
		var prob apiv0.Problem
		if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
			t.Fatalf("%s %s: decode problem: %v", tc.method, tc.path, err)
		}
		if !strings.Contains(prob.Detail, "/saved-states") {
			t.Errorf("%s %s: detail %q does not point to the saved state route", tc.method, tc.path, prob.Detail)
		}
	}
}
