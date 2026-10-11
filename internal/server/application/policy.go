package application

import (
	"context"

	"github.com/kipitix/growscada/internal/server/domain/event"
)

// Policy brings state into line with the current state of what an event
// names, outside the transaction that recorded the event (ADR 0008). The
// outbox dispatcher enqueues a job for every delivered event the policy keys;
// a policy runner runs it later, again and again until it succeeds. Jobs with
// the same key merge, so Run must be idempotent and read what it brings into
// line when it runs, never take it from an event.
type Policy interface {
	// Name identifies the policy's jobs; it must not change between runs of
	// the server while jobs of the policy may wait.
	Name() string
	// JobKey reports whether the policy needs to run for e, and the key of
	// the job it needs: what it brings into line, e.g. a WidgetType's ID.
	JobKey(e event.Event) (string, bool)
	// Run brings into line what key names.
	Run(ctx context.Context, key string) error
}
