package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// checkProjectEditLock refuses a write of a Project's Draft made by a client
// that does not hold the Draft's EditLock (ADR 0015). It runs in the write's
// own transaction: it takes the lock row's exclusive lock, so a takeover —
// which writes it — serializes with the write, and a write the client was
// allowed to start finishes before the takeover.
//
// The identity comes from the context: a request names its client, a request
// without a client is ErrNoClient, and the server's own writes (policies,
// cleanup) are editlock.System and pass. A context with no identity at all
// also refuses: a forgotten key fails closed.
//
// The holder's write raises the lock's last_seen, which is how a client
// without an SSE stream (growctl, Bruno) keeps its lock alive. That UPDATE
// comes first: it both touches the lease and takes the row's exclusive lock,
// while a SELECT ... FOR SHARE followed by the UPDATE would deadlock two of
// the holder's writes, each holding the share lock the other's UPDATE waits
// for.
func checkProjectEditLock(ctx context.Context, tx *sql.Tx, projectID id.ID[project.Project]) error {
	key, pass, err := clientOf(ctx, projectID)
	if pass || err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE engineering.edit_locks SET last_seen = now() WHERE project_id = $1 AND holder_key = $2`,
		projectID.UUID(), key.UUID(),
	); err != nil {
		return fmt.Errorf("cannot touch edit lock of project %s: %w", projectID, err)
	}
	state, err := lockState(ctx, tx, `project_id = $1`, projectID.UUID())
	if err != nil {
		return err
	}
	if !state.Held || state.Holder.Key() != key {
		// A free lock (a missing row, or one never checked out) refuses the
		// write just the same; the state says so, so the Problem can tell a
		// client that nobody edits the Draft.
		return editlock.NotHeldError(state)
	}
	return nil
}

// clientOf returns the key of the client ctx names, or pass when the write is
// the server's own. A context that names no client is ErrNoClient.
func clientOf(ctx context.Context, projectID id.ID[project.Project]) (key editlock.Key, pass bool, err error) {
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
		return editlock.Key{}, false, fmt.Errorf("%w: a write of the Draft of project %s", editlock.ErrNoClient, projectID)
	}
}

// lockState reads the state of the lock row matching condition (its arguments
// numbered from $1) FOR SHARE. A missing row is the free lock of a Draft never
// checked out; a row whose holder is NULL the free lock that keeps its
// Version.
func lockState(ctx context.Context, q rowQuerier, condition string, args ...any) (editlock.Snapshot, error) {
	var (
		rawKey                uuid.NullUUID
		rawName               sql.NullString
		rawSince, rawLastSeen sql.NullTime
		rawVersion            int
	)
	err := q.QueryRowContext(ctx,
		`SELECT holder_key, holder_name, since, last_seen, version FROM engineering.edit_locks WHERE `+condition+` FOR SHARE`,
		args...,
	).Scan(&rawKey, &rawName, &rawSince, &rawLastSeen, &rawVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return editlock.Snapshot{}, nil
	}
	if err != nil {
		return editlock.Snapshot{}, fmt.Errorf("cannot read edit lock: %w", err)
	}
	state := editlock.Snapshot{Held: rawKey.Valid, Since: rawSince.Time, LastSeen: rawLastSeen.Time, Version: rawVersion}
	if !state.Held {
		return state, nil
	}
	key, err := editlock.ParseKey(rawKey.UUID.String())
	if err != nil {
		return editlock.Snapshot{}, fmt.Errorf("stored edit lock key: %w", err)
	}
	holder, err := editlock.NewHolder(key, rawName.String)
	if err != nil {
		return editlock.Snapshot{}, fmt.Errorf("stored edit lock holder: %w", err)
	}
	state.Holder = holder
	return state, nil
}
