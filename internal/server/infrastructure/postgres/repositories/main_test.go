package repositories_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/migrations"
)

// mainProject is the project "Main" the migrations create: the tests' content
// lives in it unless they make projects of their own.
var mainProject = id.NewID(id.IDWithUUID[project.Project](uuid.MustParse("b0000000-0000-4000-8000-000000000001")))

var (
	testDB *sql.DB
	// testConnStr connects to testDB; tests that need a database of their own
	// (migrationDB) create it on the same server.
	testConnStr string
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
		),
	)
	if err != nil {
		panic("failed to start postgres container: " + err.Error())
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		pgContainer.Terminate(ctx)
		panic("failed to get connection string: " + err.Error())
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		pgContainer.Terminate(ctx)
		panic("failed to open db: " + err.Error())
	}

	if err := migrations.Up(ctx, db); err != nil {
		db.Close()
		pgContainer.Terminate(ctx)
		panic("failed to run migrations: " + err.Error())
	}

	testDB = db
	testConnStr = connStr

	code := m.Run()

	db.Close()
	pgContainer.Terminate(ctx)
	os.Exit(code)
}

// cleanDrafts removes every Draft row and its DraftChanges: the tests start
// from a fresh Draft.
func cleanDrafts(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(),
		`DELETE FROM engineering.draft_changes; DELETE FROM engineering.drafts`); err != nil {
		t.Fatalf("cleanDrafts: %v", err)
	}
}
