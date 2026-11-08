# Operation renders the running Revision from a Runtime read model and the Journal

Task 43 (Player and Operation). ADR 0014 made the Revision the only truth of what Operation runs; ADR 0019 made the Journal the record of every change of operational state and gave it a live stream that reads the table and resumes exactly. Until now Operation assembled its picture from several endpoints (`GET …/revision`, `GET …/tag-states`, and the content reads) and followed `tag_updated`/`revision_created` on `/events`. That reads the pieces of one picture at different moments, makes Operation a consumer of content events that are not its own, and leaves the recovery of a lost stream to a generic resync.

## Decision

- **Operation is a client-side Player over the Journal.** The Player begins from a starting state and applies Journal entries; it does not know whether the entries are live or replayed. Operation is the live instantiation (task 45 will be the replayed one). The Player owns the operational model — the Revision's configuration, the Tag states by ID, the Journal position and the state of its stream; the mode keeps its selection and rendering.
- **One read model, `GET /api/v0/projects/{id}/operation`.** The Runtime node answers `{position, revision, tag_states}`: the Project's current Journal position, the running Revision's document (`RevisionResponse` with `projectv0.ProjectFile`, `null` until the first Deploy) and the states of all its Tags, an unwritten Tag as its TagType's zero value with Quality bad and position 0. A live Project answers 200 even before its first Deploy, so Operation can subscribe and catch the first Revision; a Project that is gone is 404.
- **The configuration is the Revision document as it is.** The read model never builds render DTOs: the transform matrix and the WidgetType content are the Scene's domain, which belongs to the Engineering node (ADR 0011), and a Runtime-node read model must not import it. The UI maps the ProjectFile to what `livescene` renders, as it did before.
- **The live feed is `GET …/journal/events`.** Operation subscribes from the snapshot's position and applies `tag_state` and `revision` entries. `/events` keeps `tag_updated` and `revision_created` for the other modes and the Device Gateway; Operation no longer reads it, and the fate of those events on `/events` is a separate task.
- **A starting state is read before every explicit connection.** The Player re-reads `GET …/operation` on mount, on a Project switch and on a reconnect the browser has given up on, then subscribes from its position. The Journal's live delivery is at-most-once (ADR 0019); the resync is the safety net the stream cannot provide alone.
- **Inside the snapshot the position is read first, then the Revision, then the states.** A Deploy that commits between the reads takes a position above the snapshot's, so the stream replays it; reading the position last could hide a Deploy from Operation forever. Entries that the states already include come again and are harmless: every entry carries the full new state, and for each Tag the greater `seq` (for each Revision the greater number) wins.
- **`revision` entries are applied by number.** The Player reads `GET …/revisions/{n}` and swaps the configuration, keeping the Tag states by ID — a new Tag is zero/bad until written, a removed Tag is no longer rendered. A failed document read is lost connectivity: the old configuration stays on screen, the view shows disconnected and the full resync retries.
- **The Player redraws in batches.** Entries are applied as they arrive, but the redraw is requested at most once per short interval, so a burst of Tag changes costs one render.
- **No periodic resync.** The stream's own table catch-up and the next entry of the same Tag heal the documented race (ADR 0019); a long-lived connection may stay unaware of an entry skipped by that race until the next reconnect. That is the accepted price of the at-most-once stream.

## Considered Options

- **Assembling Operation client-side from `/revision` + `/tag-states` + a position.** Fewer new contracts, but several requests whose moments differ, and a position read after the states could hide entries; the resync would have to paper over that. Rejected.
- **A render-shaped read model (scenes and widgets with matrices).** It would duplicate the Scene's geometry and WidgetType content in the Runtime node, which may not import the Engineering node's domain (ADR 0011), or force a second implementation of the matrix. Rejected.
- **Keeping Operation on `/events`.** The event stream carries no Journal position, so a reconnect cannot resume exactly, and the mode would keep consuming content events that must not affect what Operation shows. Rejected for Operation; the stream stays for the other modes.
- **A server-side Player.** The PWA renders live in the browser; a server-side picture would need its own transport and duplicate the UI. Rejected.
- **A periodic resync.** It bounds the staleness left by the documented race, at the cost of re-reading the ProjectFile and restarting the stream every few minutes. Deferred until staleness is observed in practice.

## Consequences

- `contract/api/v0` gains `OperationPath` and `OperationStateResponse` (MINOR 0.22 → 0.23); `contract/record/v0` is unchanged and no migration is needed.
- The Runtime node gains a query service (`application/runtime.OperationStateService`) composing the Revision, Tag-state and Journal reads; the Engineering node never calls it.
- Operation makes its own SSE connection next to the Bar's; `/tag-states` and `/revision` stay for the other readers, but Operation no longer uses them.
- The Player is the piece History reuses: a starting state plus a source of entries, live or replayed (tasks 44–45).
- `tag_updated` and `revision_created` stay on `/events`; removing them is not part of this decision.

## Amendment 2026-11-08: Operation reads a Deployment (task 72_9)

The running configuration is a **Deployment** now. `GET …/operation` answers `{position, deployment, tag_states}` (the member was `revision`), the Player applies `deployment` Journal entries and reads a document with `GET …/deployments/{n}`, and the live signal is `deployment_created`. Nothing of the read model's order, resync or batching changes. Read every "Revision" below as "Deployment".

## Amendment 2026-11-08: the Deployment carries its Release (task 72_4)

`GET …/deployment` and `GET …/operation` serve the Deployment's `release_number` beside its cause; Operation and the Player ignore it, while the Project mode compares it with the Release list to mark the running Release. Nothing of the read model, the Journal order or the resync changes.
