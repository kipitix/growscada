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

// Chain is one schema's migration chain.
type Chain struct {
	// Dir is the chain's directory, beside this file.
	Dir string
	// Table is the goose table recording the chain's applied versions.
	Table string
}

// Chains are the server's chains in the order they are applied: public, the
// server's, first; the roles' schemas reference it.
var Chains = []Chain{
	{Dir: "public", Table: "goose_db_version"},
	{Dir: "engineering", Table: "engineering.goose_db_version"},
	{Dir: "runtime", Table: "runtime.goose_db_version"},
}

// Up applies every chain to db, in order.
func Up(ctx context.Context, db *sql.DB) error {
	for _, chain := range Chains {
		dir, err := fs.Sub(files, chain.Dir)
		if err != nil {
			return fmt.Errorf("migrations %s: %w", chain.Dir, err)
		}
		provider, err := goose.NewProvider(goose.DialectPostgres, db, dir, goose.WithTableName(chain.Table))
		if err != nil {
			return fmt.Errorf("migrations %s: %w", chain.Dir, err)
		}
		if _, err := provider.Up(ctx); err != nil {
			return fmt.Errorf("migrations %s: %w", chain.Dir, err)
		}
	}
	return nil
}
