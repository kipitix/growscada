// Package servertest runs the real GrowSCADA REST API over Postgres in
// testcontainers, for tests of the tools that talk to the server (growctl, the
// device simulator and its link).
package servertest

import (
	"context"
	"database/sql"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
	"github.com/kipitix/growscada/internal/server/interface/restapi"
)

// Postgres is a migrated database in a container, started on first use and
// shared by the tests of one package.
type Postgres struct {
	once      sync.Once
	db        *sql.DB
	container *tcpostgres.PostgresContainer
	err       error
}

// DB returns the database, starting the container on the first call.
func (p *Postgres) DB(t *testing.T) *sql.DB {
	t.Helper()
	p.once.Do(func() { p.db, p.container, p.err = startPostgres(context.Background()) })
	if p.err != nil {
		t.Fatalf("postgres: %v", p.err)
	}
	return p.db
}

// Close stops the container if it was started; call it from TestMain.
func (p *Postgres) Close() {
	if p.db != nil {
		p.db.Close()
	}
	if p.container != nil {
		p.container.Terminate(context.Background())
	}
}

func startPostgres(ctx context.Context) (*sql.DB, *tcpostgres.PostgresContainer, error) {
	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("start container: %w", err)
	}
	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		container.Terminate(ctx)
		return nil, nil, err
	}
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		container.Terminate(ctx)
		return nil, nil, err
	}

	_, currentFile, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(currentFile), "..", "infrastructure", "postgres", "migrations")
	if err := goose.SetDialect("postgres"); err != nil {
		db.Close()
		container.Terminate(ctx)
		return nil, nil, err
	}
	if err := goose.Up(db, migrationsDir); err != nil {
		db.Close()
		container.Terminate(ctx)
		return nil, nil, fmt.Errorf("migrations: %w", err)
	}
	return db, container, nil
}

// StartAPI cleans the tags table and serves the real REST API over the
// database. The returned TagService is the one behind the API, for arranging
// and checking state directly.
func StartAPI(t *testing.T, db *sql.DB) (*httptest.Server, application.TagService) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), "DELETE FROM tags"); err != nil {
		t.Fatalf("clean tags: %v", err)
	}
	tagSvc := application.NewTagService(repositories.NewTagRepositoryPostgres(db), event.NewEventBus())
	wtRepo := repositories.NewWidgetTypeRepositoryPostgres(db)
	sceneRepo := repositories.NewSceneRepositoryPostgres(db)
	wtSvc := application.NewWidgetTypeService(wtRepo, sceneRepo, event.NewEventBus())
	sceneSvc := application.NewSceneService(sceneRepo, wtRepo, event.NewEventBus())
	router := restapi.NewRouter(tagSvc, wtSvc, sceneSvc, event.NewEventBus(), 100)

	srv := httptest.NewServer(router.ServeMux())
	t.Cleanup(srv.Close)
	return srv, tagSvc
}
