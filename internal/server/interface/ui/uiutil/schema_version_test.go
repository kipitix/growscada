package uiutil

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// Only a request with a body states the SchemaVersion: the server reads it
// only when decoding a body, and on a GET the custom header would cost a CORS
// preflight.
func TestSchemaVersionTransport_SetsHeaderOnlyOnRequestsWithBody(t *testing.T) {
	got := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got[r.Method] = r.Header.Get(contract.SchemaVersionHeader)
	}))
	defer srv.Close()
	client := &http.Client{Transport: schemaVersionTransport{base: http.DefaultTransport}}

	for _, tc := range []struct {
		method string
		body   bool
		want   string
	}{
		{http.MethodGet, false, ""},
		{http.MethodDelete, false, ""},
		{http.MethodPost, true, apiv0.SchemaVersion().String()},
		{http.MethodPut, true, apiv0.SchemaVersion().String()},
		{http.MethodPatch, true, apiv0.SchemaVersion().String()},
	} {
		var req *http.Request
		if tc.body {
			req, _ = http.NewRequest(tc.method, srv.URL, strings.NewReader("{}"))
		} else {
			req, _ = http.NewRequest(tc.method, srv.URL, nil)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if got[tc.method] != tc.want {
			t.Errorf("%s: %s = %q, want %q", tc.method, contract.SchemaVersionHeader, got[tc.method], tc.want)
		}
	}
}
