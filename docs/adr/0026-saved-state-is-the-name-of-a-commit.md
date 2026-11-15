# A saved state is the name of a Commit

ADR 0021 keeps the model: a Save fixes the whole Draft as an immutable, numbered
snapshot, a Deploy makes a Project's one running and a Project imports a
Library's one. What this ADR settles is the word. The amendment of 2026-11-13
(task 84) gave a person the name "saved state" and kept `Commit`, `/commits`
and `commit_number` in the code, the tables and the API: one concept, two
vocabularies, with `GLOSSARY.md` recording both. That split did not survive the
next year of use. A reader of the code translates `Commit` into "saved state" on
every screen they open, a bug report names a route the UI never shows, and the
two names drift (the CHANGELOG of the rename has to say, at every step, which
half of the system still says which). Task 88 therefore cancels the second half
of that amendment: the concept has one name everywhere.

The decision is dated 2026-11-15 and is taken while `api/v0` is still MAJOR 0,
so the breaks below are allowed and the CHANGELOG lists them as breaks.

## Decision

- **Saved state is the only name.** Prose says saved state; the code says
  `draft.SavedState`, `SavedStateService`, `SavedStateFilter`; the database says
  `engineering.saved_states`, `saved_state_labels`,
  `saved_state_library_imports`, `saved_state_number`; the API says
  `/saved-states`, `saved_state_number`, `SavedStateResponse`, events
  `saved_state_created`/`saved_state_title_updated` and the error
  `saved_state_not_found`; growctl says `saved-states` and `--saved-state`. The
  UI's internal names (`ui/project/saved_states.go`, `data-saved-state-*`)
  follow, since they are the code's names and not a person's.
- **ADR 0021 is amended, not rewritten.** Its model, its numbers, its Version
  rule and the order Draft → Save → Deploy stay exactly as they are. The
  reverse of the 2026-11-13 amendment is recorded there with a pointer here:
  a Commit was never a second thing, only a second word.
- **The term reaches the code, the tables and the wire in that order.** Part
  88_1 renames the domain, the application services, the repositories and the
  tables with their columns and constraints; part 88_2 renames the contracts,
  the REST routes, the events, `apiclient` and the stored JSONB documents, and
  turns the old routes into `410 Gone`; part 88_3 renames what a person types
  and reads in growctl and the UI's internal names; part 88_4 writes it down
  (`GLOSSARY.md`, `AGENTS.md`, `docs/architecture/`, the CHANGELOG and this
  ADR). Each part leaves `make build` and `make test` green on its own, because
  a rename that spans a layer boundary breaks the build until both sides move.
- **The contracts rise by a MINOR.** `contract/api/v0` 0.42 → 0.43,
  `contract/record/v0` 0.6 → 0.7, `contract/project/v0` 0.3 → 0.4;
  `contract/manifest/v0` stays 0.5, because `spec.libraryImports` keeps its key
  and its number. `make schemas` regenerates the schemas and freezes the old
  ones. While MAJOR is 0 every change is a MINOR, even one an old reader cannot
  ignore.
- **An old client is told where to go, not left with a 404.** Every method of
  `/commits…`, its `load`, `deploy`, `projectfile` and `labels`, answers
  `410 Gone` with `application/problem+json` naming the saved-state route; the
  stubs are removed together with `/api/v1`.
- **An old file is refused, not half-read.** A ProjectFile whose
  `library_imports[]` carries `commit` instead of `saved_state` is `400` with
  `file_issues`: the strict decoder names the unknown field, and the validator
  never opens a document it would have to guess the meaning of. A deliberate
  break while MAJOR is 0, named in the CHANGELOG.
- **The database is renamed in two migrations per chain.** The first renames
  the tables, the columns, the constraints and the indexes (`engineering` and
  `runtime`), the second rewrites the JSONB documents —
  `saved_states.content`, `deployments.config`, the Journal and the
  Checkpoints — to the new key and raises their `schema_version`. Both have a
  `Down` that returns the old names
  over the same rows; the old migrations themselves are history and are not
  edited.
- **Rows written before the rename are read, not rewritten twice.** The
  DraftChange codec reads an import's number under the old member `commit`, the
  outbox codec reads the retired event types `commit_created` and
  `commit_title_updated`, and the Journal/Checkpoint codec reads
  `commit_number`: a stack, a queue or a record may still hold a row of the
  previous release, and an undo of an old DraftChange must restore the import it
  captured rather than fail on a missing number. A new row never carries an old
  name.
- **What keeps the word.** "Commit" as an act of the database transaction or of
  git (`after commit`, `racing commits`, `git commit`) is not the domain and
  keeps its name. So does the history of the format: `EntryTypeRevision` in
  `contract/record/v0`, the reading of old Journal entries, the retired event
  type numbers and the names of the old migrations. ADRs and old CHANGELOG
  entries are history too and are not rewritten.
- **No behaviour changes.** The rename moves no aggregate boundary, writes no
  extra transaction, changes no policy and no retention rule. Every test that
  passes before a part passes after it, with the names it asserts updated.

## Considered Options

- **Keep the two vocabularies of task 84.** Rejected: it was a transition, not
  a resting point. The cost is paid once per layer, and leaving it half-done
  means every future reader pays the translation and every future rename pays
  again.
- **Keep `commits` as a growctl alias and the old routes as aliases.** Rejected:
  growctl is ours and its callers are scripts in this repository; two names for
  one command make the help text and the error messages ambiguous, and an alias
  that lives until API 1.0 is a promise nobody tests. A `410` with the new path
  is the honest answer.
- **Rename the wire fields but keep the routes.** Rejected: the route is what a
  person reads in a log and what the UI names in its files; a half-renamed URL
  keeps the translation where it is most visible.
- **Accept an old ProjectFile and translate `commit` on read.** Rejected: the
  file's schema version must mean one shape. Guessing which of two keys a
  document meant is how a silent zero number gets imported; the validator's
  `file_issues` says which field is wrong and the client fixes it.
- **Rewrite the old migrations to the new names.** Rejected: a migration is the
  record of what the schema was; renaming one makes a database that already ran
  it disagree with the chain. New migrations rename; old ones stay.
- **One commit for the whole chain.** Rejected: a rename of ~1200 identifiers
  across the domain, the API, the database, the tools and the docs cannot be
  reviewed or bisected as one change; each part is green on its own.

## Consequences

- A saved state is what a person, a developer, a DBA and an API client call the
  same thing. `GLOSSARY.md` keeps one entry for the concept and one for the
  retired name, marked `_Avoid_`.
- growctl's `commits` command and `--commit` flag are gone without an alias:
  scripts in this repository are updated, and anyone else's must move to
  `saved-states` and `--saved-state`.
- An older client that still calls `/commits…` gets a `410` that names its
  replacement instead of a `404` that looks like a deleted Project. The stubs
  are deleted with API 1.0.
- A ProjectFile of the previous shape is refused with `file_issues`; the
  documents stored in the database are migrated, so a Deployment, a saved state
  or a Checkpoint of an older release still reads.
- The JSONB migrations and their `Down` are checked by a migration test over
  real rows, because a wrong document rewrite would lose the link between a
  Project's imports and the saved states they name.
- The word survives only where it means a transaction, a git act, a retired
  event type or an old migration's name; `grep -rniE "\bcommits?\b"` over the
  code, `docs/architecture/`, `GLOSSARY.md` and `AGENTS.md` finds those and
  nothing else.
