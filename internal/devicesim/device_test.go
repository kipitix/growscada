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

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/devicelink"
)

// forbidden is the server's refusal of a request the Device may not make.
func forbidden() error {
	return &apiclient.StatusError{Method: http.MethodPost, Problem: apiv0.Problem{Status: http.StatusForbidden}}
}

// fakeLink is an in-memory devicelink.Link recording every write.
type fakeLink struct {
	mu        sync.Mutex
	writes    []write
	missing   bool // Write reports the tag as deleted
	forbidden bool // Write reports the tag as one the Device may not write
	writeErrs int
	// block, if set, holds every Write until it is closed.
	block chan struct{}
}

type write struct{ name, value, quality string }

func newFakeLink() *fakeLink { return &fakeLink{} }

// fakeDevice is a link and a source of the given tag types.
func fakeDevice(types map[string]devicelink.TagType) (*fakeLink, *fakeSource) {
	return newFakeLink(), newFakeSource(types)
}

// fakeSource is an in-memory Source: the Device's configuration on the server.
type fakeSource struct {
	mu         sync.Mutex
	tags       map[string]devicelink.TagType
	ids        map[string]uuid.UUID
	failures   int // Configuration fails transiently this many times
	refusals   int // Configuration is refused (403) this many times
	configs    int
	deviceName string
	notify     func(apiv0.Event) // set once Events is followed
	eventsOnce sync.Once
	started    chan struct{} // closed when Events is first followed
}

func newFakeSource(tags map[string]devicelink.TagType) *fakeSource {
	return &fakeSource{tags: tags, ids: map[string]uuid.UUID{}, deviceName: "sim", started: make(chan struct{})}
}

func (s *fakeSource) Configuration(context.Context) (apiv0.DeviceConfigResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configs++
	if s.failures > 0 {
		s.failures--
		return apiv0.DeviceConfigResponse{}, &net503{}
	}
	if s.refusals > 0 {
		s.refusals--
		return apiv0.DeviceConfigResponse{}, forbidden()
	}
	config := apiv0.DeviceConfigResponse{
		Device:      apiv0.DeviceConfigDeviceResponse{Name: s.deviceName, Protocol: "simulation"},
		Environment: apiv0.DeviceConfigEnvironmentResponse{Name: "bench"},
	}
	for name, tagType := range s.tags {
		id, ok := s.ids[name]
		if !ok {
			id = uuid.New()
			s.ids[name] = id
		}
		config.Tags = append(config.Tags, apiv0.DeviceConfigTagResponse{ID: id, Name: name, Type: tagType.String()})
	}
	return config, nil
}

func (s *fakeSource) Events(ctx context.Context, onChange func(apiv0.Event)) error {
	s.mu.Lock()
	s.notify = onChange
	s.mu.Unlock()
	s.eventsOnce.Do(func() { close(s.started) })
	<-ctx.Done()
	return ctx.Err()
}

// change replaces the server's configuration and tells the Device, as a
// configuration event would.
func (s *fakeSource) change(tags map[string]devicelink.TagType, deviceName string) {
	s.mu.Lock()
	s.tags, s.deviceName = tags, deviceName
	notify := s.notify
	s.mu.Unlock()
	if notify != nil {
		notify(apiv0.Event{})
	}
}

// fail makes the next Configuration call fail transiently, as an
// unreachable server would.
func (s *fakeSource) fail(times int) {
	s.mu.Lock()
	s.failures = times
	s.mu.Unlock()
}

// refuse makes the next Configuration calls refused (403), as a server does
// when the Device is not in the configuration the Gateway asks about.
func (s *fakeSource) refuse(times int) {
	s.mu.Lock()
	s.refusals = times
	s.mu.Unlock()
}

func (s *fakeSource) snapshot() (configs int, tags map[string]devicelink.TagType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tags = make(map[string]devicelink.TagType, len(s.tags))
	for name, t := range s.tags {
		tags[name] = t
	}
	return s.configs, tags
}

// Resolve is the Link's contract; the Device reads its Tags from the
// configuration, so nothing calls it here.
func (l *fakeLink) Resolve(context.Context, string) (devicelink.Tag, error) {
	return devicelink.Tag{}, devicelink.ErrTagNotFound
}

