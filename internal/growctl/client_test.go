package growctl_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/growctl"
	"github.com/kipitix/growscada/internal/versiontest"
)

func TestListTagsMatching_ServerIgnoringFilter_FiltersClientSide(t *testing.T) {
	// A server that predates ?name_pattern= returns every tag; the client must
	// not widen the selection (apply --prune --pattern would delete everything).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"tags":[
			{"id":"00000000-0000-0000-0000-000000000001","name":"sim.a","type":"integer"},
			{"id":"00000000-0000-0000-0000-000000000002","name":"real.a","type":"integer"}]}`))
	}))
	defer srv.Close()
	client := growctl.NewClient(srv.URL, srv.Client(), io.Discard)
	filter, err := growctl.NewPatternFilter("sim.*")
	if err != nil {
		t.Fatalf("NewPatternFilter: %v", err)
	}

	tags, err := client.TagsOf(apiv0.ProjectResponse{ID: uuid.New(), Name: "p"}).ListTagsMatching(context.Background(), filter)

	if err != nil || len(tags) != 1 || tags[0].Name != "sim.a" {
		t.Errorf("got %+v, %v; want only sim.a", tags, err)
	}
}

func TestNewClient_WarnsOnceAboutAnOlderServer(t *testing.T) {
	older := versiontest.Older(apiv0.SchemaVersion())
	newer := versiontest.Newer(apiv0.SchemaVersion())
	for name, tc := range map[string]struct {
		server    contract.SchemaVersion
		wantLines int
	}{
		"older": {older, 1},
		"same":  {apiv0.SchemaVersion(), 0},
		"newer": {newer, 0},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set(contract.SchemaVersionHeader, tc.server.String())
				w.Write([]byte(`{"projects":[]}`))
			}))
			defer srv.Close()
			var warnings bytes.Buffer
			client := growctl.NewClient(srv.URL, srv.Client(), &warnings)

			for range 3 {
				if _, err := client.ListProjects(context.Background()); err != nil {
					t.Fatal(err)
				}
			}

			if got := strings.Count(warnings.String(), "\n"); got != tc.wantLines {
				t.Errorf("warnings %q: want %d line(s)", warnings.String(), tc.wantLines)
			}
			if tc.wantLines > 0 && !strings.Contains(warnings.String(), tc.server.String()) {
				t.Errorf("warning %q does not name the server's version", warnings.String())
			}
		})
	}
}
