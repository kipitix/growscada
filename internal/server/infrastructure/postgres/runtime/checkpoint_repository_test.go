package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	recordv0 "github.com/kipitix/growscada/contract/record/v0"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	runtimepg "github.com/kipitix/growscada/internal/server/infrastructure/postgres/runtime"
)

// storedCheckpoint is a row of runtime.checkpoints as the tests read it: the
// envelope with its payload still encoded, so a test checks the stored format.
type storedCheckpoint struct {
	Seq           int64
	ProjectID     uuid.UUID
	TakenAt       time.Time
	Payload       json.RawMessage
	SchemaVersion string
}

// checkpointPayload is what a Checkpoint row stores (task 44, ADR 0023).
type checkpointPayload struct {
	Deployment *recordv0.Deployment          `json:"deployment,omitempty"`
	TagStates  []recordv0.CheckpointTagState `json:"tag_states"`
}

// checkpointsOf returns the Checkpoints of the Project in the order of their
// positions.
func checkpointsOf(t *testing.T, projectID uuid.UUID) []storedCheckpoint {
	t.Helper()
	rows, err := postgres.DB(t).QueryContext(context.Background(),
		`SELECT seq, project_id, taken_at, payload, schema_version FROM runtime.checkpoints
		 WHERE project_id = $1 ORDER BY seq`, projectID)
	if err != nil {
		t.Fatalf("read the checkpoints: %v", err)
	}
	defer rows.Close()
	var checkpoints []storedCheckpoint
	for rows.Next() {
		var c storedCheckpoint
		if err := rows.Scan(&c.Seq, &c.ProjectID, &c.TakenAt, &c.Payload, &c.SchemaVersion); err != nil {
			t.Fatalf("scan a checkpoint: %v", err)
		}
		checkpoints = append(checkpoints, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate over the checkpoints: %v", err)
	}
	return checkpoints
}

// payloadOf decodes the payload of a stored Checkpoint.
func payloadOf(t *testing.T, c storedCheckpoint) checkpointPayload {
	t.Helper()
	var payload checkpointPayload
	if err := json.Unmarshal(c.Payload, &payload); err != nil {
		t.Fatalf("decode the payload of checkpoint %d: %v", c.Seq, err)
	}
	return payload
}

// journalPositionOf returns the Project's current Journal position.
func journalPositionOf(t *testing.T, projectID uuid.UUID) int64 {
	t.Helper()
	var position int64
	if err := postgres.DB(t).QueryRowContext(context.Background(),
		`SELECT COALESCE(MAX(seq), 0) FROM runtime.journal WHERE project_id = $1`, projectID,
	).Scan(&position); err != nil {
		t.Fatalf("read the journal position: %v", err)
	}
	return position
}

// writeTagState writes a Tag state of the Project as a person does, appending a
// Journal entry when the value changes.
func writeTagState(t *testing.T, projectID id.ID[project.Project], tagID uuid.UUID, value string) {
	t.Helper()
	repo := runtimepg.NewTagStateRepositoryPostgres(postgres.DB(t))
	state := mustFind(t, repo, projectID, id.NewID(id.IDWithUUID[tag.Tag](tagID)))
	written, err := state.Write(value, tag.TagQualityGood)
	if err != nil {
		t.Fatalf("write tag %s with %q: %v", tagID, value, err)
	}
	if _, _, err := repo.Write(context.Background(), written, tag.NoSupplier()); err != nil {
		t.Fatalf("store tag %s with %q: %v", tagID, value, err)
	}
}

// containsProject reports whether projects names the Project.
func containsProject(projects []uuid.UUID, projectID uuid.UUID) bool {
	for _, candidate := range projects {
		if candidate == projectID {
			return true
		}
	}
	return false
}

// TestCreateDeployment_WritesACheckpointAtTheDeploymentEntry: a Deploy of a
// Release writes one Checkpoint in its own transaction, at the position of the
// deployment entry, naming the Deployment and holding the initial states of
// its Tags (task 44, ADR 0023).
func TestCreateDeployment_WritesACheckpointAtTheDeploymentEntry(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	tagID := uuid.New()
	file := documentOf(projectID, withTags(projectv0.TagDefinition{ID: tagID, Name: "speed", Type: "integer"}))
	releaseNumber := 5
	at := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)

	deployment, err := repo.CreateDeployment(ctx, projectID.UUID(), file, runtimeapp.DeploymentCauseDeploy, &releaseNumber, at)
	if err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}
	entries := journalEntriesOf(t, projectID.UUID())
	if len(entries) == 0 {
		t.Fatal("the deploy appended no journal entry")
	}
	entry := entries[len(entries)-1]
	if payload := deploymentPayload(t, entry); payload.Number != deployment.Number {
		t.Fatalf("the last entry names deployment %d, the deploy made %d", payload.Number, deployment.Number)
	}

	checkpoints := checkpointsOf(t, projectID.UUID())
	if len(checkpoints) != 1 {
		t.Fatalf("%d checkpoints, want 1", len(checkpoints))
	}
	checkpoint := checkpoints[0]
	if checkpoint.Seq != entry.Seq || checkpoint.ProjectID != projectID.UUID() ||
		checkpoint.SchemaVersion != recordv0.SchemaVersion().String() ||
		checkpoint.TakenAt.UTC().Format(time.RFC3339) != at.Format(time.RFC3339) {
		t.Errorf("checkpoint %+v, want one at %d of project %s in schema %s taken at %s",
			checkpoint, entry.Seq, projectID.UUID(), recordv0.SchemaVersion(), at.Format(time.RFC3339))
	}
	payload := payloadOf(t, checkpoint)
	if payload.Deployment == nil || payload.Deployment.Number != deployment.Number ||
		payload.Deployment.Cause != recordv0.DeploymentCauseDeploy ||
		payload.Deployment.ReleaseNumber == nil || *payload.Deployment.ReleaseNumber != releaseNumber ||
		payload.Deployment.RolledBackFrom != nil {
		t.Errorf("deployment payload %+v", payload.Deployment)
	}
	if len(payload.TagStates) != 1 {
		t.Fatalf("%d tag states, want 1", len(payload.TagStates))
	}
	state := payload.TagStates[0]
	if state.TagID != tagID || state.Value != "0" || state.Quality != "bad" || state.Seq != 0 {
		t.Errorf("tag state %+v, want the initial state of %s", state, tagID)
	}
}

