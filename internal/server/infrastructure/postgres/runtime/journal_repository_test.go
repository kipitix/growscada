package runtime_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	recordv0 "github.com/kipitix/growscada/contract/record/v0"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/tagstate"
	runtimepg "github.com/kipitix/growscada/internal/server/infrastructure/postgres/runtime"
)

// storedEntry is a row of runtime.journal as the tests read it: the envelope
// with its payload still encoded, so a test checks the stored format.
type storedEntry struct {
	Seq           int64
	ProjectID     uuid.UUID
	Timestamp     time.Time
	Type          string
	Payload       json.RawMessage
	SchemaVersion string
}

// journalEntriesOf returns the Journal of the Project in the order of its
// positions.
func journalEntriesOf(t *testing.T, projectID uuid.UUID) []storedEntry {
	t.Helper()
	rows, err := postgres.DB(t).QueryContext(context.Background(),
		`SELECT seq, project_id, ts, type, payload, schema_version FROM runtime.journal
		 WHERE project_id = $1 ORDER BY seq`, projectID)
	if err != nil {
		t.Fatalf("read the journal: %v", err)
	}
	defer rows.Close()
	var entries []storedEntry
	for rows.Next() {
		var e storedEntry
		if err := rows.Scan(&e.Seq, &e.ProjectID, &e.Timestamp, &e.Type, &e.Payload, &e.SchemaVersion); err != nil {
			t.Fatalf("scan a journal entry: %v", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate over the journal: %v", err)
	}
	return entries
}

// tagStatePayload decodes the payload of a tag_state entry.
func tagStatePayload(t *testing.T, e storedEntry) recordv0.TagState {
	t.Helper()
	if e.Type != string(recordv0.EntryTypeTagState) {
		t.Fatalf("entry %d is a %s, want tag_state", e.Seq, e.Type)
	}
	var payload recordv0.TagState
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		t.Fatalf("decode the payload of entry %d: %v", e.Seq, err)
	}
	return payload
}

// deploymentPayload decodes the payload of a deployment entry.
func deploymentPayload(t *testing.T, e storedEntry) recordv0.Deployment {
	t.Helper()
	if e.Type != string(recordv0.EntryTypeDeployment) {
		t.Fatalf("entry %d is a %s, want deployment", e.Seq, e.Type)
	}
	var payload recordv0.Deployment
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		t.Fatalf("decode the payload of entry %d: %v", e.Seq, err)
	}
	return payload
}

