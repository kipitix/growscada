# The Journal records changes of operational state, not writes; its live stream is at-most-once with resume

ADR 0002 chose state tables plus a single, totally ordered Journal of "fat" entries — the full new state of what changed — and left the format open (task 42). Since then the Runtime node settled around that choice: Tag state writes are the hot path and publish `tag_updated` after the commit, at-most-once, without an outbox row (ADR 0011); the Runtime copies of the configuration are rebuilt by a Revision's transaction, so configuration changes only with a Revision; the Draft keeps its own before/after log (ADR 0017) and is not journaled. The Journal is now designed against that.

## Decision

- **One Journal, one order.** `runtime.journal` is append-only; every row belongs to exactly one Project (`project_id`, cascading with it) and takes the next number of one sequence of the server, `seq`. `seq` is the Journal position: the total order by which a reader catches up, a Checkpoint fixes a moment and History reads a range.
- **Two entry types, configuration never among them.** `tag_state` carries `{tag_id, value, quality}`; `revision` carries `{number, cause, rolled_back_from?}`, never the ProjectFile — the document is immutable in `runtime.revisions` and the entry's number addresses it. Tags, Devices and Environments are configuration: they change only through a Deploy, so the Revision entry is the address of their whole new state. Runtime-origin entities (task 48) will add their own types when they exist.
- **An entry only when the state changes.** A write whose value and Quality equal the stored state appends nothing and keeps the previous position; `runtime.tag_states.last_write_at` (and `written_at` in the API) records that the write happened. A stable signal therefore costs no rows, and "is the Device alive" is answered by the write moment, not by the Journal.
- **Written in the transaction that changes the state.** The repository of the changed state appends the entry — the tag state's write locks the Tag's row, compares and appends in one transaction, so two writers of one Tag are serialized by the row lock and take their positions in its order; a Revision appends inside the transaction that numbers it. This is the outbox's pattern (ADR 0008), with the Journal in place of the outbox: no second writer, no policy, no aggregate split.
- **The envelope names the change, the payload names the entity.** `{schema_version, seq, timestamp, project_id, type, tag_state|revision}` with the entity's ID inside the payload. No `aggregate_id`: a Journal is read by time and by position, never by entity, and a NULL for the Revision's identity would be a hole rather than a key.
- **The live stream reads the Journal and is at-most-once.** The Runtime node serves the entries as an SSE stream whose messages carry `id: <seq>`; a connection starts from `Last-Event-ID` or an explicit position and first replays what the table holds, then follows the writes. Replay is exact because it reads the table; live delivery is at-most-once, like `tag_updated` (ADR 0011) — concurrent commits may arrive out of order or, rarely, one may be skipped, and the next entry of the same Tag carries its whole state. History never depends on the live stream: it reads the table.
- **Positions for server-side subscribers.** `runtime.journal_cursors(subscriber, last_seq)` holds the position of a subscriber that reacts to operation (alerts, computations); those subscribers are the Runtime node's, as the Journal is.
- **The record goes with its Project.** Deleting a Project deletes its Journal, as it deletes its Tag states, Revisions and Device Tokens. Archiving a period is PlaybackFile's job (task 46), and retention, partitioning and a time-series store stay deferred, as ADR 0002 said.

## Considered Options

- **An entry for every accepted write.** A literal "everything that happened", and the write moment would need no column — but one stable Tag at 1 Hz is 86 million rows a day, History would be mostly duplicates, and the Journal would become a call log rather than a record of change. Rejected.
- **Change-only with a deadband on numeric values.** Less noise still, but it hides small changes, needs a threshold per TagType, and turns a number that future tasks would have to justify. Rejected; a historian-style deadband can be added as a filter when someone needs it.
- **The Revision's document inside the entry.** The Journal would be self-sufficient for a PlaybackFile, at the price of duplicating every ProjectFile (potentially megabytes) and making the Revision stop being the single truth (ADR 0014). Rejected.
- **One sequence per Project.** A self-contained log per Project and no gaps after a deletion, but the server would need a lock or a per-Project counter on the hot path, and the outbox-style ordering machinery of ADR 0008 would have to be repeated. Rejected.
- **Journal rows through the outbox.** One row per value change for an order `seq` already gives, and a second table to keep in step. Rejected (ADR 0011 rejected the same for the outbox).
- **An in-memory ring buffer for replay.** Cheap resume until the process restarts, when every client would silently lose the window. The table is the record; replay reads it. Rejected.
- **A gap-free live stream (the outbox's `tx_id` ordering).** It would order entries by transaction rather than by position and put a `tx_id` column and a hold-back rule into the Journal only to close a race that full-state entries and an exact replay already tolerate. Rejected.

## Consequences

- `runtime.tag_state_seq` disappears: a Tag state's `seq` is the Journal position of its latest change, and the API `seq` means that position. A write that changes nothing answers the previous `seq` and publishes no `tag_updated`.
- The Journal is neither the outbox nor event sourcing: it is a record beside the state, and the state tables stay the source of truth.
- Checkpoints (task 44), PlaybackFile (task 46) and History (task 45) are built on positions; Player (task 43) starts from a Checkpoint or the current state and applies entries after it.
- The record format is a contract of its own (`contract/record/v0`, ADR 0005): it is stored forever, so it never imports the API's types — it mirrors the Revision causes, as `devicelink` mirrors the API's enums (ADR 0004).
- A client that needs every entry gets it by replaying from a position, not by trusting the live stream; this is the same trade ADR 0011 made for `tag_updated`.

## Amendment 2026-11-08: the configuration entry is `deployment` (task 72_9)

The Journal entry that addresses a configuration change was `revision` before task 72_9 and is `deployment` now; its payload is unchanged (`{number, cause, rolled_back_from?}`), and it still addresses the document in the Runtime node's store, now `runtime.deployments`. The migration rewrites the type of existing rows, and readers still accept a row or PlaybackFile that carries the old type and the old payload member. Read every "revision" entry below as a "deployment" entry.

## Amendment 2026-11-08: a deployment entry names its Release (task 72_4)

The `deployment` payload gains `release_number` (nullable, MINOR): the number of the Project Release the Deployment made, or nothing for a file deployment and for every entry written before task 72_4. The document is still not in the entry; the number addresses it. An old reader ignores the field, and an old row decodes as `nil`.
