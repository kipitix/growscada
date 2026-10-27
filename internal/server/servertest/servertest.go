// Package servertest runs the real GrowSCADA REST API over Postgres in
// testcontainers, for tests of what talks to the server over it (growctl, the
// device simulator and its link, the apiclient, the PWA UI).
package servertest

import (
	"context"
	"database/sql"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/kipitix/growscada/internal/server/application"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/migrations"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/outbox"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
	runtimepg "github.com/kipitix/growscada/internal/server/infrastructure/postgres/runtime"
	"github.com/kipitix/growscada/internal/server/inprocess"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
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

	if err := migrations.Up(ctx, db); err != nil {
		db.Close()
		container.Terminate(ctx)
		return nil, nil, fmt.Errorf("migrations: %w", err)
	}
	return db, container, nil
}

// MainProject is the project "Main" the migrations create, of the one
// Organization (ADR 0006): the only project after StartAPI.
var MainProject = uuid.MustParse("b0000000-0000-4000-8000-000000000001")

// MainProjectName is the name of MainProject.
const MainProjectName = "Main"

// StartAPI cleans every table (the Tag states, tags, widgets, scenes, Library
// Pins, libraries with their WidgetTypes and releases, projects but
// MainProject — restored live if a test deleted it —, outbox, policy_jobs) and serves the real REST API over the
// database, with the outbox dispatcher delivering committed events to its
// event streams, the Runtime node publishing tag_updated, and the policy
// runner running the policies, as the server does. The returned TagService
// is the one behind the API, for arranging and checking state directly; a
// tag's state is written through the API.
func StartAPI(t *testing.T, db *sql.DB) (*httptest.Server, application.TagService) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), "DELETE FROM runtime.tags; DELETE FROM engineering.tags; DELETE FROM engineering.widgets; "+
		"DELETE FROM engineering.scenes; DELETE FROM engineering.library_pins; DELETE FROM engineering.libraries; "+
		"DELETE FROM outbox; DELETE FROM policy_jobs; "+
		"DELETE FROM projects WHERE id <> '"+MainProject.String()+"'; "+
		// A test that deleted Main may have left its row removed by the
		// cleanup policy, not only marked.
		"INSERT INTO projects (id, organization_id, name, version) "+
		"SELECT '"+MainProject.String()+"', id, '"+MainProjectName+"', 1 FROM organizations "+
		"ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, version = 1, deleted_at = NULL"); err != nil {
		t.Fatalf("clean the database: %v", err)
	}
	bus := eventbus.NewEventBus()
	dispatcher := outbox.NewDispatcher(db, bus.Publish)
	onCommit := repositories.NotifyOnCommit(dispatcher.Notify)
	orgID, err := repositories.FindSoleOrganization(context.Background(), db)
	if err != nil {
		t.Fatalf("organization: %v", err)
	}
	projectRepo := repositories.NewProjectRepositoryPostgres(db, onCommit)
	scopes := repositories.NewProjectScopes(db, onCommit)
	content := application.NewProjectContent(projectRepo, scopes, orgID)
	tagUpdates := eventbus.NewAsyncPublisher(bus, 1024)
	tagStateSvc := runtimeapp.NewTagStateService(runtimepg.NewTagStateRepositoryPostgres(db), orgID, tagUpdates.Publish)
	runtimeNode := inprocess.NewRuntimeNode(tagStateSvc)
	tagSvc := application.NewTagService(content, runtimeNode)
	libraries := repositories.NewLibraryRepositoryPostgres(db, onCommit)
	releases := repositories.NewLibraryReleaseRepositoryPostgres(db, onCommit)
	widgetTypes := repositories.NewWidgetTypeRepositories(db, onCommit)
	runner := outbox.NewPolicyRunner(db, []application.Policy{
		application.NewWidgetReconciliationPolicy(scopes),
		application.NewLibraryCleanupPolicy(libraries),
		application.NewDiscardLibraryPolicy(libraries, releases, widgetTypes),
		application.NewUnbindDeletedTagPolicy(projectRepo, scopes),
		application.NewProjectCleanupPolicy(projectRepo, scopes, runtimeNode),
		application.NewRuntimeTagDefinitionsPolicy(scopes, runtimeNode),
	})
	dispatcher.SetPolicyRunner(runner)
	ctx, stop := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); dispatcher.Run(ctx) }()
	go func() { defer wg.Done(); tagUpdates.Run(ctx) }()
	go func() { defer wg.Done(); runner.Run(ctx) }()
	t.Cleanup(func() {
		stop()
		wg.Wait()
	})

	router := restapi.NewRouter(restapi.Services{
		Projects:    application.NewProjectService(projectRepo, orgID),
		Tags:        tagSvc,
		TagStates:   tagStateSvc,
		Libraries:   application.NewLibraryService(libraries, releases, widgetTypes, orgID),
		WidgetTypes: application.NewWidgetTypeService(libraries, widgetTypes, orgID),
		LibraryPins: application.NewLibraryPinService(content, libraries, releases),
		Scenes:      application.NewSceneService(content),
	}, bus, 100)

	srv := httptest.NewServer(router.ServeMux())
	t.Cleanup(srv.Close)
	return srv, tagSvc
}
