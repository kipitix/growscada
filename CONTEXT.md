# GrowSCADA

A web-based SCADA (Supervisory Control and Data Acquisition) system: it exposes process variables (Tags) and lets operators view and interact with them through custom-built visual screens (Scenes) made of reusable widgets.

## Language

### Data

**Tag**:
The atomic process variable — an identifier, name, type, current value, and quality. Aggregate. Its name and TagType form the Tag's definition (configuration, part of the Draft); its value and Quality are process state, never part of the Draft or a Revision.
_Avoid_: variable, point, signal

**Quality**:
A Tag's reliability indicator: `Bad`, `Uncertain`, or `Good`. Describes how trustworthy the value is, never where it came from. There is no "unknown" Quality — every Tag always has one of the three, stated explicitly by whoever creates or updates it.
_Avoid_: status, state; simulated/forced/unknown as a Quality

**TagType**:
A Tag's data type: `String`, `Boolean`, or `Integer`. Governs which values the Tag accepts. There is no "unknown" TagType.
_Avoid_: analog/discrete, data type

**Device**:
A source of Tag values — real equipment, a protocol adapter, or a simulator. Whether it is a simulator is descriptive metadata of the Device, not a Quality. Not yet modelled.
_Avoid_: source, adapter, PLC

### Visualization

**WidgetType**:
A reusable template defining how a widget renders: an HTML template plus a script, a default size, and the InputPorts it exposes. Aggregate.
_Avoid_: component, widget definition

**InputPort**:
A named, typed input slot declared on a WidgetType, referenced in its template/script as `input.<name>`. Its type hint either names one TagType or accepts any TagType — "any" is a property of the hint, not a TagType.
_Avoid_: parameter, slot

**Widget**:
A placed instance of a WidgetType on a Scene — positioned, sized, rotated, optionally labelled, and bound to Tags via PortBindings. Entity of the Scene aggregate: it has no identity or version outside the Scene that contains it.
_Avoid_: element, control

**PortBinding**:
The link between one of a Widget's InputPorts and the Tag that feeds it.
_Avoid_: mapping, connection

**Scene**:
A named canvas (a page inside a project) with fixed dimensions, a static HTML background, and the Widgets placed on it. Aggregate — the sole consistency boundary for itself and its Widgets.
_Avoid_: screen, mnemonic, page, display

### Roles

**Engineer**:
The role that builds the project: edits the Library, Scenes and Tag definitions in the Draft, and deploys it. A role (a function), not an account.
_Avoid_: designer, developer, admin

**Operator**:
The role that works on site in Operation mode: watches and controls the process and may create Runtime-origin Tags and Widgets, but never changes WidgetTypes. A role, not an account.
_Avoid_: user

### Nodes

**Engineering node**:
A GrowSCADA server acting in the role that holds the Draft, the EditLock and the DraftChanges, and deploys to a Runtime node.
_Avoid_: dev server, designer server, studio

**Runtime node**:
A GrowSCADA server acting in the role that runs Revisions and keeps the Journal and Checkpoints, serving Operation and History. Runtime-origin entities are born here. One server may play both roles.
_Avoid_: production server, station, runtime (unqualified)

### Project lifecycle

**EditLock**:
The marker that one Engineer currently holds the project for editing; acquired and released explicitly, expires when its holder disconnects, and may be taken over by another Engineer. Without it the Draft is read-only.
_Avoid_: checkout, lock (unqualified)

**Draft**:
The project's configuration as it is being edited — Scenes with their Widgets, WidgetTypes (the Library) and Tag definitions — not yet visible in Operation. Changes to it become operational only through Deploy.
_Avoid_: working copy, unsaved changes

**DraftChange**:
One Engineer intent applied to the Draft — possibly touching several aggregates at once — and the unit of undo/redo. The Draft keeps its DraftChanges since the last Deploy or Discard; nothing further back can be undone.
_Avoid_: edit, step, command, operation

**Discard**:
Resetting the Draft to the Revision currently running in Operation, dropping all DraftChanges.
_Avoid_: revert, reset, cancel

**Deploy**:
The act of delivering the whole Draft, as a ProjectFile, to a Runtime node, which atomically makes it a new Revision and switches Operation to it. Loading a ProjectFile into a Runtime node by hand is also a Deploy.
_Avoid_: publish, apply, save

**Revision**:
An immutable, numbered snapshot of the project configuration produced by Deploy. Operation always runs exactly one Revision.
_Avoid_: release, build, version (that is the concurrency counter)

**Origin**:
Where a Tag or Widget came from: `Project` (defined in the Draft and deployed) or `Runtime` (created while the system runs — by a Device, an integration, or a person in Operation). WidgetTypes have no Origin: they are always project-defined.
_Avoid_: source, dynamic/static

**Adopt**:
Taking a Runtime-origin Tag or Widget into the Draft, keeping its identity; after the next Deploy its Origin is `Project`. Runtime-origin entities are never touched by Deploy otherwise, except being removed when their Scene or WidgetType no longer exists.
_Avoid_: import, promote, claim

**Rollback**:
Making an earlier Revision operational again, performed on the Runtime node alone; it produces a new Revision with the earlier content, never rewrites history.
_Avoid_: revert, undo (that is for DraftChanges)

**ProjectFile**:
A portable file holding one project configuration (the content of a Revision) and no operational history. The way a project is moved between servers outside Deploy.
_Avoid_: export, package, backup

### Operation and History

**PlaybackFile**:
A portable file holding the operational record for a period — the Revisions in force, the Journal entries and the Checkpoints needed to replay it — so that History can be played back elsewhere.
_Avoid_: backup, archive, dump

**Journal**:
The single append-only, totally ordered record of everything that happened in operation — Deploys, Runtime-origin entity changes, Tag value and Quality changes. Each entry carries the full new state of what it describes. Draft editing is not journaled.
_Avoid_: event log, audit log, history table

**Checkpoint**:
A full snapshot of the operational state at one Journal position — the running Revision, all Runtime-origin entities, and every Tag's value and Quality. Taken on every Deploy and periodically, so any past moment is a Checkpoint plus a bounded run of Journal entries.
_Avoid_: snapshot (unqualified), backup

**Player**:
The client-side mechanism that renders the operational picture from a starting state plus Journal entries. Operation is the Player pinned to "now"; History is the same Player positioned at any past moment, without control.
_Avoid_: viewer, replayer

### Cross-cutting

**Version**:
The optimistic-concurrency counter carried by every aggregate (Tag, WidgetType, Scene), incremented on each committed change. Not to be confused with Revision.
_Avoid_: etag

**Client**:
Identity marker for a connected SSE (Server-Sent Events) client, used by connection-lifecycle domain events. Not yet a full aggregate — no access rights or behavior defined.
_Avoid_: session, connection, user
