// Package pgtest gives tests a migrated Postgres database of their own.
//
// By default it runs Postgres in a container (testcontainers, needs Docker).
// When the environment variable GROWSCADA_TEST_POSTGRES holds the URL of a
// running server (postgres://user:password@host:port/db?sslmode=disable, e.g.
// a CI service container), it creates a fresh database on that server instead
// and drops it on Close, so test packages do not share tables. Run them with
// "go test -p 1" then: the outbox dispatcher waits for the oldest transaction
// of the whole server (pg_snapshot_xmin), so packages running in parallel on
// one server delay each other's deliveries.
package pgtest

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// EnvServer names the environment variable with the URL of an external server.
const EnvServer = "GROWSCADA_TEST_POSTGRES"

// Database is a migrated database for one test package.
type Database struct {
	DB *sql.DB
	// ConnStr is a postgres:// URL of DB; ConnStrFor gives the URL of another
	// database on the same server.
	ConnStr string
	close   func()
}

// ConnStrFor returns the URL of the database name on the same server.
func (d *Database) ConnStrFor(name string) string {
	u, err := url.Parse(d.ConnStr)
	if err != nil {
		panic(fmt.Sprintf("pgtest: connection string: %v", err))
	}
	u.Path = "/" + name
	return u.String()
}

// Close closes DB and removes the database (or its container).
func (d *Database) Close() {
	if d == nil {
		return
	}
	d.DB.Close()
	d.close()
}

// Start returns a migrated database: on the server from EnvServer if set,
// in a new container otherwise.
func Start(ctx context.Context) (*Database, error) {
	var (
		d   *Database
		err error
	)
	if server := os.Getenv(EnvServer); server != "" {
		d, err = onServer(ctx, server)
	} else {
		d, err = inContainer(ctx)
	}
	if err != nil {
		return nil, err
	}
	if err := migrate(d.DB); err != nil {
		d.Close()
		return nil, fmt.Errorf("migrations: %w", err)
	}
	return d, nil
}

func onServer(ctx context.Context, server string) (*Database, error) {
	admin, err := sql.Open("postgres", server)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", EnvServer, err)
	}
	name := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close()
		return nil, fmt.Errorf("create database on %s: %w", EnvServer, err)
	}
	d := &Database{ConnStr: server}
	d.ConnStr = d.ConnStrFor(name)
	d.close = func() {
		_, _ = admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		admin.Close()
	}
	if d.DB, err = sql.Open("postgres", d.ConnStr); err != nil {
		d.close()
		return nil, err
	}
	return d, nil
}

func inContainer(ctx context.Context) (*Database, error) {
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
		return nil, fmt.Errorf("start container (set %s to use a running server): %w", EnvServer, err)
	}
	terminate := func() { container.Terminate(context.Background()) }
	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		terminate()
		return nil, err
	}
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		terminate()
		return nil, err
	}
	return &Database{DB: db, ConnStr: connStr, close: terminate}, nil
}

func migrate(db *sql.DB) error {
	_, currentFile, _, _ := runtime.Caller(0)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(db, filepath.Join(filepath.Dir(currentFile), "..", "migrations"))
}
