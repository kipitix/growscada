# One aggregate per transaction; an aggregate's events are stored with it in an outbox

Every transaction changes exactly one aggregate. The aggregate records its own domain events (Created on `Create`, never on `Reconstitute`; Deleted on `Delete()`, a Scene also recording one per Widget), and the repository's `Save`/`Delete` writes the new state (compare-and-swap on the Version it was loaded with) and those events into an `outbox` table in one transaction. A single dispatcher delivers outbox rows after commit, in global order: first to reliable subscribers (policies, which may fail and are retried), then to the in-process EventBus (SSE), then deletes the row — at-least-once. Changes spanning aggregates, such as reconciling Widgets with a changed WidgetType, are idempotent policies triggered by events, each Scene in its own transaction: eventual consistency, not a multi-aggregate transaction. Operations that state no expected Version (removal, policies) reread and retry on a write race instead of reporting an edit conflict. We chose this so that "rolled back ⇒ nothing published" and "committed ⇒ eventually delivered" hold by construction, and events cannot be forgotten by a service. We assume one server process per database.

## Considered Options

- A unit of work spanning several repositories (`uow.Do`, events raised by the service, published after commit): rejected. It makes multi-aggregate transactions the easy path and keeps event publication a service-side duty that can be forgotten (task 21).
- Publishing events from memory right after commit, without an outbox: rejected. A crash between commit and a policy loses the reconciliation for good.
- The Journal (ADR 0002) as the outbox: rejected. The Journal is the permanent operational record of the Runtime node with its own contract; Draft edits are not journaled, yet their events (e.g. `widget_type_updated`) still need reliable delivery. The Journal will be written in the same `Save` transaction, beside the outbox.

## Consequences

- An atomic DraftChange touching several aggregates (task 38) is not served by a multi-aggregate transaction; it calls for the Draft as the aggregate. EditLock makes it a single-writer boundary, so its size costs no contention.
- The outbox payload is an internal codec, not a contract (ADR 0005): rows live seconds, and a format change must keep reading the previous one until the outbox has drained.
- With a second server process on the same database both dispatchers would deliver every event and SSE clients of one would miss the other's events; that needs a different design.
