# Every operational entity is keyed by Project, every Project by Organization

A server hosts many Projects in both node roles, and each Project belongs to one Organization (the tenant). Tags, Scenes with their Widgets, Devices, the Draft, Revisions, the Journal and Checkpoints all carry `project_id`, natural keys are scoped to the Project (a Tag is identified by Project + name, see ADR 0003), and Projects never reference each other's entities. We introduce these keys before the Engineering/Runtime storage split, Revisions and the Journal exist, because adding them afterwards would be a second migration of every table, every journaled payload and every Checkpoint. Until Users exist (task 49) the server runs with one seeded Organization; the key is there, the multi-tenant UX comes later.

A Project's identity (its UUID) is global: it is kept through Deploy and ProjectFile, so a Runtime node recognises a new Revision of a Project it already runs. Its name and its owning Organization are local to each server and never part of a ProjectFile or Revision: an integrator's Engineering node and a plant's Runtime node hold the same Project under different Organizations, and the same company on two servers is two Organizations. The name is unique among the live Projects of an Organization; the REST API addresses a Project by UUID, tools (growctl, the device simulator) by name.

Tags and Scenes carry their `ProjectID`, immutable after creation: the source of truth of what a Project contains. Repositories are scoped to a Project (`ForProject(p)` → `Tags()`, `Scenes()`), so a query cannot forget its Project; saving an aggregate through another Project's scope is a programming error. A PortBinding binds only a Tag of the Scene's own Project, checked by the Scene. The outbox row carries `project_id`, and a policy works through the repositories of its event's Project.

Deleting a Project respects one aggregate per transaction (ADR 0008): the Project is marked deleted and records `project_deleted`; from then on it does not exist for the API — not listed, every resource under it `404`, its name free. A policy then deletes its Scenes, then its Tags, each in its own transaction with its own events, and the Project's row last; foreign keys without `CASCADE` refuse removing a row that still has content, and the job is retried. The policy also keys `tag_created` and `scene_created` of a deleted Project, so a creation that read the Project as live just before the mark is cleaned up too. Until the cleanup ends, Tags and Scenes belong to a Project that no longer exists: a transient break of "belongs to exactly one Project", invisible outside because every access to a Project's content first checks the Project.

## Considered Options

- One Runtime node = one Project, isolation by deployment: rejected. A plant with several lines, or an integrator hosting several customers, would need a server per Project.
- Many Projects in the Engineering node only, one Project per Runtime node: rejected for the same reason on the runtime side.
- One shared pool of Tags per server with Projects as different views over it: rejected. Two Projects built from one template would collide on Tag names.
- `project_id` and `organization_id` both global, carried in the ProjectFile: rejected. The integrator and the plant are different Organizations on different servers; ownership is a relation of the server, not of the configuration.
- A per-server `project_id`, the Engineering↔Runtime link kept as a Deploy target setting: rejected. A ProjectFile loaded by hand could not be told apart from a different Project.
- The Project known only to the repository scope, not to the aggregates: rejected. Events, and later Journal entries and Checkpoints, have to say which Project they describe on their own.
- `projectID` in `context.Context`: rejected. A forgotten Project would surface only at run time, and every policy would have to put it there itself.
- Deleting a Project with `ON DELETE CASCADE` in one transaction, one `project_deleted` event: rejected. It changes many aggregates in one transaction and gives their subscribers no events.
- Refusing to delete a non-empty Project: rejected. Emptying a Project by hand is tedious and races with creation.

## Consequences

- WidgetTypes get no key until Libraries exist (task 33): a WidgetType is shared by every Project of the server, cannot be removed while a Widget of any Project uses it, and its change reconciles Scenes of all Projects. A known, temporary exception to "Projects never reference each other's entities".
- Once Revisions and the Journal exist, deleting a Project would delete its operational record too; whether that record must be kept is decided with the Journal (task 42).
