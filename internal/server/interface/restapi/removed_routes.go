package restapi

import "net/http"

// removedDraftLockPath is where a Draft's lock lived before ADR 0022: a
// Project's and a Library's. The routes stay, answering 410 Gone, so an old
// tab or tool learns the lock is gone instead of getting a bare 404; they are
// removed when the API reaches 1.0, as the pre-versioning path is.
const removedDraftLockPath = "/edit-lock"

// draftLockRemoved answers every removed lock route: the Draft is edited
// and saved freely, and its Version is the only concurrency token (ADR 0022).
func draftLockRemoved(w http.ResponseWriter, r *http.Request) {
	sendProblem(w, NewGone(
		"EditLock is gone: edit and save the Draft; the Version of the Draft is the only concurrency token",
		r.URL.Path))
}
