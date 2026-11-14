# Labels are an aggregate apart at the Organization's level

Engineers need to mark saved states — the Commits a Save fixes — and then find
them again when they deploy a Project or import a Library's saved state. The
mark must be shared by everyone working on the server, must be usable on any
saved state of any Project or Library, and must not become part of the content:
a label on a Commit must not raise a Draft Version, enter undo/redo, change a
ProjectFile, a Commit or a Deployment, or make two Engineers' edits conflict.

The obvious places are all wrong for one reason or another. A column on a
Commit would make labeling an edit of immutable content. A per-user setting
would hide the mark from the next Engineer, while ADR 0024 is deliberately
per-person. A label inside the Draft would be undone by undo, cleared by a
Discard and written into every ProjectFile. An assignment table owned by no
aggregate would let a label be deleted while its assignments still point at it.

Task 85. The term is `GLOSSARY.md` (Label, Label assignment). The same task
removes Release; labels take over the "find the right saved state" role that
Release numbers had.

## Decision

- **Label is an aggregate of the Engineering node, scoped by Organization.**
  `engineering.labels` carries `id`, `organization_id` referencing
  `organizations`, `name`, an optional `color` and `created_at`. Logically the
  label belongs to the Organization; physically it sits in `engineering`
  because only the Engineering node reads and writes it and its assignments
  name `engineering.commits`. The API is server-wide while the server has one
  Organization: `organization_id` is written from the first day, so a second
  tenant changes only the route and the lookup.
- **A label is a name and an optional color.** The name is trimmed, 1–100
  runes, has no line breaks, and is unique in the Organization without regard
  to case (`Prod` and `prod` are one name). The color is `#RRGGBB` or empty;
  empty means the theme's neutral color. A chip always shows the name, so
  color is never the only difference between labels.
- **Assignments are entities of the Label.** A Label owns the set of its
  assignments to saved states: assign, unassign, rename, recolor and delete
  each change one Label in one transaction (ADR 0008). A saved state may carry
  any number of labels; one label may mark any number of saved states. The
  repository reads and writes the assignment rows, and the aggregate exposes
  the set; a rename does not need to load it, but the model does not pretend
  the assignment is a second aggregate.
- **Deleting a label removes its assignments.** The cascade is inside the
  aggregate. The UI warns with the number of saved states that lose the mark.
- **Deleting a Project or a Library removes the assignments of its saved
  states through the database.** This is a deliberate exception to ADR 0008:
  the alternative — a policy that loads every Label touched by the scope and
  unassigns in its own transaction — would make deleting a Project depend on
  an unbounded number of labels and would still race the scope's deletion.
  The `project_deleted`/`library_deleted` events make clients reread the
  history; no per-label event is sent.
- **Labels are not content.** No DraftChange, Version, undo/redo, ProjectFile,
  Commit or Deployment carries a label. A label change is therefore never a
  conflict with an Engineer editing the Draft, and no Save is needed for it.
- **The events are server events.** `label_created`, `label_updated` (name
  and/or color), `label_deleted`, `label_assigned` and `label_unassigned` go
  to the server's stream. An assignment payload names the saved state's scope
  and number, so History can reload the right list; it does not route to the
  Project's stream alone, because the global Labels window and the History of
  another mode must see it too. A client following `?project=` receives the
  server's events as well.
- **The API is small and separate.** `GET/POST /api/v0/labels`,
  `PATCH/DELETE /api/v0/labels/{id}`, and assignment as
  `PUT/DELETE /api/v0/projects/{id}/commits/{n}/labels/{label_id}` (and the
  same under a Library). `GET …/commits?label=<id>` filters a saved-state
  list. Assignment states no Draft Version and is idempotent.
- **Assignment lives in History; management lives in one global window.** A
  saved state's row shows its colored chips and a "+ label" control; the
  Labels window creates, renames, recolors and deletes names, and is opened
  from the header in every mode. The Import and Deploy windows filter their
  saved-state lists by one label at a time.
- **Two starter labels are ordinary labels.** The migration inserts
  `ready_for_import` and `ready_for_deploy` with no color for the sole
  Organization. They can be renamed, recolored and deleted; no operation
  depends on them. They are seed data, not built-in behavior.
- **`growctl` gets the minimum.** `labels` CRUD and `label assign/unassign`
  with a commit number; `commits` shows the labels. No label manifest kind:
  labels are metadata, not declarative content.

## Considered Options

- **Assignment as its own aggregate.** Rejected: deleting a label would either
  touch many aggregates in one transaction or need a policy and a window of
  dangling assignments. The label's set is bounded in practice by the number
  of saved states an Engineer marks, and the operations stay one transaction.
- **A label as part of a Commit.** Rejected: a Commit is immutable, and a Save
  would have to know every label change; a label must be assignable after a
  Save without rewriting history.
- **A label inside the Draft (an entity like a Tag).** Rejected: undo, redo,
  Discard and Revert would change labels, and every ProjectFile, Commit and
  Deployment would carry them as content.
- **A per-user setting (ADR 0024).** Rejected: the mark is for the whole team;
  the next Engineer must see why a saved state is ready.
- **A policy for deleting a scope's assignments.** Rejected: an unbounded
  number of Label aggregates, eventual consistency and a race with the scope
  deletion. The database cascade plus the scope's deletion event is simpler and
  is documented here as the one exception.
- **A required color, or a fixed palette.** Rejected: the color is a
  convenience; an empty color is a valid neutral label, and a fixed palette
  would make two labels choose the same color anyway. A hex value keeps the
  field small and the rendering predictable.
- **Labels as gates for Import and Deploy.** Rejected: a saved state may be
  ready for one and not the other, and a gate would turn metadata into a
  policy. The starter labels only name the habit; the operations do not check
  them.

## Consequences

- A migration in `engineering` (`labels`, `commit_labels`), a new
  `domain/label` package, a repository and a service, `api/v0` MINOR, the
  Bruno collection, and a global Labels window.
- A Project or Library deletion prunes assignments without a per-label event;
  clients refresh their history on the scope's deletion event.
- A label's assignment to a Commit is a weak reference: the Commit is
  immutable, and only deleting its scope removes the assignment. The label
  itself outlives both; it stays in the list with a smaller `assigned_count`.
- The name's case-insensitive uniqueness is an expression index
  (`lower(name)`); a rename that changes only case is allowed.
- The seed labels are created once. Deleting them is a normal delete and a
  later migration does not recreate them.
- The API stays server-wide until Organizations are selectable; then
  `organization_id` already holds the scope and only the route and the lookup
  change.
