package growctl_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kipitix/growscada/internal/growctl"
)

func TestFindTagByName_ServerIgnoringNameFilter_MatchesByName(t *testing.T) {
	// A server that predates ?name= returns every tag.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"tags":[
			{"id":"00000000-0000-0000-0000-000000000001","name":"other","type":"integer"},
			{"id":"00000000-0000-0000-0000-000000000002","name":"wanted","type":"boolean"}]}`))
	}))
	defer srv.Close()
	client := growctl.NewClient(srv.URL, srv.Client())

	found, ok, err := client.FindTagByName(context.Background(), "wanted")
	if err != nil || !ok || found.Name != "wanted" {
		t.Errorf("wanted: got %+v, %v, %v", found, ok, err)
	}

	_, ok, err = client.FindTagByName(context.Background(), "missing")
	if err != nil || ok {
		t.Errorf("missing: expected not found, got ok=%v err=%v", ok, err)
	}
}

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
	client := growctl.NewClient(srv.URL, srv.Client())
	filter, err := growctl.NewPatternFilter("sim.*")
	if err != nil {
		t.Fatalf("NewPatternFilter: %v", err)
	}

	tags, err := client.ListTagsMatching(context.Background(), filter)

	if err != nil || len(tags) != 1 || tags[0].Name != "sim.a" {
		t.Errorf("got %+v, %v; want only sim.a", tags, err)
	}
}
