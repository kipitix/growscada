package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/event"
)

const (
	// defaultPollInterval is how often Run looks into the outbox without a
	// Notify: it picks up rows written by a repository that has no signal.
	defaultPollInterval = time.Second
	// batchSize is how many rows DrainOnce reads at a time.
	batchSize = 100
)

// Dispatcher delivers the outbox rows to its sink after commit, in commit
// order, and deletes each delivered row: at-least-once, since a row
// delivered but not yet deleted when the process stops is delivered again.
// There is one Dispatcher per database (ADR 0008).
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
// every poll interval. An error is logged and retried on the next round.
func (d *Dispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()
	for {
		if _, err := d.DrainOnce(ctx); err != nil && ctx.Err() == nil {
			slog.Error("outbox delivery failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-d.wake:
		case <-ticker.C:
		}
	}
}

// DrainOnce delivers every row in the outbox, oldest first, and returns how
// many it delivered. A row whose event cannot be decoded can never be
// delivered: it is logged and dropped so that it does not hold back the rest.
func (d *Dispatcher) DrainOnce(ctx context.Context) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	delivered := 0
	for {
		rows, err := d.readBatch(ctx)
		if err != nil {
			return delivered, err
		}
		if len(rows) == 0 {
			return delivered, nil
		}
		for _, row := range rows {
			e, err := row.decode()
			if err != nil {
				slog.Error("dropping undeliverable outbox row", "seq", row.seq, "type", row.eventType, "error", err)
			} else {
				d.deliver(e)
				delivered++
			}
			if _, err := d.db.ExecContext(ctx, `DELETE FROM outbox WHERE seq = $1`, row.seq); err != nil {
				return delivered, fmt.Errorf("cannot delete outbox row %d: %w", row.seq, err)
			}
		}
	}
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

func (d *Dispatcher) readBatch(ctx context.Context) ([]outboxRow, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT seq, type, payload FROM outbox ORDER BY seq LIMIT $1`, batchSize)
	if err != nil {
		return nil, fmt.Errorf("cannot read outbox: %w", err)
	}
	defer rows.Close()

	var batch []outboxRow
	for rows.Next() {
		var row outboxRow
		if err := rows.Scan(&row.seq, &row.eventType, &row.payload); err != nil {
			return nil, fmt.Errorf("cannot scan outbox row: %w", err)
		}
		batch = append(batch, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read outbox: %w", err)
	}
	return batch, nil
}
