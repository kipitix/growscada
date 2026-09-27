package devicesim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/devicelink"
)

// fakeLink is an in-memory devicelink.Link recording every write.
type fakeLink struct {
	mu        sync.Mutex
	types     map[string]string // tag name → type
	failures  int               // resolve fails transiently this many times
	resolves  int
	writes    []write
	missing   bool // Write reports the tag as deleted
	writeErrs int
}

type write struct{ name, value, quality string }

func newFakeLink(types map[string]string) *fakeLink { return &fakeLink{types: types} }

func (l *fakeLink) Resolve(_ context.Context, name string) (devicelink.Tag, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.resolves++
	if l.failures > 0 {
		l.failures--
		return devicelink.Tag{}, &net503{}
	}
	tagType, ok := l.types[name]
	if !ok {
		return devicelink.Tag{}, fmt.Errorf("%w: %q", devicelink.ErrTagNotFound, name)
	}
	return devicelink.Tag{ID: uuid.New(), Name: name, Type: tagType}, nil
}

func (l *fakeLink) Write(_ context.Context, tag devicelink.Tag, value, quality string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.missing {
		l.writeErrs++
		return devicelink.ErrTagNotFound
	}
	l.writes = append(l.writes, write{tag.Name, value, quality})
	return nil
}

func (l *fakeLink) snapshot() []write {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]write(nil), l.writes...)
}

func (l *fakeLink) last(name string) (write, bool) {
	ws := l.snapshot()
	for i := len(ws) - 1; i >= 0; i-- {
		if ws[i].name == name {
			return ws[i], true
		}
	}
	return write{}, false
}

// net503 is a transient error, like an unreachable server.
type net503 struct{}

func (*net503) Error() string   { return "connection refused" }
func (*net503) Timeout() bool   { return false }
func (*net503) Temporary() bool { return true }

var quietLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func testConfig(t *testing.T, yaml string) Config {
	t.Helper()
	cfg, err := ReadConfig(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	return cfg
}

// runDevice runs a new Device over the link until the test ends, without
// waiting for it to connect.
func runDevice(t *testing.T, cfg Config, link devicelink.Link, opts Options) *Device {
	t.Helper()
	if opts.Logger == nil {
		opts.Logger = quietLogger
	}
	if opts.RetryDelay == 0 {
		opts.RetryDelay = time.Millisecond
	}
	d := New(cfg, link, opts)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	return d
}

// startDevice runs a new Device over the link until the test ends and waits
// until it is connected.
func startDevice(t *testing.T, cfg Config, link devicelink.Link) *Device {
	t.Helper()
	d := runDevice(t, cfg, link, Options{})
	eventually(t, "the device to connect", func() bool { return d.Status().State == StateConnected })
	return d
}

// eventually polls cond until it holds or the deadline passes.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

const rampConfig = `
defaultInterval: 10ms
tags:
  - tag: counter
    pattern: {kind: ramp, start: 0, rate: 1000}
`

func TestRun_WaitsForUnavailableServer(t *testing.T) {
	link := newFakeLink(map[string]string{"counter": TypeInteger})
	link.failures = 3

	d := runDevice(t, testConfig(t, rampConfig), link, Options{})

	eventually(t, "the device to connect", func() bool { return d.Status().State == StateConnected })
	link.mu.Lock()
	defer link.mu.Unlock()
	if link.resolves != 4 {
		t.Errorf("resolves: got %d, want 4", link.resolves)
	}
}

func TestRun_WaitsForServerWithoutLimit(t *testing.T) {
	link := newFakeLink(map[string]string{"counter": TypeInteger})
	link.failures = 1 << 30
	d := New(testConfig(t, rampConfig), link, Options{Logger: quietLogger, RetryDelay: time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	time.Sleep(100 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("Run gave up waiting for the server: %v", err)
	default:
	}
	if got := d.Status().State; got != StateConnecting {
		t.Errorf("state: got %q, want %q", got, StateConnecting)
	}

	cancel()
	if err := <-done; err != nil {
		t.Errorf("Run after cancel: got %v, want nil", err)
	}
}

func TestRun_WaitingForServerDoesNotRunPatternTime(t *testing.T) {
	link := newFakeLink(map[string]string{"counter": TypeInteger})
	link.failures = 10
	runDevice(t, testConfig(t, rampConfig), link, Options{RetryDelay: 20 * time.Millisecond})

	eventually(t, "a write", func() bool { return len(link.snapshot()) > 0 })
	// rate 1000/s: counting ~200ms of retries, the first value would be ~200.
	if v := atoi(t, link.snapshot()[0].value); v > 100 {
		t.Errorf("first value %d: pattern time ran while waiting for the server", v)
	}
}

func TestRun_MissingTagFailsAtOnce(t *testing.T) {
	link := newFakeLink(nil)

	err := New(testConfig(t, rampConfig), link, Options{Logger: quietLogger}).Run(context.Background())

	if !errors.Is(err, devicelink.ErrTagNotFound) || link.resolves != 1 {
		t.Errorf("got %v after %d resolves, want ErrTagNotFound at once", err, link.resolves)
	}
}

func TestRun_IncompatiblePatternFails(t *testing.T) {
	link := newFakeLink(map[string]string{"counter": TypeBoolean})

	err := New(testConfig(t, rampConfig), link, Options{Logger: quietLogger}).Run(context.Background())

	if err == nil || !strings.Contains(err.Error(), "does not apply to a boolean tag") {
		t.Errorf("got %v, want an incompatible pattern error", err)
	}
	if len(link.snapshot()) != 0 {
		t.Error("nothing must be written")
	}
}

func TestControlAPI_WhileConnecting(t *testing.T) {
	link := newFakeLink(map[string]string{"counter": TypeInteger})
	link.failures = 1 << 30
	d := runDevice(t, testConfig(t, rampConfig), link, Options{})
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	status := func(method, path, body string) (int, map[string]any) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}

	if code, out := status("GET", "/api/v1/device", ""); code != 200 || out["state"] != StateConnecting {
		t.Errorf("status: %d %v", code, out)
	}
	if code, out := status("POST", "/api/v1/device/stop", ""); code != 200 || out["running"] != false {
		t.Errorf("stop: %d %v", code, out)
	}
	if code, out := status("POST", "/api/v1/device/tags/counter/pattern", `{"kind":"constant","value":1}`); code != 503 {
		t.Errorf("pattern: %d %v, want 503", code, out)
	}
	if code, out := status("POST", "/api/v1/device/tags/counter/quality", `{"quality":"bad"}`); code != 503 {
		t.Errorf("quality: %d %v, want 503", code, out)
	}
	if code, _ := status("POST", "/api/v1/device/tags/nope/pattern", `{"kind":"constant","value":1}`); code != 404 {
		t.Errorf("unknown tag: got %d, want 404", code)
	}
}

func TestRun_WritesEveryTickWithConfiguredQuality(t *testing.T) {
	link := newFakeLink(map[string]string{"counter": TypeInteger, "mode": TypeString})
	startDevice(t, testConfig(t, rampConfig+`
  - tag: mode
    quality: uncertain
    pattern: {kind: constant, value: auto}
`), link)

	eventually(t, "several writes of each tag", func() bool {
		var counter, mode int
		for _, w := range link.snapshot() {
			switch w.name {
			case "counter":
				counter++
			case "mode":
				mode++
				if w.value != "auto" || w.quality != "uncertain" {
					t.Fatalf("mode write: got %+v", w)
				}
			}
		}
		return counter >= 3 && mode >= 3 // constant is written every tick too
	})
}

func TestStop_PausesWritesAndPatternTime(t *testing.T) {
	link := newFakeLink(map[string]string{"counter": TypeInteger})
	d := startDevice(t, testConfig(t, rampConfig), link)
	eventually(t, "a write", func() bool { return len(link.snapshot()) > 0 })

	d.Stop()
	time.Sleep(20 * time.Millisecond) // let an in-flight write land
	stoppedAt := len(link.snapshot())
	lastBefore, _ := link.last("counter")
	time.Sleep(200 * time.Millisecond)
	if n := len(link.snapshot()); n != stoppedAt {
		t.Fatalf("writes while stopped: %d → %d", stoppedAt, n)
	}
	if d.Status().Running {
		t.Error("status: want stopped")
	}

	d.Start()
	eventually(t, "writes after start", func() bool { return len(link.snapshot()) > stoppedAt })
	first := link.snapshot()[stoppedAt]
	// rate 1000/s: had pattern time kept running while stopped, the value
	// would have jumped by ~200.
	before, after := atoi(t, lastBefore.value), atoi(t, first.value)
	if after-before > 100 {
		t.Errorf("pattern time ran while stopped: %d → %d", before, after)
	}
}

func TestRun_WithoutAutostartWaitsForStart(t *testing.T) {
	link := newFakeLink(map[string]string{"counter": TypeInteger})
	d := startDevice(t, testConfig(t, "autostart: false\n"+rampConfig), link)

	time.Sleep(50 * time.Millisecond)
	if n := len(link.snapshot()); n != 0 {
		t.Fatalf("writes before start: %d", n)
	}
	d.Start()
	eventually(t, "a write after start", func() bool { return len(link.snapshot()) > 0 })
}

func TestForceQuality_StickyUntilReset(t *testing.T) {
	link := newFakeLink(map[string]string{"counter": TypeInteger})
	d := startDevice(t, testConfig(t, rampConfig), link)

	if err := d.ForceQuality("counter", "bad"); err != nil {
		t.Fatalf("ForceQuality: %v", err)
	}
	eventually(t, "bad quality written", func() bool { w, _ := link.last("counter"); return w.quality == "bad" })
	n := len(link.snapshot())
	eventually(t, "more writes", func() bool { return len(link.snapshot()) > n+3 })
	if w, _ := link.last("counter"); w.quality != "bad" {
		t.Errorf("forced quality did not stick: %+v", w)
	}

	if err := d.ResetQuality("counter"); err != nil {
		t.Fatalf("ResetQuality: %v", err)
	}
	eventually(t, "configured quality again", func() bool { w, _ := link.last("counter"); return w.quality == "good" })
}

func TestForceQuality_RepeatsRandomWalkValue(t *testing.T) {
	link := newFakeLink(map[string]string{"level": TypeInteger})
	d := startDevice(t, testConfig(t, `
defaultInterval: 1h
tags:
  - tag: level
    pattern: {kind: random_walk, start: 50, step: 10, min: 0, max: 100}
`), link)
	eventually(t, "the first write", func() bool { return len(link.snapshot()) == 1 })

	for i := range 20 {
		if err := d.ForceQuality("level", "bad"); err != nil {
			t.Fatalf("ForceQuality: %v", err)
		}
		eventually(t, "the kicked write", func() bool { return len(link.snapshot()) >= i*2+2 })
		if err := d.ResetQuality("level"); err != nil {
			t.Fatalf("ResetQuality: %v", err)
		}
		eventually(t, "the kicked write", func() bool { return len(link.snapshot()) >= i*2+3 })
	}
	for _, w := range link.snapshot() {
		if w.value != "50" {
			t.Fatalf("quality overrides moved the random walk: %+v", w)
		}
	}
}

func TestForceQuality_Rejects(t *testing.T) {
	d := startDevice(t, testConfig(t, rampConfig), newFakeLink(map[string]string{"counter": TypeInteger}))

	if err := d.ForceQuality("counter", "great"); err == nil {
		t.Error("unknown quality: want an error")
	}
	if err := d.ForceQuality("nope", "bad"); !errors.Is(err, ErrUnknownTag) {
		t.Errorf("unknown tag: got %v", err)
	}
}

func TestSetPattern_ChangesValuesOnTheFly(t *testing.T) {
	link := newFakeLink(map[string]string{"counter": TypeInteger})
	d := startDevice(t, testConfig(t, rampConfig), link)

	err := d.SetPattern("counter", PatternSpec{Kind: KindConstant, Value: scalar("-7")})
	if err != nil {
		t.Fatalf("SetPattern: %v", err)
	}
	eventually(t, "the new pattern", func() bool { w, _ := link.last("counter"); return w.value == "-7" })

	err = d.SetPattern("counter", PatternSpec{Kind: KindConstant, Value: scalar("x")})
	if err == nil || !strings.Contains(err.Error(), "is not an integer") {
		t.Errorf("invalid pattern: got %v", err)
	}
	if got := d.Status().Tags[0].Pattern.Value; got == nil || *got != "-7" {
		t.Errorf("a rejected pattern must not replace the current one, got %v", got)
	}
}

func TestRun_DeletedTagIsNoLongerWritten(t *testing.T) {
	link := newFakeLink(map[string]string{"counter": TypeInteger})
	link.missing = true
	d := startDevice(t, testConfig(t, rampConfig), link)

	eventually(t, "the tag to be lost", func() bool { return d.Status().Tags[0].Lost })
	time.Sleep(50 * time.Millisecond)
	link.mu.Lock()
	defer link.mu.Unlock()
	if link.writeErrs != 1 {
		t.Errorf("writes of a deleted tag: got %d, want 1", link.writeErrs)
	}
}

func TestControlAPI(t *testing.T) {
	link := newFakeLink(map[string]string{"counter": TypeInteger})
	d := startDevice(t, testConfig(t, rampConfig), link)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	call := func(method, path, body string) (int, map[string]any) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}

	if code, out := call("POST", "/api/v1/device/stop", ""); code != 200 || out["running"] != false {
		t.Errorf("stop: %d %v", code, out)
	}
	if code, out := call("GET", "/api/v1/device", ""); code != 200 || out["running"] != false {
		t.Errorf("status: %d %v", code, out)
	}
	if code, out := call("POST", "/api/v1/device/start", ""); code != 200 || out["running"] != true {
		t.Errorf("start: %d %v", code, out)
	}

	code, out := call("POST", "/api/v1/device/tags/counter/pattern", `{"kind":"sine","offset":5,"amplitude":1,"period":"1s"}`)
	if code != 200 {
		t.Errorf("pattern: %d %v", code, out)
	}
	if got := d.Status().Tags[0].Pattern.Kind; got != KindSine {
		t.Errorf("pattern: got kind %q", got)
	}
	if code, out := call("POST", "/api/v1/device/tags/counter/pattern", `{"kind":"step","values":["a"],"hold":"1s"}`); code != 400 || !strings.Contains(fmt.Sprint(out["detail"]), "is not an integer") {
		t.Errorf("incompatible pattern: %d %v", code, out)
	}
	if code, _ := call("POST", "/api/v1/device/tags/counter/pattern", `{"kind":"constant","value":1,"extra":1}`); code != 400 {
		t.Errorf("unknown field: got %d, want 400", code)
	}
	if code, out := call("POST", "/api/v1/device/tags/nope/pattern", `{"kind":"constant","value":1}`); code != 404 || out["status"] != float64(404) {
		t.Errorf("unknown tag: %d %v", code, out)
	}

	if code, _ := call("POST", "/api/v1/device/tags/counter/quality", `{"quality":"bad"}`); code != 200 {
		t.Errorf("force quality: got %d", code)
	}
	if !d.Status().Tags[0].Forced {
		t.Error("quality: want forced")
	}
	if code, _ := call("POST", "/api/v1/device/tags/counter/quality", `{"quality":"great"}`); code != 400 {
		t.Errorf("invalid quality: got %d, want 400", code)
	}
	if code, _ := call("DELETE", "/api/v1/device/tags/counter/quality", ""); code != 200 {
		t.Errorf("reset quality: got %d", code)
	}
	if s := d.Status().Tags[0]; s.Forced || s.Quality != "good" {
		t.Errorf("reset quality: got %+v", s)
	}
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	var v int
	if _, err := fmt.Sscan(s, &v); err != nil {
		t.Fatalf("not an integer: %q", s)
	}
	return v
}
