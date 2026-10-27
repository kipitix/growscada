package repositories

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kipitix/growscada/internal/server/domain/id"
)

type tableName string

const (
	tableNameTags        tableName = "engineering.tags"
	tableNameScenes      tableName = "engineering.scenes"
	tableNameWidgetTypes tableName = "engineering.widget_types"
	tableNameProjects    tableName = "projects"
	tableNameLibraries   tableName = "engineering.libraries"
	tableNameReleases    tableName = "engineering.library_releases"
	tableNameLibraryPins tableName = "engineering.library_pins"
)

// rowQuerier is a *sql.DB or a *sql.Tx.
type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// classifyUpdateConflict tells why a compare-and-swap on anID's row matched
// nothing: the row is gone from the store's scope (notFound) or its version
// moved on (conflict).
func (s aggregateStore[T]) classifyUpdateConflict(ctx context.Context, q rowQuerier, anID id.ID[T]) error {
	cond, args := s.where(anID)
	var exists bool
	err := q.QueryRowContext(ctx,
		fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE %s)", s.table, cond),
		args...,
	).Scan(&exists)

	if err != nil {
		return fmt.Errorf("cannot check %s existence: %w", s.table, err)
	}
	if !exists {
		return id.NotFound(s.notFound, anID)
	}
	return s.conflict
}
