// Package recordv0 is MAJOR 0 of the operational record format: Journal
// entries, Checkpoints and PlaybackFiles (ADR 0005, ADR 0019). A record is
// stored forever, so this contract never imports the API's types: it mirrors
// the enums it needs, as devicelink mirrors the API's (ADR 0004).
package recordv0

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/contract"
)

// SchemaVersion is the version of the operational record format this package
// describes.
// A function rather than a variable, so no importer can change it.
func SchemaVersion() contract.SchemaVersion { return contract.SchemaVersion{Major: 0, Minor: 2} }

// Header is the root of every record document: the SchemaVersion the rest of
// the document follows. A reader checks it before anything else.
type Header struct {
	SchemaVersion contract.SchemaVersion `json:"schema_version"`
}

// EntryType is what an Entry records. The names are the record's own, not the
// names of the live events: the Journal outlives them (ADR 0019).
type EntryType string

// The entry types. Configuration is never an entry of its own: it changes
// only through a Revision, and the Revision entry is the address of its whole
// new state.
const (
	EntryTypeTagState EntryType = "tag_state"
	EntryTypeRevision EntryType = "revision"
)

// EntryTypes returns the names of every entry type, so a reader checks a type
// against the whole set.
func EntryTypes() []string {
	return []string{string(EntryTypeTagState), string(EntryTypeRevision)}
}

// Entry is one Journal entry: a change of a Project's operational state. It
// embeds Header like a ProjectFile does, and names the Project the entry
// belongs to. The entity that changed is named inside the payload, not by an
// aggregate_id: the Journal is read by time and by position, never by entity.
//
// Seq is the entry's Journal position: the total order of every entry of the
// server. Timestamp is RFC 3339.
type Entry struct {
	Header
	Seq       int64     `json:"seq"`
	Timestamp string    `json:"timestamp"`
	ProjectID uuid.UUID `json:"project_id"`
	Type      EntryType `json:"type"`
	TagState  *TagState `json:"tag_state,omitempty"`
	Revision  *Revision `json:"revision,omitempty"`
}

// TagState is the payload of a tag_state entry: the Tag's new value and
// Quality. Its name, TagType and Version are configuration and are not part
// of the record.
type TagState struct {
	TagID   uuid.UUID `json:"tag_id"`
	Value   string    `json:"value"`
	Quality string    `json:"quality"`
}

// Revision is the payload of a revision entry: the Revision that appeared.
// The document itself is not part of the record — it is immutable in the
// Runtime node's store, and the number addresses it (ADR 0014).
type Revision struct {
	Number         int           `json:"number"`
	Cause          RevisionCause `json:"cause"`
	RolledBackFrom *int          `json:"rolled_back_from,omitempty"`
}

// RevisionCause is why a Revision exists, as the record names it. It mirrors
// the API's RevisionCause instead of importing it: the record is stored
// forever (ADR 0004, ADR 0019).
type RevisionCause string

// The Revision causes: a Deploy made it from the Draft, a Rollback copied an
// earlier one, or a project file was deployed as it is (ADR 0014, ADR 0018).
const (
	RevisionCauseDeploy   RevisionCause = "deploy"
	RevisionCauseRollback RevisionCause = "rollback"
	RevisionCauseFile     RevisionCause = "file"
)
