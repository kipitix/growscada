// Package recordv0 is MAJOR 0 of the operational record format: Journal
// entries, Checkpoints and PlaybackFiles (ADR 0005, ADR 0019). A record is
// stored forever, so this contract never imports the API's types: it mirrors
// the enums it needs, as devicelink mirrors the API's (ADR 0004).
package recordv0

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/contract"
)

// SchemaVersion is the version of the operational record format this package
// describes.
// A function rather than a variable, so no importer can change it.
func SchemaVersion() contract.SchemaVersion { return contract.SchemaVersion{Major: 0, Minor: 5} }

// Header is the root of every record document: the SchemaVersion the rest of
// the document follows. A reader checks it before anything else.
type Header struct {
	SchemaVersion contract.SchemaVersion `json:"schema_version"`
}

// EntryType is what an Entry records. The names are the record's own, not the
// names of the live events: the Journal outlives them (ADR 0019).
type EntryType string

// The entry types. Configuration is never an entry of its own: it changes
// only through a Deployment, and the Deployment entry is the address of its
// whole new state.
const (
	EntryTypeTagState   EntryType = "tag_state"
	EntryTypeDeployment EntryType = "deployment"
	// EntryTypeRevision is the name EntryTypeDeployment had before task 72_9.
	// Readers accept it, so a Journal row or a PlaybackFile written then still
	// reads; nothing writes it.
	EntryTypeRevision EntryType = "revision"
)

// EntryTypes returns the names of every entry type this version writes, so a
// writer picks one of them. A stored entry may still carry EntryTypeRevision;
// UnmarshalJSON and the Journal codec read it as EntryTypeDeployment before a
// reader checks the type, so the set names what a reader sees, not the bytes
// of an old row.
func EntryTypes() []string {
	return []string{string(EntryTypeTagState), string(EntryTypeDeployment)}
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
	Seq        int64       `json:"seq"`
	Timestamp  string      `json:"timestamp"`
	ProjectID  uuid.UUID   `json:"project_id"`
	Type       EntryType   `json:"type"`
	TagState   *TagState   `json:"tag_state,omitempty"`
	Deployment *Deployment `json:"deployment,omitempty"`
}

// UnmarshalJSON reads an entry as it is, accepting the names a Deployment had
// before task 72_9: an entry of type "revision" is read as a Deployment, and
// the payload of the "revision" member is read as the Deployment's. A reader
// of an old Journal row or PlaybackFile therefore sees the entry a new one
// carries, while encoding an Entry writes only the Deployment names.
func (e *Entry) UnmarshalJSON(data []byte) error {
	type entryJSON struct {
		Header
		Seq              int64       `json:"seq"`
		Timestamp        string      `json:"timestamp"`
		ProjectID        uuid.UUID   `json:"project_id"`
		Type             EntryType   `json:"type"`
		TagState         *TagState   `json:"tag_state,omitempty"`
		Deployment       *Deployment `json:"deployment,omitempty"`
		LegacyDeployment *Deployment `json:"revision,omitempty"`
	}
	var raw entryJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.Type == EntryTypeRevision {
		raw.Type = EntryTypeDeployment
	}
	if raw.Deployment == nil {
		raw.Deployment = raw.LegacyDeployment
	}
	*e = Entry{
		Header:     raw.Header,
		Seq:        raw.Seq,
		Timestamp:  raw.Timestamp,
		ProjectID:  raw.ProjectID,
		Type:       raw.Type,
		TagState:   raw.TagState,
		Deployment: raw.Deployment,
	}
	return nil
}

// TagState is the payload of a tag_state entry: the Tag's new value and
// Quality. Its name, TagType and Version are configuration and are not part
// of the record.
type TagState struct {
	TagID   uuid.UUID `json:"tag_id"`
	Value   string    `json:"value"`
	Quality string    `json:"quality"`
}

// Deployment is the payload of a deployment entry: the Deployment that appeared.
// The document itself is not part of the record — it is immutable in the
// Runtime node's store, and the number addresses it (ADR 0014). ReleaseNumber
// is the Project Release the Deployment made (task 72_4); absent for a
// Deployment from a file or one made before Releases existed.
type Deployment struct {
	Number         int             `json:"number"`
	Cause          DeploymentCause `json:"cause"`
	RolledBackFrom *int            `json:"rolled_back_from,omitempty"`
	ReleaseNumber  *int            `json:"release_number,omitempty"`
}

// Checkpoint is an immutable snapshot of a Project's operational state at a
// Journal position (task 44, ADR 0023): the Deployment in force and every Tag
// state, stored beside the Journal and never an entry of it. Seq is the
// position the snapshot describes — the state after the entry at that
// position; Timestamp is RFC 3339, when the snapshot was taken.
//
// The Deployment is addressed, not copied: the document stays in the Runtime
// node's store (ADR 0014). Deployment is nil for a Project that runs no
// Deployment, as a Project before its first Deploy does.
type Checkpoint struct {
	Header
	Seq        int64                `json:"seq"`
	ProjectID  uuid.UUID            `json:"project_id"`
	Timestamp  string               `json:"timestamp"`
	Deployment *Deployment          `json:"deployment,omitempty"`
	TagStates  []CheckpointTagState `json:"tag_states"`
}

// CheckpointTagState is one Tag's state in a Checkpoint: its value, Quality
// and the position of its latest change. WrittenAt is not part of the record:
// a past state has no write moment the snapshot keeps (ADR 0023).
type CheckpointTagState struct {
	TagID   uuid.UUID `json:"tag_id"`
	Value   string    `json:"value"`
	Quality string    `json:"quality"`
	Seq     int64     `json:"seq"`
}

// DeploymentCause is why a Deployment exists, as the record names it. It mirrors
// the API's DeploymentCause instead of importing it: the record is stored
// forever (ADR 0004, ADR 0019).
type DeploymentCause string

// The Deployment causes: a Deploy made it from a Project Release, a Rollback
// copied an earlier one, or a project file was deployed as it is (ADR 0014,
// ADR 0018, task 72_4).
const (
	DeploymentCauseDeploy   DeploymentCause = "deploy"
	DeploymentCauseRollback DeploymentCause = "rollback"
	DeploymentCauseFile     DeploymentCause = "file"
)
