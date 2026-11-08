# A DraftChange is a before/after snapshot log on the Draft; undo and redo move its pointer

An Engineer must be able to undo an intent that touched several parts of the Draft — deleting a Tag with its bindings, deleting a Scene with its Widgets, switching a pinned Library Release and removing the bindings it breaks — and the undo history must survive a browser reload and a change of the EditLock's holder. Compensating commands would replay the reverse of each operation against whatever the Draft looks like when they run, which breaks as soon as the operation's code changes; a full Draft snapshot per change would grow with the size of the Draft times the depth of the stack.

## Decision

- **One DraftChange per successful write of a Draft's content.** Creating, updating or deleting a Tag definition, Scene, Widget, Device, Environment, Environment Device, Library Pin or WidgetType — with every effect that belongs to the same intent, such as a Tag's bindings or a pin switch's broken bindings — is one row of `engineering.draft_changes` in the same transaction that writes the Draft. Renaming or deleting a Project or a Library, Deploy, Release, Discard, taking or giving the EditLock and undo/redo itself are not DraftChanges.
- **A row holds the full before and after state of every entity it touched.** The snapshot names each touched entity by kind and ID and carries its whole stored state; an entity a change created or removed has only one side. The encoding is internal, like the outbox's (ADR 0005), and carries a format version. A short human-readable description is stored with the row ("Delete tag pressure") and is what the UI shows as the next undo or redo step.
- **Undo and redo apply snapshots, not commands.** Undo writes the before states of the change at the stack's position, redo writes its after states; both raise the Draft's Version and publish the inverse of the change's domain events (undoing a Tag deletion publishes `tag_created` and the `widget_updated` of the Scenes whose bindings return) so the SSE stream, the UI and any policy react as they would to the forward edit. Undo and redo are not recorded as DraftChanges.
- **The stack lives on the Draft.** The Draft row holds the position; a new change after an undo deletes the redo tail and appends at the position. The depth is 100 changes; older rows are deleted and cannot be undone. Check in, Check out and a Take over leave the stack alone: the next holder undoes the previous holder's changes.
- **The stack is cleared by publishing.** After a successful Deploy (the Runtime node has made the Revision) and after a successful Release (the release rows are committed) the stack is cleared in a separate transaction, because Deploy crosses to the Runtime node and a Library Release is another aggregate (ADR 0007, ADR 0008). A failure to clear is reported as 500 while the Revision or Release stands and the stack remains. Discard clears the stack in the transaction that resets the Draft. Deleting the Draft takes its rows by the foreign key.
- **The REST API exposes the state and the two commands.** `GET …/draft-changes` answers `{version, can_undo, can_redo, undo_description?, redo_description?}`, and `POST …/draft-changes/undo` and `…/redo` take `{version}` and answer the same state after the operation; both are under a Project's and a Library's path, gated by the EditLock, and a stale Version or an empty stack is 409.

## Considered Options

- **Compensating commands**: undo replays the inverse of the original operation. Rejected: the inverse depends on the current state and on the code, and a change to an operation silently changes what an old undo does.
- **A full Draft snapshot per change**: simple and self-contained, but the volume is the whole Draft times the depth. Rejected.
- **A single `draft_changed` event with the list of affected entities**: the UI would reload the whole Draft and every policy would need to interpret it. The existing per-entity events keep the stream and the read-only modes working, and undo's inverse events make undo look like the edit it reverses. Rejected.
- **Silent undo and redo**: only the holder edits, so the response could be enough, but a second read-only tab and the History view would miss the change, and the UI would need a special reload path. Rejected.
- **Clearing the stack before Deploy or Release**: no window, but a failed publish would lose the history for nothing. Clearing after success, with the Revision or Release already the truth, is the smaller loss. Rejected.
- **Storing DraftChanges in the Journal**: the Journal is the Runtime node's operational record; Draft edits are not journaled (ADR 0002). Rejected.

## Consequences

- The outbox carries the inverse events of an undo or redo like any other Draft event; the policies that remain (cleanup of a deleted Project or Library) do not react to them.
- The stack is not a public contract: its rows and their encoding are internal, and a change of the encoding must read the previous one until the stack drains.
- A client that loses the EditLock loses the ability to undo, not the history; the next holder sees the same stack.
- The UI's `Ctrl+Z`/`Ctrl+Shift+Z` and its buttons read `GET …/draft-changes` and refresh on `EventResync` and on the content events.
- The contract grows by the DraftChange state and the two commands; `SchemaVersion` rises by a MINOR.

## Amendment 2026-11-08: publishing a Release clears no stack

A Project Release (task 72_3, task 72_4) and a Library Release (task 72_5) publish a Commit: they copy the content and change no Draft, so they no longer clear the DraftChange stack. Only a Discard and a Revert, which replace the Draft's content in one transaction, clear it. A Save clears nothing either (ADR 0021). The stack therefore survives a Release and the next undo may reach changes made before it.
