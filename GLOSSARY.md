# GrowSCADA

A web-based SCADA system: it exposes process variables (Tags) and lets operators view and interact with them through custom-built visual screens (Scenes) made of reusable widgets. This glossary names the concepts of that domain; the decisions behind them are in `docs/adr/`.

## Language

### Data

**Tag**:
The atomic process variable — an identifier, name, type, current value, and quality — belonging to exactly one Project and supplied by at most one Device, or by people.
_Avoid_: variable, point, signal

**Tag definition**:
A Tag's name, TagType and supplier: configuration, part of the Draft and of a Deployment.
_Avoid_: tag config, tag metadata

**Supplier**:
What writes a Tag's value and Quality: one Device, or people — at most one per Tag, part of its definition.
_Avoid_: writer, producer, owner, source

**Tag state**:
A Tag's current value and Quality: process state, held by the Runtime node, never part of the Draft or a Deployment. The latest write wins.
_Avoid_: tag value, reading, live data

**Quality**:
A Tag's reliability indicator: `Bad`, `Uncertain`, or `Good`. Describes how trustworthy the value is, never where it came from.
_Avoid_: status, state; simulated/forced/unknown as a Quality

**TagType**:
A Tag's data type: `String`, `Boolean`, or `Integer`.
_Avoid_: analog/discrete, data type

**Device**:
A piece of equipment that supplies Tag values, as described in a Project: its protocol and how each of its Tags is extracted from that protocol. Where it stands and how to reach it depends on the Environment.
_Avoid_: source, adapter, PLC, driver

**Environment**:
A named set of one Project's Devices as they stand in one context of operation — a test bench, a plant, a simulator. A Deploy does not choose an Environment.
_Avoid_: deployment, stage, profile, instance

**Environment Device**:
One Device as configured in one Environment: its connection parameters.
_Avoid_: device binding, deployment, connection

**Device Gateway**:
The external process that executes one Device's configuration and delivers its Tag values to the Runtime node. One Device Gateway serves one Device.
_Avoid_: connector, adapter, driver, agent

**Device Token**:
The secret by which a Device Gateway acts for one Device: it writes that Device's Tags and nothing else. Issued on the Runtime node, never part of a Deployment; outlives Deploys and Rollbacks.
_Avoid_: API key, password, credential (unqualified)

**Draft Tag state**:
A Tag's value and Quality as set in the Draft — state, not configuration, living apart from the Tag definition. Never carried into a Deployment, which gives the Tag its ordinary initial state instead.
_Avoid_: initial value, default value, sample value

### Visualization

**WidgetType**:
A reusable template defining how a widget renders: an HTML template plus a script, a default size, and the InputPorts it exposes. Part of one Library's Draft.
_Avoid_: component, widget definition

**InputPort**:
A named, typed input slot declared on a WidgetType; the script receives it by name carrying the bound Tag's current value.
_Avoid_: parameter, slot

**Widget**:
A placed instance of a WidgetType on a Scene — positioned, sized, rotated, optionally labelled, and bound to Tags via PortBindings.
_Avoid_: element, control

**PortBinding**:
The link between one of a Widget's InputPorts and the Tag that feeds it. A binding that stops being valid is removed, never the change that broke it refused.
_Avoid_: mapping, connection

**Scene**:
A named canvas with fixed dimensions, a static HTML background, and the Widgets placed on it; the Scene and its Widgets change together.
_Avoid_: screen, mnemonic, page, display

### Organisation

**Organization**:
The tenant: the top-level owner of Projects and Libraries. Everything a server stores belongs to exactly one Organization.
_Avoid_: tenant (in UI), company, account, team

**Project**:
Owned by one Organization. The unit of operational ownership and isolation: Tags, Scenes with their Widgets, Devices, Environments, the Draft, Deployments and the Journal belong to exactly one Project. It carries a name, unique among the live Projects of its Organization, and an optional description.
_Avoid_: workspace, site, solution

**Library**:
A reusable collection of WidgetTypes owned by an Organization, outside any Project, and used by any number of Projects. It carries a name, unique among the live Libraries of its Organization, and an optional description.
_Avoid_: catalog, toolbox, package

**Library Import**:
The choice, in a Project's Draft, of one saved state (Commit) of one Library: the WidgetTypes of that Commit are what the Project uses. Switching or removing it is refused while Widgets would be left without their WidgetType; the PortBindings it breaks need confirmation. A Library is removed only while no live Project imports it and no live Project's running Deployment names it (task 71).
_Avoid_: pin, dependency, reference, subscription, release

**Import**:
The act of choosing the saved state of a Library a Project uses. A Project imports at most one saved state per Library; the choice is part of its Draft and is saved into the ProjectFile as the copy's provenance.
_Avoid_: pin, attach, subscribe

### Roles and clients

**Engineer**:
The role that builds the project: edits the Library, Scenes, Tag definitions, Devices and Environments in the Draft, and deploys it. A role, not an account.
_Avoid_: designer, developer, admin

