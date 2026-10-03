package restapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// schemaVersionMiddleware stamps every response with the SchemaVersion of the
// server API, so a client can tell which MINOR it is talking to (ADR 0005).
func schemaVersionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contract.SchemaVersionHeader, apiv0.SchemaVersion.String())
		next.ServeHTTP(w, r)
	})
}

// decodeRequest decodes a request body strictly into v and reports whether it
// succeeded; on failure it has already answered 400. The server keeps what a
// request carries, so a field it does not know is an error rather than
// silently dropped data (ADR 0005). When the client states a newer
// SchemaVersion, the error says so instead of only naming the field.
func decodeRequest(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(v)
	if err == nil {
		return true
	}
	detail := err.Error()
	if client, err := contract.ParseSchemaVersion(r.Header.Get(contract.SchemaVersionHeader)); err == nil &&
		client.NewerThan(apiv0.SchemaVersion) {
		detail = fmt.Sprintf("the client speaks server API %v, newer than this server's %v: update the server (%s)",
			client, apiv0.SchemaVersion, detail)
	}
	sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(detail, r.URL.Path))
	return false
}
