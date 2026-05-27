package repositories

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kipitix/growscada/internal/domain/id"
)

type tableName string

const (
	tableNameTags        tableName = "tags"
	tableNameScenes      tableName = "scenes"
	tableNameWidgets     tableName = "widgets"
	tableNameWidgetTypes tableName = "widget_types"
)

func classifyUpdateConflict[T any](ctx context.Context, db *sql.DB, table tableName, anID id.ID[T], notFoundError, conflictError error) error {
	var exists bool
	err := db.QueryRowContext(ctx,
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
