package restapi_test

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/restapi"
)

// testClientKey is the client the tests' HTTP requests act as. The EditLock of
// a project under test holds it (holdEditLockFor): a test writes the Draft
// through the API, which needs the lock (ADR 0015). A test of the lock itself
// drives the routes with its own keys through restapi.NewRouter directly.
var testClientKey = uuid.MustParse("d0000000-0000-4000-8000-00000000d00d")

// servingRouter is anything an HTTP test hands requests to: the API router or
// a testRouter wrapping it.
type servingRouter interface {
	ServeMux() http.Handler
}

// testRouter is the API router with testClientKey on every request, holding
// the EditLock of every project the request names.
type testRouter struct {
	APIRouter *restapi.APIRouter
}

func (r testRouter) ServeMux() http.Handler {
	inner := r.APIRouter.ServeMux()
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req.Header.Set(apiv0.ClientHeader, testClientKey.String())
		if projectID := projectIDInPath(req.URL.Path); projectID != uuid.Nil {
			holdEditLockFor(projectID)
		}
		inner.ServeHTTP(w, req)
	})
}

// holdTestEditLock makes testClientKey the holder of the Main project's
// EditLock, whatever it was.
func holdTestEditLock() { holdEditLockFor(mainProject) }

// holdEditLockFor makes testClientKey the holder of the EditLock of the
// project projectID. A project that does not exist gets no row: the request
// is answered 404 before any write.
func holdEditLockFor(projectID uuid.UUID) {
	db := testDB()
	_, _ = db.Exec(
		`INSERT INTO engineering.edit_locks (project_id, holder_key, holder_name, since, last_seen, version)
		 VALUES ($1, $2, 'restapi-test', now(), now(), 1)
		 ON CONFLICT (project_id) WHERE project_id IS NOT NULL DO UPDATE
		 SET holder_key = EXCLUDED.holder_key, holder_name = EXCLUDED.holder_name, last_seen = now()`,
		projectID, testClientKey,
	)
}

// projectIDInPath is the project a request path names, uuid.Nil if none.
func projectIDInPath(path string) uuid.UUID {
	rest, ok := strings.CutPrefix(path, apiv0.PathPrefix+"/projects/")
	if !ok {
		return uuid.Nil
	}
	raw, _, _ := strings.Cut(rest, "/")
	projectID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil
	}
	return projectID
}
