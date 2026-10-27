package devicelink

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/versiontest"
)

func TestWarnOlderServer_LogsBothVersions(t *testing.T) {
	older := versiontest.Older(apiv0.SchemaVersion())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contract.SchemaVersionHeader, older.String())
		w.Write([]byte(`{"libraries":[]}`))
	}))
	defer srv.Close()
	var log bytes.Buffer
	client := apiclient.New(srv.URL, srv.Client(), WarnOlderServer(slog.New(slog.NewTextHandler(&log, nil))))

	for range 2 {
		if _, err := client.ListLibraries(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	if got := strings.Count(log.String(), "\n"); got != 1 {
		t.Errorf("%d log record(s), want 1:\n%s", got, log.String())
	}
	if !strings.Contains(log.String(), "server_api="+older.String()) ||
		!strings.Contains(log.String(), "device_api="+apiv0.SchemaVersion().String()) {
		t.Errorf("the record does not name both versions:\n%s", log.String())
	}
}
