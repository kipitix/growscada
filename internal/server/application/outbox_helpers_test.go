package application_test

import (
	"context"
	"testing"

	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/outbox"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
)

func cleanOutbox(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM outbox"); err != nil {
		t.Fatalf("cleanOutbox: %v", err)
	}
}

// deliveredOnCommit empties the outbox and returns an EventBus that receives
// the outbox's events right after each commit of a repository built with the
// returned option: the dispatcher drains the outbox in the committing call,
// so a test sees the events as soon as the service returns.
func deliveredOnCommit(t *testing.T) (eventbus.EventBus, repositories.Option) {
	t.Helper()
	cleanOutbox(t)
	bus := eventbus.NewEventBus()
	d := outbox.NewDispatcher(testDB, bus.Publish)
	return bus, repositories.NotifyOnCommit(func() {
		if _, err := d.DrainOnce(context.Background()); err != nil {
			t.Errorf("DrainOnce: %v", err)
		}
	})
}
