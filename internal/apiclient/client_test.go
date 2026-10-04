package apiclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
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
	client := apiclient.New(srv.URL, srv.Client())

	found, ok, err := client.FindTagByName(context.Background(), "wanted")
	if err != nil || !ok || found.Name != "wanted" {
		t.Errorf("wanted: got %+v, %v, %v", found, ok, err)
	}

	_, ok, err = client.FindTagByName(context.Background(), "missing")
	if err != nil || ok {
		t.Errorf("missing: expected not found, got ok=%v err=%v", ok, err)
	}
}

func TestListTags_WrongServer404_ErrorNamesRequest(t *testing.T) {
	// a server URL pointing at something that is not the GrowSCADA API: the 404
	// must say which request failed, not just "not found".
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	client := apiclient.New(srv.URL, srv.Client())

	_, err := client.ListTags(context.Background())

	want := "GET " + srv.URL + "/api/v0/tags: 404 Not Found"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %v, want it to contain %q", err, want)
	}
}

func TestRequests_StateSchemaVersion(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path+" "+r.Header.Get(contract.SchemaVersionHeader))
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
		case http.MethodPatch:
			w.Write([]byte(`{"version":2}`))
		default:
			w.Write([]byte(`{"tags":[]}`))
		}
	}))
	defer srv.Close()
	client := apiclient.New(srv.URL, srv.Client())
	ctx := context.Background()

	if _, err := client.ListTags(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.CreateTag(ctx, apiv0.CreateTagRequest{Name: "t", Type: "integer", Value: "0", Quality: "good"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SetTagValue(ctx, uuid.New(), "1", "good", 1); err != nil {
		t.Fatal(err)
	}

	want := " " + apiv0.SchemaVersion().String()
	for _, request := range got {
		if !strings.HasPrefix(strings.SplitN(request, " ", 3)[1], apiv0.PathPrefix+"/") || !strings.HasSuffix(request, want) {
			t.Errorf("request %q: want path under %s and %s%s", request, apiv0.PathPrefix, contract.SchemaVersionHeader, want)
		}
	}
	if len(got) != 3 {
		t.Errorf("requests = %v, want 3", got)
	}
}

func TestResponse_UnknownFields_AreSkipped(t *testing.T) {
	// A newer server (higher MINOR) may add fields; this client must keep working.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"tags":[{"id":"00000000-0000-0000-0000-000000000001","name":"t","type":"integer",
			"value":"1","quality":"good","version":3,"unit":"rpm"}],"next_page":"x"}`))
	}))
	defer srv.Close()

	tags, err := apiclient.New(srv.URL, srv.Client()).ListTags(context.Background())
	if err != nil || len(tags) != 1 || tags[0].Version != 3 {
		t.Fatalf("got %+v, %v", tags, err)
	}
}
