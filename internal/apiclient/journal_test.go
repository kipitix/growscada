package apiclient_test

import (
	"testing"

	"github.com/google/uuid"

	recordv0 "github.com/kipitix/growscada/contract/record/v0"
	"github.com/kipitix/growscada/internal/apiclient"
)

// TestJournalEventsURL: the stream URL names the project and the position to
// read after, under the API's prefix.
func TestJournalEventsURL(t *testing.T) {
	client := apiclient.New("http://localhost:9090/", nil)
	projectID := uuid.MustParse("b0000000-0000-4000-8000-000000000001")

	got := client.Project(projectID).JournalEventsURL(42)
	want := "http://localhost:9090/api/v0/projects/" + projectID.String() + "/journal/events?from=42"
	if got != want {
		t.Errorf("JournalEventsURL(42) = %q, want %q", got, want)
	}
}

// TestParseEntry: an entry's data is read as it is, unknown fields and
// unknown types included; garbage, an entry without a type and one without a
// position are no entry.
func TestParseEntry(t *testing.T) {
	tagID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	entry, ok := apiclient.ParseEntry(`{"schema_version":"0.2","seq":7,
		"timestamp":"2026-11-07T10:00:00Z","project_id":"b0000000-0000-4000-8000-000000000001",
		"type":"tag_state","future_field":true,
		"tag_state":{"tag_id":"00000000-0000-0000-0000-000000000001","value":"42","quality":"good"}}`)
	if !ok {
		t.Fatal("a tag_state entry was rejected")
	}
	if entry.Seq != 7 || entry.Type != "tag_state" || entry.TagState == nil {
		t.Fatalf("entry read as %+v", entry)
	}
	if entry.TagState.TagID != tagID || entry.TagState.Value != "42" || entry.TagState.Quality != "good" {
		t.Errorf("tag_state read as %+v", entry.TagState)
	}

	entry, ok = apiclient.ParseEntry(`{"seq":8,"type":"deployment","deployment":{"number":3,"cause":"rollback","rolled_back_from":1}}`)
	if !ok || entry.Deployment == nil || entry.Deployment.Number != 3 || entry.Deployment.RolledBackFrom == nil || *entry.Deployment.RolledBackFrom != 1 {
		t.Errorf("deployment entry read as %+v, ok %v", entry, ok)
	}

	// An entry written before task 72_9 carries the old type and payload
	// member: it is read as a Deployment entry.
	entry, ok = apiclient.ParseEntry(`{"seq":8,"type":"revision","revision":{"number":3,"cause":"rollback","rolled_back_from":1}}`)
	if !ok || entry.Type != recordv0.EntryTypeDeployment || entry.Deployment == nil || entry.Deployment.Number != 3 {
		t.Errorf("a revision entry read as %+v, ok %v", entry, ok)
	}

	// A type a reader has not learnt yet is still an entry: the reader decides
	// to ignore it (ADR 0005).
	if _, ok := apiclient.ParseEntry(`{"seq":9,"type":"future"}`); !ok {
		t.Error("an unknown entry type was rejected")
	}

	for _, bad := range []string{
		``,
		`not json`,
		`null`,
		`[]`,
		`{"seq":1}`,
		`{"type":"tag_state"}`,
		`{"seq":0,"type":"tag_state"}`,
		`{"seq":-1,"type":"tag_state"}`,
		`{"seq":"7","type":"tag_state"}`,
	} {
		if entry, ok := apiclient.ParseEntry(bad); ok {
			t.Errorf("ParseEntry(%q) accepted it as %+v", bad, entry)
		}
	}
}
