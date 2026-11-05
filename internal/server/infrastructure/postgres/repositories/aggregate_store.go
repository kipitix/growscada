package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/outbox"
)

// Option configures a Postgres repository.
type Option func(*options)

type options struct {
	onCommit func()
}

// NotifyOnCommit makes the repository call fn after each commit that wrote
// events to the outbox (outbox.Dispatcher.Notify). Without it the events are
// still delivered, on the dispatcher's next poll.
func NotifyOnCommit(fn func()) Option {
	return func(o *options) { o.onCommit = fn }
}

func newOptions(opts []Option) options {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// aggregateStore is the Save/Delete template of the Postgres repositories
// (ADR 0008): one transaction writes the aggregate's row, compare-and-swap on
// the Version it was read at, and the events it recorded into the outbox.
// The store of a Project's content (project set) touches only rows of that
// Project, the store of a Library's Draft (library set) only rows of that
// Library.
type aggregateStore[T any] struct {
	db      *sql.DB
	table   tableName
	project *id.ID[project.Project]
	library *id.ID[library.Library]
	// lock, if set, runs first in the transaction of every write: it locks
	// and checks what the write rests on (e.g. the row of its Library).
	lock func(ctx context.Context, tx *sql.Tx) error
	// discard marks the store of a Project's Draft opened by a Discard: its
	// writes are made only while one is under way (lockProject).
	discard  bool
	notFound error
	conflict error
	onCommit func()
}

// errOtherProject marks the write of an aggregate of another Project through
// the repositories of a Project: a programming error (ADR 0006).
var errOtherProject = errors.New("aggregate of another project")

// checkProject returns an error unless an aggregate of aProjectID may be
// written through the store: one of the store's Project.
func (s aggregateStore[T]) checkProject(aProjectID id.ID[project.Project]) error {
	if s.project == nil || *s.project != aProjectID {
		return fmt.Errorf("cannot write to %s: %w: %s, not %v", s.table, errOtherProject, aProjectID, s.project)
	}
	return nil
}

// where returns the condition matching anID's row, within the store's
// Project or Library if it has one, and its arguments, numbered from $1.
func (s aggregateStore[T]) where(anID id.ID[T]) (string, []any) {
	switch {
	case s.project != nil:
		return `id = $1 AND project_id = $2`, []any{anID.UUID(), s.project.UUID()}
	case s.library != nil:
		return `id = $1 AND library_id = $2`, []any{anID.UUID(), s.library.UUID()}
	default:
		return `id = $1`, []any{anID.UUID()}
	}
}

// rowWrite writes an aggregate's rows in a Save's transaction.
type rowWrite[T any] struct {
	// insert inserts an aggregate of the initial version, stored at next.
	insert func(tx *sql.Tx, next version.Version[T]) error
	// update replaces the stored row only if it is still at current (the
	// CAS), storing next, and reports whether it did.
	update func(tx *sql.Tx, current, next version.Version[T]) (bool, error)
}

// save writes an aggregate read at current, with the events it recorded, and
// returns the Version stored. A failed CAS is classified as notFound or
// conflict; any error rolls everything back.
func (s aggregateStore[T]) save(ctx context.Context, anID id.ID[T], current version.Version[T], events []event.Event, w rowWrite[T]) (version.Version[T], error) {
	var next version.Version[T]
	switch {
	case current == version.Initial[T]():
		next = version.Committed[T]()
	case current.IsCommitted():
		next = current.Next()
	default:
		return next, fmt.Errorf("undefined behavior with version %d", current.Number())
	}

	err := s.inTx(ctx, events, func(tx *sql.Tx) error {
		if s.lock != nil {
			if err := s.lock(ctx, tx); err != nil {
				return err
			}
		}
		if current == version.Initial[T]() {
			if s.project != nil {
				if err := s.lockLiveProject(ctx, tx); err != nil {
					return err
				}
			}
			return w.insert(tx, next)
		}
		found, err := w.update(tx, current, next)
		if err != nil {
			return err
		}
		if !found {
			return s.classifyUpdateConflict(ctx, tx, anID)
		}
		return nil
	})
	return next, err
}

// withProjectLock returns the store of a Project's content whose every write
// first locks the Project's row and checks it (lockProject).
func (s aggregateStore[T]) withProjectLock() aggregateStore[T] {
	s.lock = s.lockProject
	return s
}

// errNotDiscardingProject refuses a write of a Discard's repository once no
// Discard of its Project is under way.
var errNotDiscardingProject = errors.New("project is not being discarded")

// lockLiveProject locks the row of the store's Project for share in tx, the
// transaction of a creation of its content; a Project marked deleted (or
// removed) is not found. Marking the Project deleted locks its row too, so it
// waits for the creation, or the creation for it: no content is created in a
// Project once its cleanup may run (ADR 0006).
func (s aggregateStore[T]) lockLiveProject(ctx context.Context, tx *sql.Tx) error {
	var deleted bool
	err := tx.QueryRowContext(ctx,
		`SELECT deleted_at IS NOT NULL FROM projects WHERE id = $1 FOR SHARE`, s.project.UUID(),
	).Scan(&deleted)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && deleted) {
		return id.NotFound(project.ErrProjectNotFound, *s.project)
	}
	if err != nil {
		return fmt.Errorf("cannot lock project %s: %w", *s.project, err)
	}
	return nil
}

