package runtime

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/contract"
	recordv0 "github.com/kipitix/growscada/contract/record/v0"
)

// checkpointRow is one row of runtime.checkpoints as the codec reads and
// writes it: the envelope's columns, with the payload still encoded. The seq
// is a Journal position, never a number of its own (task 44, ADR 0023).
type checkpointRow struct {
	Seq           int64
	ProjectID     uuid.UUID
	TakenAt       time.Time
	Payload       []byte
	SchemaVersion string
}

// checkpointPayload is the payload of a Checkpoint row: the Deployment in force
// and every Tag state. The Deployment is addressed, never copied — the
// document stays in runtime.deployments (ADR 0014).
type checkpointPayload struct {
	Deployment *recordv0.Deployment          `json:"deployment,omitempty"`
	TagStates  []recordv0.CheckpointTagState `json:"tag_states"`
}

// encodeCheckpoint returns the row a Checkpoint is stored as: its timestamp
// parsed, its schema version written down and its payload. A Checkpoint of
// another schema version than this server writes is refused: the stored row is
// always in the current format.
func encodeCheckpoint(checkpoint recordv0.Checkpoint) (checkpointRow, error) {
	if checkpoint.SchemaVersion != recordv0.SchemaVersion() {
		return checkpointRow{}, fmt.Errorf("cannot encode a checkpoint of schema version %s: this server writes %s",
			checkpoint.SchemaVersion, recordv0.SchemaVersion())
	}
	takenAt, err := time.Parse(time.RFC3339, checkpoint.Timestamp)
	if err != nil {
		return checkpointRow{}, fmt.Errorf("cannot parse the timestamp of checkpoint %d: %w", checkpoint.Seq, err)
	}
	states := checkpoint.TagStates
	if states == nil {
		states = []recordv0.CheckpointTagState{}
	}
	payload, err := json.Marshal(checkpointPayload{Deployment: checkpoint.Deployment, TagStates: states})
	if err != nil {
		return checkpointRow{}, fmt.Errorf("cannot encode the payload of checkpoint %d: %w", checkpoint.Seq, err)
	}
	return checkpointRow{
		Seq:           checkpoint.Seq,
		ProjectID:     checkpoint.ProjectID,
		TakenAt:       takenAt,
		Payload:       payload,
		SchemaVersion: checkpoint.SchemaVersion.String(),
	}, nil
}

// decodeCheckpoint rebuilds a Checkpoint from its row. A payload that does not
// decode is an error, not a panic: a row of a later server must not take a
// reader down.
func decodeCheckpoint(row checkpointRow) (recordv0.Checkpoint, error) {
	version, err := contract.ParseSchemaVersion(row.SchemaVersion)
	if err != nil {
		return recordv0.Checkpoint{}, fmt.Errorf("cannot decode checkpoint %d of project %s: %w", row.Seq, row.ProjectID, err)
	}
	var payload checkpointPayload
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		return recordv0.Checkpoint{}, fmt.Errorf("cannot decode the payload of checkpoint %d of project %s: %w", row.Seq, row.ProjectID, err)
	}
	if payload.TagStates == nil {
		payload.TagStates = []recordv0.CheckpointTagState{}
	}
	return recordv0.Checkpoint{
		Header:     recordv0.Header{SchemaVersion: version},
		Seq:        row.Seq,
		ProjectID:  row.ProjectID,
		Timestamp:  row.TakenAt.UTC().Format(time.RFC3339),
		Deployment: payload.Deployment,
		TagStates:  payload.TagStates,
	}, nil
}
