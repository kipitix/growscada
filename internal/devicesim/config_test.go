package devicesim

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/devicelink"
)

func TestReadConfig_FillsDefaults(t *testing.T) {
	cfg, err := ReadConfig(strings.NewReader(`
token: gsc_dev_test
environment: bench
tags:
  - tag: pump.speed
    pattern:
      kind: constant
      value: 5
`))
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	if cfg.Server != DefaultServer || cfg.Control != DefaultControl || !*cfg.Autostart || cfg.DefaultInterval.D != DefaultInterval {
		t.Errorf("device defaults: got %+v", cfg)
	}
	tag := cfg.Tags[0]
	if tag.Interval.D != DefaultInterval || tag.Quality != DefaultQuality || *tag.Pattern.Value != "5" {
		t.Errorf("tag defaults: got %+v", tag)
	}
}

func TestReadConfig_FullExample(t *testing.T) {
	cfg, err := ReadConfig(strings.NewReader(`
server: http://scada:9090
token: gsc_dev_test
environment: bench
control: 127.0.0.1:9999
autostart: false
defaultInterval: 2s
tags:
  - tag: a
    interval: 500ms
    quality: uncertain
    pattern: {kind: sine, offset: 50, amplitude: 20, period: 10s}
  - tag: b
    pattern: {kind: step, values: [false, true], hold: 5s}
`))
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	if cfg.Server != "http://scada:9090" || cfg.Token != "gsc_dev_test" || cfg.Environment != "bench" || cfg.Control != "127.0.0.1:9999" || *cfg.Autostart {
		t.Errorf("device: got %+v", cfg)
	}
	if a := cfg.Tags[0]; a.Interval.D != 500*time.Millisecond || a.Quality != devicelink.QualityUncertain || a.Pattern.Period.D != 10*time.Second {
		t.Errorf("tag a: got %+v", a)
	}
	if b := cfg.Tags[1]; b.Interval.D != 2*time.Second || len(b.Pattern.Values) != 2 || b.Pattern.Values[0] != "false" {
		t.Errorf("tag b: got %+v", b)
	}
}

func TestReadConfig_Rejects(t *testing.T) {
	tests := []struct {
		name, yaml, wantErr string
	}{
		{"empty", "", "config is empty"},
		{"no token", "environment: bench\ntags: [{tag: a, pattern: {kind: constant, value: 1}}]", "token (the Device Token) is required"},
		{"no environment", "token: gsc_dev_t\ntags: [{tag: a, pattern: {kind: constant, value: 1}}]", "environment (the Environment name or ID) is required"},
		{"unknown field", "token: gsc_dev_t\nenvironment: bench\ntagz: []", "field tagz not found"},
		{"unknown pattern field", "token: gsc_dev_t\nenvironment: bench\ntags: [{tag: a, pattern: {kind: constant, value: 1, colour: red}}]", "field colour not found"},
		{"no tags", "token: gsc_dev_t\nenvironment: bench\nserver: http://x", "at least one tag"},
		{"no tag name", "token: gsc_dev_t\nenvironment: bench\ntags: [{pattern: {kind: constant, value: 1}}]", "tag (the tag name) is required"},
		{"duplicate tag", "token: gsc_dev_t\nenvironment: bench\ntags: [{tag: a, pattern: {kind: constant, value: 1}}, {tag: a, pattern: {kind: constant, value: 2}}]", `tag "a" is listed twice`},
		{"bad quality", "token: gsc_dev_t\nenvironment: bench\ntags: [{tag: a, quality: great, pattern: {kind: constant, value: 1}}]", `unknown quality "great"`},
		{"bad interval", "token: gsc_dev_t\nenvironment: bench\ntags: [{tag: a, interval: soon, pattern: {kind: constant, value: 1}}]", "invalid duration"},
		{"zero interval", "token: gsc_dev_t\nenvironment: bench\ntags: [{tag: a, interval: 0s, pattern: {kind: constant, value: 1}}]", "interval must be positive"},
		{"bad pattern", "token: gsc_dev_t\nenvironment: bench\ntags: [{tag: a, pattern: {kind: ramp}}]", `tag "a": pattern ramp: start is required`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ReadConfig(strings.NewReader(tt.yaml))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestPatternSpec_JSONAcceptsNumbersBooleansAndDurations(t *testing.T) {
	var p PatternSpec
	err := json.Unmarshal([]byte(`{"kind":"step","values":[1,true,"x"],"hold":"250ms"}`), &p)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(p.Values) != 3 || p.Values[0] != "1" || p.Values[1] != "true" || p.Values[2] != "x" || p.Hold.D != 250*time.Millisecond {
		t.Errorf("got %+v", p)
	}
}
