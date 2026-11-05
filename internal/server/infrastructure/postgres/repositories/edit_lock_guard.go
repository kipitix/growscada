package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
)

// clientOf returns the key of the client ctx names, or pass when the write is
// the server's own. A context that names no client is ErrNoClient.
func clientOf(ctx context.Context, scope editlock.Scope) (key editlock.Key, pass bool, err error) {
	switch editlock.IdentityFrom(ctx) {
	case editlock.IdentitySystem:
		return editlock.Key{}, true, nil
	case editlock.IdentityClient:
		key, ok := editlock.ClientFrom(ctx)
		if !ok {
			return editlock.Key{}, false, editlock.ErrNoClient
		}
		return key, false, nil
	default:
		return editlock.Key{}, false, fmt.Errorf("%w: a write of the Draft of %s", editlock.ErrNoClient, scope)
	}
}

// scopeColumn is the column of engineering.drafts naming the Draft of scope,
// and its ID.
func scopeColumn(scope editlock.Scope) (string, uuid.UUID) {
	if scope.IsLibrary() {
		return "library_id", scope.LibraryID().UUID()
	}
	return "project_id", scope.ProjectID().UUID()
}

// scopeCondition is the WHERE condition matching the row of scope, whose key
// is its only argument, numbered from $1.
func scopeCondition(scope editlock.Scope) (string, []any) {
	column, value := scopeColumn(scope)
	return column + ` = $1`, []any{value}
}

// checkLibraryEditLock refuses a write of a Library's Draft made by a client
// that does not hold its EditLock: the lock is a field of the Draft's row
// (ADR 0016). The row's exclusive lock is taken first, so a takeover — which
// writes it — serializes with the write; the holder's write raises last_seen.
func checkLibraryEditLock(ctx context.Context, tx *sql.Tx, libraryID id.ID[library.Library]) error {
	scope := editlock.LibraryScope(libraryID)
	key, pass, err := clientOf(ctx, scope)
	if pass || err != nil {
		return err
	}
	var (
		rawKey   uuid.NullUUID
		rawName  sql.NullString
		rawSince sql.NullTime
		rawSeen  sql.NullTime
		rawLockV int
	)
	err = tx.QueryRowContext(ctx,
		`SELECT holder_key, holder_name, held_since, last_seen, lock_version
		 FROM engineering.drafts WHERE library_id = $1 FOR UPDATE`, libraryID.UUID(),
	).Scan(&rawKey, &rawName, &rawSince, &rawSeen, &rawLockV)
	if errors.Is(err, sql.ErrNoRows) {
		return editlock.NotHeldError(editlock.Snapshot{})
	}
	if err != nil {
		return fmt.Errorf("cannot lock the draft of library %s: %w", libraryID, err)
	}
	state := editlock.Snapshot{Held: rawKey.Valid, Since: rawSince.Time, LastSeen: rawSeen.Time, Version: rawLockV}
	if !state.Held {
		return editlock.NotHeldError(state)
	}
	holderKey, err := editlock.ParseKey(rawKey.UUID.String())
	if err != nil {
		return fmt.Errorf("stored edit lock key: %w", err)
	}
	holder, err := editlock.NewHolder(holderKey, rawName.String)
	if err != nil {
		return fmt.Errorf("stored edit lock holder: %w", err)
	}
	state.Holder = holder
	if state.Holder.Key() != key {
		return editlock.NotHeldError(state)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE engineering.drafts SET last_seen = now() WHERE library_id = $1 AND holder_key = $2`,
		libraryID.UUID(), key.UUID()); err != nil {
		return fmt.Errorf("cannot touch the draft of library %s: %w", libraryID, err)
	}
	return nil
}
