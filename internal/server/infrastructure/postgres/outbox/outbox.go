package outbox

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kipitix/growscada/internal/server/domain/event"
)

// commitOrderLock is the transaction-level advisory lock Append takes before
// writing: transactions that write events then take their seq values and
// commit one at a time, so seq order is commit order. The lock is the last
// one such a transaction takes, held only to its commit.
const commitOrderLock = 0x6f7574626f78 // "outbox"

// Append writes events to the outbox in tx, the transaction that saves the
// aggregate which recorded them, keeping their order. Nothing is written for
// no events.
func Append(ctx context.Context, tx *sql.Tx, events []event.Event) error {
	if len(events) == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, commitOrderLock); err != nil {
		return fmt.Errorf("cannot lock outbox: %w", err)
	}
	for _, e := range events {
		data, err := Encode(e)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO outbox (type, payload) VALUES ($1, $2)`,
			e.Type().String(), data,
		); err != nil {
			return fmt.Errorf("cannot write %s to outbox: %w", e.Type(), err)
		}
	}
	return nil
}
