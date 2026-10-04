package uiutil

import (
	"net/http"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// SendSchemaVersion makes every request with a body sent by
// http.DefaultClient — which all UI calls to the REST API go through — state
// the server API SchemaVersion the UI was built against, so the server can
// tell a newer client apart from a malformed request (ADR 0005). Call it once,
// in the browser only.
//
// Requests without a body are left alone: the server reads the header only
// when decoding a body, and a custom header would turn every cross-origin GET
// into one with a CORS preflight.
func SendSchemaVersion() {
	http.DefaultClient.Transport = schemaVersionTransport{base: http.DefaultTransport}
}

type schemaVersionTransport struct {
	base http.RoundTripper
}

func (t schemaVersionTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Body == nil || r.Body == http.NoBody {
		return t.base.RoundTrip(r)
	}
	r = r.Clone(r.Context())
	r.Header.Set(contract.SchemaVersionHeader, apiv0.SchemaVersion().String())
	return t.base.RoundTrip(r)
}
