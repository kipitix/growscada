# GrowSCADA

A web-based SCADA system: it exposes process variables (Tags) and lets operators view and interact with them through custom-built visual screens (Scenes) made of reusable widgets. This glossary names the concepts of that domain; the decisions behind them are in `docs/adr/`.

## Language

### Data

**Tag**:
The atomic process variable — an identifier, name, type, current value, and quality — belonging to exactly one Project and supplied by at most one Device, or by people.
_Avoid_: variable, point, signal

**Tag definition**:
A Tag's name, TagType and supplier: configuration, part of the Draft and of a Revision.
_Avoid_: tag config, tag metadata

**Supplier**:
What writes a Tag's value and Quality: one Device, or people — at most one per Tag, part of its definition.
_Avoid_: writer, producer, owner, source

**Tag state**:
A Tag's current value and Quality: process state, held by the Runtime node, never part of the Draft or a Revision. The latest write wins.
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
The secret by which a Device Gateway acts for one Device: it writes that Device's Tags and nothing else. Issued on the Runtime node, never part of a Revision; outlives Deploys and Rollbacks.
_Avoid_: API key, password, credential (unqualified)

**Draft Tag state**:
A Tag's value and Quality as set in the Draft — state, not configuration, living apart from the Tag definition. Never carried into a Revision, which gives the Tag its ordinary initial state instead.
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
Owned by one Organization. The unit of operational ownership and isolation: Tags, Scenes with their Widgets, Devices, Environments, the Draft, Revisions and the Journal belong to exactly one Project.
_Avoid_: workspace, site, solution

**Library**:
A reusable collection of WidgetTypes owned by an Organization, outside any Project, and used by any number of Projects.
_Avoid_: catalog, toolbox, package

**Library Release**:
An immutable, numbered snapshot of a Library's Draft; never removed.
_Avoid_: library version, snapshot

**Library Pin**:
The choice, in a Project's Draft, of one Library Release of one Library. Switching or removing it is refused while Widgets would be left without their WidgetType.
_Avoid_: dependency, import, reference, subscription

### Roles and clients

**Engineer**:
The role that builds the project: edits the Library, Scenes, Tag definitions, Devices and Environments in the Draft, and deploys it. A role, not an account.
_Avoid_: designer, developer, admin

**Operator**:
The role that works on site in Operation mode: watches and controls the process. A role, not an account.
_Avoid_: user

**Client**:
A browser tab following the server's events over one SSE connection: the holder of an EditLock until Users exist (task 49). It is identified by a key the tab keeps for itself.
_Avoid_: user, session, account

### Nodes

**Engineering node**:
A GrowSCADA server acting in the role that holds the Draft, the EditLock and the DraftChanges, and deploys to a Runtime node.
_Avoid_: dev server, designer server, studio

**Runtime node**:
A GrowSCADA server acting in the role that runs Revisions and keeps the Journal and Checkpoints, serving Operation and History. Runtime-origin entities are born here.
_Avoid_: production server, station, runtime (unqualified)

### Project lifecycle

**EditLock**:
The marker that one holder — today a Client, later a User — currently holds one Draft for editing. Without it the Draft is read-only.
_Avoid_: lock (unqualified); acquire/release

**Check out**:
Taking the EditLock of a Draft for editing, declaring the holder's name.
_Avoid_: acquire, lock, grab

**Check in**:
Giving the EditLock of a Draft back, leaving the Draft and its DraftChanges as they are.
_Avoid_: release, unlock

**Draft**:
The working state of a Project or of a Library as it is being edited, and the unit of consistency: every change to it is one DraftChange. A Project's Draft holds Scenes with their Widgets, Tag definitions, Devices, Environments and the pinned Library Releases; a Library's holds its WidgetTypes.
_Avoid_: working copy, unsaved changes

**DraftChange**:
One Engineer intent applied to the Draft — possibly touching several parts of the Draft at once — and the unit of undo/redo. The Draft keeps the stack of them; Deploy, Release and Discard clear it.
_Avoid_: edit, step, command, operation

**Discard**:
Resetting a Draft to its last published state, dropping all DraftChanges.
_Avoid_: revert, reset, cancel

**Deploy**:
The act of delivering the whole Draft, as a ProjectFile, to a Runtime node, which atomically makes it a new Revision and switches Operation to it.
_Avoid_: publish, apply, save

**Release**:
The act of turning a Library's Draft into a new Library Release. The Library counterpart of Deploy.
_Avoid_: publish, tag, version

**Revision**:
An immutable, numbered snapshot of the project configuration produced by Deploy or by Rollback. Operation always runs exactly one Revision.
_Avoid_: release, build, version (the concurrency counter)

**Origin**:
Where a Tag or Widget came from: `Project` (defined in the Draft and deployed) or `Runtime` (created while the system runs).
_Avoid_: source, dynamic/static

**Adopt**:
Taking a Runtime-origin Tag or Widget into the Draft, keeping its identity.
_Avoid_: import, promote, claim

**Rollback**:
Making an earlier Revision operational again, performed on the Runtime node alone; it produces a new Revision with the earlier content that records the one it copied, never rewrites history.
_Avoid_: revert, undo

**ProjectFile**:
A portable file holding one project configuration (the content of a Revision) and no operational history.
_Avoid_: export, package, backup

**Open a ProjectFile**:
Replacing the whole content of a Project's Draft with a ProjectFile — one DraftChange that undo reverses — after checking that the file names the Project and that its pinned Library Releases exist locally. Requires the EditLock.
_Avoid_: import, upload, load, restore

**Deploy a file**:
Making a valid ProjectFile the new Revision of a Project on the Runtime node, without touching the Draft and without the EditLock; the Revision's cause is `file`.
_Avoid_: import, upload, restore

### Operation and History

**PlaybackFile**:
A portable file holding the operational record for a period — the Revisions in force, the Journal entries and the Checkpoints needed to replay it.
_Avoid_: backup, archive, dump

**Journal**:
The single append-only, totally ordered record of every change of a Project's operational state — a Tag's value and Quality, or the Revision Operation runs — each entry carrying the full new state of what changed and never the configuration itself.
_Avoid_: event log, audit log, history table, write log

**Journal position**:
The number of a Journal entry — its place in the totally ordered record. The operational state at a position is a starting state plus the entries up to it; a Tag's state carries the position of its latest change.
_Avoid_: `seq` (outside the wire format), offset, timestamp

**Checkpoint**:
A full snapshot of the operational state at one Journal position.
_Avoid_: snapshot (unqualified), backup

**Player**:
The client-side mechanism that renders the operational picture from a starting state plus Journal entries.
_Avoid_: viewer, replayer

### Cross-cutting

**Version**:
The optimistic-concurrency counter carried by every aggregate edited by people — not by Tag state, whose latest change is named by a Journal position instead. A Draft's content is versioned as a whole: its entities carry no Version of their own.
_Avoid_: etag, stale client, base version

**SchemaVersion**:
The `MAJOR.MINOR` version of the JSON schema that a message or document conforms to.
_Avoid_: version (unqualified), protocol version, format version

**Client**:
Identity marker for a connected SSE client, used by connection-lifecycle domain events.
_Avoid_: session, connection, user
