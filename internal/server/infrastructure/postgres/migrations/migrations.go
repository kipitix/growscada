// Package migrations holds the server's database migrations: one goose chain
// per schema (ADR 0011), in the subdirectories of the same names.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

//go:embed public/*.sql engineering/*.sql runtime/*.sql
var files embed.FS

// chain is one schema's migration chain.
type chain struct {
	// Dir is the chain's directory, beside this file.
	Dir string
	// Table is the goose table recording the chain's applied versions.
	Table string
}

// chains are the server's chains in the order they are applied: public, the
// server's, first; the roles' schemas reference it. The docker compose files
// repeat them, one goose service each (TestComposeFilesApplyEveryChain).
var chains = []chain{
	{Dir: "public", Table: "goose_db_version"},
	{Dir: "engineering", Table: "engineering.goose_db_version"},
	{Dir: "runtime", Table: "runtime.goose_db_version"},
}

// Up applies every chain to db, in order.
func Up(ctx context.Context, db *sql.DB) error {
	for _, c := range chains {
		dir, err := fs.Sub(files, c.Dir)
		if err != nil {
			return fmt.Errorf("migrations %s: %w", c.Dir, err)
		}
		provider, err := goose.NewProvider(goose.DialectPostgres, db, dir, goose.WithTableName(c.Table))
		if err != nil {
			return fmt.Errorf("migrations %s: %w", c.Dir, err)
		}
		if _, err := provider.Up(ctx); err != nil {
			return fmt.Errorf("migrations %s: %w", c.Dir, err)
		}
	}
	return nil
}
