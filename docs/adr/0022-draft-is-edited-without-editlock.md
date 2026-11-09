# The Draft is edited without an EditLock; its Version is the only concurrency token, and the Author is a declared name

ADR 0015 gave every Draft one editor at a time: a client key checked out a lock, every write of the Draft was refused without it, and the UI was read-only without it. In practice the lock was the hardest part of the editor to explain: the key is not an identity, the name is not an account, a tab that lost its stream lost its Draft five minutes later, and a tool had to wrap every write in Check out/Check in. The Draft is now one aggregate with one Version (ADR 0016); a stale Version already refuses a write honestly. The lock only added a second, unrelated token and a read-only mode on top of it. This ADR removes the EditLock. It is the first part of `72_save_release_deploy`; the declared author and an explicit Save come in 72_2.

## Decision

- **No lock over a Draft.** `internal/server/domain/editlock` is gone. Its `Scope` — a Project's or a Library's Draft — moves to `domain/draft` as `draft.Scope`, with `ProjectScope` and `LibraryScope`, and carries no version of its own. `engineering.drafts` loses `holder_key`, `holder_name`, `held_since`, `last_seen` and `lock_version` (migration `engineering/20261106000000_remove_edit_lock`).
- **The Version is the concurrency token.** Every write of a Draft's content states the Version the client saw. `draftRepositoryPostgres.Update` locks the Draft's row and compares it: a stale Version is `draft.ConflictError` — 409 `draft_conflict`, the current Version in `Problem.draft`. Two Engineers on one Draft see each other's writes this way and retry; there is no 400 "no client" and no 409 "the lock is held".
- **Undo and redo are shared.** Nothing says whose DraftChange a stack row is: the second Engineer may undo the first one's change. This is accepted until Users exist (tasks 49/50).
- **A build outside a transaction is checked against the Version.** Deploy of the Draft (before task 72_4) read the Draft's Version V, built the ProjectFile and checked V again in a short transaction (`draft.Repository.CheckVersion`). Save does the same now, under the Draft row's lock (`draft.Repository.SaveCommit`), and Deploy takes a Release instead and reads no Draft (ADR 0021). `CheckVersion` therefore has no caller since task 72_4.
- **Release, Rollback and Discard need no such check.** Release publishes an immutable Commit and reads no Draft Version; Discard replaces the whole Draft in one transaction; Rollback reads no Draft (ADR 0014).
- **The lock's surface goes.** The domain aggregate, `Holder`, `Snapshot`, the `edit_lock_*` events, `application.EditLockService`, `EditLockGate`, `RunEditLockReaper`, presence (`Attach`/`Detach`) and `editlock.System` are deleted. The `Problem.edit_lock` member, the DTOs and `contract/api/v0/editlock.go` are deleted; `client_connected`/`client_disconnected` stay. The four routes on a Project and on a Library (`GET/POST/DELETE …/edit-lock`, `POST …/edit-lock/takeover`) answer **410 Gone** in Problem Details with the hint that the lock is gone and the Draft is edited and saved; they are removed with API 1.0, as `/api/v1` is.
- **No client key.** The `X-GrowSCADA-Client` middleware goes; `?client=` on `/events` and `/projects/{id}/events` is accepted and ignored, so an older tab does not fail. `eventHub` counts no presence.
- **The UI loses the read-only mode.** The lock row, `editlock:client`, `editlock:name`, the lost-lock toast and the disabled controls go; edits, Deploy, Release and Discard are always available, gated by the Version and the server's invariants. A stale Version is a toast and a reread of the content and the stack.
- **growctl no longer checks out.** A mutating run just states the Version; `--client-name` and `--take-over` are gone. Bruno drops its lock setup and teardown and its client header; the four removed routes keep a `410` request each.
- **The author is not introduced here.** The name the user declared for a lock has no meaning without a lock. 72_2 adds `author:name` (the declared name of whoever saves); Save and Release record it (ADR 0021).

## Considered Options

- **Keep the lock and add Save**: it would still be a second concurrency token beside the Version, and the Save would have to check both. Rejected.
- **Keep the lock but drop its enforcement** (the UI shows who edits, writes pass): a marker that refuses nothing and frees itself by a timer is a comment, not a lock. Rejected.
- **Make Undo/Redo personal** so the second Engineer cannot undo the first's change: it needs Users and a per-user stack. Left to tasks 49/50.
- **Delete the `/edit-lock` routes outright** (404): an older tab would look like a wrong URL; a 410 says what happened and where the API went, at the cost of four routes until 1.0. Chosen 410.
- **Check the Version once, before the build**: a write during the build would produce a ProjectFile mixing two states, exactly what the lock was hiding. The second check is cheap and precise.

## Consequences

- A write of a Draft with no Version, or with a stale one, is the only refusal; the 400 "no client" and the 409 "lock held" disappear.
- The UI is simpler: one header row less, no disabled state from a missing lock, and one toast for a conflict that a reread resolves.
- Deploy is not serialized with an edit, and does not need to be: it sends a snapshot of one Version. A write after the check does not affect the Revision being made.
- A second tab that writes after the first wins the race; it receives the current Version and can retry without rereading. A tab that reads while another edits does not see a read-only screen, and its write may be refused — the intended behaviour.
- The Engineering node serves no presence and runs no reaper: one background task less, one table column set less.

## Amendment 2026-11-09: the Author is a declared name, and nothing but a reset clears the stack (task 72_8)

- **The Author is declared, not authenticated.** A Save and a Release name an Author: the UI keeps `author:name` in LocalStorage and fills the Save/Release dialogs from it; growctl takes `--author`, else `GROWSCADA_AUTHOR`, else the operating system's user name, and refuses an empty one. It is recorded with the Commit and the Release (ADR 0021) and shown to other Engineers. It is not an account and blocks nobody; Users come with tasks 49/50.
- **The UI is fully editable.** No mode has a read-only state from a missing lock: Project and Library edit, Save, Release and Deploy freely, and the only read-only views are a Release or a Commit shown for reading. A stale Version is a 409 that rereads the content; the shared undo/redo stack remains accepted until Users exist.
- **Deploy and Release read no Draft.** Task 72_4 made Deploy take a Project Release, so it checks no Version and builds nothing; task 72_5 made Release take an immutable Commit. The Version guards the writes of the Draft and the Save alone.
- **Only a reset clears the stack.** Save adds no DraftChange and raises no Version; Release and Deploy change no Draft. Discard and Revert, which replace the Draft's content in one transaction, are the only acts that clear it (ADR 0017's amendment). This supersedes the sentence in ADR 0021's introduction that Deploy, Release and Discard clear the stack.