// TestCreateDeployment_FileWritesACheckpoint: a Deploy of a project file is a
// Deployment too, with cause file and no Release (ADR 0018, task 44).
func TestCreateDeployment_FileWritesACheckpoint(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()

	deployment, err := repo.CreateDeployment(ctx, projectID.UUID(), documentOf(projectID), runtimeapp.DeploymentCauseFile, nil, time.Now())
	if err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}
	entries := journalEntriesOf(t, projectID.UUID())
	checkpoints := checkpointsOf(t, projectID.UUID())
	if len(checkpoints) != 1 {
		t.Fatalf("%d checkpoints, want 1", len(checkpoints))
	}
	if checkpoints[0].Seq != entries[len(entries)-1].Seq {
		t.Errorf("checkpoint at %d, the deployment entry at %d", checkpoints[0].Seq, entries[len(entries)-1].Seq)
	}
	payload := payloadOf(t, checkpoints[0])
	if payload.Deployment == nil || payload.Deployment.Number != deployment.Number ||
		payload.Deployment.Cause != recordv0.DeploymentCauseFile || payload.Deployment.ReleaseNumber != nil {
		t.Errorf("deployment payload %+v", payload.Deployment)
	}
	if payload.TagStates == nil || len(payload.TagStates) != 0 {
		t.Errorf("tag states %#v, want an empty list", payload.TagStates)
	}
}

// TestRollback_WritesACheckpoint: a Rollback names the new Deployment, the one
// it copied and the Release of that one, at its own entry's position (task 44).
func TestRollback_WritesACheckpoint(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	tagID := uuid.New()
	first := documentOf(projectID, withTags(projectv0.TagDefinition{ID: tagID, Name: "speed", Type: "integer"}))
	second := documentOf(projectID, withTags(
		projectv0.TagDefinition{ID: tagID, Name: "speed", Type: "integer"},
		projectv0.TagDefinition{ID: uuid.New(), Name: "temp", Type: "integer"},
	))
	firstRelease := 7
	if _, err := repo.CreateDeployment(ctx, projectID.UUID(), first, runtimeapp.DeploymentCauseDeploy, &firstRelease, time.Now()); err != nil {
		t.Fatalf("first deploy: %v", err)
	}
	secondRelease := 8
	if _, err := repo.CreateDeployment(ctx, projectID.UUID(), second, runtimeapp.DeploymentCauseDeploy, &secondRelease, time.Now()); err != nil {
		t.Fatalf("second deploy: %v", err)
	}

	rolledBack, err := repo.Rollback(ctx, projectID.UUID(), 1, time.Now())
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	entries := journalEntriesOf(t, projectID.UUID())
	checkpoints := checkpointsOf(t, projectID.UUID())
	last := checkpoints[len(checkpoints)-1]
	if last.Seq != entries[len(entries)-1].Seq {
		t.Errorf("the rollback checkpoint at %d, its entry at %d", last.Seq, entries[len(entries)-1].Seq)
	}
	payload := payloadOf(t, last)
	if payload.Deployment == nil || payload.Deployment.Number != rolledBack.Number ||
		payload.Deployment.Cause != recordv0.DeploymentCauseRollback ||
		payload.Deployment.RolledBackFrom == nil || *payload.Deployment.RolledBackFrom != 1 ||
		payload.Deployment.ReleaseNumber == nil || *payload.Deployment.ReleaseNumber != firstRelease {
		t.Errorf("deployment payload %+v", payload.Deployment)
	}
	// The rolled-back document names one Tag, and its state is the one the
	// first Deployment left: a Tag the second added is gone with its copy.
	if len(payload.TagStates) != 1 || payload.TagStates[0].TagID != tagID {
		t.Errorf("tag states %+v, want the state of %s alone", payload.TagStates, tagID)
	}
}

