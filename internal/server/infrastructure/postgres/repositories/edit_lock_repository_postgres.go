package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/outbox"
)

// editLockRepositoryPostgres keeps engineering.edit_locks: one row per Draft
// (ADR 0015). The row outlives a free lock, so version is the monotonic
// counter a client compares its observation with; last_seen is raised without
// touching it.
type editLockRepositoryPostgres struct {
	db       *sql.DB
	onCommit func()
}

var _ editlock.Repository = (*editLockRepositoryPostgres)(nil)

// NewEditLockRepositoryPostgres returns the repository of the EditLocks.
func NewEditLockRepositoryPostgres(db *sql.DB, opts ...Option) editlock.Repository {
	o := newOptions(opts)
	return &editLockRepositoryPostgres{db: db, onCommit: o.onCommit}
}

const selectEditLockColumns = `project_id, library_id, holder_key, holder_name, since, last_seen, version`

func (r editLockRepositoryPostgres) Find(ctx context.Context, scope editlock.Scope) (editlock.EditLock, bool, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+selectEditLockColumns+` FROM engineering.edit_locks WHERE `+scopeCondition(scope), scopeArgs(scope)...)
	lock, err := scanEditLock(scope, row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("cannot read edit lock of %s: %w", scope, err)
	}
	return lock, true, nil
}

