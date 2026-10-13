// Package servertest runs the real GrowSCADA REST API over Postgres (pgtest), for tests of what talks to the server over it (growctl, the
// device simulator and its link, the apiclient, the PWA UI).
package servertest

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/outbox"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/pgtest"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
	"github.com/kipitix/growscada/internal/server/interface/restapi"
)

// Postgres is a migrated database (see pgtest), started on first use and
// shared by the tests of one package.
type Postgres struct {
	once sync.Once
	db   *pgtest.Database
	err  error
}

// DB returns the database, starting it on the first call.
func (p *Postgres) DB(t *testing.T) *sql.DB {
	t.Helper()
	p.once.Do(func() { p.db, p.err = pgtest.Start(context.Background()) })
	if p.err != nil {
		t.Fatalf("postgres: %v", p.err)
	}
	return p.db.DB
}

// Close removes the database if it was started; call it from TestMain.
func (p *Postgres) Close() {
	p.db.Close()
}

// StartAPI cleans every table (tags, widgets, scenes, widget_types, outbox,
// policy_jobs) and serves the real REST API over the database, with the
// outbox dispatcher delivering committed events to its event stream and the
// policy runner running the policies, as the server does. The returned
// TagService is the one behind the API, for arranging and checking state
// directly.
func StartAPI(t *testing.T, db *sql.DB) (*httptest.Server, application.TagService) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), "DELETE FROM tags; DELETE FROM widgets; DELETE FROM scenes; DELETE FROM widget_types; "+
		"DELETE FROM outbox; DELETE FROM policy_jobs"); err != nil {
		t.Fatalf("clean the database: %v", err)
	}
	bus := eventbus.NewEventBus()
	dispatcher := outbox.NewDispatcher(db, bus.Publish)
	onCommit := repositories.NotifyOnCommit(dispatcher.Notify)
	tagSvc := application.NewTagService(repositories.NewTagRepositoryPostgres(db, onCommit))
	wtRepo := repositories.NewWidgetTypeRepositoryPostgres(db, onCommit)
	sceneRepo := repositories.NewSceneRepositoryPostgres(db, onCommit)
	runner := outbox.NewPolicyRunner(db, []application.Policy{
		application.NewWidgetReconciliationPolicy(wtRepo, sceneRepo),
	})
	dispatcher.SetPolicyRunner(runner)
	ctx, stop := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); dispatcher.Run(ctx) }()
	go func() { defer wg.Done(); runner.Run(ctx) }()
	t.Cleanup(func() {
		stop()
		wg.Wait()
	})

	wtSvc := application.NewWidgetTypeService(wtRepo)
	sceneSvc := application.NewSceneService(sceneRepo, wtRepo)
	router := restapi.NewRouter(tagSvc, wtSvc, sceneSvc, bus, 100)

	srv := httptest.NewServer(router.ServeMux())
	t.Cleanup(srv.Close)
	return srv, tagSvc
}
