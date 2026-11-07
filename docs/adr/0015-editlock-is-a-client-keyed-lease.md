> **Superseded by [ADR 0022](0022-draft-is-edited-without-editlock.md).** Kept as the history of the decision: the EditLock, the client key and the read-only Draft are gone.

# The EditLock is a lease held by a client key, kept alive by the SSE connection and checked in the write's transaction

The Draft of a Project or of a Library has one editor at a time. There are no Users yet (task 49), so the only identity a client has is what it can say about itself: the PWA is a browser tab with one SSE connection, and every edit is a separate HTTP request that no connection can be inferred from. We make the EditLock an Engineering node's marker aggregate over one Draft, held by a key the tab keeps for itself and kept alive by the tab's SSE connection, and we check it inside the transaction of every gated write.

## Decision

- **One aggregate per Draft.** `engineering.edit_locks` holds one row per Draft: two nullable foreign keys, `project_id` (to `projects`) and `library_id` (to `engineering.libraries`), with a CHECK that exactly one is set, both `ON DELETE CASCADE`, and partial unique indexes. The row stays when the lock is free, so its `version` grows monotonically across holders and a client can tell a stale observation from a fresh one.
- **The holder is a Client: a key plus a declared name.** The UI generates a UUID once and keeps it in `sessionStorage` (one key per tab, surviving a reload), and the user declares a display name. The key travels in the `X-GrowSCADA-Client` header of every gated write and as the `client` query parameter of the SSE stream. It is coordination, not authentication: there is no secret, any client may declare any name, and any client may take the lock over.
- **Liveness: SSE presence, or a recent request.** A held lock is alive while an SSE stream with the holder's key is attached (the in-memory presence of the Engineering node, by reference count) or while `last_seen` is younger than 5 minutes. A reaper runs every 30 s: for a held lock it refreshes `last_seen` while the key is present, and releases the lock (`edit_lock_expired`) once the key is absent and `last_seen` is older than the TTL. Refreshing `last_seen` while present is what makes a server restart harmless: a tab reconnects within the TTL and keeps its lock. A client without SSE (growctl, Bruno) keeps its lock alive with its writes and loses it 5 minutes after the last one.
- **Gated operations.** Every write of a Draft's content (Tag definitions, Scenes with their Widgets, Devices, Environments and their pairs, Library Pins, WidgetTypes) and the acts over the Draft as a whole: Deploy, Release, Discard. Not gated: Tag state writes, Device Tokens, creating, renaming or deleting a Project or a Library, and Rollback — the Runtime node's act, which reads no Draft (ADR 0014).
- **The check lives in the write's transaction.** The scope repository reads the holder row `FOR SHARE` before changing anything and refuses a non-holder with `ErrEditLockNotHeld` (409, the current holder in the Problem Details). A takeover writes the same row, so it serializes with a write. Release and Discard check the lock in the transaction that writes the release rows or the discard mark; Deploy, whose build spans several queries and has no single transaction, checks the lock before it builds. Policies write as `System` and pass no check.
- **Taking, giving and taking over.** Check out on a free lock (409 with the holder when held; the same key again is 200), Check in by the holder (a no-op when the lock is free, 409 when held by another), and Take over while held, stating the version the client saw (a stale version is 409; the holder's own key is a no-op). Each publishes `edit_lock_acquired`, `edit_lock_released`, `edit_lock_taken_over` or `edit_lock_expired`; a Project's on the Project's stream, a Library's on the server's.
- **Checkout is refused while a Discard of the scope runs**, like any write of it.

## Considered Options

- The holder is the SSE connection itself, its server-generated ID delivered in the stream: a reload or a reconnect would lose the lock for everyone until the TTL; rejected.
- A pure lease refreshed by a heartbeat request, with no SSE presence: no coupling to delivery, but the server cannot tell a closed tab from a paused one and the UI must invent a timer; rejected as the primary signal, though a request with the key still refreshes the lock for clients that have no stream.
- No server enforcement, the UI alone: the read-only Draft would be a convention, and growctl or a second tab would ignore it; rejected.
- The lock in memory only: a restart, or a second process, loses or duplicates it; rejected.
- `423 Locked`: semantically exact, but a status no other route uses; `409` with the holder is how the rest of the API words state conflicts.
- A trusted "automation" bypass for growctl and the Bruno collection: it would make the server's read-only claim false; they take the lock like any client.
- Gating Project/Library rename and delete as well: a rename is not an edit of the Draft and a delete is a lifecycle act that must not wait for a forgotten tab; the check would also have to live in a row write shared by gated and ungated acts. Left out.

## Consequences

- Every writer must check out first. growctl wraps a mutating run in Check out/Check in with a key of its own, the Bruno collection has a setup and a teardown, and the PWA checks out from the header; a gated write without a key is 400.
- The check reads another aggregate's row inside the write transaction — the same transitional row lock as the Scene↔Library Pin one of ADR 0012. Task 38 folds the lock into the Draft aggregate: the lock becomes a field of the Draft, its row is the Draft's, and the extra read goes away.
- Presence is per server process, like the event bus; ADR 0008 already assumes one process per database.
- It is coordination, not security. User identity (task 49) and access rights (task 50) later replace the key with a User and can restrict who may take over.
- A tab that is closed, crashes or loses the network keeps the Draft for up to 5 minutes; a colleague takes over instead of waiting, and the former holder's UI switches to read-only on the event.
- Checkout gates Deploy, Release and Discard, so publishing a Draft someone edits means taking the lock first. Rollback stays available to anyone.
- A Project or a Library can be deleted while its Draft is held; the holder learns from the deletion event. The lock row goes with the row by the foreign key.
