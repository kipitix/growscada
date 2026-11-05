package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/draftchange"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/outbox"
)

// draftChangeDepth is how many DraftChanges a Draft keeps: older ones are
// deleted and cannot be undone (ADR 0017).
const draftChangeDepth = 100

// draftRepositoryPostgres keeps engineering.drafts and engineering.draft_changes
// (ADR 0016, ADR 0017). One write of a Draft's content is one transaction
// over its row: it locks the row, checks the EditLock and the Version the
// write states, runs the operation through a draft.Session and writes the
// touched rows, the DraftChange, the new Version and stack position and the
// outbox's events.
type draftRepositoryPostgres struct {
	db       *sql.DB
	releases library.LibraryReleaseRepository
	onCommit func()
}

var _ draft.Repository = (*draftRepositoryPostgres)(nil)

// NewDraftRepositoryPostgres returns the repository of the Drafts. It also
// serves the EditLocks (ADR 0016): the lock is a field of the Draft's row.
func NewDraftRepositoryPostgres(db *sql.DB, releases library.LibraryReleaseRepository, opts ...Option) draft.Repository {
	o := newOptions(opts)
	return &draftRepositoryPostgres{db: db, releases: releases, onCommit: o.onCommit}
}

// draftRow is a drafts row as stored.
type draftRow struct {
	id            uuid.UUID
	projectID     uuid.NullUUID
	libraryID     uuid.NullUUID
	version       int
	lockVersion   int
	holderKey     uuid.NullUUID
	holderName    sql.NullString
	heldSince     sql.NullTime
	lastSeen      sql.NullTime
	stackPosition int
}

const selectDraftColumns = `id, project_id, library_id, version, lock_version,
	holder_key, holder_name, held_since, last_seen, stack_position`

func (r draftRepositoryPostgres) Read(ctx context.Context, scope editlock.Scope) (draft.Draft, bool, error) {
	cond, args := scopeCondition(scope)
	row := r.db.QueryRowContext(ctx,
		`SELECT `+selectDraftColumns+` FROM engineering.drafts WHERE `+cond, args...)
	found, err := scanDraftRow(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("cannot read the draft of %s: %w", scope, err)
	}
	return draftOf(*found, scope), true, nil
}

// draftOf is the Draft a row describes, its EditLock included.
func draftOf(row draftRow, scope editlock.Scope) draft.Draft {
	ver, err := version.New(version.WithNumber[draft.Draft](row.version))
	if err != nil {
		ver = version.Initial[draft.Draft]()
	}
	lockVer, err := version.New(version.WithNumber[editlock.EditLock](row.lockVersion))
	if err != nil {
		lockVer = version.Initial[editlock.EditLock]()
	}
	lock := editlock.ReconstituteEditLock(scope, editlock.Holder{}, time.Time{}, time.Time{}, lockVer)
	if row.holderKey.Valid {
		if key, keyErr := editlock.ParseKey(row.holderKey.UUID.String()); keyErr == nil {
			if holder, holderErr := editlock.NewHolder(key, row.holderName.String); holderErr == nil {
				lock = editlock.ReconstituteEditLock(scope, holder, row.heldSince.Time, row.lastSeen.Time, lockVer)
			}
		}
	}
	return draft.NewDraft(id.NewID(id.IDWithUUID[draft.Draft](row.id)), scope, ver, row.stackPosition, lock)
}