func (l *fakeLink) Write(_ context.Context, tag devicelink.Tag, value string, quality devicelink.Quality) error {
	if l.block != nil {
		<-l.block
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.missing {
		l.writeErrs++
		return devicelink.ErrTagNotFound
	}
	if l.forbidden {
		l.writeErrs++
		return forbidden()
	}
	l.writes = append(l.writes, write{tag.Name, value, quality.String()})
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

// testConfig reads a config of a Device Gateway.
func testConfig(t *testing.T, yaml string) Config {
	t.Helper()
	cfg, err := ReadConfig(strings.NewReader("token: gsc_dev_test\nenvironment: bench\n" + yaml))
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	return cfg
}

// runDevice runs a new Device over the link and source until the test ends,
// without waiting for it to connect.
func runDevice(t *testing.T, cfg Config, link devicelink.Link, source Source, opts Options) *Device {
	t.Helper()
	if opts.Logger == nil {
		opts.Logger = quietLogger
	}
	if opts.RetryDelay == 0 {
		opts.RetryDelay = time.Millisecond
	}
	d := New(cfg, link, source, opts)
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

// startDevice runs a new Device over the link and source until the test ends
// and waits until it is connected.
func startDevice(t *testing.T, cfg Config, link devicelink.Link, source Source) *Device {
	t.Helper()
	d := runDevice(t, cfg, link, source, Options{})
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
	link, source := fakeDevice(map[string]devicelink.TagType{"counter": devicelink.TagTypeInteger})
	source.fail(3)

	d := runDevice(t, testConfig(t, rampConfig), link, source, Options{})

	eventually(t, "the device to connect", func() bool { return d.Status().State == StateConnected })
	if configs, _ := source.snapshot(); configs != 4 {
		t.Errorf("configuration reads: got %d, want 4", configs)
	}
}

func TestRun_WaitsForServerWithoutLimit(t *testing.T) {
	link, source := fakeDevice(map[string]devicelink.TagType{"counter": devicelink.TagTypeInteger})
	source.fail(1 << 30)
	d := New(testConfig(t, rampConfig), link, source, Options{Logger: quietLogger, RetryDelay: time.Millisecond})
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
	link, source := fakeDevice(map[string]devicelink.TagType{"counter": devicelink.TagTypeInteger})
	source.fail(10)
	runDevice(t, testConfig(t, rampConfig), link, source, Options{RetryDelay: 20 * time.Millisecond})

	eventually(t, "a write", func() bool { return len(link.snapshot()) > 0 })
	// rate 1000/s: counting ~200ms of retries, the first value would be ~200.
	if v := atoi(t, link.snapshot()[0].value); v > 100 {
		t.Errorf("first value %d: pattern time ran while waiting for the server", v)
	}
}

func TestRun_MissingTagFailsAtOnce(t *testing.T) {
	link, source := fakeDevice(nil)

	err := New(testConfig(t, rampConfig), link, source, Options{Logger: quietLogger}).Run(context.Background())

	if !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), `"counter"`) {
		t.Errorf("got %v, want a configuration error naming the missing tag", err)
	}
	if len(link.snapshot()) != 0 {
		t.Error("nothing must be written")
	}
}

func TestRun_IncompatiblePatternFails(t *testing.T) {
	link, source := fakeDevice(map[string]devicelink.TagType{"counter": devicelink.TagTypeBoolean})

	err := New(testConfig(t, rampConfig), link, source, Options{Logger: quietLogger}).Run(context.Background())

	if err == nil || !strings.Contains(err.Error(), "does not apply to a boolean tag") {
		t.Errorf("got %v, want an incompatible pattern error", err)
	}
	if len(link.snapshot()) != 0 {
		t.Error("nothing must be written")
	}
}

// waitEvents waits until the Device follows the configuration event stream.
func waitEvents(t *testing.T, source *fakeSource) {
	t.Helper()
	select {
	case <-source.started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the event stream")
	}
}

// TestRun_ReloadsConfigurationOnEvent: a configuration event makes the Device
// reread its configuration; a tag the Device no longer supplies stops being
// written, and one it supplies again resumes.
func TestRun_ReloadsConfigurationOnEvent(t *testing.T) {
	link, source := fakeDevice(map[string]devicelink.TagType{"counter": devicelink.TagTypeInteger})
	d := startDevice(t, testConfig(t, rampConfig), link, source)
	waitEvents(t, source)
	eventually(t, "a write", func() bool { return len(link.snapshot()) > 0 })

	source.change(nil, "sim-renamed")

	eventually(t, "the tag to be lost", func() bool { return d.Status().Tags[0].Lost })
	if got := d.Status().DeviceName; got != "sim-renamed" {
		t.Errorf("device name: got %q, want the reloaded one", got)
	}
	n := len(link.snapshot())
	time.Sleep(50 * time.Millisecond)
	if got := len(link.snapshot()); got != n {
		t.Fatalf("writes of a tag the device no longer supplies: %d → %d", n, got)
	}

	source.change(map[string]devicelink.TagType{"counter": devicelink.TagTypeInteger}, "sim")

	eventually(t, "writes to resume", func() bool { return len(link.snapshot()) > n })
	if d.Status().Tags[0].Lost {
		t.Error("the tag supplied again must be written again")
	}
}

// TestRun_ChangedTypeKeepsTheTagStopped: a tag whose type changed to one its
// pattern does not fit stops being written, and a reload of the same
// configuration does not resume it with the generator of the old type.
func TestRun_ChangedTypeKeepsTheTagStopped(t *testing.T) {
	link, source := fakeDevice(map[string]devicelink.TagType{"counter": devicelink.TagTypeInteger})
	d := startDevice(t, testConfig(t, rampConfig), link, source)
	waitEvents(t, source)
	eventually(t, "a write", func() bool { return len(link.snapshot()) > 0 })

	// The Tag is recreated as a boolean: the ramp does not fit it any more.
	boolean := map[string]devicelink.TagType{"counter": devicelink.TagTypeBoolean}
	source.change(boolean, "sim")
	eventually(t, "the tag to be lost", func() bool { return d.Status().Tags[0].Lost })

	configs, _ := source.snapshot()
	source.change(boolean, "sim")
	eventually(t, "another configuration read", func() bool {
		c, _ := source.snapshot()
		return c > configs
	})

	time.Sleep(50 * time.Millisecond) // a resumed tag would write by now
	if !d.Status().Tags[0].Lost {
		t.Error("a tag whose pattern does not fit its type must stay lost on a reload")
	}
	if sourceType := d.Status().Tags[0].Type; sourceType != devicelink.TagTypeBoolean.String() {
		t.Errorf("tag type: got %q, want the server's %q", sourceType, devicelink.TagTypeBoolean)
	}
}

// TestRun_RereadsConfigurationOnTimer: the event stream is not the only way:
// the configuration is reread on a timer as a fallback.
func TestRun_RereadsConfigurationOnTimer(t *testing.T) {
	link, source := fakeDevice(map[string]devicelink.TagType{"counter": devicelink.TagTypeInteger})
	runDevice(t, testConfig(t, rampConfig), link, source, Options{ConfigReload: 20 * time.Millisecond})

	eventually(t, "a fallback read", func() bool {
		configs, _ := source.snapshot()
		return configs >= 2
	})
}

func TestControlAPI_WhileConnecting(t *testing.T) {
	link, source := fakeDevice(map[string]devicelink.TagType{"counter": devicelink.TagTypeInteger})
	source.fail(1 << 30)
	d := runDevice(t, testConfig(t, rampConfig), link, source, Options{})
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

	if code, out := status("GET", "/control/device", ""); code != 200 || out["state"] != StateConnecting {
		t.Errorf("status: %d %v", code, out)
	}
	if code, out := status("POST", "/control/device/stop", ""); code != 200 || out["running"] != false {
		t.Errorf("stop: %d %v", code, out)
	}
	if code, out := status("POST", "/control/device/tags/counter/pattern", `{"kind":"constant","value":1}`); code != 503 {
		t.Errorf("pattern: %d %v, want 503", code, out)
	}
	if code, out := status("POST", "/control/device/tags/counter/quality", `{"quality":"bad"}`); code != 503 {
		t.Errorf("quality: %d %v, want 503", code, out)
	}
	if code, _ := status("POST", "/control/device/tags/nope/pattern", `{"kind":"constant","value":1}`); code != 404 {
		t.Errorf("unknown tag: got %d, want 404", code)
	}
}

func TestRun_WritesEveryTickWithConfiguredQuality(t *testing.T) {
	link, source := fakeDevice(map[string]devicelink.TagType{"counter": devicelink.TagTypeInteger, "mode": devicelink.TagTypeString})
	startDevice(t, testConfig(t, rampConfig+`
  - tag: mode
    quality: uncertain
    pattern: {kind: constant, value: auto}
`), link, source)

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
	link, source := fakeDevice(map[string]devicelink.TagType{"counter": devicelink.TagTypeInteger})
	d := startDevice(t, testConfig(t, rampConfig), link, source)
	eventually(t, "a write", func() bool { return len(link.snapshot()) > 0 })

	d.Stop()
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

func TestStop_WaitsForWriteInFlight(t *testing.T) {
	link, source := fakeDevice(map[string]devicelink.TagType{"counter": devicelink.TagTypeInteger})
	link.block = make(chan struct{})
	d := startDevice(t, testConfig(t, rampConfig), link, source)
	release := sync.OnceFunc(func() { close(link.block) })
	t.Cleanup(release) // before the Device stops, should the test fail early
	eventually(t, "a write in flight", func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.inFlight > 0
	})

	stopped := make(chan struct{})
	go func() {
		d.Stop()
		close(stopped)
	}()
	eventually(t, "Stop to pause the device", func() bool { return !d.Status().Running })
	select {
	case <-stopped:
		t.Fatal("Stop returned while a write was in flight")
	default:
	}

	release()
	<-stopped
	if n := len(link.snapshot()); n != 1 {
		t.Errorf("writes: got %d, want the one in flight", n)
	}
}

func TestRun_WithoutAutostartWaitsForStart(t *testing.T) {
	link, source := fakeDevice(map[string]devicelink.TagType{"counter": devicelink.TagTypeInteger})
	d := startDevice(t, testConfig(t, "autostart: false\n"+rampConfig), link, source)

	time.Sleep(50 * time.Millisecond)
	if n := len(link.snapshot()); n != 0 {
		t.Fatalf("writes before start: %d", n)
	}
	d.Start()
	eventually(t, "a write after start", func() bool { return len(link.snapshot()) > 0 })
}

func TestOverrideQuality_StickyUntilReset(t *testing.T) {
	link, source := fakeDevice(map[string]devicelink.TagType{"counter": devicelink.TagTypeInteger})
	d := startDevice(t, testConfig(t, rampConfig), link, source)

	if err := d.OverrideQuality("counter", devicelink.QualityBad); err != nil {
		t.Fatalf("OverrideQuality: %v", err)
	}
	eventually(t, "bad quality written", func() bool { w, _ := link.last("counter"); return w.quality == "bad" })
	n := len(link.snapshot())
	eventually(t, "more writes", func() bool { return len(link.snapshot()) > n+3 })
	if w, _ := link.last("counter"); w.quality != "bad" {
		t.Errorf("overridden quality did not stick: %+v", w)
	}

	if err := d.ResetQuality("counter"); err != nil {
		t.Fatalf("ResetQuality: %v", err)
	}
	eventually(t, "configured quality again", func() bool { w, _ := link.last("counter"); return w.quality == "good" })
}

func TestOverrideQuality_RepeatsRandomWalkValue(t *testing.T) {
	link, source := fakeDevice(map[string]devicelink.TagType{"level": devicelink.TagTypeInteger})
	d := startDevice(t, testConfig(t, `
defaultInterval: 1h
tags:
  - tag: level
    pattern: {kind: random_walk, start: 50, step: 10, min: 0, max: 100}
`), link, source)
	eventually(t, "the first write", func() bool { return len(link.snapshot()) == 1 })

	for i := range 20 {
		if err := d.OverrideQuality("level", devicelink.QualityBad); err != nil {
			t.Fatalf("OverrideQuality: %v", err)
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

func TestOverrideQuality_Rejects(t *testing.T) {
	link, source := fakeDevice(map[string]devicelink.TagType{"counter": devicelink.TagTypeInteger})
	d := startDevice(t, testConfig(t, rampConfig), link, source)

	if err := d.OverrideQuality("counter", devicelink.Quality{}); err == nil {
		t.Error("zero quality: want an error")
	}
	if err := d.OverrideQuality("nope", devicelink.QualityBad); !errors.Is(err, ErrUnknownTag) {
		t.Errorf("unknown tag: got %v", err)
	}
}

func TestSetPattern_ChangesValuesOnTheFly(t *testing.T) {
	link, source := fakeDevice(map[string]devicelink.TagType{"counter": devicelink.TagTypeInteger})
	d := startDevice(t, testConfig(t, rampConfig), link, source)

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
	link, source := fakeDevice(map[string]devicelink.TagType{"counter": devicelink.TagTypeInteger})
	link.missing = true
	d := startDevice(t, testConfig(t, rampConfig), link, source)

	eventually(t, "the tag to be lost", func() bool { return d.Status().Tags[0].Lost })
	time.Sleep(50 * time.Millisecond)
	link.mu.Lock()
	defer link.mu.Unlock()
	if link.writeErrs != 1 {
		t.Errorf("writes of a deleted tag: got %d, want 1", link.writeErrs)
	}
}

// TestRun_ForbiddenWriteStopsTheTag: a write the server refuses (403: the
// Device is not the Tag's supplier, or has left the configuration) stops the
// tag instead of being retried every tick.
func TestRun_ForbiddenWriteStopsTheTag(t *testing.T) {
	link, source := fakeDevice(map[string]devicelink.TagType{"counter": devicelink.TagTypeInteger})
	link.forbidden = true
	d := startDevice(t, testConfig(t, rampConfig), link, source)

	eventually(t, "the tag to be lost", func() bool { return d.Status().Tags[0].Lost })
	time.Sleep(50 * time.Millisecond)
	link.mu.Lock()
	defer link.mu.Unlock()
	if link.writeErrs != 1 {
		t.Errorf("writes of a refused tag: got %d, want 1", link.writeErrs)
	}
}

// TestRun_ConfigRefusalIsWaitedFor: a 403 of the configuration read means the
// Device is not in the configuration the Gateway asks about. The Gateway goes
// on writing and picks the configuration up when the server serves it again.
func TestRun_ConfigRefusalIsWaitedFor(t *testing.T) {
	link, source := fakeDevice(map[string]devicelink.TagType{"counter": devicelink.TagTypeInteger})
	d := runDevice(t, testConfig(t, rampConfig), link, source, Options{ConfigReload: 20 * time.Millisecond})
	eventually(t, "a write", func() bool { return len(link.snapshot()) > 0 })

	configs, _ := source.snapshot()
	source.refuse(2)
	eventually(t, "the refused reads", func() bool {
		c, _ := source.snapshot()
		return c >= configs+2
	})

	// The refusals are behind and writes go on: Run has not ended (its cleanup
	// would report the error) and the Device is still connected.
	before := len(link.snapshot())
	time.Sleep(50 * time.Millisecond)
	if got := len(link.snapshot()); got <= before {
		t.Errorf("writes after a refused configuration: %d → %d", before, got)
	}
	if d.Status().State != StateConnected {
		t.Errorf("state after a refused configuration: got %q, want connected", d.Status().State)
	}
}

func TestControlAPI(t *testing.T) {
	link, source := fakeDevice(map[string]devicelink.TagType{"counter": devicelink.TagTypeInteger})
	d := startDevice(t, testConfig(t, rampConfig), link, source)
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

	if code, out := call("POST", "/control/device/stop", ""); code != 200 || out["running"] != false {
		t.Errorf("stop: %d %v", code, out)
	}
	if code, out := call("GET", "/control/device", ""); code != 200 || out["running"] != false {
		t.Errorf("status: %d %v", code, out)
	}
	if code, out := call("POST", "/control/device/start", ""); code != 200 || out["running"] != true {
		t.Errorf("start: %d %v", code, out)
	}

	code, out := call("POST", "/control/device/tags/counter/pattern", `{"kind":"sine","offset":5,"amplitude":1,"period":"1s"}`)
	if code != 200 {
		t.Errorf("pattern: %d %v", code, out)
	}
	if got := d.Status().Tags[0].Pattern.Kind; got != KindSine {
		t.Errorf("pattern: got kind %q", got)
	}
	if code, out := call("POST", "/control/device/tags/counter/pattern", `{"kind":"step","values":["a"],"hold":"1s"}`); code != 400 || !strings.Contains(fmt.Sprint(out["detail"]), "is not an integer") {
		t.Errorf("incompatible pattern: %d %v", code, out)
	}
	if code, _ := call("POST", "/control/device/tags/counter/pattern", `{"kind":"constant","value":1,"extra":1}`); code != 400 {
		t.Errorf("unknown field: got %d, want 400", code)
	}
	if code, out := call("POST", "/control/device/tags/nope/pattern", `{"kind":"constant","value":1}`); code != 404 || out["status"] != float64(404) {
		t.Errorf("unknown tag: %d %v", code, out)
	}

	if code, _ := call("POST", "/control/device/tags/counter/quality", `{"quality":"bad"}`); code != 200 {
		t.Errorf("override quality: got %d", code)
	}
	if !d.Status().Tags[0].Overridden {
		t.Error("quality: want overridden")
	}
	if code, _ := call("POST", "/control/device/tags/counter/quality", `{"quality":"great"}`); code != 400 {
		t.Errorf("invalid quality: got %d, want 400", code)
	}
	if code, _ := call("DELETE", "/control/device/tags/counter/quality", ""); code != 200 {
		t.Errorf("reset quality: got %d", code)
	}
	if s := d.Status().Tags[0]; s.Overridden || s.Quality != devicelink.QualityGood {
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
