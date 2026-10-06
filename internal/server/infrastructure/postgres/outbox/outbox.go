package outbox

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kipitix/growscada/internal/server/domain/event"
)

// Append writes events to the outbox in tx, the transaction that saves the
// aggregate which recorded them, keeping their order. Nothing is written for
// no events. Each row carries the id of tx (the column's default); the
// Dispatcher orders by it, so writers take no lock here.
func Append(ctx context.Context, tx *sql.Tx, events []event.Event) error {
	if len(events) == 0 {
		return nil
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