// lockProject locks the row of the store's Project for share in tx, the
// transaction of a write of its content, and checks the Discard mark: a write
// of the Draft is refused while one is under way, and a Discard's own is
// refused once none is (task 35, Q10). A Project marked deleted takes no
// write of the Draft — lockLiveProject refuses a creation, and the cleanup's
// removals are its business — but the mark of a Discard goes with it: nothing
// will clear it, so it no longer keeps the cleanup out.
func (s aggregateStore[T]) lockProject(ctx context.Context, tx *sql.Tx) error {
	var deleted, discarding bool
	err := tx.QueryRowContext(ctx,
		`SELECT deleted_at IS NOT NULL, discarding FROM projects WHERE id = $1 FOR SHARE`, s.project.UUID(),
	).Scan(&deleted, &discarding)
	if errors.Is(err, sql.ErrNoRows) {
		return id.NotFound(project.ErrProjectNotFound, *s.project)
	}
	if err != nil {
		return fmt.Errorf("cannot lock project %s: %w", *s.project, err)
	}
	switch {
	case discarding && !deleted && !s.discard:
		return fmt.Errorf("%w: %s", project.ErrProjectDiscarding, *s.project)
	case !discarding && s.discard:
		return fmt.Errorf("%w: %s", errNotDiscardingProject, *s.project)
	}
	return nil
}

// delete removes an aggregate read at current (CAS), with the events it
// recorded. Rows the aggregate owns go by the cascade of their foreign keys.
func (s aggregateStore[T]) delete(ctx context.Context, anID id.ID[T], current version.Version[T], events []event.Event) error {
	return s.deleteWhere(ctx, anID, &current, events)
}

// deleteAnyVersion removes an aggregate whatever Version is stored, with the
// events it recorded: for an aggregate whose removal does not rest on the
// state it was read in. A missing one is notFound.
func (s aggregateStore[T]) deleteAnyVersion(ctx context.Context, anID id.ID[T], events []event.Event) error {
	return s.deleteWhere(ctx, anID, nil, events)
}

// deleteWhere removes an aggregate, only if it is still at current unless
// current is nil.
func (s aggregateStore[T]) deleteWhere(ctx context.Context, anID id.ID[T], current *version.Version[T], events []event.Event) error {
	return s.inTx(ctx, events, func(tx *sql.Tx) error {
		if s.lock != nil {
			if err := s.lock(ctx, tx); err != nil {
				return err
			}
		}
		cond, args := s.where(anID)
		query := fmt.Sprintf(`DELETE FROM %s WHERE %s`, s.table, cond)
		if current != nil {
			args = append(args, current.Number())
			query += fmt.Sprintf(` AND version = $%d`, len(args))
		}
		res, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("cannot delete from %s: %w", s.table, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("cannot get rows affected on delete: %w", err)
		}
		if n != 1 {
			return s.classifyUpdateConflict(ctx, tx, anID)
		}
		return nil
	})
}

// inTx runs write and appends events to the outbox in one transaction, and
// signals the dispatcher once it has committed.
func (s aggregateStore[T]) inTx(ctx context.Context, events []event.Event, write func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// The Draft's EditLock gates every write of its content (ADR 0015): the
	// check reads the lock row in this transaction, so a takeover waits for
	// this write and this write waits for a takeover.
	switch {
	case s.project != nil:
		if err := checkProjectEditLock(ctx, tx, *s.project); err != nil {
			return err
		}
	case s.library != nil:
		if err := checkLibraryEditLock(ctx, tx, *s.library); err != nil {
			return err
		}
	}
	if err := write(tx); err != nil {
		return err
	}
	if err := outbox.Append(ctx, tx, events); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot commit %s change: %w", s.table, err)
	}
	if len(events) > 0 && s.onCommit != nil {
		s.onCommit()
	}
	return nil
}
