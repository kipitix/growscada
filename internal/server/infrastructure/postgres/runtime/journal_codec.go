package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/contract"
	recordv0 "github.com/kipitix/growscada/contract/record/v0"
)

// journalRow is one row of runtime.journal as the codec reads and writes it:
// the envelope's columns, with the payload still encoded. The seq is the
// table's, taken from runtime.journal_seq; an entry being written carries
// none.
type journalRow struct {
	Seq           int64
	ProjectID     uuid.UUID
	Timestamp     time.Time
	Type          string
	Payload       []byte
	SchemaVersion string
}

// newTagStateEntry returns the Journal entry of a change of a Tag's state.
// The Timestamp is RFC 3339, the format the record contract carries.
func newTagStateEntry(projectID uuid.UUID, at time.Time, tagID uuid.UUID, value, quality string) recordv0.Entry {
	return recordv0.Entry{
		Header:    recordv0.Header{SchemaVersion: recordv0.SchemaVersion()},
		Timestamp: at.UTC().Format(time.RFC3339),
		ProjectID: projectID,
		Type:      recordv0.EntryTypeTagState,
		TagState:  &recordv0.TagState{TagID: tagID, Value: value, Quality: quality},
	}
}

// newDeploymentEntry returns the Journal entry of a Deployment that appeared: its
// number, its cause and the Deployment a Rollback copied. The document itself
// is not part of the entry (ADR 0014).
func newDeploymentEntry(projectID uuid.UUID, at time.Time, number int, cause recordv0.DeploymentCause, rolledBackFrom *int) recordv0.Entry {
	return recordv0.Entry{
		Header:     recordv0.Header{SchemaVersion: recordv0.SchemaVersion()},
		Timestamp:  at.UTC().Format(time.RFC3339),
		ProjectID:  projectID,
		Type:       recordv0.EntryTypeDeployment,
		Deployment: &recordv0.Deployment{Number: number, Cause: cause, RolledBackFrom: rolledBackFrom},
	}
}

// encodeJournalEntry returns the row an entry is stored as: its timestamp
// parsed, its schema version written down, and the payload of its type. An
// entry of another schema version than this server writes is refused: the
// stored row is always in the current format.
func encodeJournalEntry(entry recordv0.Entry) (journalRow, error) {
	if entry.SchemaVersion != recordv0.SchemaVersion() {
		return journalRow{}, fmt.Errorf("cannot encode a journal entry of schema version %s: this server writes %s",
			entry.SchemaVersion, recordv0.SchemaVersion())
	}
	at, err := time.Parse(time.RFC3339, entry.Timestamp)
	if err != nil {
		return journalRow{}, fmt.Errorf("cannot parse the timestamp of journal entry %q: %w", entry.Timestamp, err)
	}
	payload, err := encodeJournalPayload(entry)
	if err != nil {
		return journalRow{}, err
	}
	return journalRow{
		ProjectID:     entry.ProjectID,
		Timestamp:     at,
		Type:          string(entry.Type),
		Payload:       payload,
		SchemaVersion: entry.SchemaVersion.String(),
	}, nil
}

// encodeJournalPayload returns the payload of the entry's type. An entry that
// carries no payload of its own type is refused.
func encodeJournalPayload(entry recordv0.Entry) ([]byte, error) {
	var payload any
	switch entry.Type {
	case recordv0.EntryTypeTagState:
		if entry.TagState == nil {
			return nil, fmt.Errorf("cannot encode a %s journal entry without its tag_state", entry.Type)
		}
		payload = entry.TagState
	case recordv0.EntryTypeDeployment:
		if entry.Deployment == nil {
			return nil, fmt.Errorf("cannot encode a %s journal entry without its deployment", entry.Type)
		}
		payload = entry.Deployment
	default:
		return nil, fmt.Errorf("cannot encode a journal entry of unknown type %q", entry.Type)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("cannot encode the payload of journal entry %s: %w", entry.Type, err)
	}
	return encoded, nil
}

// decodeJournalEntry rebuilds an entry from its row. A type this package does
// not know is an error, not a panic: a row of a later server must not take a
// reader down.
func decodeJournalEntry(row journalRow) (recordv0.Entry, error) {
	version, err := contract.ParseSchemaVersion(row.SchemaVersion)
	if err != nil {
		return recordv0.Entry{}, fmt.Errorf("cannot decode journal entry %d: %w", row.Seq, err)
	}
	entry := recordv0.Entry{
		Header:    recordv0.Header{SchemaVersion: version},
		Seq:       row.Seq,
		Timestamp: row.Timestamp.UTC().Format(time.RFC3339),
		ProjectID: row.ProjectID,
		Type:      recordv0.EntryType(row.Type),
	}
	// A row written before task 72_9 carries the entry type's old name: it is
	// a Deployment entry, and a reader of the Journal sees it as one.
	if entry.Type == recordv0.EntryTypeRevision {
		entry.Type = recordv0.EntryTypeDeployment
	}
	switch entry.Type {
	case recordv0.EntryTypeTagState:
		var payload recordv0.TagState
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			return recordv0.Entry{}, fmt.Errorf("cannot decode the tag_state payload of journal entry %d: %w", row.Seq, err)
		}
		entry.TagState = &payload
	case recordv0.EntryTypeDeployment:
		var payload recordv0.Deployment
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			return recordv0.Entry{}, fmt.Errorf("cannot decode the deployment payload of journal entry %d: %w", row.Seq, err)
		}
		entry.Deployment = &payload
	default:
		return recordv0.Entry{}, fmt.Errorf("cannot decode journal entry %d: unknown type %q", row.Seq, row.Type)
	}
	return entry, nil
}

// appendJournalEntry writes the entry in the caller's transaction and returns
// its Journal position. The Journal is append-only: nothing updates or
// deletes a row here, and the rows go only with their Project (ADR 0019).
func appendJournalEntry(ctx context.Context, tx *sql.Tx, entry recordv0.Entry) (int64, error) {
	row, err := encodeJournalEntry(entry)
	if err != nil {
		return 0, err
	}
	var seq int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO runtime.journal (project_id, ts, type, payload, schema_version)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING seq`,
		row.ProjectID, row.Timestamp, row.Type, row.Payload, row.SchemaVersion,
	).Scan(&seq); err != nil {
		return 0, fmt.Errorf("cannot append a %s entry to the journal of project %s: %w", row.Type, row.ProjectID, err)
	}
	return seq, nil
}
