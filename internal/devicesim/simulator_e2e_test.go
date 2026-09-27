package devicesim_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/devicelink"
	"github.com/kipitix/growscada/internal/devicesim"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
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

const e2eConfig = `
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

// simulator is a running simulator against the real REST API over Postgres.
type simulator struct {
	svc     application.TagService
	control *httptest.Server
	tags    map[string]appdto.Tag // as created
}

// startSimulator seeds the config's tags on a fresh server (as growctl apply
// would) and runs the simulator over the REST API until the test ends.
func startSimulator(t *testing.T, yaml string, tagTypes map[string]string) *simulator {
	t.Helper()
	api, svc := servertest.StartAPI(t, postgres.DB(t))
	s := &simulator{svc: svc, tags: make(map[string]appdto.Tag)}
	initial := map[string]string{"integer": "0", "boolean": "false", "string": ""}
	for name, tagType := range tagTypes {
		created, err := svc.CreateTag(context.Background(), appdto.CreateTagInput{
			Name: name, Type: tagType, Value: initial[tagType], Quality: "bad",
		})
		if err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
		s.tags[name] = created
	}

	cfg, err := devicesim.ReadConfig(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	cfg.Server = api.URL
	link := devicelink.NewRESTLink(apiclient.New(cfg.Server, api.Client()))
	device := devicesim.New(cfg, link, devicesim.Options{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	s.control = httptest.NewServer(device.Handler())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- device.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Run: %v", err)
		}
		s.control.Close()
	})
	deadline := time.Now().Add(10 * time.Second)
	for device.Status().State != devicesim.StateConnected {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the simulator to connect")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return s
}

// get reads a tag's current state from the server.
func (s *simulator) get(t *testing.T, name string) appdto.Tag {
	t.Helper()
	got, err := s.svc.FindTagByID(context.Background(), s.tags[name].ID)
	if err != nil {
		t.Fatalf("find %s: %v", name, err)
	}
	return got
}

// eventually polls the tag until cond holds or the deadline passes.
func (s *simulator) eventually(t *testing.T, name, what string, cond func(appdto.Tag) bool) appdto.Tag {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		got := s.get(t, name)
		if cond(got) {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: timed out waiting for %s; last %+v", name, what, got)
		}
		time.Sleep(10 * time.Millisecond)
	}
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

func atoi(t *testing.T, s string) int {
	t.Helper()
	v, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("not an integer: %q", s)
	}
	return v
}

var e2eTags = map[string]string{"sim.speed": "integer", "sim.running": "boolean", "sim.mode": "string"}

func TestE2E_ValuesAndVersionsFollowPatterns(t *testing.T) {
	s := startSimulator(t, e2eConfig, e2eTags)

	// ramp: the value grows with every write, and so does the version.
	first := s.eventually(t, "sim.speed", "a few writes", func(tag appdto.Tag) bool {
		return tag.Version >= s.tags["sim.speed"].Version+3
	})
	later := s.eventually(t, "sim.speed", "more writes", func(tag appdto.Tag) bool {
		return tag.Version >= first.Version+3
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

	// constant: written every tick, so its version keeps growing too.
	mode := s.eventually(t, "sim.mode", "auto", func(tag appdto.Tag) bool { return tag.Value == "auto" })
	s.eventually(t, "sim.mode", "rewrites", func(tag appdto.Tag) bool { return tag.Version >= mode.Version+2 })
}

func TestE2E_ConcurrentWriterConflictIsResolved(t *testing.T) {
	s := startSimulator(t, e2eConfig, e2eTags)
	s.eventually(t, "sim.speed", "a write", func(tag appdto.Tag) bool { return tag.Quality == "good" })

	// Another writer (an operator, a second Device) changes the tag, so the
	// simulator's next write carries a stale version and gets 409 Conflict.
	var theirs appdto.Tag
	for {
		cur := s.get(t, "sim.speed")
		var err error
		theirs, err = s.svc.SetTagValueByID(context.Background(), appdto.UpdateTagInput{
			ID: cur.ID, Value: "-1000", Quality: "bad", Version: cur.Version,
		})
		if err == nil {
			break // otherwise the simulator wrote in between: try again
		}
	}

	// The simulator rereads the version and keeps writing its own values.
	got := s.eventually(t, "sim.speed", "the simulator to take over again", func(tag appdto.Tag) bool {
		return tag.Version > theirs.Version+2 && tag.Quality == "good"
	})
	if atoi(t, got.Value) < 0 {
		t.Errorf("value: got %s, want the ramp's again", got.Value)
	}
}

func TestE2E_ControlAPI(t *testing.T) {
	s := startSimulator(t, e2eConfig, e2eTags)
	s.eventually(t, "sim.speed", "a write", func(tag appdto.Tag) bool { return tag.Quality == "good" })

	// Forced quality reaches the server and sticks until reset.
	s.call(t, "POST", "/api/v1/device/tags/sim.speed/quality", `{"quality":"bad"}`)
	bad := s.eventually(t, "sim.speed", "bad quality", func(tag appdto.Tag) bool { return tag.Quality == "bad" })
	s.eventually(t, "sim.speed", "more bad writes", func(tag appdto.Tag) bool {
		if tag.Quality != "bad" {
			t.Fatalf("forced quality did not stick: %+v", tag)
		}
		return tag.Version >= bad.Version+3
	})
	s.call(t, "DELETE", "/api/v1/device/tags/sim.speed/quality", "")
	s.eventually(t, "sim.speed", "good quality again", func(tag appdto.Tag) bool { return tag.Quality == "good" })

	// A new pattern takes effect on the fly.
	s.call(t, "POST", "/api/v1/device/tags/sim.speed/pattern", `{"kind":"constant","value":7}`)
	s.eventually(t, "sim.speed", "the constant", func(tag appdto.Tag) bool { return tag.Value == "7" })

	// Stop freezes every tag; start resumes.
	s.call(t, "POST", "/api/v1/device/stop", "")
	time.Sleep(50 * time.Millisecond) // let in-flight writes land
	frozen := s.get(t, "sim.mode")
	time.Sleep(150 * time.Millisecond)
	if got := s.get(t, "sim.mode"); got.Version != frozen.Version {
		t.Fatalf("written while stopped: version %d → %d", frozen.Version, got.Version)
	}
	s.call(t, "POST", "/api/v1/device/start", "")
	s.eventually(t, "sim.mode", "writes after start", func(tag appdto.Tag) bool { return tag.Version > frozen.Version })
}

func TestE2E_RunFailsOnMissingTag(t *testing.T) {
	api, _ := servertest.StartAPI(t, postgres.DB(t))
	cfg, err := devicesim.ReadConfig(strings.NewReader(e2eConfig))
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	link := devicelink.NewRESTLink(apiclient.New(api.URL, api.Client()))

	err = devicesim.New(cfg, link, devicesim.Options{}).Run(context.Background())

	if err == nil || !strings.Contains(err.Error(), `"sim.speed"`) {
		t.Errorf("got %v, want an error naming the missing tag", err)
	}
}
