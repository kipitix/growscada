package restapi

import (
	"net/http"
	"strconv"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// versionQuery reads the Version of the Draft a DELETE of a Project's content
// states (ADR 0016). A request without it, or with one that is not a number,
// is 400: a delete is a write of the Draft and states the Version it saw.
func versionQuery(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := r.URL.Query().Get(apiv0.VersionQuery)
	if raw == "" {
		sendProblem(w, NewBadRequest("version: is required", r.URL.Path))
		return 0, false
	}
	number, err := strconv.Atoi(raw)
	if err != nil {
		sendProblem(w, NewBadRequest("version: '"+raw+"' is not a number", r.URL.Path))
		return 0, false
	}
	return number, true
}
