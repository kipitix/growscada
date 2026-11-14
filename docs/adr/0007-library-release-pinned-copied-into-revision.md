# Libraries are shared by pinned release and copied into the Revision on Deploy

A Library (WidgetTypes, later ComputationTypes and Themes) belongs to an Organization, lives outside any Project and may be public to the whole server. It is edited in its own Draft, with the same EditLock and DraftChange mechanics as a Project's Draft, and becomes usable only through Release, which produces an immutable, numbered Library Release. A Project's Draft pins at most one Library Release per Library, for any number of Libraries. On Deploy, the content the Project actually uses is copied from the pinned releases into the Revision and the ProjectFile, so a Runtime node and History never depend on a Library. We chose release-level versioning over versioning each WidgetType, so that one Project never mixes two versions of one type and a release can be reviewed as a whole. We chose to copy into the Revision rather than reference it, so that the ProjectFile stays portable and a Rollback or playback cannot be broken by later Library changes.

## Amendment 2026-11-04: the ProjectFile also names the pins

The ProjectFile carries the list of Library Pins the Draft used — the Library and the release number — beside the WidgetType copies. The pins are provenance, not configuration a Runtime node reads: they let a Discard restore the Project's pins to the Revision it runs, and they say from which release each copy came. A ProjectFile with no pins is legitimate: a Project may use no Library.

## Considered Options

- Library inside each Project (copy on import): rejected. No reuse; fixing a WidgetType means editing every Project by hand.
- Per-WidgetType versions referenced by Widgets: rejected for mixing versions within one Project and multiplying versioning of Themes and ComputationTypes.
- Revision references the Library Release instead of copying it: rejected. The Runtime node would need the Library, and the ProjectFile would stop being self-contained.

## Amendment 2026-11-08: a running Deployment keeps the Library it names

A Library Release is never removed alone: it goes with its Library. A Commit of a Project and a Project Release copy the WidgetType content they need, so they outlive the Library and keep only a link to it — the History and a PlaybackFile render after its deletion. A **running Deployment** is different: a Discard restores the Project's Draft from its ProjectFile, pins included, so a Deployment whose document names a Release of a deleted Library could not restore it. A Library is therefore removed only while no live Project pins it **and** no live Project's running Deployment names one of its Releases (task 71): the Engineering node asks the Runtime node, which reads the pins of each Project's running document. The frontend warns that a Commit or a Release of the past keeps a link to the deleted Library.

## Amendment 2026-11-08: a Library Release is made from a Commit

A Library Release is now made from a Commit of the Library's Draft, exactly as a Project Release is made from a Commit of a Project's Draft (task 72_5; ADR 0021): one Commit is published at most once, its `released_widget_types` copies are written in the same transaction, and the number is the next in the Library. The Release no longer reads the Draft's Version and is refused only when the Commit is unknown or already published; publishing changes no Draft and leaves the undo stack alone. A Release made before Commits has `origin = legacy`, no `commit_number` and the Author `system`; it stays pinnable, and a Project cannot tell it from a Release of a Commit except by the provenance the pin response carries. The `library_pins` table is unchanged: a pin still names a Library Release's number, and the copies in `released_widget_types` remain the projection every Project read uses.

## Amendment 2026-11-14: a Project imports a saved state, not a Release (task 85)

The mechanism stays the same — a Project names exactly one saved state of one Library, and the Deployment/ProjectFile copies the WidgetTypes it uses — but the thing it names is a **Commit**, not a Library Release. The names change with it: `library_pins` → `library_imports`, Library Pin → Library Import, `release` in the ProjectFile → `commit`; the domain package becomes `libraryimport`. A Project no longer publishes a Library Release to import it.

- **A Library is deleted only while no live Project imports it and no running Deployment names it.** The check is unchanged in meaning; it reads imports now. The copies in old ProjectFiles are self-contained, so history survives the Library, as before.
- **Legacy Release is removed with the migration.** A Library Release made before Commits cannot be named by a Commit; task 85 deletes it, its imports and the Widgets of its types. There is no synthetic Commit and no compatible pin: the content is lost by decision, and the CHANGELOG says so.
- **The migration rewrites stored documents.** It maps commit-backed Releases to Commits, removes legacy imports from Commit and Deployment documents (including the running one) and clears the DraftChange stacks, whose pin snapshots name the old numbers. This is the one-time exception recorded in ADR 0021.
- **The API and growctl speak Import.** `…/library-imports`, `ImportResponse` and `spec.libraryImports` replace the pin names. A ProjectFile with the old `library_pins`/`release` fields is rejected with `file_issues`, not guessed.
