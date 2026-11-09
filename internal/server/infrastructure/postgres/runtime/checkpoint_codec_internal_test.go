package runtime

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/contract"
	recordv0 "github.com/kipitix/growscada/contract/record/v0"
)

// TestCheckpointCodec_RoundTrips: a row written by the codec decodes back to
// the Checkpoint it came from, its envelope and its payload both (task 44).
func TestCheckpointCodec_RoundTrips(t *testing.T) {
	at := time.Date(2026, 11, 11, 9, 15, 0, 0, time.UTC)
	projectID, tagID := uuid.New(), uuid.New()
	rolledBackFrom, releaseNumber := 2, 5
	checkpoint := recordv0.Checkpoint{
		Header:    recordv0.Header{SchemaVersion: recordv0.SchemaVersion()},
		Seq:       42,
		ProjectID: projectID,
		Timestamp: at.Format(time.RFC3339),
		Deployment: &recordv0.Deployment{
			Number:         3,
			Cause:          recordv0.DeploymentCauseRollback,
			RolledBackFrom: &rolledBackFrom,
			ReleaseNumber:  &releaseNumber,
		},
		TagStates: []recordv0.CheckpointTagState{
			{TagID: tagID, Value: "42", Quality: "good", Seq: 40},
		},
	}

	row, err := encodeCheckpoint(checkpoint)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if row.Seq != 42 || row.ProjectID != projectID || !row.TakenAt.Equal(at) ||
		row.SchemaVersion != recordv0.SchemaVersion().String() {
		t.Errorf("row %+v", row)
	}

	decoded, err := decodeCheckpoint(row)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(decoded, checkpoint) {
		t.Errorf("decoded %+v, want %+v", decoded, checkpoint)
	}
}

// TestCheckpointCodec_RoundTripsAnEmptyTagStates: a Checkpoint of a Project
// with no Tag state still carries its list, and reads back empty.
func TestCheckpointCodec_RoundTripsAnEmptyTagStates(t *testing.T) {
	checkpoint := recordv0.Checkpoint{
		Header:    recordv0.Header{SchemaVersion: recordv0.SchemaVersion()},
		Seq:       7,
		ProjectID: uuid.New(),
		Timestamp: time.Date(2026, 11, 11, 9, 15, 0, 0, time.UTC).Format(time.RFC3339),
	}
	row, err := encodeCheckpoint(checkpoint)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !strings.Contains(string(row.Payload), `"tag_states":[]`) {
		t.Errorf("payload %s does not name an empty tag_states list", row.Payload)
	}
	decoded, err := decodeCheckpoint(row)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.TagStates == nil || len(decoded.TagStates) != 0 {
		t.Errorf("decoded tag states %#v, want an empty non-nil list", decoded.TagStates)
	}
	if decoded.Deployment != nil {
		t.Errorf("decoded deployment %+v, want none", decoded.Deployment)
	}
}

// TestCheckpointCodec_RefusesAnotherSchemaVersion: the row is always stored in
// the format this server writes, so a Checkpoint of another version is refused
// rather than written.
func TestCheckpointCodec_RefusesAnotherSchemaVersion(t *testing.T) {
	checkpoint := recordv0.Checkpoint{
		Header:    recordv0.Header{SchemaVersion: contract.SchemaVersion{Major: 1}},
		Seq:       1,
		ProjectID: uuid.New(),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	if _, err := encodeCheckpoint(checkpoint); err == nil {
		t.Error("a checkpoint of another schema version was encoded")
	}
}