func (r editLockRepositoryPostgres) Create(ctx context.Context, lock editlock.EditLock) (editlock.EditLock, error) {
	state := lock.Snapshot()
	err := r.inTx(ctx, lock.PendingEvents(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO engineering.edit_locks (project_id, library_id, holder_key, holder_name, since, last_seen, version)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			nullableProject(lock.Scope()), nullableLibrary(lock.Scope()),
			nullableKey(state.Holder), nullableName(state.Holder), nullableTime(state.Since), nullableTime(state.LastSeen),
			version.Committed[editlock.EditLock]().Number(),
		)
		if err != nil {
			if isUniqueViolation(err, constraintEditLocksProject) || isUniqueViolation(err, constraintEditLocksLibrary) {
				return editlock.ErrEditLockExists
			}
			return fmt.Errorf("cannot create edit lock of %s: %w", lock.Scope(), err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return editlock.ReconstituteEditLock(lock.Scope(), state.Holder, state.Since, state.LastSeen,
		version.Committed[editlock.EditLock]()), nil
}

func (r editLockRepositoryPostgres) Save(ctx context.Context, lock editlock.EditLock) (editlock.EditLock, error) {
	state := lock.Snapshot()
	next := lock.Version().Next()
	err := r.inTx(ctx, lock.PendingEvents(), func(tx *sql.Tx) error {
		cond, condArgs := scopeConditionAt(lock.Scope(), 6)
		args := append([]any{nullableKey(state.Holder), nullableName(state.Holder), nullableTime(state.Since),
			nullableTime(state.LastSeen), next.Number()}, condArgs...)
		args = append(args, lock.Version().Number())
		res, err := tx.ExecContext(ctx,
			`UPDATE engineering.edit_locks
			 SET holder_key = $1, holder_name = $2, since = $3, last_seen = $4, version = $5
			 WHERE `+cond+` AND version = $7`,
			args...,
		)
		if err != nil {
			return fmt.Errorf("cannot save edit lock of %s: %w", lock.Scope(), err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("cannot get rows affected on edit lock update: %w", err)
		}
		if n != 1 {
			return fmt.Errorf("%w: %s", editlock.ErrEditLockVersionStale, lock.Scope())
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return editlock.ReconstituteEditLock(lock.Scope(), state.Holder, state.Since, state.LastSeen, next), nil
}

func (r editLockRepositoryPostgres) FindAllHeld(ctx context.Context) ([]editlock.EditLock, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+selectEditLockColumns+` FROM engineering.edit_locks WHERE holder_key IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("cannot query held edit locks: %w", err)
	}
	defer rows.Close()

	locks := []editlock.EditLock{}
	for rows.Next() {
		var (
			rawProject, rawLibrary uuid.NullUUID
			rawKey                 uuid.NullUUID
			rawName                sql.NullString
			rawSince, rawLastSeen  sql.NullTime
			rawVersion             int
		)
		if err := rows.Scan(&rawProject, &rawLibrary, &rawKey, &rawName, &rawSince, &rawLastSeen, &rawVersion); err != nil {
			return nil, fmt.Errorf("cannot scan edit lock: %w", err)
		}
		scope, err := scanScope(rawProject, rawLibrary)
		if err != nil {
			return nil, err
		}
		lock, err := reconstituteEditLock(scope, rawKey, rawName, rawSince, rawLastSeen, rawVersion)
		if err != nil {
			return nil, err
		}
		locks = append(locks, lock)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot iterate over edit locks: %w", err)
	}
	return locks, nil
}

func (r editLockRepositoryPostgres) Touch(ctx context.Context, scope editlock.Scope, key editlock.Key, at time.Time) error {
	cond, condArgs := scopeConditionAt(scope, 2)
	args := append([]any{at}, condArgs...)
	args = append(args, key.UUID())
	_, err := r.db.ExecContext(ctx,
		`UPDATE engineering.edit_locks SET last_seen = $1 WHERE `+cond+` AND holder_key = $3`,
		args...,
	)
	if err != nil {
		return fmt.Errorf("cannot touch edit lock of %s: %w", scope, err)
	}
	return nil
}

// inTx runs write and appends events to the outbox in one transaction, and
// signals the dispatcher once it has committed.
func (r editLockRepositoryPostgres) inTx(ctx context.Context, events []event.Event, write func(tx *sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := write(tx); err != nil {
		return err
	}
	if err := outbox.Append(ctx, tx, events); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot commit edit lock change: %w", err)
	}
	if len(events) > 0 && r.onCommit != nil {
		r.onCommit()
	}
	return nil
}

// scopeCondition is the WHERE condition matching the row of scope, whose key
// is its only argument, numbered from $1.
func scopeCondition(scope editlock.Scope) string {
	if scope.IsLibrary() {
		return `library_id = $1`
	}
	return `project_id = $1`
}

// scopeConditionAt is scopeCondition with the placeholder numbered from at,
// and its argument.
func scopeConditionAt(scope editlock.Scope, at int) (string, []any) {
	column := "project_id"
	anID := scope.ProjectID().UUID()
	if scope.IsLibrary() {
		column, anID = "library_id", scope.LibraryID().UUID()
	}
	return fmt.Sprintf("%s = $%d", column, at), []any{anID}
}

// scopeArgs is the argument of scopeCondition.
func scopeArgs(scope editlock.Scope) []any {
	if scope.IsLibrary() {
		return []any{scope.LibraryID().UUID()}
	}
	return []any{scope.ProjectID().UUID()}
}

func nullableLibrary(scope editlock.Scope) any {
	if !scope.IsLibrary() {
		return nil
	}
	return scope.LibraryID().UUID()
}

func nullableKey(holder editlock.Holder) any {
	if holder.IsZero() {
		return nil
	}
	return holder.Key().UUID()
}

func nullableName(holder editlock.Holder) any {
	if holder.IsZero() {
		return nil
	}
	return holder.Name()
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func nullableProject(scope editlock.Scope) any {
	if !scope.IsProject() {
		return nil
	}
	return scope.ProjectID().UUID()
}

func scanEditLock(scope editlock.Scope, scan func(...any) error) (editlock.EditLock, error) {
	var (
		rawProject, rawLibrary uuid.NullUUID
		rawKey                 uuid.NullUUID
		rawName                sql.NullString
		rawSince, rawLastSeen  sql.NullTime
		rawVersion             int
	)
	if err := scan(&rawProject, &rawLibrary, &rawKey, &rawName, &rawSince, &rawLastSeen, &rawVersion); err != nil {
		return nil, err
	}
	return reconstituteEditLock(scope, rawKey, rawName, rawSince, rawLastSeen, rawVersion)
}

func reconstituteEditLock(scope editlock.Scope, rawKey uuid.NullUUID, rawName sql.NullString,
	rawSince, rawLastSeen sql.NullTime, rawVersion int,
) (editlock.EditLock, error) {
	ver, err := version.New(version.WithNumber[editlock.EditLock](rawVersion))
	if err != nil {
		return nil, fmt.Errorf("cannot create edit lock version: %w", err)
	}
	if !rawKey.Valid {
		return editlock.ReconstituteEditLock(scope, editlock.Holder{}, time.Time{}, time.Time{}, ver), nil
	}
	key, err := editlock.ParseKey(rawKey.UUID.String())
	if err != nil {
		return nil, fmt.Errorf("stored edit lock key: %w", err)
	}
	holder, err := editlock.NewHolder(key, rawName.String)
	if err != nil {
		return nil, fmt.Errorf("stored edit lock holder: %w", err)
	}
	return editlock.ReconstituteEditLock(scope, holder, rawSince.Time, rawLastSeen.Time, ver), nil
}

func scanScope(rawProject, rawLibrary uuid.NullUUID) (editlock.Scope, error) {
	switch {
	case rawProject.Valid && !rawLibrary.Valid:
		return editlock.ProjectScope(id.NewID(id.IDWithUUID[project.Project](rawProject.UUID))), nil
	case rawLibrary.Valid && !rawProject.Valid:
		return editlock.LibraryScope(id.NewID(id.IDWithUUID[library.Library](rawLibrary.UUID))), nil
	default:
		return editlock.Scope{}, fmt.Errorf("edit lock row names no single scope")
	}
}
