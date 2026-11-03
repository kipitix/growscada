package devicesim

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
)

const gatewayProjectID = "b0000000-0000-4000-8000-000000000001"

// fakeDeviceAPI is the Device API of the server as a Gateway sees it: what the
// token acts for, the Project's Environments, and the configuration of the
// Device in one of them.
type fakeDeviceAPI struct {
	mu           sync.Mutex
	environments []apiv0.EnvironmentResponse
	pairs        map[uuid.UUID]bool         // the Environments the Device stands in
	renameOnMiss *apiv0.EnvironmentResponse // applied when a configuration 404s
	identity     int
	lists        int
	configs      []string // the Environment IDs the configuration was asked for
}

func newFakeDeviceAPI(environments ...apiv0.EnvironmentResponse) *fakeDeviceAPI {
	pairs := make(map[uuid.UUID]bool, len(environments))
	for _, environment := range environments {
		pairs[environment.ID] = true
	}
	return &fakeDeviceAPI{environments: environments, pairs: pairs}
}

func (f *fakeDeviceAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case apiv0.PathPrefix + "/device":
		f.identity++
		_ = json.NewEncoder(w).Encode(apiv0.DeviceIdentityResponse{
			ProjectID: uuid.MustParse(gatewayProjectID), DeviceID: uuid.New(),
		})
	case apiv0.PathPrefix + apiv0.ProjectPath(gatewayProjectID) + "/environments":
		f.lists++
		_ = json.NewEncoder(w).Encode(apiv0.GetEnvironmentsResponse{Environments: f.environments})
	case apiv0.PathPrefix + "/device/config":
		id := r.URL.Query().Get(apiv0.DeviceConfigQueryEnvironment)
		f.configs = append(f.configs, id)
		if !f.pairs[uuid.MustParse(id)] {
			if f.renameOnMiss != nil {
				f.environments = []apiv0.EnvironmentResponse{*f.renameOnMiss}
				f.pairs = map[uuid.UUID]bool{f.renameOnMiss.ID: true}
				f.renameOnMiss = nil
			}
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(apiv0.Problem{Status: http.StatusNotFound, Title: "not found"})
			return
		}
		_ = json.NewEncoder(w).Encode(apiv0.DeviceConfigResponse{
			Device:      apiv0.DeviceConfigDeviceResponse{Name: "sim", Protocol: "simulation"},
			Environment: apiv0.DeviceConfigEnvironmentResponse{ID: uuid.MustParse(id), Name: "bench"},
		})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// serve starts the API and returns a client of it.
func serve(t *testing.T, api *fakeDeviceAPI) *apiclient.Client {
	t.Helper()
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	return apiclient.New(srv.URL, srv.Client())
}

// benchEnvironment is the Environment the tests configure the Gateway with.
func benchEnvironment() apiv0.EnvironmentResponse {
	return apiv0.EnvironmentResponse{ID: uuid.MustParse("e1000000-0000-4000-8000-000000000001"), Name: "bench"}
}

// TestGateway_EnvironmentByName: a name is looked up among the Project's
// Environments once, and remembered.
func TestGateway_EnvironmentByName(t *testing.T) {
	bench := benchEnvironment()
	api := newFakeDeviceAPI(bench)
	client := serve(t, api)
	gateway := NewGateway(client, bench.Name)

	for i := range 2 {
		config, err := gateway.Configuration(context.Background())
		if err != nil {
			t.Fatalf("Configuration %d: %v", i, err)
		}
		if config.Device.Name != "sim" || config.Environment.ID != bench.ID {
			t.Errorf("configuration %d: got %+v", i, config)
		}
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.identity != 1 || api.lists != 1 {
		t.Errorf("identity and list calls: got %d and %d, want 1 and 1", api.identity, api.lists)
	}
	if len(api.configs) != 2 || api.configs[0] != bench.ID.String() || api.configs[1] != bench.ID.String() {
		t.Errorf("configuration calls: got %v, want the Environment twice", api.configs)
	}
}

// TestGateway_EnvironmentByNameNotFound: a name the Project's Environments do
// not have is a configuration error, not a request error.
func TestGateway_EnvironmentByNameNotFound(t *testing.T) {
	api := newFakeDeviceAPI(benchEnvironment())
	client := serve(t, api)

	_, err := NewGateway(client, "nowhere").Configuration(context.Background())

	if !errors.Is(err, ErrEnvironmentNotFound) {
		t.Fatalf("got %v, want ErrEnvironmentNotFound", err)
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if len(api.configs) != 0 {
		t.Errorf("configuration calls: got %v, want none", api.configs)
	}
}

// TestGateway_EnvironmentRenamed: the Environment the name resolved to was
// renamed meanwhile; the name is looked up once more and the configuration of
// the new Environment served.
func TestGateway_EnvironmentRenamed(t *testing.T) {
	old := apiv0.EnvironmentResponse{ID: uuid.MustParse("e1000000-0000-4000-8000-000000000001"), Name: "bench"}
	renamed := apiv0.EnvironmentResponse{ID: uuid.MustParse("e1000000-0000-4000-8000-000000000002"), Name: "bench"}
	api := newFakeDeviceAPI()
	api.environments = []apiv0.EnvironmentResponse{old}
	api.renameOnMiss = &renamed
	client := serve(t, api)
	gateway := NewGateway(client, "bench")

	config, err := gateway.Configuration(context.Background())
	if err != nil {
		t.Fatalf("Configuration: %v", err)
	}
	if config.Environment.ID != renamed.ID {
		t.Errorf("environment: got %s, want the renamed one %s", config.Environment.ID, renamed.ID)
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.lists != 2 {
		t.Errorf("environment lists: got %d, want 2", api.lists)
	}
	want := []string{old.ID.String(), renamed.ID.String()}
	if len(api.configs) != len(want) || api.configs[0] != want[0] || api.configs[1] != want[1] {
		t.Errorf("configuration calls: got %v, want %v", api.configs, want)
	}
}

// TestGateway_EnvironmentByID: a UUID is asked for as it is, with no lookup at
// all; a 404 of it is a configuration error like a name's.
func TestGateway_EnvironmentByID(t *testing.T) {
	bench := benchEnvironment()
	api := newFakeDeviceAPI(bench)
	client := serve(t, api)

	if _, err := NewGateway(client, bench.ID.String()).Configuration(context.Background()); err != nil {
		t.Fatalf("Configuration: %v", err)
	}
	api.mu.Lock()
	if api.identity != 0 || api.lists != 0 {
		t.Errorf("identity and list calls: got %d and %d, want none", api.identity, api.lists)
	}
	if len(api.configs) != 1 || api.configs[0] != bench.ID.String() {
		t.Errorf("configuration calls: got %v, want the Environment once", api.configs)
	}
	api.mu.Unlock()

	missing := uuid.MustParse("e1000000-0000-4000-8000-0000000000ff")
	_, err := NewGateway(client, missing.String()).Configuration(context.Background())
	if !errors.Is(err, ErrEnvironmentNotFound) {
		t.Errorf("a missing Environment by ID: got %v, want ErrEnvironmentNotFound", err)
	}
}
