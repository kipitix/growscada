package runtime

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/contract"
	recordv0 "github.com/kipitix/growscada/contract/record/v0"
)

// TestJournalCodec_RoundTripsEveryType: a row written by the codec decodes
// back to the entry it came from, its envelope and its payload both.
func TestJournalCodec_RoundTripsEveryType(t *testing.T) {
	at := time.Date(2026, 11, 6, 12, 30, 15, 0, time.UTC)
	projectID, tagID := uuid.New(), uuid.New()
	rolledBackFrom := 3
	for _, entry := range []recordv0.Entry{
		newTagStateEntry(projectID, at, tagID, "42", "good"),
		newDeploymentEntry(projectID, at, 7, recordv0.DeploymentCauseRollback, &rolledBackFrom),
		newDeploymentEntry(projectID, at, 8, recordv0.DeploymentCauseFile, nil),
	} {
		row, err := encodeJournalEntry(entry)
		if err != nil {
			t.Fatalf("encode %s: %v", entry.Type, err)
		}
		if row.Type != string(entry.Type) || row.SchemaVersion != recordv0.SchemaVersion().String() || !row.Timestamp.Equal(at) {
			t.Errorf("row of %s: %+v", entry.Type, row)
		}
		row.Seq = 11
		decoded, err := decodeJournalEntry(row)
		if err != nil {
			t.Fatalf("decode %s: %v", entry.Type, err)
		}
		if decoded.Seq != 11 || decoded.Type != entry.Type || decoded.ProjectID != projectID ||
			decoded.Timestamp != at.Format(time.RFC3339) || decoded.SchemaVersion != recordv0.SchemaVersion() {
			t.Errorf("decoded %s: %+v", entry.Type, decoded)
		}
		if !reflect.DeepEqual(decoded.TagState, entry.TagState) || !reflect.DeepEqual(decoded.Deployment, entry.Deployment) {
			t.Errorf("decoded payload %+v/%+v, want %+v/%+v", decoded.TagState, decoded.Deployment, entry.TagState, entry.Deployment)
		}
	}
}

// TestJournalCodec_RefusesAnUnknownType: a row of a later server is an error
// to a reader, not a panic.
func TestJournalCodec_RefusesAnUnknownType(t *testing.T) {
	row := journalRow{
		Seq:           1,
		ProjectID:     uuid.New(),
		Timestamp:     time.Now(),
		Type:          "widget_state",
		Payload:       []byte(`{}`),
		SchemaVersion: recordv0.SchemaVersion().String(),
	}
	if _, err := decodeJournalEntry(row); err == nil {
		t.Error("an unknown entry type was decoded")
	}
	entry := newTagStateEntry(uuid.New(), time.Now(), uuid.New(), "1", "good")
	entry.Type = "widget_state"
	if _, err := encodeJournalEntry(entry); err == nil {
		t.Error("an entry of an unknown type was encoded")
	}
}

// TestJournalCodec_RefusesAnotherSchemaVersion: the row is always stored in
// the format this server writes, so an entry of another version, or one
// without the payload of its type, is refused rather than written.
func TestJournalCodec_RefusesAnotherSchemaVersion(t *testing.T) {
	entry := newTagStateEntry(uuid.New(), time.Now(), uuid.New(), "1", "good")
	entry.SchemaVersion = contract.SchemaVersion{Major: 1}
	if _, err := encodeJournalEntry(entry); err == nil {
		t.Error("an entry of another schema version was encoded")
	}
	entry = newTagStateEntry(uuid.New(), time.Now(), uuid.New(), "1", "good")
	entry.TagState = nil
	if _, err := encodeJournalEntry(entry); err == nil {
		t.Error("a tag_state entry without its payload was encoded")
	}
}
