package restapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/restapi"
)

func TestResponses_CarrySchemaVersion(t *testing.T) {
	router := newRouter()

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, apiv0.PathPrefix + "/tags"},
		{http.MethodGet, apiv0.PathPrefix + "/tags/not-a-uuid"}, // errors too
		{http.MethodOptions, apiv0.PathPrefix + "/tags"},        // CORS preflight
	} {
		rec := httptest.NewRecorder()
		router.ServeMux().ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))

		if got := rec.Header().Get(contract.SchemaVersionHeader); got != apiv0.SchemaVersion().String() {
			t.Errorf("%s %s: %s = %q, want %q", tc.method, tc.path, contract.SchemaVersionHeader, got, apiv0.SchemaVersion())
		}
	}
}

func TestCORS_AllowsAndExposesSchemaVersionHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	newRouter().ServeMux().ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, apiv0.PathPrefix+"/tags", nil))

	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, contract.SchemaVersionHeader) {
		t.Errorf("Access-Control-Allow-Headers = %q, want it to contain %s", got, contract.SchemaVersionHeader)
	}
	if got := rec.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(got, contract.SchemaVersionHeader) {
		t.Errorf("Access-Control-Expose-Headers = %q, want it to contain %s", got, contract.SchemaVersionHeader)
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got == "" {
		t.Error("Access-Control-Max-Age is not set: the browser would repeat the preflight before every request")
	}
}

func TestPreVersioningAPIPath_IsGoneWithHint(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		rec := httptest.NewRecorder()
		newRouter().ServeMux().ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/tags", nil))

		if rec.Code != http.StatusGone {
			t.Errorf("%s /api/v1/tags: status %d, want 410", method, rec.Code)
		}
		var prob restapi.ProblemDetails
		if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
			t.Fatalf("decode problem: %v", err)
		}
		if !strings.Contains(prob.Detail, apiv0.PathPrefix) {
			t.Errorf("detail %q does not point to %s", prob.Detail, apiv0.PathPrefix)
		}
	}
}

// postTagWithUnknownField sends a create request carrying a field the server
// does not know, as a newer client would, and returns the problem detail.
func postTagWithUnknownField(t *testing.T, clientVersion string) string {
	t.Helper()
	cleanTags(t)
	body := `{"name":"t.unknown","type":"integer","value":"0","quality":"good","unit":"rpm"}`
	req := httptest.NewRequest(http.MethodPost, apiv0.PathPrefix+"/tags", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if clientVersion != "" {
		req.Header.Set(contract.SchemaVersionHeader, clientVersion)
	}
	rec := httptest.NewRecorder()
	newRouter().ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400 (the field must not be silently dropped): %s", rec.Code, rec.Body)
	}
	var prob restapi.ProblemDetails
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	return prob.Detail
}

func TestRequest_UnknownField_Returns400(t *testing.T) {
	for _, version := range []string{"", apiv0.SchemaVersion().String(), "not-a-version"} {
		detail := postTagWithUnknownField(t, version)
		if !strings.Contains(detail, `unknown field "unit"`) {
			t.Errorf("client version %q: detail %q does not name the unknown field", version, detail)
		}
		if strings.Contains(detail, "update the server") {
			t.Errorf("client version %q: detail %q blames the server version", version, detail)
		}
	}
}

func TestRequest_NewerClient_UnknownField_SaysUpdateServer(t *testing.T) {
	newer := contract.SchemaVersion{Major: apiv0.SchemaVersion().Major, Minor: apiv0.SchemaVersion().Minor + 1}

	detail := postTagWithUnknownField(t, newer.String())

	for _, want := range []string{newer.String(), apiv0.SchemaVersion().String(), "update the server", `unknown field "unit"`} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q does not contain %q", detail, want)
		}
	}
}

func TestRequest_NewerClient_KnownFields_IsAccepted(t *testing.T) {
	cleanTags(t)
	newer := contract.SchemaVersion{Major: apiv0.SchemaVersion().Major, Minor: apiv0.SchemaVersion().Minor + 1}
	body := `{"name":"t.newer","type":"integer","value":"0","quality":"good"}`
	req := httptest.NewRequest(http.MethodPost, apiv0.PathPrefix+"/tags", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(contract.SchemaVersionHeader, newer.String())
	rec := httptest.NewRecorder()
	newRouter().ServeMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d, want 201: %s", rec.Code, rec.Body)
	}
}
