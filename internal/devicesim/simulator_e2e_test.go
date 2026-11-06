package devicesim_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/devicelink"
	"github.com/kipitix/growscada/internal/devicesim"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/servertest"
)

// The Postgres container starts only when an end-to-end test runs, so the unit
// tests of this package do not need Docker.
var postgres servertest.Postgres

func TestMain(m *testing.M) {
	code := m.Run()
	postgres.Close()
	os.Exit(code)
}

// e2eConfig is the simulator's config; the token and the environment name
// are replaced by the seeded Device's.
const e2eConfig = `
token: gsc_dev_placeholder
environment: placeholder
defaultInterval: 20ms
tags:
  - tag: sim.speed
    pattern: {kind: ramp, start: 0, rate: 100}
  - tag: sim.running
    quality: uncertain
    pattern: {kind: step, values: [false, true], hold: 60ms}
  - tag: sim.mode
    pattern: {kind: constant, value: auto}
`

// simulator is a running Device Gateway against the real REST API over
// Postgres, acting for one seeded Device.
type simulator struct {
	svc     application.TagService
	api     *apiclient.Client // of the same server, for requests of others
	control *httptest.Server
	tags    map[string]appdto.Tag // as created; the Device supplies them
	device  servertest.Device
}

// startSimulator seeds the config's tags on a fresh server (as growctl apply
// would), creates the Device that supplies them and its Environment and runs
// the simulator with the Device's token until the test ends.
func startSimulator(t *testing.T, yaml string, tagTypes map[string]string) *simulator {
	t.Helper()
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	s := &simulator{svc: svc, api: apiclient.New(srv.URL, srv.Client()), tags: make(map[string]appdto.Tag)}
	tagIDs := make([]uuid.UUID, 0, len(tagTypes))
	for name, tagType := range tagTypes {
		created, err := svc.CreateTag(editlock.System(context.Background()), servertest.MainProject, appdto.CreateTagInput{Version: servertest.DraftVersion(t, postgres.DB(t), servertest.MainProject),
			Name: name, Type: tagType,
		})
		if err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
		s.tags[name] = created
		tagIDs = append(tagIDs, created.ID)
	}
	s.device = servertest.NewDevice(t, srv, postgres.DB(t), "sim", tagIDs...)
	servertest.DeployProject(t, postgres.DB(t), servertest.MainProject)

	cfg, err := devicesim.ReadConfig(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	cfg.Server = srv.URL
	cfg.Token = s.device.Token
	cfg.Environment = s.device.EnvironmentName

	client := apiclient.New(cfg.Server, srv.Client()).WithDeviceToken(cfg.Token)
	link := devicelink.NewRESTLink(client, cfg.Token)
	gateway := devicesim.NewGateway(client, cfg.Environment)
	device := devicesim.New(cfg, link, gateway, devicesim.Options{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	s.control = httptest.NewServer(device.Handler())
	t.Cleanup(s.control.Close)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- device.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	eventually(t, "the simulator to connect", func() bool {
		return device.Status().State == devicesim.StateConnected
	})
	return s
}

// get reads a tag's current state from the server.
func (s *simulator) get(t *testing.T, name string) appdto.Tag {
	t.Helper()
	got, err := s.svc.FindTagByID(context.Background(), servertest.MainProject, s.tags[name].ID)
	if err != nil {
		t.Fatalf("find %s: %v", name, err)
	}
	return got
}

// eventually polls the tag until cond holds or the deadline passes.
func (s *simulator) eventually(t *testing.T, name, what string, cond func(appdto.Tag) bool) appdto.Tag {
	t.Helper()
	var got appdto.Tag
	eventually(t, what, func() bool {
		got = s.get(t, name)
		return cond(got)
	})
	return got
}

// call calls the control API and fails the test unless it answers 200.
func (s *simulator) call(t *testing.T, method, path, body string) {
	t.Helper()
	req, err := http.NewRequest(method, s.control.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s: %s %s", method, path, resp.Status, b)
	}
}

// eventually polls cond until it holds or the deadline passes.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	v, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("not an integer: %q", s)
	}
	return v
}

var e2eTags = map[string]string{"sim.speed": "integer", "sim.running": "boolean", "sim.mode": "string"}

// written reports whether tag's state was written at least n times since
// since's: each write takes a seq above the last, though not always the next
// one (other tags' writes take their own).
func written(tag, since appdto.Tag, n int64) bool {
	return tag.Seq >= since.Seq+n
}

func TestE2E_ValuesAndSeqsFollowPatterns(t *testing.T) {
	s := startSimulator(t, e2eConfig, e2eTags)

	// ramp: the value grows with every write, and so does the seq.
	first := s.eventually(t, "sim.speed", "a few writes", func(tag appdto.Tag) bool {
		return written(tag, s.tags["sim.speed"], 3)
	})
	later := s.eventually(t, "sim.speed", "more writes", func(tag appdto.Tag) bool {
		return written(tag, first, 3)
	})
	if atoi(t, later.Value) <= atoi(t, first.Value) {
		t.Errorf("ramp did not grow: %s → %s", first.Value, later.Value)
	}
	if later.Quality != "good" {
		t.Errorf("speed quality: got %q, want the default good", later.Quality)
	}

	// step: both values show up, with the configured quality.
	s.eventually(t, "sim.running", "true", func(tag appdto.Tag) bool { return tag.Value == "true" })
	got := s.eventually(t, "sim.running", "false again", func(tag appdto.Tag) bool { return tag.Value == "false" })
	if got.Quality != "uncertain" {
		t.Errorf("running quality: got %q, want uncertain", got.Quality)
	}

	// constant: written every tick without a change, so the Journal keeps the
	// position of the first write and only the write moment moves (ADR 0019).
	mode := s.eventually(t, "sim.mode", "auto", func(tag appdto.Tag) bool { return tag.Value == "auto" })
	if mode.WrittenAt == nil {
		t.Error("the constant pattern has no written_at")
	}
	time.Sleep(200 * time.Millisecond)
	if again := s.get(t, "sim.mode"); again.Seq != mode.Seq {
		t.Errorf("a repeat of the constant value moved the seq: %d → %d", mode.Seq, again.Seq)
	}
}

func TestE2E_ControlAPI(t *testing.T) {
	s := startSimulator(t, e2eConfig, e2eTags)
	s.eventually(t, "sim.speed", "a write", func(tag appdto.Tag) bool { return tag.Quality == "good" })

	// An overridden quality reaches the server and sticks until reset.
	s.call(t, "POST", "/control/device/tags/sim.speed/quality", `{"quality":"bad"}`)
	bad := s.eventually(t, "sim.speed", "bad quality", func(tag appdto.Tag) bool { return tag.Quality == "bad" })
	s.eventually(t, "sim.speed", "more bad writes", func(tag appdto.Tag) bool {
		if tag.Quality != "bad" {
			t.Fatalf("overridden quality did not stick: %+v", tag)
		}
		return written(tag, bad, 3)
	})
	s.call(t, "DELETE", "/control/device/tags/sim.speed/quality", "")
	s.eventually(t, "sim.speed", "good quality again", func(tag appdto.Tag) bool { return tag.Quality == "good" })

	// A new pattern takes effect on the fly.
	s.call(t, "POST", "/control/device/tags/sim.speed/pattern", `{"kind":"constant","value":7}`)
	s.eventually(t, "sim.speed", "the constant", func(tag appdto.Tag) bool { return tag.Value == "7" })

	// A ramp again: its writes change the value, so a resumed Device is
	// visible as new Journal positions (a constant pattern would move only
	// written_at, ADR 0019).
	s.call(t, "POST", "/control/device/tags/sim.speed/pattern", `{"kind":"ramp","start":0,"rate":100}`)
	switched := s.get(t, "sim.speed")
	s.eventually(t, "sim.speed", "the ramp again", func(tag appdto.Tag) bool { return written(tag, switched, 1) })

	// Stop freezes every tag (it answers once the writes in flight are done);
	// start resumes. The frozen check reads the ramp: a constant pattern
	// writes the same value and Quality, so its seq would stay put whether
	// the Device is stopped or not (ADR 0019).
	s.call(t, "POST", "/control/device/stop", "")
	frozen := s.get(t, "sim.speed")
	time.Sleep(150 * time.Millisecond)
	if got := s.get(t, "sim.speed"); got.Seq != frozen.Seq {
		t.Fatalf("written while stopped: seq %d → %d", frozen.Seq, got.Seq)
	}
	s.call(t, "POST", "/control/device/start", "")
	s.eventually(t, "sim.speed", "writes after start", func(tag appdto.Tag) bool { return written(tag, frozen, 1) })
}

// TestE2E_AnotherWriter_TheLatestWriteWins: a person writes the Tag of a
// Device — refused, the Device is its supplier — while another writer's write
// to a Tag of its own does not disturb the patterns.
func TestE2E_AnotherWriter_CannotWriteTheDevicesTag(t *testing.T) {
	s := startSimulator(t, e2eConfig, e2eTags)
	s.eventually(t, "sim.speed", "a write", func(tag appdto.Tag) bool { return tag.Quality == "good" })

	_, err := s.api.Project(servertest.MainProject).SetTagValue(context.Background(), s.tags["sim.speed"].ID, "-1000", "bad")

	if err == nil {
		t.Errorf("a person wrote a Device's Tag: want a refusal, got %+v", s.get(t, "sim.speed"))
	}
	if got := s.get(t, "sim.speed"); atoi(t, got.Value) < 0 {
		t.Errorf("value: got %+v, want the ramp's", got)
	}
}

// TestE2E_MissingTagIsAConfigError: a local config names a Tag the Device does
// not supply; the Gateway cannot run (ExitConfig of the command).
func TestE2E_MissingTagIsAConfigError(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	cfg := simulatorConfig(t, srv.URL, servertest.NewDevice(t, srv, postgres.DB(t), "sim"))
	servertest.DeployProject(t, postgres.DB(t), servertest.MainProject)
	// The Device supplies no Tag at all.
	client := apiclient.New(srv.URL, srv.Client()).WithDeviceToken(cfg.Token)
	link := devicelink.NewRESTLink(client, cfg.Token)

	err := devicesim.New(cfg, link, devicesim.NewGateway(client, cfg.Environment), devicesim.Options{}).Run(context.Background())

	if !errors.Is(err, devicesim.ErrConfig) || !strings.Contains(err.Error(), `"sim.speed"`) {
		t.Errorf("got %v, want ErrConfig naming the missing tag", err)
	}
}

// TestE2E_UnknownTokenIsNoPermission: a token the server does not know stops
// the Gateway (ExitNoPerm of the command).
func TestE2E_UnknownTokenIsNoPermission(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	cfg, err := devicesim.ReadConfig(strings.NewReader(e2eConfig))
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	cfg.Server = srv.URL
	cfg.Token = "gsc_dev_0000000000000000000000000000000000000000000000000000000000000000"
	cfg.Environment = "nowhere"
	client := apiclient.New(srv.URL, srv.Client()).WithDeviceToken(cfg.Token)

	err = devicesim.New(cfg, devicelink.NewRESTLink(client, cfg.Token),
		devicesim.NewGateway(client, cfg.Environment), devicesim.Options{}).Run(context.Background())

	if !devicelink.IsNoPermission(err) {
		t.Errorf("got %v, want no permission", err)
	}
}

// TestE2E_RevokedTokenStopsTheGateway: revocation has no event, so the Gateway
// learns of it on its next write (401): it stops rather than retrying.
func TestE2E_RevokedTokenStopsTheGateway(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	created, err := svc.CreateTag(editlock.System(context.Background()), servertest.MainProject, appdto.CreateTagInput{Version: servertest.DraftVersion(t, postgres.DB(t), servertest.MainProject),
		Name: "sim.speed", Type: "integer",
	})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	device := servertest.NewDevice(t, srv, postgres.DB(t), "sim", created.ID)
	servertest.DeployProject(t, postgres.DB(t), servertest.MainProject)
	cfg := simulatorConfigOf(t, singleTagConfig, srv.URL, device)
	client := apiclient.New(srv.URL, srv.Client()).WithDeviceToken(cfg.Token)
	d := devicesim.New(cfg, devicelink.NewRESTLink(client, cfg.Token),
		devicesim.NewGateway(client, cfg.Environment),
		devicesim.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), RetryDelay: 10 * time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	eventually(t, "the simulator to connect", func() bool { return d.Status().State == devicesim.StateConnected })

	revokeToken(t, srv, cfg.Token)

	select {
	case err := <-done:
		if !devicelink.IsNoPermission(err) {
			t.Errorf("Run after revocation: got %v, want no permission", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the gateway kept running with a revoked token")
	}
}

// singleTagConfig is the simulator's config of one Tag, for the tests that
// seed a Device with one Tag only.
const singleTagConfig = `
token: gsc_dev_placeholder
environment: placeholder
defaultInterval: 20ms
tags:
  - tag: sim.speed
    pattern: {kind: ramp, start: 0, rate: 100}
`

// simulatorConfig is the e2e config for a Device, pointed at srv.
func simulatorConfig(t *testing.T, serverURL string, device servertest.Device) devicesim.Config {
	t.Helper()
	return simulatorConfigOf(t, e2eConfig, serverURL, device)
}

// simulatorConfigOf is simulatorConfig over the given YAML.
func simulatorConfigOf(t *testing.T, yaml, serverURL string, device servertest.Device) devicesim.Config {
	t.Helper()
	cfg, err := devicesim.ReadConfig(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	cfg.Server = serverURL
	cfg.Token = device.Token
	cfg.Environment = device.EnvironmentName
	return cfg
}

// revokeToken revokes the token of the Device by naming it in a delete of the
// token route: the seeded Device of the test is the only one with a token.
func revokeToken(t *testing.T, srv *httptest.Server, token string) {
	t.Helper()
	var identity apiv0.DeviceIdentityResponse
	req, _ := http.NewRequest(http.MethodGet, srv.URL+apiv0.PathPrefix+"/device", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&identity); err != nil {
		t.Fatalf("identity: %v", err)
	}
	req, _ = http.NewRequest(http.MethodDelete,
		srv.URL+apiv0.PathPrefix+apiv0.ProjectPath(identity.ProjectID.String())+"/devices/"+identity.DeviceID.String()+"/token", nil)
	resp2, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("revoke: %s", resp2.Status)
	}
}