// TestNearest_FindsTheLatestCheckpointAtOrBefore: the read side gives the
// snapshot a position falls into, and none before the first one (task 44).
func TestNearest_FindsTheLatestCheckpointAtOrBefore(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	checkpoints := runtimepg.NewCheckpointRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	tagID := uuid.New()
	file := documentOf(projectID, withTags(projectv0.TagDefinition{ID: tagID, Name: "speed", Type: "integer"}))
	mustCreateDeployment(t, repo, projectID.UUID(), file, time.Now())
	deployedAt := journalPositionOf(t, projectID.UUID())
	writeTagState(t, projectID, tagID, "1")
	writtenAt := journalPositionOf(t, projectID.UUID())
	if writtenAt <= deployedAt {
		t.Fatalf("the tag state at %d, the deploy at %d", writtenAt, deployedAt)
	}

	if _, ok, err := checkpoints.Nearest(ctx, projectID.UUID(), deployedAt-1); err != nil || ok {
		t.Errorf("Nearest before the first checkpoint: %v, %v", ok, err)
	}
	for _, position := range []int64{deployedAt, writtenAt, writtenAt + 100} {
		found, ok, err := checkpoints.Nearest(ctx, projectID.UUID(), position)
		if err != nil {
			t.Fatalf("Nearest at %d: %v", position, err)
		}
		if !ok || found.Seq != deployedAt {
			t.Errorf("Nearest at %d: %+v, %v; want the deploy checkpoint at %d", position, found, ok, deployedAt)
		}
	}
}

// TestNearest_RefusesAProjectThatIsNotLive: a deleted Project keeps its rows
// until its cleanup removes them, and they are not its Checkpoints to serve.
func TestNearest_RefusesAProjectThatIsNotLive(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	checkpoints := runtimepg.NewCheckpointRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	mustCreateDeployment(t, repo, projectID.UUID(), documentOf(projectID), time.Now())
	if _, err := postgres.DB(t).ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, projectID.UUID()); err != nil {
		t.Fatalf("delete the project: %v", err)
	}
	if _, _, err := checkpoints.Nearest(ctx, projectID.UUID(), 100); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("Nearest of a deleted project: %v, want project.ErrProjectNotFound", err)
	}
}

// TestDueProjects_CountsTheThresholdInEntries: a Project is due once its
// position has moved everyEntries past its latest Checkpoint (task 44).
func TestDueProjects_CountsTheThresholdInEntries(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	checkpoints := runtimepg.NewCheckpointRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	tagID := uuid.New()
	file := documentOf(projectID, withTags(projectv0.TagDefinition{ID: tagID, Name: "speed", Type: "integer"}))
	mustCreateDeployment(t, repo, projectID.UUID(), file, time.Now())
	deployedAt := journalPositionOf(t, projectID.UUID())

	due, err := checkpoints.DueProjects(ctx, 1)
	if err != nil {
		t.Fatalf("DueProjects: %v", err)
	}
	if containsProject(due, projectID.UUID()) {
		t.Errorf("project %s is due one entry after its checkpoint at %d", projectID, deployedAt)
	}

	writeTagState(t, projectID, tagID, "1")
	writeTagState(t, projectID, tagID, "2")
	position := journalPositionOf(t, projectID.UUID())
	if position != deployedAt+2 {
		t.Fatalf("position %d, want %d", position, deployedAt+2)
	}
	due, err = checkpoints.DueProjects(ctx, 2)
	if err != nil {
		t.Fatalf("DueProjects: %v", err)
	}
	if !containsProject(due, projectID.UUID()) {
		t.Errorf("project %s is not due two entries after its checkpoint", projectID)
	}
	due, err = checkpoints.DueProjects(ctx, 3)
	if err != nil {
		t.Fatalf("DueProjects: %v", err)
	}
	if containsProject(due, projectID.UUID()) {
		t.Errorf("project %s is due three entries after its checkpoint", projectID)
	}
}

