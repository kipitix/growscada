package apiclient_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
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

// Only a request with a body states the SchemaVersion: the server reads it
// only when decoding a body, and on a GET from a browser the custom header
// would cost a CORS preflight.
func TestRequests_StateSchemaVersionOnlyWithBody(t *testing.T) {
	got := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, apiv0.PathPrefix+"/") {
			t.Errorf("%s %s: want a path under %s", r.Method, r.URL.Path, apiv0.PathPrefix)
		}
		got[r.Method] = r.Header.Get(contract.SchemaVersionHeader)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"00000000-0000-0000-0000-000000000001"}`))
		case http.MethodPatch:
			w.Write([]byte(`{"version":2}`))
		case http.MethodDelete:
			w.Write([]byte(`{}`))
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
	if _, err := client.CreateTag(ctx, apiv0.CreateTagRequest{Name: "t", Type: "integer", Value: "0", Quality: "good"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SetTagValue(ctx, uuid.New(), "1", "good", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DeleteTag(ctx, uuid.New()); err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		http.MethodGet:    "",
		http.MethodDelete: "",
		http.MethodPost:   apiv0.SchemaVersion().String(),
		http.MethodPatch:  apiv0.SchemaVersion().String(),
	}
	for method, v := range want {
		if got[method] != v {
			t.Errorf("%s: %s = %q, want %q", method, contract.SchemaVersionHeader, got[method], v)
		}
	}
}

func TestStatusError_CarriesTheProblem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		// "errors" is a member this client does not know: skipped.
		w.Write([]byte(`{"type":"https://api.growscada.com/errors/conflict","title":"Conflict: Scene",
			"status":409,"detail":"Scene was changed concurrently; reload it and retry",
			"instance":"/api/v0/scenes/x","errors":[{"field":"name"}]}`))
	}))
	defer srv.Close()

	_, err := apiclient.New(srv.URL, srv.Client()).UpdateScene(context.Background(), uuid.New(), apiv0.UpdateSceneRequest{})

	var se *apiclient.StatusError
	if !errors.As(err, &se) {
		t.Fatalf("error %v is not a *StatusError", err)
	}
	want := apiv0.Problem{
		Type:     "https://api.growscada.com/errors/conflict",
		Title:    "Conflict: Scene",
		Status:   http.StatusConflict,
		Detail:   "Scene was changed concurrently; reload it and retry",
		Instance: "/api/v0/scenes/x",
	}
	if se.Problem != want || se.Method != http.MethodPut {
		t.Errorf("got %s %+v, want PUT %+v", se.Method, se.Problem, want)
	}
	if !errors.Is(err, apiclient.ErrConflict) || errors.Is(err, apiclient.ErrNotFound) {
		t.Errorf("errors.Is: want ErrConflict only")
	}
	if !strings.HasSuffix(err.Error(), ": 409 Conflict: "+want.Detail) || !strings.HasPrefix(err.Error(), "PUT "+srv.URL) {
		t.Errorf("message %q: want the request, the status and the detail", err)
	}
}

func TestStatusError_NotProblemDetails_TitledByStatus(t *testing.T) {
	page := "<html><body><h1>502 Bad Gateway</h1>" + strings.Repeat("x", 500) + "</body></html>"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(page))
	}))
	defer srv.Close()

	_, err := apiclient.New(srv.URL, srv.Client()).ListScenes(context.Background())

	var se *apiclient.StatusError
	if !errors.As(err, &se) {
		t.Fatalf("error %v is not a *StatusError", err)
	}
	if se.Problem.Title != "Bad Gateway" || se.Problem.Status != http.StatusBadGateway || se.Method != http.MethodGet {
		t.Errorf("got %s %+v, want GET 502 Bad Gateway", se.Method, se.Problem)
	}
	if !strings.HasPrefix(se.Problem.Detail, "<html><body><h1>502 Bad Gateway</h1>") || len(se.Problem.Detail) > 210 {
		t.Errorf("detail %q: want the start of the body", se.Problem.Detail)
	}
}

func TestStatusError_JSONNotProblemDetails_KeepsTheBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"message":"rate limited"}`))
	}))
	defer srv.Close()

	_, err := apiclient.New(srv.URL, srv.Client()).ListScenes(context.Background())

	var se *apiclient.StatusError
	if !errors.As(err, &se) {
		t.Fatalf("error %v is not a *StatusError", err)
	}
	if se.Problem.Title != "Too Many Requests" || se.Problem.Detail != `{"message":"rate limited"}` {
		t.Errorf("got %+v, want the body as the detail", se.Problem)
	}
}

func TestDelete_EmptyBody_Succeeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			return // 200 without a body, as a proxy may answer
		}
		w.WriteHeader(http.StatusOK) // a GET without a body is not a valid answer
	}))
	defer srv.Close()
	client := apiclient.New(srv.URL, srv.Client())

	if _, err := client.DeleteTag(context.Background(), uuid.New()); err != nil {
		t.Errorf("delete: %v", err)
	}
	if _, err := client.GetTag(context.Background(), uuid.New()); err == nil {
		t.Error("get with an empty body: want an error")
	}
}

func TestDelete_BodyNotJSON_Succeeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK")) // 200 with a proxy's text
	}))
	defer srv.Close()
	client := apiclient.New(srv.URL, srv.Client())

	if _, err := client.DeleteTag(context.Background(), uuid.New()); err != nil {
		t.Errorf("delete: %v", err)
	}
}

func TestRequest_CannotBeBuilt_IsNoNetworkError(t *testing.T) {
	client := apiclient.New("http://bad host:port", http.DefaultClient)

	_, err := client.ListTags(context.Background())

	var ue *url.Error
	if err == nil || errors.As(err, &ue) {
		t.Errorf("got %v, want an error that is not a *url.Error: nothing was sent", err)
	}
}

func TestOnServerVersion_SeveralAreAllCalled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contract.SchemaVersionHeader, "0.7")
		w.Write([]byte(`{"tags":[]}`))
	}))
	defer srv.Close()
	var calls []string
	client := apiclient.New(srv.URL, srv.Client(),
		apiclient.OnServerVersion(func(contract.SchemaVersion) { calls = append(calls, "first") }),
		apiclient.OnServerVersion(func(contract.SchemaVersion) { calls = append(calls, "second") }))

	client.ListTags(context.Background())

	if want := []string{"first", "second"}; !slices.Equal(calls, want) {
		t.Errorf("got %v, want %v", calls, want)
	}
}

func TestOnServerVersion(t *testing.T) {
	for name, tc := range map[string]struct {
		header string
		status int
		want   []contract.SchemaVersion
	}{
		"stated":                {"0.7", http.StatusOK, []contract.SchemaVersion{{Major: 0, Minor: 7}}},
		"stated on an error":    {"0.7", http.StatusNotFound, []contract.SchemaVersion{{Major: 0, Minor: 7}}},
		"missing":               {"", http.StatusOK, nil},
		"does not parse":        {"zero.seven", http.StatusOK, nil},
		"MAJOR without a MINOR": {"1", http.StatusOK, nil},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.header != "" {
					w.Header().Set(contract.SchemaVersionHeader, tc.header)
				}
				w.WriteHeader(tc.status)
				w.Write([]byte(`{"tags":[]}`))
			}))
			defer srv.Close()
			var got []contract.SchemaVersion
			client := apiclient.New(srv.URL, srv.Client(),
				apiclient.OnServerVersion(func(v contract.SchemaVersion) { got = append(got, v) }))

			client.ListTags(context.Background())

			if !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
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