**Operator**:
The role that works on site in Operation mode: watches and controls the process. A role, not an account.
_Avoid_: user

**Client**:
A browser tab following the server's events over one SSE connection. It is a connection, not an identity: no write of a Draft is attributed to it (ADR 0022).
_Avoid_: user, session, account

**Author**:
The name an Engineer declares when saving a Draft; it is recorded with the save and shown to other Engineers (task 72_2). A declared name, not an account: Users come with tasks 49/50.
_Avoid_: user, owner, editor

### Nodes

**Engineering node**:
A GrowSCADA server acting in the role that holds the Draft and its DraftChanges, and deploys to a Runtime node.
_Avoid_: dev server, designer server, studio

**Runtime node**:
A GrowSCADA server acting in the role that runs Deployments and keeps the Journal and Checkpoints, serving Operation and History. Runtime-origin entities are born here.
_Avoid_: production server, station, runtime (unqualified)

### Project lifecycle

**Draft**:
The working state of a Project or of a Library as it is being edited, and the unit of consistency: every change to it is one DraftChange, and its Version is the only token a write states (ADR 0016, ADR 0022). It is edited without a lock; two Engineers see each other's writes as a conflict on the Version. A Project's Draft holds Scenes with their Widgets, Tag definitions, Devices, Environments and the imported Library saved states; a Library's holds its WidgetTypes. It also keeps the history of its Commits.
_Avoid_: working copy, unsaved changes

**Clean Version**:
The Version of a Draft at which its content last equalled its last Commit: a Save puts the Draft there, and so does a reset whose target is that Commit's content. A Draft has unsaved changes exactly while its Version is above its Clean Version; one nothing was ever written to is clean, and one written but never saved has none. It is a fact of the Draft, not a flag a tab keeps.
_Avoid_: saved version, base version, dirty flag

**DraftChange**:
One Engineer intent applied to the Draft — possibly touching several parts of the Draft at once — and the unit of undo/redo. The Draft keeps the stack of them; only a Discard or a Revert, which replace the content, clear it — a Save and a Deploy leave the stack alone, and a Label change never touches it (ADR 0017, ADR 0021).
_Avoid_: edit, step, command, operation

**Commit**:
An immutable, numbered snapshot of the whole Draft, fixed by a Save: its canonical ProjectFile for a Project, its WidgetTypes for a Library, with a title, the Author and the time (task 72_2; ADR 0021). It is an entity of the Draft, not a DraftChange: writing it raises no Version and clears no undo stack. A Project's Commit can be deployed; a Library's can be imported by a Project. The UI calls it a **saved state** (task 84) — the code and the REST API keep the name Commit, and the two name one thing.
_Avoid_: revision, version, checkpoint, snapshot (unqualified)

**Saved state**:
The name the PWA gives a Commit (task 84): what a Save fixes, what the History lists, and what a person may load back into the Draft (`POST …/commits/{n}/load`) or choose in Deploy and Import. It is a name for people, not a second concept: the domain, the tables and the API say Commit, and a document that speaks of the API may say either.
_Avoid_: snapshot, checkpoint, revision, save point

**Label**:
A named mark with an optional color that an Engineer puts on saved states to group and filter them: a saved state may carry any number of labels, and one label may mark any number of saved states. Its name is unique in the Organization without regard to case. It belongs to the Organization, not to a Project or a Library, and it is not content: no Draft, Commit's ProjectFile or Deployment carries it. A Label owns its assignments.
_Avoid_: tag (a Tag is a process variable), status, release, version

**Label assignment**:
The placing of one Label on one saved state: an entity of the Label, not an aggregate of its own. It changes no Draft, states no Version and is not part of undo/redo. Deleting a Label removes its assignments; deleting a Project or a Library removes the assignments of its saved states.
_Avoid_: tag, mark, label (as the relation)

**Save**:
The act of fixing the Draft's current state as a new Commit under a title and an Author (task 72_2; ADR 0021). It does not write the Draft: edits are written at once as DraftChanges, and a Save of a Draft at its Clean Version is refused with `nothing_to_save`. The UI calls the result a saved state (task 84).
_Avoid_: commit (as a verb), publish

**Discard**:
Resetting a Draft to its last Commit, dropping the unsaved DraftChanges (task 72_4, task 72_5). A Project or a Library with no Commit is refused with `nothing_to_discard`; **Revert to deployed** (a Project) is the different act of resetting it to the running state instead. The Draft is its Clean Version afterwards, unless the Library Imports the Commit names could not be restored (task 71).
_Avoid_: revert, reset, cancel

**Deploy**:
The act of making a saved state (Commit) of a Project — or, on a Runtime node without an Engineering node, a ProjectFile — the running Deployment (task 72_4). A Deploy takes a saved state, not the Draft: the order is Save → Deploy. A Deploy of the saved state that already runs is refused `nothing_to_deploy`.
_Avoid_: publish, apply, save

