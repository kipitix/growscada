# Libraries are shared by pinned release and copied into the Revision on Deploy

A Library (WidgetTypes, later ComputationTypes and Themes) belongs to an Organization, lives outside any Project and may be public to the whole server. It is edited in its own Draft, with the same EditLock and DraftChange mechanics as a Project's Draft, and becomes usable only through Release, which produces an immutable, numbered Library Release. A Project's Draft pins at most one Library Release per Library, for any number of Libraries. On Deploy, the content the Project actually uses is copied from the pinned releases into the Revision and the ProjectFile, so a Runtime node and History never depend on a Library. We chose release-level versioning over versioning each WidgetType, so that one Project never mixes two versions of one type and a release can be reviewed as a whole. We chose to copy into the Revision rather than reference it, so that the ProjectFile stays portable and a Rollback or playback cannot be broken by later Library changes.

## Considered Options

- Library inside each Project (copy on import): rejected. No reuse; fixing a WidgetType means editing every Project by hand.
- Per-WidgetType versions referenced by Widgets: rejected for mixing versions within one Project and multiplying versioning of Themes and ComputationTypes.
- Revision references the Library Release instead of copying it: rejected. The Runtime node would need the Library, and the ProjectFile would stop being self-contained.