// Update runs one write of the Draft's content (see draft.Repository).
func (r draftRepositoryPostgres) Update(ctx context.Context, scope editlock.Scope, expected version.Version[draft.Draft], description string, change func(*draft.Session) error) (draft.Draft, error) {
	var result draft.Draft
	err := r.inTx(ctx, func(tx *sql.Tx) ([]event.Event, error) {
		row, err := lockDraftRow(ctx, tx, scope, true, true)
		if err != nil {
			return nil, err
		}
		if err := gateDraft(ctx, tx, row, scope); err != nil {
			return nil, err
		}
		if expected.Number() != row.version {
			return nil, draft.ConflictError{Expected: expected.Number(), Current: row.version}
		}
		current := draftOf(*row, scope)
		loader := &draftLoader{tx: tx, project: scope.ProjectID(), releases: r.releases}
		session := draft.NewSession(current, loader)
		if err := change(session); err != nil {
			return nil, err
		}
		before, after := session.Changes()
		if err := applySnapshots(ctx, tx, scope, before, after); err != nil {
			return nil, err
		}
		position, err := appendChange(ctx, tx, row.id, row.stackPosition, description, before, after)
		if err != nil {
			return nil, err
		}
		next := row.version + 1
		if err := updateDraftProgress(ctx, tx, row.id, next, position); err != nil {
			return nil, err
		}
		result = current.WithVersion(versionOfDraft(next)).WithStackPosition(position)
		return session.Events(), nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Replace replaces the whole content of the Draft (see draft.Repository).
func (r draftRepositoryPostgres) Replace(ctx context.Context, scope editlock.Scope, change func(*draft.Session) error) (draft.Draft, error) {
	var result draft.Draft
	err := r.inTx(ctx, func(tx *sql.Tx) ([]event.Event, error) {
		row, err := lockDraftRow(ctx, tx, scope, true, true)
		if err != nil {
			return nil, err
		}
		if err := gateDraft(ctx, tx, row, scope); err != nil {
			return nil, err
		}
		current := draftOf(*row, scope)
		loader := &draftLoader{tx: tx, project: scope.ProjectID(), releases: r.releases}
		session := draft.NewSession(current, loader)
		if err := change(session); err != nil {
			return nil, err
		}
		before, after := session.Changes()
		if err := applySnapshots(ctx, tx, scope, before, after); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM engineering.draft_changes WHERE draft_id = $1`, row.id); err != nil {
			return nil, fmt.Errorf("cannot clear the draft changes of %s: %w", scope, err)
		}
		next := row.version + 1
		if err := updateDraftProgress(ctx, tx, row.id, next, 0); err != nil {
			return nil, err
		}
		result = current.WithVersion(versionOfDraft(next)).WithStackPosition(0)
		return session.Events(), nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Purge removes the whole content of a deleted Project's Draft (see
// draft.Repository).
func (r draftRepositoryPostgres) Purge(ctx context.Context, scope editlock.Scope) (draft.Draft, error) {
	var result draft.Draft
	err := r.inTx(ctx, func(tx *sql.Tx) ([]event.Event, error) {
		row, err := lockDraftRow(ctx, tx, scope, false, false)
		if errors.Is(err, draft.ErrDraftNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if err := gateDraft(ctx, tx, row, scope); err != nil {
			return nil, err
		}
		current := draftOf(*row, scope)
		loader := &draftLoader{tx: tx, project: scope.ProjectID(), releases: r.releases}
		session := draft.NewSession(current, loader)
		if err := session.Purge(ctx); err != nil {
			return nil, err
		}
		before, after := session.Changes()
		if err := applySnapshots(ctx, tx, scope, before, after); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM engineering.draft_changes WHERE draft_id = $1`, row.id); err != nil {
			return nil, fmt.Errorf("cannot clear the draft changes of %s: %w", scope, err)
		}
		next := row.version + 1
		if err := updateDraftProgress(ctx, tx, row.id, next, 0); err != nil {
			return nil, err
		}
		result = current.WithVersion(versionOfDraft(next)).WithStackPosition(0)
		return session.Events(), nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ClearStack drops every DraftChange of the Draft (see draft.Repository).
func (r draftRepositoryPostgres) ClearStack(ctx context.Context, scope editlock.Scope) error {
	cond, args := scopeCondition(scope)
	return r.inTx(ctx, func(tx *sql.Tx) ([]event.Event, error) {
		var draftID uuid.UUID
		err := tx.QueryRowContext(ctx,
			`UPDATE engineering.drafts SET stack_position = 0 WHERE `+cond+` RETURNING id`, args...).Scan(&draftID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("cannot clear the draft stack of %s: %w", scope, err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM engineering.draft_changes WHERE draft_id = $1`, draftID); err != nil {
			return nil, fmt.Errorf("cannot clear the draft changes of %s: %w", scope, err)
		}
		return nil, nil
	})
}

// inTx runs write in one transaction, appends the events it recorded and
// signals the dispatcher once committed.
func (r draftRepositoryPostgres) inTx(ctx context.Context, write func(*sql.Tx) ([]event.Event, error)) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	events, err := write(tx)
	if err != nil {
		return err
	}
	if err := outbox.Append(ctx, tx, events); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot commit draft change: %w", err)
	}
	if len(events) > 0 && r.onCommit != nil {
		r.onCommit()
	}
	return nil
}

// lockDraftRow selects the Draft of scope FOR UPDATE, creating its row when
// asked and it does not exist yet (a Project or Library made before the
// Draft's table, ADR 0016). A Project that is not live has no Draft.
func lockDraftRow(ctx context.Context, tx *sql.Tx, scope editlock.Scope, create, mustBeLive bool) (*draftRow, error) {
	cond, args := scopeCondition(scope)
	if create {
		projectID, libraryID := nullableProject(scope), nullableLibrary(scope)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO engineering.drafts (id, project_id, library_id, version, lock_version)
			 VALUES (gen_random_uuid(), $1, $2, 1, 1)
			 ON CONFLICT DO NOTHING`,
			projectID, libraryID,
		); err != nil {
			if scope.IsProject() && isForeignKeyViolation(err, "fk_drafts_project_id") {
				return nil, id.NotFound(project.ErrProjectNotFound, scope.ProjectID())
			}
			if scope.IsLibrary() && isForeignKeyViolation(err, "fk_drafts_library_id") {
				return nil, id.NotFound(library.ErrLibraryNotFound, scope.LibraryID())
			}
			return nil, fmt.Errorf("cannot create the draft of %s: %w", scope, err)
		}
	}
	row := tx.QueryRowContext(ctx,
		`SELECT `+selectDraftColumns+` FROM engineering.drafts WHERE `+cond+` FOR UPDATE`, args...)
	found, err := scanDraftRow(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", draft.ErrDraftNotFound, scope)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot lock the draft of %s: %w", scope, err)
	}
	if scope.IsProject() && mustBeLive {
		var live bool
		err := tx.QueryRowContext(ctx,
			`SELECT deleted_at IS NULL FROM projects WHERE id = $1`, scope.ProjectID().UUID()).Scan(&live)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && !live) {
			return nil, id.NotFound(project.ErrProjectNotFound, scope.ProjectID())
		}
		if err != nil {
			return nil, fmt.Errorf("cannot check the project of %s: %w", scope, err)
		}
	}
	return found, nil
}

// gateDraft refuses a write of the Draft made by a client that does not hold
// its EditLock, raising last_seen for the holder (ADR 0015).
func gateDraft(ctx context.Context, tx *sql.Tx, row *draftRow, scope editlock.Scope) error {
	key, pass, err := clientOf(ctx, scope)
	if pass || err != nil {
		return err
	}
	state := stateOfDraftRow(row, scope)
	if !state.Held || state.Holder.Key() != key {
		return editlock.NotHeldError(state)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE engineering.drafts SET last_seen = now() WHERE id = $1`, row.id); err != nil {
		return fmt.Errorf("cannot touch the draft of %s: %w", scope, err)
	}
	return nil
}

func stateOfDraftRow(row *draftRow, scope editlock.Scope) editlock.Snapshot {
	state := editlock.Snapshot{Held: row.holderKey.Valid, Since: row.heldSince.Time, LastSeen: row.lastSeen.Time, Version: row.lockVersion}
	if !state.Held {
		return state
	}
	key, err := editlock.ParseKey(row.holderKey.UUID.String())
	if err != nil {
		return state
	}
	holder, err := editlock.NewHolder(key, row.holderName.String)
	if err != nil {
		return state
	}
	state.Holder = holder
	_ = scope
	return state
}

// appendChange deletes the redo tail, appends the change at position+1 and
// trims the stack to draftChangeDepth. It returns the new stack position.
func appendChange(ctx context.Context, tx *sql.Tx, draftID uuid.UUID, position int, description string, before, after []draft.Snapshot) (int, error) {
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM engineering.draft_changes WHERE draft_id = $1 AND seq > $2`, draftID, position); err != nil {
		return 0, fmt.Errorf("cannot trim the redo tail of a draft: %w", err)
	}
	beforeJSON, err := draftchange.Encode(before)
	if err != nil {
		return 0, err
	}
	afterJSON, err := draftchange.Encode(after)
	if err != nil {
		return 0, err
	}
	next := position + 1
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO engineering.draft_changes (draft_id, seq, description, before, after)
		 VALUES ($1, $2, $3, $4::jsonb, $5::jsonb)`,
		draftID, next, description, beforeJSON, afterJSON,
	); err != nil {
		return 0, fmt.Errorf("cannot append a draft change: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM engineering.draft_changes WHERE draft_id = $1 AND seq <= $2`,
		draftID, next-draftChangeDepth); err != nil {
		return 0, fmt.Errorf("cannot trim the stack of a draft: %w", err)
	}
	return next, nil
}

func updateDraftProgress(ctx context.Context, tx *sql.Tx, draftID uuid.UUID, contentVersion, stackPosition int) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE engineering.drafts SET version = $2, stack_position = $3, updated_at = now() WHERE id = $1`,
		draftID, contentVersion, stackPosition); err != nil {
		return fmt.Errorf("cannot update the draft's progress: %w", err)
	}
	return nil
}

func versionOfDraft(number int) version.Version[draft.Draft] {
	ver, err := version.New(version.WithNumber[draft.Draft](number))
	if err != nil {
		return version.Initial[draft.Draft]()
	}
	return ver
}

func scanDraftRow(scan func(...any) error) (*draftRow, error) {
	var row draftRow
	if err := scan(&row.id, &row.projectID, &row.libraryID, &row.version, &row.lockVersion,
		&row.holderKey, &row.holderName, &row.heldSince, &row.lastSeen, &row.stackPosition); err != nil {
		return nil, err
	}
	return &row, nil
}

// --- The EditLock store over the Draft's row ---

func (r draftRepositoryPostgres) Find(ctx context.Context, scope editlock.Scope) (editlock.EditLock, bool, error) {
	found, ok, err := r.Read(ctx, scope)
	if err != nil || !ok {
		return nil, false, err
	}
	return found.Lock(), true, nil
}

func (r draftRepositoryPostgres) Create(ctx context.Context, lock editlock.EditLock) (editlock.EditLock, error) {
	state := lock.Snapshot()
	err := r.inTx(ctx, func(tx *sql.Tx) ([]event.Event, error) {
		row, err := lockDraftRow(ctx, tx, lock.Scope(), true, true)
		if err != nil {
			return nil, err
		}
		if row.holderKey.Valid {
			return nil, editlock.ErrEditLockExists
		}
		if err := writeLock(ctx, tx, row.id, state, 1); err != nil {
			return nil, err
		}
		return lock.PendingEvents(), nil
	})
	if err != nil {
		return nil, err
	}
	return lockResult(lock.Scope(), state, 1), nil
}

func (r draftRepositoryPostgres) Save(ctx context.Context, lock editlock.EditLock) (editlock.EditLock, error) {
	state := lock.Snapshot()
	next := lock.Version().Next()
	err := r.inTx(ctx, func(tx *sql.Tx) ([]event.Event, error) {
		row, err := lockDraftRow(ctx, tx, lock.Scope(), false, true)
		if err != nil {
			return nil, err
		}
		if row.lockVersion != lock.Version().Number() {
			return nil, fmt.Errorf("%w: %s", editlock.ErrEditLockVersionStale, lock.Scope())
		}
		if err := writeLock(ctx, tx, row.id, state, next.Number()); err != nil {
			return nil, err
		}
		return lock.PendingEvents(), nil
	})
	if err != nil {
		return nil, err
	}
	return lockResult(lock.Scope(), state, next.Number()), nil
}

func writeLock(ctx context.Context, tx *sql.Tx, draftID uuid.UUID, state editlock.Snapshot, lockVersion int) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE engineering.drafts SET holder_key = $2, holder_name = $3, held_since = $4, last_seen = $5, lock_version = $6
		 WHERE id = $1`,
		draftID, nullableKey(state.Holder), nullableName(state.Holder), nullableTime(state.Since), nullableTime(state.LastSeen), lockVersion)
	if err != nil {
		return fmt.Errorf("cannot write the edit lock of a draft: %w", err)
	}
	return nil
}

func lockResult(scope editlock.Scope, state editlock.Snapshot, lockVersion int) editlock.EditLock {
	ver, err := version.New(version.WithNumber[editlock.EditLock](lockVersion))
	if err != nil {
		ver = version.Initial[editlock.EditLock]()
	}
	return editlock.ReconstituteEditLock(scope, state.Holder, state.Since, state.LastSeen, ver)
}

func (r draftRepositoryPostgres) FindAllHeld(ctx context.Context) ([]editlock.EditLock, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+selectDraftColumns+` FROM engineering.drafts WHERE holder_key IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("cannot query held edit locks: %w", err)
	}
	defer rows.Close()
	locks := []editlock.EditLock{}
	for rows.Next() {
		row, err := scanDraftRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("cannot scan draft lock: %w", err)
		}
		scope, err := scopeOfRow(row)
		if err != nil {
			return nil, err
		}
		locks = append(locks, draftOf(*row, scope).Lock())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot iterate over held locks: %w", err)
	}
	return locks, nil
}

func (r draftRepositoryPostgres) Touch(ctx context.Context, scope editlock.Scope, key editlock.Key, at time.Time) error {
	cond, args := scopeCondition(scope)
	args = append(args, key.UUID(), at)
	_, err := r.db.ExecContext(ctx,
		fmt.Sprintf(`UPDATE engineering.drafts SET last_seen = $%d WHERE %s AND holder_key = $%d`, len(args), cond, len(args)-1),
		args...)
	if err != nil {
		return fmt.Errorf("cannot touch the draft of %s: %w", scope, err)
	}
	return nil
}

func scopeOfRow(row *draftRow) (editlock.Scope, error) {
	switch {
	case row.projectID.Valid && !row.libraryID.Valid:
		return editlock.ProjectScope(id.NewID(id.IDWithUUID[project.Project](row.projectID.UUID))), nil
	case row.libraryID.Valid && !row.projectID.Valid:
		return editlock.LibraryScope(id.NewID(id.IDWithUUID[library.Library](row.libraryID.UUID))), nil
	default:
		return editlock.Scope{}, fmt.Errorf("draft row names no single scope")
	}
}

// nullableKey, nullableName and nullableTime are the NULLs of a free lock.
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

func nullableLibrary(scope editlock.Scope) any {
	if !scope.IsLibrary() {
		return nil
	}
	return scope.LibraryID().UUID()
}
