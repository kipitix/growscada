# Engineering/Runtime node split; state tables + Journal instead of event sourcing

Project editing and plant operation are separated into two roles of the same binary — the Engineering node (Draft, EditLock, DraftChanges) and the Runtime node (Revisions, Runtime-origin entities, Tag values, Journal, Checkpoints) — with separate Postgres schemas (`engineering.*`, `runtime.*`), separate migration chains, and no cross-schema queries: the Engineering node learns about the Runtime node only through its API, and Deploy is the delivery of a ProjectFile. This keeps the boundary honest even when both roles run in one process, so a future standalone Runtime node is a matter of connection strings and network Deploy, not a redesign.

We deliberately did **not** adopt full event sourcing. Current state stays in ordinary tables and is the source of truth; every operational mutation additionally appends a "fat" entry (full new state of what changed) to a single, totally ordered Journal, and full Checkpoints are taken on every Deploy and periodically. History at moment T = nearest Checkpoint before T + Journal entries up to T. Fat entries make replay idempotent and independent of event-application logic, which is what keeps the scheme simple. Undo/redo in the Draft uses its own before/after log (DraftChanges), cleared on Deploy/Discard, and is not journaled.

## Considered Options

- Full event sourcing (state rebuilt from events, undo as compensating events): rejected as too heavy for the value; schema evolution of old events becomes a permanent burden.
- Revision as copied rows with a `revision` column: rejected for making every repository and key revision-aware; Revision is an immutable jsonb document in ProjectFile format instead.
- Separate time-series store for Tag values: rejected for now in favour of one Journal; partitioning/Timescale can come later without changing the model.