// TestSnapshot_TakesOneSnapshotOfADuePosition: a periodic snapshot reads the
// position, the Deployment and the states from one moment and inserts one row;
// a second pass adds nothing, and an already checkpointed position is not
// written twice (task 44, ADR 0023).
func TestSnapshot_TakesOneSnapshotOfADuePosition(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	checkpoints := runtimepg.NewCheckpointRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	tagID := uuid.New()
	file := documentOf(projectID, withTags(projectv0.TagDefinition{ID: tagID, Name: "speed", Type: "integer"}))
	mustCreateDeployment(t, repo, projectID.UUID(), file, time.Now())

	taken, err := checkpoints.Snapshot(ctx, projectID.UUID(), 2)
	if err != nil || taken {
		t.Fatalf("Snapshot before the threshold: %v, taken %v", err, taken)
	}

	writeTagState(t, projectID, tagID, "5")
	writeTagState(t, projectID, tagID, "6")
	position := journalPositionOf(t, projectID.UUID())
	taken, err = checkpoints.Snapshot(ctx, projectID.UUID(), 2)
	if err != nil || !taken {
		t.Fatalf("Snapshot: %v, taken %v", err, taken)
	}
	checkpointsOfProject := checkpointsOf(t, projectID.UUID())
	if len(checkpointsOfProject) != 2 {
		t.Fatalf("%d checkpoints, want the deploy's and the periodic one", len(checkpointsOfProject))
	}
	periodic := checkpointsOfProject[len(checkpointsOfProject)-1]
	if periodic.Seq != position {
		t.Errorf("periodic checkpoint at %d, the position at %d", periodic.Seq, position)
	}
	payload := payloadOf(t, periodic)
	if payload.Deployment == nil || payload.Deployment.Number != 1 {
		t.Errorf("deployment payload %+v, want deployment 1", payload.Deployment)
	}
	if len(payload.TagStates) != 1 || payload.TagStates[0].TagID != tagID ||
		payload.TagStates[0].Value != "6" || payload.TagStates[0].Quality != "good" {
		t.Errorf("tag states %+v", payload.TagStates)
	}
	assertTagStatesMatch(t, projectID.UUID(), payload.TagStates)

	// The position has not moved far enough again.
	if taken, err := checkpoints.Snapshot(ctx, projectID.UUID(), 2); err != nil || taken {
		t.Errorf("a second Snapshot: %v, taken %v", err, taken)
	}
	// The same position is never written twice, whatever the threshold.
	if taken, err := checkpoints.Snapshot(ctx, projectID.UUID(), 0); err != nil || taken {
		t.Errorf("Snapshot at a checkpointed position: %v, taken %v", err, taken)
	}
	if got := len(checkpointsOf(t, projectID.UUID())); got != 2 {
		t.Errorf("%d checkpoints after repeated snapshots, want 2", got)
	}
}

// assertTagStatesMatch checks a Checkpoint's states against runtime.tag_states.
func assertTagStatesMatch(t *testing.T, projectID uuid.UUID, states []recordv0.CheckpointTagState) {
	t.Helper()
	rows, err := postgres.DB(t).QueryContext(context.Background(),
		`SELECT s.tag_id, s.value, s.quality, s.seq
		 FROM runtime.tag_states s JOIN runtime.tags t ON t.id = s.tag_id
		 WHERE t.project_id = $1 ORDER BY s.tag_id`, projectID)
	if err != nil {
		t.Fatalf("read the tag states: %v", err)
	}
	defer rows.Close()
	var stored []recordv0.CheckpointTagState
	for rows.Next() {
		var state recordv0.CheckpointTagState
		if err := rows.Scan(&state.TagID, &state.Value, &state.Quality, &state.Seq); err != nil {
			t.Fatalf("scan a tag state: %v", err)
		}
		stored = append(stored, state)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate over the tag states: %v", err)
	}
	if len(stored) != len(states) {
		t.Fatalf("%d checkpoint states, %d stored", len(states), len(stored))
	}
	for i := range stored {
		if stored[i] != states[i] {
			t.Errorf("checkpoint state %+v, stored %+v", states[i], stored[i])
		}
	}
}

// TestCheckpoints_GoWithTheirProject: a Checkpoint is kept forever and removed
// only with its Project, as the Journal is (ADR 0023).
func TestCheckpoints_GoWithTheirProject(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	mustCreateDeployment(t, repo, projectID.UUID(), documentOf(projectID), time.Now())
	if got := len(checkpointsOf(t, projectID.UUID())); got != 1 {
		t.Fatalf("%d checkpoints, want 1", got)
	}
	if _, err := postgres.DB(t).ExecContext(context.Background(),
		`DELETE FROM projects WHERE id = $1`, projectID.UUID()); err != nil {
		t.Fatalf("delete the project: %v", err)
	}
	if got := len(checkpointsOf(t, projectID.UUID())); got != 0 {
		t.Errorf("%d checkpoints left after the project was deleted", got)
	}
}