// TestWrite_RecordsTheChangeInTheJournal: a change appends exactly one entry
// in the write's transaction, its seq is the state's, and its payload is the
// value and Quality, the entity named inside it (ADR 0019).
func TestWrite_RecordsTheChangeInTheJournal(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewTagStateRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	speed := definition(t, projectID, "speed", tag.TagTypeInteger)
	if err := repo.UpsertDefinitions(ctx, projectID, []tagstate.Definition{speed}); err != nil {
		t.Fatalf("UpsertDefinitions: %v", err)
	}

	written, _ := mustFind(t, repo, projectID, speed.TagID).Write("42", tag.TagQualityGood)
	stored, changed, err := repo.Write(ctx, written, tag.NoSupplier())
	if err != nil || !changed {
		t.Fatalf("Write: %v, changed %v", err, changed)
	}

	entries := journalEntriesOf(t, projectID.UUID())
	if len(entries) != 1 {
		t.Fatalf("%d journal entries, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Seq != int64(stored.Seq()) {
		t.Errorf("entry at %d, the state at %d", entry.Seq, stored.Seq())
	}
	if entry.ProjectID != projectID.UUID() || entry.SchemaVersion != recordv0.SchemaVersion().String() {
		t.Errorf("entry %d of project %s in schema %q", entry.Seq, entry.ProjectID, entry.SchemaVersion)
	}
	if got, want := entry.Timestamp.UTC().Format(time.RFC3339), stored.WrittenAt().UTC().Format(time.RFC3339); got != want {
		t.Errorf("entry at %s, the state written at %s", got, want)
	}
	if payload := tagStatePayload(t, entry); payload.TagID != speed.TagID.UUID() || payload.Value != "42" || payload.Quality != "good" {
		t.Errorf("payload %+v", payload)
	}
}

// TestWrite_WithoutAChange_KeepsTheSeqAndMovesTheWriteMoment: a repeat of the
// stored value and Quality appends nothing and keeps the position; only the
// write moment moves, so a stable Device stays visibly alive (ADR 0019).
func TestWrite_WithoutAChange_KeepsTheSeqAndMovesTheWriteMoment(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewTagStateRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	speed := definition(t, projectID, "speed", tag.TagTypeInteger)
	if err := repo.UpsertDefinitions(ctx, projectID, []tagstate.Definition{speed}); err != nil {
		t.Fatalf("UpsertDefinitions: %v", err)
	}

	first, _ := mustFind(t, repo, projectID, speed.TagID).Write("1", tag.TagQualityGood)
	stored, changed, err := repo.Write(ctx, first, tag.NoSupplier())
	if err != nil || !changed {
		t.Fatalf("first write: %v, changed %v", err, changed)
	}
	again, _ := mustFind(t, repo, projectID, speed.TagID).Write("1", tag.TagQualityGood)
	repeated, changed, err := repo.Write(ctx, again, tag.NoSupplier())
	if err != nil {
		t.Fatalf("repeat: %v", err)
	}
	if changed {
		t.Error("the repeat of the stored state changed it")
	}
	if repeated.Seq() != stored.Seq() {
		t.Errorf("the repeat took seq %d, want the previous %d", repeated.Seq(), stored.Seq())
	}
	if repeated.WrittenAt().Before(*stored.WrittenAt()) {
		t.Errorf("the repeat written at %s, before %s", repeated.WrittenAt(), stored.WrittenAt())
	}
	if entries := journalEntriesOf(t, projectID.UUID()); len(entries) != 1 {
		t.Errorf("%d journal entries after the repeat, want the first change alone", len(entries))
	}

	// A change of the Quality alone is a change of the state.
	requalified, _ := mustFind(t, repo, projectID, speed.TagID).Write("1", tag.TagQualityUncertain)
	changedState, changed, err := repo.Write(ctx, requalified, tag.NoSupplier())
	if err != nil || !changed {
		t.Fatalf("a new quality: %v, changed %v", err, changed)
	}
	if changedState.Seq() <= stored.Seq() {
		t.Errorf("the new quality took seq %d, not above %d", changedState.Seq(), stored.Seq())
	}
	entries := journalEntriesOf(t, projectID.UUID())
	if len(entries) != 2 {
		t.Fatalf("%d journal entries, want the change of the quality too", len(entries))
	}
	if payload := tagStatePayload(t, entries[1]); payload.Quality != "uncertain" {
		t.Errorf("the payload of the new quality: %+v", payload)
	}
}

// TestWrite_OfAnotherSupplier_RecordsNothing: a refused write changes neither
// the state nor the Journal.
func TestWrite_OfAnotherSupplier_RecordsNothing(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewTagStateRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	speed := definition(t, projectID, "speed", tag.TagTypeInteger)
	speed.Supplier = tag.SuppliedByDevice(id.NewID[device.Device]())
	if err := repo.UpsertDefinitions(ctx, projectID, []tagstate.Definition{speed}); err != nil {
		t.Fatalf("UpsertDefinitions: %v", err)
	}
	written, _ := mustFind(t, repo, projectID, speed.TagID).Write("1", tag.TagQualityGood)
	if _, _, err := repo.Write(ctx, written, tag.NoSupplier()); err == nil {
		t.Fatal("a person wrote a supplied tag")
	}
	if entries := journalEntriesOf(t, projectID.UUID()); len(entries) != 0 {
		t.Errorf("%d journal entries of a refused write", len(entries))
	}
	if stored := mustFind(t, repo, projectID, speed.TagID); stored.WrittenAt() != nil {
		t.Errorf("the refused write moved the write moment: %s", stored.WrittenAt())
	}
}

// TestJournal_CascadesWithTheProjectAndKeepsCursors: the record goes with its
// Project, as the Tag states do; the positions of the subscribers are the
// server's and stay (ADR 0019).
func TestJournal_CascadesWithTheProjectAndKeepsCursors(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewTagStateRepositoryPostgres(db)
	ctx := context.Background()
	speed := definition(t, projectID, "speed", tag.TagTypeInteger)
	if err := repo.UpsertDefinitions(ctx, projectID, []tagstate.Definition{speed}); err != nil {
		t.Fatalf("UpsertDefinitions: %v", err)
	}
	written, _ := mustFind(t, repo, projectID, speed.TagID).Write("1", tag.TagQualityGood)
	if _, _, err := repo.Write(ctx, written, tag.NoSupplier()); err != nil {
		t.Fatalf("Write: %v", err)
	}

	const subscriber = "test_journal_cascade"
	if _, err := db.ExecContext(ctx, `INSERT INTO runtime.journal_cursors (subscriber, last_seq) VALUES ($1, 5)
	                                   ON CONFLICT (subscriber) DO UPDATE SET last_seq = EXCLUDED.last_seq`, subscriber); err != nil {
		t.Fatalf("insert a cursor: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM runtime.journal_cursors WHERE subscriber = $1`, subscriber)
	})

	// The cleanup of a deleted Project removes its Tag copies, then the
	// Project's row; the Journal goes with the row.
	if _, err := db.ExecContext(ctx, `DELETE FROM runtime.tags WHERE project_id = $1`, projectID.UUID()); err != nil {
		t.Fatalf("delete the tag copies: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, projectID.UUID()); err != nil {
		t.Fatalf("delete the project: %v", err)
	}
	if entries := journalEntriesOf(t, projectID.UUID()); len(entries) != 0 {
		t.Errorf("%d journal entries left after the project went", len(entries))
	}
	var lastSeq int64
	if err := db.QueryRowContext(ctx, `SELECT last_seq FROM runtime.journal_cursors WHERE subscriber = $1`, subscriber).Scan(&lastSeq); err != nil || lastSeq != 5 {
		t.Errorf("the cursor of the subscriber: %d, %v", lastSeq, err)
	}
}

// TestCreateDeployment_RecordsADeploymentEntry: a Deploy, a project file and a
// Rollback each append one deployment entry in the Deployment's transaction, with
// the Deployment's time and cause and never the document (ADR 0019).
func TestCreateDeployment_RecordsADeploymentEntry(t *testing.T) {
	projectID, _ := newProject(t)
	deployments := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()

	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	deployed, err := deployments.CreateDeployment(ctx, projectID.UUID(), documentOf(projectID), runtimeapp.DeploymentCauseDeploy, at)
	if err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}
	fromFile, err := deployments.CreateDeployment(ctx, projectID.UUID(), documentOf(projectID, withTags(documentTag(definition(t, projectID, "speed", tag.TagTypeInteger)))), runtimeapp.DeploymentCauseFile, at.Add(time.Minute))
	if err != nil {
		t.Fatalf("CreateDeployment(file): %v", err)
	}
	rolledBack, err := deployments.Rollback(ctx, projectID.UUID(), deployed.Number, at.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	entries := journalEntriesOf(t, projectID.UUID())
	if len(entries) != 3 {
		t.Fatalf("%d journal entries, want one per Deployment", len(entries))
	}
	for i, want := range []struct {
		number int
		cause  recordv0.DeploymentCause
		from   *int
		at     time.Time
	}{
		{deployed.Number, recordv0.DeploymentCauseDeploy, nil, at},
		{fromFile.Number, recordv0.DeploymentCauseFile, nil, at.Add(time.Minute)},
		{rolledBack.Number, recordv0.DeploymentCauseRollback, &deployed.Number, at.Add(2 * time.Minute)},
	} {
		entry := entries[i]
		payload := deploymentPayload(t, entry)
		if payload.Number != want.number || payload.Cause != want.cause {
			t.Errorf("entry %d: deployment %d by %s, want %d by %s", i, payload.Number, payload.Cause, want.number, want.cause)
		}
		if (payload.RolledBackFrom == nil) != (want.from == nil) ||
			(payload.RolledBackFrom != nil && *payload.RolledBackFrom != *want.from) {
			t.Errorf("entry %d: rolled back from %v, want %v", i, payload.RolledBackFrom, want.from)
		}
		if got := entry.Timestamp.UTC(); !got.Equal(want.at) {
			t.Errorf("entry %d at %s, want the Deployment's %s", i, got, want.at)
		}
	}
}

// TestCreateDeployment_Refused_LeavesNoJournalEntry: a transaction that rolls
// back takes its entry with it.
func TestCreateDeployment_Refused_LeavesNoJournalEntry(t *testing.T) {
	projectID, _ := newProject(t)
	deployments := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	twice := documentOf(projectID,
		withEnvironments(
			environmentOf(uuid.New(), "shop"),
			environmentOf(uuid.New(), "shop"),
		))
	if _, err := deployments.CreateDeployment(context.Background(), projectID.UUID(), twice, runtimeapp.DeploymentCauseDeploy, time.Now()); err == nil {
		t.Fatal("two environments of one name were stored")
	}
	if entries := journalEntriesOf(t, projectID.UUID()); len(entries) != 0 {
		t.Errorf("%d journal entries of a refused Deployment", len(entries))
	}
}

// environmentOf is an Environment of a ProjectFile named by the test alone: a
// document with two Environments of one name is the refused one.
func environmentOf(environmentID uuid.UUID, name string) projectv0.Environment {
	return projectv0.Environment{ID: environmentID, Name: name}
}