**Revert to deployed**:
Resetting a Draft to the Deployment the Project runs, dropping the unsaved DraftChanges — `POST …/projects/{id}/revert-to-deployed`. Discard resets to the last Commit instead; the Draft is its Clean Version afterwards only when that Deployment's document is the last Commit's content.
_Avoid_: discard, rollback

**Deployment**:
The Runtime node's immutable record that a configuration runs: its number, when it was deployed, its cause, the saved state it came from (`commit_number`, nil for a Deployment from a file or one made before saved states were deployed) and the document it holds. A Deploy of a saved state, a Rollback or a direct file Deploy on a Runtime-only node makes it; Operation always runs exactly one Deployment. The running Deployment of a Project keeps the Libraries its document imports from being deleted; a Revert of one that names an import this server lacks drops it with a warning (task 71).
_Avoid_: release, build, version (the concurrency counter), commit

**Revision**:
The former name of a Deployment, used before task 72_9; it survives only as the name of old Journal entries, PlaybackFiles and documents.
_Avoid_: using it for anything current

**Origin**:
Where a Tag or Widget came from: `Project` (defined in the Draft and deployed) or `Runtime` (created while the system runs).
_Avoid_: source, dynamic/static

**Adopt**:
Taking a Runtime-origin Tag or Widget into the Draft, keeping its identity.
_Avoid_: import, promote, claim

**Rollback**:
Making an earlier Deployment operational again, performed on the Runtime node alone; it produces a new Deployment with the earlier content and the earlier saved state (`commit_number`), records the one it copied and never rewrites history. The Project mode leaves it to the API and History (task 72_4).
_Avoid_: revert, undo

**ProjectFile**:
A portable file holding one project configuration (the content of a Deployment) and no operational history.
_Avoid_: export, package, backup

**Open a ProjectFile**:
Replacing the whole content of a Project's Draft with a ProjectFile — one DraftChange that undo reverses — after checking that the file names the Project and that the Library saved states it imports exist locally. Requires the Draft's Version.
_Avoid_: import, upload, load, restore

**Deploy a file**:
Making a valid ProjectFile the new Deployment of a Project without touching the Draft, with cause `file` and no saved state (task 72_4; ADR 0018).
_Avoid_: import, upload, restore

### Operation and History

**Operation**:
The mode in which an Operator watches and controls the running Deployment with the live Tag values; it follows the Journal from the present.
_Avoid_: runtime mode, live mode, production

**History**:
The mode in which one reads a Project's operational past from Checkpoints and Journal entries.
_Avoid_: replay, playback (a PlaybackFile is the file)

**PlaybackFile**:
A portable file holding the operational record for a period — the Deployments in force, the Journal entries and the Checkpoints needed to replay it.
_Avoid_: backup, archive, dump

**Journal**:
The single append-only, totally ordered record of every change of a Project's operational state — a Tag's value and Quality, or the Deployment Operation runs — each entry carrying the full new state of what changed and never the configuration itself.
_Avoid_: event log, audit log, history table, write log

**Journal position**:
The number of a Journal entry — its place in the totally ordered record. The operational state at a position is a starting state plus the entries up to it; a Tag's state carries the position of its latest change.
_Avoid_: `seq` (outside the wire format), offset, timestamp

**Checkpoint**:
An immutable record of a Project's full operational state at one Journal position: the Deployment it ran, by number and with its saved state (`commit_number`), and every Tag state with the position of its latest change. It is a record beside the Journal, not an entry of it, and not an aggregate — it has no behaviour, events or Version. It is written when a Deployment appears and periodically; History begins from the nearest Checkpoint before a moment and applies the Journal entries after it.
_Avoid_: snapshot (unqualified), backup, savepoint, cursor, offset

**Starting state**:
The Deployment and the Tag states a Player begins from at one Journal position.
_Avoid_: snapshot (a Checkpoint is a full snapshot), initial state

**Player**:
The client-side mechanism that renders the operational picture from a starting state plus Journal entries, live (Operation) or replayed (History).
_Avoid_: viewer, replayer

### Cross-cutting

**Version**:
The optimistic-concurrency counter carried by every aggregate edited by people — not by Tag state, whose latest change is named by a Journal position instead. A Draft's content is versioned as a whole: its entities carry no Version of their own.
_Avoid_: etag, stale client, base version

**SchemaVersion**:
The `MAJOR.MINOR` version of the JSON schema that a message or document conforms to.
_Avoid_: version (unqualified), protocol version, format version

**User settings**:
The per-user preferences of the server — for now the order of the libraries and of the projects, later more — kept in their own aggregate, apart from the content they view (task 81, task 83; ADR 0024). They are a view of the content, never part of it: no ProjectFile, Commit or Deployment carries them, and a change of them records no event. Until Users and authentication exist (task 49, task 50), the one person every request acts for owns them.
_Avoid_: preferences, profile, config, options
