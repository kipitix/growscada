// Package servertest runs the real GrowSCADA REST API over Postgres in
// testcontainers, for tests of what talks to the server over it (growctl, the
// device simulator and its link, the apiclient, the PWA UI).
package servertest

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
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

// Device is a Device of the Main project, the Environment it stands in and
// the token it acts with, created through the real API.
type Device struct {
	ID              uuid.UUID
	Name            string
	EnvironmentID   uuid.UUID
	EnvironmentName string
	Token           string
}

// NewDevice creates a Device of the Main project and an Environment it stands
// in, through the real API, makes the given Tags the Device's suppliers and
// issues the Device's token. The API has no way to change a Tag's supplier
// yet (task 68), so the supplier is set in the database: the Engineering
// node's row. A Deploy copies it to the Runtime node with the rest of the
// Draft.
func NewDevice(t *testing.T, srv *httptest.Server, db *sql.DB, name string, tagIDs ...uuid.UUID) Device {
	t.Helper()
	return NewDeviceInProject(t, srv, db, MainProject, name, tagIDs...)
}

// NewDeviceInProject is NewDevice for a Device of the given project.
func NewDeviceInProject(t *testing.T, srv *httptest.Server, db *sql.DB, projectID uuid.UUID, name string, tagIDs ...uuid.UUID) Device {
	t.Helper()
	ctx := context.Background()
	projectPath := apiv0.ProjectPath(projectID.String())
	device := postJSON[apiv0.CreateDeviceResponse](t, srv, projectPath+"/devices",
		apiv0.CreateDeviceRequest{Name: name, Protocol: "simulation", Config: apiv0.JSONObject(`{}`)})
	environment := postJSON[apiv0.CreateEnvironmentResponse](t, srv, projectPath+"/environments",
		apiv0.CreateEnvironmentRequest{Name: name + "-env"})
	putJSON[apiv0.EnvironmentDeviceResponse](t, srv,
		projectPath+"/environments/"+environment.ID.String()+"/devices/"+device.ID.String(),
		apiv0.UpdateEnvironmentDeviceRequest{Params: apiv0.JSONObject(`{"address":"127.0.0.1:1502"}`)})
	for _, tagID := range tagIDs {
		if _, err := db.ExecContext(ctx, "UPDATE engineering.tags SET device_id = $1 WHERE id = $2", device.ID, tagID); err != nil {
			t.Fatalf("supply tag %s: %v", tagID, err)
		}
	}
	token := postJSON[apiv0.DeviceTokenResponse](t, srv, projectPath+"/devices/"+device.ID.String()+"/token", nil)
	return Device{
		ID:              device.ID,
		Name:            name,
		EnvironmentID:   environment.ID,
		EnvironmentName: name + "-env",
		Token:           token.Token,
	}
}

// postJSON sends body (if any) as JSON to path and decodes the 201 response.
func postJSON[T any](t *testing.T, srv *httptest.Server, path string, body any) T {
	t.Helper()
	return sendJSON[T](t, srv, http.MethodPost, path, body, http.StatusCreated)
}

// putJSON sends body as JSON to path and decodes the response: 200 for an
// update, 201 when the request created what it named (the pair of an
// Environment and a Device).
func putJSON[T any](t *testing.T, srv *httptest.Server, path string, body any) T {
	t.Helper()
	return sendJSON[T](t, srv, http.MethodPut, path, body, http.StatusOK, http.StatusCreated)
}

func sendJSON[T any](t *testing.T, srv *httptest.Server, method, path string, body any, wantStatuses ...int) T {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, srv.URL+apiv0.PathPrefix+path, reader)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if !slices.Contains(wantStatuses, resp.StatusCode) {
		t.Fatalf("%s %s: %s: %s", method, path, resp.Status, data)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("%s %s: cannot decode %s: %v", method, path, data, err)
	}
	return out
}

// StartAPI cleans every table (the Revisions, the Runtime node's copies of
// Tags, Devices and Environments with their states and tokens, the Draft's
// Tags, widgets, scenes, Library Pins, libraries with their WidgetTypes and
// releases, projects but MainProject — restored live if a test deleted it —,
// outbox, policy_jobs) and serves the real REST API over the
// database, with the outbox dispatcher delivering committed events to its
// event streams, the Runtime node publishing tag_updated and revision_created,
// and the policy runner running the policies, as the server does. The returned TagService
// is the one behind the API, for arranging and checking state directly; a
// tag's state is written through the API.
func StartAPI(t *testing.T, db *sql.DB) (*httptest.Server, application.TagService) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), "DELETE FROM runtime.revisions; DELETE FROM runtime.device_tokens; DELETE FROM runtime.tags; "+
		"DELETE FROM runtime.environment_devices; DELETE FROM runtime.devices; DELETE FROM runtime.environments; "+
		"DELETE FROM engineering.project_files; DELETE FROM engineering.tags; "+
		"DELETE FROM engineering.environment_devices; DELETE FROM engineering.devices; DELETE FROM engineering.environments; "+
		"DELETE FROM engineering.widgets; "+
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
	runtimeEvents := eventbus.NewAsyncPublisher(bus, 1024)
	tagStateSvc := runtimeapp.NewTagStateService(runtimepg.NewTagStateRepositoryPostgres(db), orgID, runtimeEvents.Publish)
	revisions := runtimeapp.NewRevisionService(runtimepg.NewRevisionRepositoryPostgres(db), runtimeEvents.Publish)
	deviceTokens := runtimeapp.NewDeviceTokenService(runtimepg.NewDeviceTokenRepositoryPostgres(db))
	runtimeNode := inprocess.NewRuntimeNode(tagStateSvc, deviceTokens, revisions)
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
		application.NewDiscardProjectPolicy(projectRepo, scopes, releases, runtimeNode),
	})
	dispatcher.SetPolicyRunner(runner)
	ctx, stop := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); dispatcher.Run(ctx) }()
	go func() { defer wg.Done(); runtimeEvents.Run(ctx) }()
	go func() { defer wg.Done(); runner.Run(ctx) }()
	t.Cleanup(func() {
		stop()
		wg.Wait()
	})

	router := restapi.NewRouter(restapi.Services{
		Projects:     application.NewProjectService(projectRepo, orgID),
		Tags:         tagSvc,
		TagStates:    tagStateSvc,
		Libraries:    application.NewLibraryService(libraries, releases, widgetTypes, orgID),
		WidgetTypes:  application.NewWidgetTypeService(libraries, widgetTypes, orgID),
		LibraryPins:  application.NewLibraryPinService(content, libraries, releases),
		Scenes:       application.NewSceneService(content),
		Devices:      application.NewDeviceService(content),
		Environments: application.NewEnvironmentService(content),
		DeviceConfig: runtimeapp.NewDeviceConfigService(runtimepg.NewDeviceConfigRepositoryPostgres(db)),
		DeviceTokens: deviceTokens,
	}, bus, 100)

	srv := httptest.NewServer(router.ServeMux())
	t.Cleanup(srv.Close)
	return srv, tagSvc
}
