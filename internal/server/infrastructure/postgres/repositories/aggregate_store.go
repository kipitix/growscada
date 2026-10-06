package repositories

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
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
type aggregateStore[T any] struct {
	db       *sql.DB
	table    tableName
	notFound error
	conflict error
	onCommit func()
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
		if current == version.Initial[T]() {
			return w.insert(tx, next)
		}
		found, err := w.update(tx, current, next)
		if err != nil {
			return err
		}
		if !found {
			return classifyUpdateConflict(ctx, tx, s.table, anID, s.notFound, s.conflict)
		}
		return nil
	})
	return next, err
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
		query := fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, s.table)
		args := []any{anID.UUID()}
		if current != nil {
			query += ` AND version = $2`
			args = append(args, current.Number())
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
			return classifyUpdateConflict(ctx, tx, s.table, anID, s.notFound, s.conflict)
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
