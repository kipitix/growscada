package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/lib/pq"
)

const (
	// defaultPollInterval is how often Run looks into the outbox without a
	// Notify: it picks up rows written by a repository that has no signal.
	defaultPollInterval = time.Second
	// heldBackRetry is how soon Run looks again while rows wait for an older
	// transaction: when that one ends no Notify may come (it rolled back, or
	// wrote no events).
	heldBackRetry = 20 * time.Millisecond
	// batchSize is how many rows DrainOnce reads at a time.
	batchSize = 100
	// deleteTimeout bounds the DELETE of the delivered rows, which goes on
	// after DrainOnce's ctx is done: a delivered row left in the outbox would
	// be delivered again.
	deleteTimeout = 5 * time.Second
)

// Dispatcher delivers the outbox rows to its sink after commit and deletes
// the delivered rows, one DELETE per batch: at-least-once, since rows
// delivered but not yet deleted when the process stops are delivered again. There is one Dispatcher
// per database (ADR 0008).
//
// Rows go in the order of the transactions that wrote them (tx_id), and a
// row waits until every older transaction has ended. A transaction gets its
// id at its first write, so a change made after another one committed is
// delivered after it: in particular every change of one aggregate, whose
// writes are serialized by the CAS on its Version. Changes that ran at the
// same time may go in either order. A long transaction anywhere in the
// database holds back the rows of all younger ones until it ends.
type Dispatcher struct {
	db           *sql.DB
	deliver      func(event.Event)
	pollInterval time.Duration
	wake         chan struct{}

	// mu makes DrainOnce calls take turns, so no row is delivered twice by
	// one process.
	mu sync.Mutex
}

// DispatcherOption configures a Dispatcher.
type DispatcherOption func(*Dispatcher)

// WithPollInterval sets how often Run looks into the outbox without a Notify.
func WithPollInterval(interval time.Duration) DispatcherOption {
	return func(d *Dispatcher) { d.pollInterval = interval }
}

// NewDispatcher returns a Dispatcher delivering the events of db's outbox to
// deliver (for now the EventBus of the SSE stream).
func NewDispatcher(db *sql.DB, deliver func(event.Event), opts ...DispatcherOption) *Dispatcher {
	d := &Dispatcher{
		db:           db,
		deliver:      deliver,
		pollInterval: defaultPollInterval,
		wake:         make(chan struct{}, 1),
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Notify tells Run that a commit wrote to the outbox. It never blocks; the
// repositories call it after each commit that wrote events.
func (d *Dispatcher) Notify() {
	select {
	case d.wake <- struct{}{}:
	default: // a wake-up is already pending
	}
}

// Run delivers the outbox until ctx is done: on every Notify and at least
// every poll interval, and soon again while rows wait for an older
// transaction. An error is logged and retried on the next round.
func (d *Dispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()
	for {
		_, heldBack, err := d.drain(ctx)
		if err != nil && ctx.Err() == nil {
			slog.Error("outbox delivery failed", "error", err)
		}
		var soon <-chan time.Time // nil: never
		if heldBack {
			soon = time.After(heldBackRetry)
		}
		select {
		case <-ctx.Done():
			return
		case <-d.wake:
		case <-ticker.C:
		case <-soon:
		}
	}
}

// DrainOnce delivers every row in the outbox that no older transaction holds
// back, oldest first, and returns how many it delivered. A row whose event
// cannot be decoded can never be delivered: it is logged and dropped so that
// it does not hold back the rest. Once ctx is done no further row is
// delivered, but the delivered ones are still deleted.
func (d *Dispatcher) DrainOnce(ctx context.Context) (int, error) {
	delivered, _, err := d.drain(ctx)
	return delivered, err
}

// drain is DrainOnce that also reports whether rows were left waiting for an
// older transaction.
func (d *Dispatcher) drain(ctx context.Context) (delivered int, heldBack bool, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for {
		rows, heldBack, err := d.readBatch(ctx)
		if err != nil {
			return delivered, false, err
		}
		done := make([]int64, 0, len(rows))
		for _, row := range rows {
			if ctx.Err() != nil {
				break
			}
			e, err := row.decode()
			if err != nil {
				slog.Error("dropping undeliverable outbox row", "seq", row.seq, "type", row.eventType, "error", err)
			} else {
				d.deliver(e)
				delivered++
			}
			done = append(done, row.seq)
		}
		if err := d.deleteRows(ctx, done); err != nil {
			return delivered, false, err
		}
		if err := ctx.Err(); err != nil {
			return delivered, false, err
		}
		if heldBack || len(rows) < batchSize {
			return delivered, heldBack, nil
		}
	}
}

// deleteRows deletes rows that have been delivered (or dropped). It is not
// stopped by ctx: a delivered row left in the outbox would be delivered again.
func (d *Dispatcher) deleteRows(ctx context.Context, seqs []int64) error {
	if len(seqs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), deleteTimeout)
	defer cancel()
	if _, err := d.db.ExecContext(ctx, `DELETE FROM outbox WHERE seq = ANY($1)`, pq.Array(seqs)); err != nil {
		return fmt.Errorf("cannot delete %d delivered outbox rows: %w", len(seqs), err)
	}
	return nil
}

type outboxRow struct {
	seq       int64
	eventType string
	payload   []byte
}

func (row outboxRow) decode() (event.Event, error) {
	eventType, err := event.NewEventType(row.eventType)
	if err != nil {
		return nil, err
	}
	return Decode(eventType, row.payload)
}

// readBatch reads the oldest committed rows up to the first one held back:
// written by a transaction that some still running transaction is older
// than, and could still commit rows that go first. Rows are ordered by
// tx_id, so every row after a held back one is held back too.
func (d *Dispatcher) readBatch(ctx context.Context) (batch []outboxRow, heldBack bool, err error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT seq, type, payload, tx_id < pg_snapshot_xmin(pg_current_snapshot())
		 FROM outbox ORDER BY tx_id, seq LIMIT $1`, batchSize)
	if err != nil {
		return nil, false, fmt.Errorf("cannot read outbox: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var row outboxRow
		var ready bool
		if err := rows.Scan(&row.seq, &row.eventType, &row.payload, &ready); err != nil {
			return nil, false, fmt.Errorf("cannot scan outbox row: %w", err)
		}
		if !ready {
			heldBack = true
			break
		}
		batch = append(batch, row)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("cannot read outbox: %w", err)
	}
	return batch, heldBack, nil
}
