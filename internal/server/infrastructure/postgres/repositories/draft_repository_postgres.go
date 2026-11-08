package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/draft"
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
// over its row: it locks the row, checks the Version the write states, runs
// the operation through a draft.Session and writes the touched rows, the
// DraftChange, the new Version and stack position and the outbox's events.
type draftRepositoryPostgres struct {
	db       *sql.DB
	releases library.LibraryReleaseRepository
	onCommit func()
}

var _ draft.Repository = (*draftRepositoryPostgres)(nil)

// NewDraftRepositoryPostgres returns the repository of the Drafts.
func NewDraftRepositoryPostgres(db *sql.DB, releases library.LibraryReleaseRepository, opts ...Option) draft.Repository {
	o := newOptions(opts)
	return &draftRepositoryPostgres{db: db, releases: releases, onCommit: o.onCommit}
}

// draftRow is a drafts row as stored.
type draftRow struct {
	id                     uuid.UUID
	projectID              uuid.NullUUID
	libraryID              uuid.NullUUID
	version                int
	stackPosition          int
	lastCommitNumber       int
	lastCommitDraftVersion int
}

const selectDraftColumns = `id, project_id, library_id, version, stack_position, last_commit_number, last_commit_draft_version`

func (r draftRepositoryPostgres) Read(ctx context.Context, scope draft.Scope) (draft.Draft, bool, error) {
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

// draftOf is the Draft a row describes.
func draftOf(row draftRow, scope draft.Scope) draft.Draft {
	ver, err := version.New(version.WithNumber[draft.Draft](row.version))
	if err != nil {
		ver = version.Initial[draft.Draft]()
	}
	return draft.NewDraft(id.NewID(id.IDWithUUID[draft.Draft](row.id)), scope, ver, row.stackPosition)
}

// Update runs one write of the Draft's content (see draft.Repository).
func (r draftRepositoryPostgres) Update(ctx context.Context, scope draft.Scope, expected version.Version[draft.Draft], description string, change func(*draft.Session) error) (draft.Draft, error) {
	var result draft.Draft
	err := r.inTx(ctx, func(tx *sql.Tx) ([]event.Event, error) {
		row, err := lockDraftRow(ctx, tx, scope, true, true)
		if err != nil {
			return nil, err
		}
		if expected.Number() != row.version {
			return nil, draft.ConflictError{Expected: expected.Number(), Current: row.version, Scope: scope}
		}
		current := draftOf(*row, scope)
		loader := &draftLoader{tx: tx, scope: scope, project: scope.ProjectID(), library: scope.LibraryID(), releases: r.releases}
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

// CheckVersion locks the Draft's row and checks it is still at expected (see
// draft.Repository).
func (r draftRepositoryPostgres) CheckVersion(ctx context.Context, scope draft.Scope, expected version.Version[draft.Draft]) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot begin the check of the draft of %s: %w", scope, err)
	}
	defer func() { _ = tx.Rollback() }()
	row, err := lockDraftRow(ctx, tx, scope, true, true)
	if err != nil {
		return err
	}
	if expected.Number() != row.version {
		return draft.ConflictError{Expected: expected.Number(), Current: row.version, Scope: scope}
	}
	return tx.Commit()
}

// Replace replaces the whole content of the Draft (see draft.Repository).
func (r draftRepositoryPostgres) Replace(ctx context.Context, scope draft.Scope, change func(*draft.Session) error) (draft.Draft, error) {
	var result draft.Draft
	err := r.inTx(ctx, func(tx *sql.Tx) ([]event.Event, error) {
		row, err := lockDraftRow(ctx, tx, scope, true, true)
		if err != nil {
			return nil, err
		}
		current := draftOf(*row, scope)
		loader := &draftLoader{tx: tx, scope: scope, project: scope.ProjectID(), library: scope.LibraryID(), releases: r.releases}
		session := draft.NewSession(current, loader)
		if err := change(session); err != nil {
			return nil, err
		}
		// A Library's Discard puts the released names back, and a name may be
		// another WidgetType's at the moment: the Draft's rows go first, so
		// the released ones are inserted with their final names.
		if scope.IsLibrary() {
			if _, err := tx.ExecContext(ctx,
				`DELETE FROM engineering.widget_types WHERE library_id = $1`, scope.LibraryID().UUID()); err != nil {
				return nil, fmt.Errorf("cannot clear the widget types of %s: %w", scope, err)
			}
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
func (r draftRepositoryPostgres) Purge(ctx context.Context, scope draft.Scope) (draft.Draft, error) {
	var result draft.Draft
	err := r.inTx(ctx, func(tx *sql.Tx) ([]event.Event, error) {
		row, err := lockDraftRow(ctx, tx, scope, false, false)
		if errors.Is(err, draft.ErrDraftNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		current := draftOf(*row, scope)
		loader := &draftLoader{tx: tx, scope: scope, project: scope.ProjectID(), library: scope.LibraryID(), releases: r.releases}
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
func (r draftRepositoryPostgres) ClearStack(ctx context.Context, scope draft.Scope) error {
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

// Stack returns the state of the Draft's DraftChange stack (see
// draft.Repository): the Draft and the changes an undo and a redo would apply.
// The row and the two changes are read in one snapshot, so a write that lands
// while the answer is built cannot make it offer an undo or a redo the stack
// does not hold at the Version it reports.
func (r draftRepositoryPostgres) Stack(ctx context.Context, scope draft.Scope) (draft.StackState, bool, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return draft.StackState{}, false, fmt.Errorf("cannot begin the read of the draft of %s: %w", scope, err)
	}
	defer func() { _ = tx.Rollback() }()
	cond, args := scopeCondition(scope)
	row := tx.QueryRowContext(ctx,
		`SELECT `+selectDraftColumns+` FROM engineering.drafts WHERE `+cond, args...)
	found, err := scanDraftRow(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return draft.StackState{}, false, nil
	}
	if err != nil {
		return draft.StackState{}, false, fmt.Errorf("cannot read the draft of %s: %w", scope, err)
	}
	current := draftOf(*found, scope)
	state := draft.StackState{Draft: current}
	position := current.StackPosition()
	undo, has, err := readChange(ctx, tx, current.ID().UUID(), position)
	if err != nil {
		return draft.StackState{}, false, err
	}
	if has {
		state.Undo = &undo
	}
	redo, has, err := readChange(ctx, tx, current.ID().UUID(), position+1)
	if err != nil {
		return draft.StackState{}, false, err
	}
	if has {
		state.Redo = &redo
	}
	return state, true, nil
}

// Undo applies the DraftChange at the stack's position (see
// draft.Repository).
func (r draftRepositoryPostgres) Undo(ctx context.Context, scope draft.Scope, expected version.Version[draft.Draft]) (draft.Draft, error) {
	return r.stepStack(ctx, scope, expected, true)
}

// Redo applies the DraftChange after the stack's position (see
// draft.Repository).
func (r draftRepositoryPostgres) Redo(ctx context.Context, scope draft.Scope, expected version.Version[draft.Draft]) (draft.Draft, error) {
	return r.stepStack(ctx, scope, expected, false)
}

// stepStack writes the change an undo (backwards) or a redo (forwards) names as
// one transaction over the Draft: the snapshots of the side it restores, the
// inverse events, the next Version and the moved position, and no DraftChange
// of its own (ADR 0017).
func (r draftRepositoryPostgres) stepStack(ctx context.Context, scope draft.Scope, expected version.Version[draft.Draft], undo bool) (draft.Draft, error) {
	var result draft.Draft
	err := r.inTx(ctx, func(tx *sql.Tx) ([]event.Event, error) {
		row, err := lockDraftRow(ctx, tx, scope, false, true)
		if errors.Is(err, draft.ErrDraftNotFound) {
			return nil, nothingToStep(scope, undo)
		}
		if err != nil {
			return nil, err
		}
		if expected.Number() != row.version {
			return nil, draft.ConflictError{Expected: expected.Number(), Current: row.version, Scope: scope}
		}
		seq := row.stackPosition
		if !undo {
			seq++
		}
		change, found, err := readChange(ctx, tx, row.id, seq)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, nothingToStep(scope, undo)
		}
		current := draftOf(*row, scope)
		loader := &draftLoader{tx: tx, scope: scope, project: scope.ProjectID(), library: scope.LibraryID(), releases: r.releases}
		session := draft.NewSession(current, loader)
		if undo {
			err = session.Undo(ctx, change)
		} else {
			err = session.Redo(ctx, change)
		}
		if err != nil {
			return nil, err
		}
		before, after := session.Changes()
		if err := applySnapshots(ctx, tx, scope, before, after); err != nil {
			return nil, err
		}
		stepped := session.Draft()
		if err := updateDraftProgress(ctx, tx, row.id, stepped.Version().Number(), stepped.StackPosition()); err != nil {
			return nil, err
		}
		result = stepped
		return session.Events(), nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func nothingToStep(scope draft.Scope, undo bool) error {
	return draft.NothingToStepError{Scope: scope, Undo: undo}
}

// queryRower is what a DraftChange read needs of *sql.DB and *sql.Tx.
type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// readChange reads one row of the Draft's stack; found is false when the
// stack has no change at seq.
func readChange(ctx context.Context, q queryRower, draftID uuid.UUID, seq int) (draft.DraftChange, bool, error) {
	var (
		description string
		beforeJSON  []byte
		afterJSON   []byte
	)
	err := q.QueryRowContext(ctx,
		`SELECT description, before, after FROM engineering.draft_changes WHERE draft_id = $1 AND seq = $2`,
		draftID, seq).Scan(&description, &beforeJSON, &afterJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return draft.DraftChange{}, false, nil
	}
	if err != nil {
		return draft.DraftChange{}, false, fmt.Errorf("cannot read the draft change %d: %w", seq, err)
	}
	before, err := draftchange.Decode(beforeJSON)
	if err != nil {
		return draft.DraftChange{}, false, fmt.Errorf("cannot read the before state of change %d: %w", seq, err)
	}
	after, err := draftchange.Decode(afterJSON)
	if err != nil {
		return draft.DraftChange{}, false, fmt.Errorf("cannot read the after state of change %d: %w", seq, err)
	}
	return draft.DraftChange{Seq: seq, Description: description, Before: before, After: after}, true, nil
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
func lockDraftRow(ctx context.Context, tx *sql.Tx, scope draft.Scope, create, mustBeLive bool) (*draftRow, error) {
	cond, args := scopeCondition(scope)
	if create {
		projectID, libraryID := nullableProject(scope), nullableLibrary(scope)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO engineering.drafts (id, project_id, library_id, version)
			 VALUES (gen_random_uuid(), $1, $2, 1)
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
	if mustBeLive {
		switch {
		case scope.IsProject():
			var live bool
			err := tx.QueryRowContext(ctx,
				`SELECT deleted_at IS NULL FROM projects WHERE id = $1`, scope.ProjectID().UUID()).Scan(&live)
			if errors.Is(err, sql.ErrNoRows) || (err == nil && !live) {
				return nil, id.NotFound(project.ErrProjectNotFound, scope.ProjectID())
			}
			if err != nil {
				return nil, fmt.Errorf("cannot check the project of %s: %w", scope, err)
			}
		case scope.IsLibrary():
			var live bool
			err := tx.QueryRowContext(ctx,
				`SELECT deleted_at IS NULL FROM engineering.libraries WHERE id = $1`, scope.LibraryID().UUID()).Scan(&live)
			if errors.Is(err, sql.ErrNoRows) || (err == nil && !live) {
				return nil, id.NotFound(library.ErrLibraryNotFound, scope.LibraryID())
			}
			if err != nil {
				return nil, fmt.Errorf("cannot check the library of %s: %w", scope, err)
			}
		}
	}
	return found, nil
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
	if err := scan(&row.id, &row.projectID, &row.libraryID, &row.version, &row.stackPosition,
		&row.lastCommitNumber, &row.lastCommitDraftVersion); err != nil {
		return nil, err
	}
	return &row, nil
}

// scopeColumn is the column of engineering.drafts naming the Draft of scope,
// and its ID.
func scopeColumn(scope draft.Scope) (string, uuid.UUID) {
	if scope.IsLibrary() {
		return "library_id", scope.LibraryID().UUID()
	}
	return "project_id", scope.ProjectID().UUID()
}

// scopeCondition is the WHERE condition matching the row of scope, whose key
// is its only argument, numbered from $1.
func scopeCondition(scope draft.Scope) (string, []any) {
	column, value := scopeColumn(scope)
	return column + ` = $1`, []any{value}
}

func nullableProject(scope draft.Scope) any {
	if !scope.IsProject() {
		return nil
	}
	return scope.ProjectID().UUID()
}

func nullableLibrary(scope draft.Scope) any {
	if !scope.IsLibrary() {
		return nil
	}
	return scope.LibraryID().UUID()
}
