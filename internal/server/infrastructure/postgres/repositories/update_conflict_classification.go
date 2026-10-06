package repositories

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kipitix/growscada/internal/server/domain/id"
)

type tableName string

const (
	tableNameTags        tableName = "tags"
	tableNameScenes      tableName = "scenes"
	tableNameWidgetTypes tableName = "widget_types"
)

// rowQuerier is a *sql.DB or a *sql.Tx.
type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// classifyUpdateConflict tells why a compare-and-swap on anID's row matched
// nothing: the row is gone (notFoundError) or its version moved on
// (conflictError).
func classifyUpdateConflict[T any](ctx context.Context, q rowQuerier, table tableName, anID id.ID[T], notFoundError, conflictError error) error {
	var exists bool
	err := q.QueryRowContext(ctx,
		fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE id = $1)", table),
		anID.UUID(),
	).Scan(&exists)

	if err != nil {
		return fmt.Errorf("cannot check %s existence: %w", table, err)
	}
	if !exists {
		return notFoundError
	}
	return conflictError
}
