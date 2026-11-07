package recordv0

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestEntry_ReadsTheRevisionNamesAsDeployment: an entry written before task
// 72_9 carries the type "revision" and the payload member "revision". A
// reader normalizes both to Deployment, and an entry it writes carries only
// the new names.
func TestEntry_ReadsTheRevisionNamesAsDeployment(t *testing.T) {
	var entry Entry
	old := `{"schema_version":"0.3","seq":7,"timestamp":"2026-11-07T10:00:00Z",
	         "type":"revision","revision":{"number":3,"cause":"rollback","rolled_back_from":1}}`
	if err := json.Unmarshal([]byte(old), &entry); err != nil {
		t.Fatalf("read an old revision entry: %v", err)
	}
	if entry.Type != EntryTypeDeployment {
		t.Errorf("the old entry is of type %q, want %q", entry.Type, EntryTypeDeployment)
	}
	if entry.Deployment == nil || entry.Deployment.Number != 3 || entry.Deployment.RolledBackFrom == nil || *entry.Deployment.RolledBackFrom != 1 {
		t.Fatalf("the old entry's payload read as %+v", entry.Deployment)
	}

	written, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("write the entry: %v", err)
	}
	for _, oldName := range []string{`"revision"`, `"type":"revision"`} {
		if strings.Contains(string(written), oldName) {
			t.Errorf("a written entry still carries %s: %s", oldName, written)
		}
	}
	if !strings.Contains(string(written), `"deployment"`) {
		t.Errorf("a written entry carries no deployment: %s", written)
	}
}
