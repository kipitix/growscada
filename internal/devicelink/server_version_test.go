package devicelink

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

func TestServerVersionWatch_LogsAnOlderServerOncePerVersion(t *testing.T) {
	own := apiv0.SchemaVersion()
	older := contract.SchemaVersion{Major: own.Major, Minor: own.Minor - 1}
	oldest := contract.SchemaVersion{Major: own.Major, Minor: own.Minor - 2}
	newer := contract.SchemaVersion{Major: own.Major, Minor: own.Minor + 1}
	var log bytes.Buffer
	w := newServerVersionWatch(slog.New(slog.NewTextHandler(&log, nil)))

	for _, step := range []struct {
		server   contract.SchemaVersion
		wantLogs int
	}{
		{older, 1},  // first seen, older
		{older, 1},  // the same server again
		{own, 1},    // replaced by a current one
		{newer, 1},  // a newer one is fine
		{older, 2},  // an older one again: a change
		{oldest, 3}, // another older one
		{oldest, 3},
	} {
		w.observe(step.server)
		if got := strings.Count(log.String(), "\n"); got != step.wantLogs {
			t.Fatalf("after %v: %d log record(s), want %d:\n%s", step.server, got, step.wantLogs, log.String())
		}
	}
	if !strings.Contains(log.String(), "server_api="+oldest.String()) {
		t.Errorf("the record does not name the server's version:\n%s", log.String())
	}
}
