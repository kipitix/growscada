package devicesim

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/devicelink"
)

func f(v float64) *float64 { return &v }

func dur(d time.Duration) *Duration { return &Duration{D: d} }

func scalar(s string) *Scalar { v := Scalar(s); return &v }

func mustGen(t *testing.T, p PatternSpec, tagType devicelink.TagType) generator {
	t.Helper()
	g, err := newGenerator(p, tagType, rand.New(rand.NewPCG(1, 2)))
	if err != nil {
		t.Fatalf("newGenerator: %v", err)
	}
	return g
}

func TestGenerators_ValuesOverTime(t *testing.T) {
	tests := []struct {
		name    string
		pattern PatternSpec
		tagType devicelink.TagType
		at      []time.Duration
		want    []string
	}{
		{
			name:    "constant integer",
			pattern: PatternSpec{Kind: KindConstant, Value: scalar("42")},
			tagType: devicelink.TagTypeInteger,
			at:      []time.Duration{0, time.Hour},
			want:    []string{"42", "42"},
		},
		{
			name:    "constant string",
			pattern: PatternSpec{Kind: KindConstant, Value: scalar("auto")},
			tagType: devicelink.TagTypeString,
			at:      []time.Duration{0},
			want:    []string{"auto"},
		},
		{
			name:    "ramp grows by rate per second, rounded",
			pattern: PatternSpec{Kind: KindRamp, Start: f(10), Rate: f(2.5)},
			tagType: devicelink.TagTypeInteger,
			at:      []time.Duration{0, time.Second, 2 * time.Second, 10 * time.Second},
			want:    []string{"10", "13", "15", "35"},
		},
		{
			name:    "ramp goes back to start at max",
			pattern: PatternSpec{Kind: KindRamp, Start: f(0), Rate: f(10), Max: f(100)},
			tagType: devicelink.TagTypeInteger,
			at:      []time.Duration{5 * time.Second, 10 * time.Second, 12 * time.Second},
			want:    []string{"50", "0", "20"},
		},
		{
			name:    "ramp may fall",
			pattern: PatternSpec{Kind: KindRamp, Start: f(0), Rate: f(-1)},
			tagType: devicelink.TagTypeInteger,
			at:      []time.Duration{3 * time.Second},
			want:    []string{"-3"},
		},
		{
			name:    "sine over one period",
			pattern: PatternSpec{Kind: KindSine, Offset: f(50), Amplitude: f(20), Period: dur(4 * time.Second)},
			tagType: devicelink.TagTypeInteger,
			at:      []time.Duration{0, time.Second, 2 * time.Second, 3 * time.Second, 4 * time.Second},
			want:    []string{"50", "70", "50", "30", "50"},
		},
		{
			name:    "step cycles through values",
			pattern: PatternSpec{Kind: KindStep, Values: []Scalar{"false", "true"}, Hold: dur(5 * time.Second)},
			tagType: devicelink.TagTypeBoolean,
			at:      []time.Duration{0, 4 * time.Second, 5 * time.Second, 10 * time.Second},
			want:    []string{"false", "false", "true", "false"},
		},
		{
			name:    "step over strings",
			pattern: PatternSpec{Kind: KindStep, Values: []Scalar{"auto", "manual", "off"}, Hold: dur(time.Second)},
			tagType: devicelink.TagTypeString,
			at:      []time.Duration{0, time.Second, 2 * time.Second, 3 * time.Second},
			want:    []string{"auto", "manual", "off", "auto"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := mustGen(t, tt.pattern, tt.tagType)
			for i, at := range tt.at {
				if got := g.next(at); got != tt.want[i] {
					t.Errorf("at %v: got %q, want %q", at, got, tt.want[i])
				}
			}
		})
	}
}

func TestRandomWalk_StartsAtStartAndStaysInBounds(t *testing.T) {
	g := mustGen(t, PatternSpec{Kind: KindRandomWalk, Start: f(5), Step: f(3), Min: f(0), Max: f(10)}, devicelink.TagTypeInteger)

	if got := g.next(0); got != "5" {
		t.Fatalf("first value: got %q, want the start 5", got)
	}
	prev := 5
	changed := false
	for i := range 1000 {
		v, err := strconv.Atoi(g.next(0))
		if err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
		if v < 0 || v > 10 {
			t.Fatalf("tick %d: %d is out of [0, 10]", i, v)
		}
		// ±3 per tick, plus up to 1 from rounding on each side.
		if d := v - prev; d < -4 || d > 4 {
			t.Fatalf("tick %d: moved by %d, more than the step", i, d)
		}
		changed = changed || v != prev
		prev = v
	}
	if !changed {
		t.Error("random walk never moved")
	}
}

func TestNewGenerator_Rejects(t *testing.T) {
	tests := []struct {
		name    string
		pattern PatternSpec
		tagType devicelink.TagType
		wantErr string
	}{
		{"missing kind", PatternSpec{}, devicelink.TagTypeInteger, "kind is required"},
		{"unknown kind", PatternSpec{Kind: "square"}, devicelink.TagTypeInteger, `unknown kind "square"`},
		{"missing field", PatternSpec{Kind: KindRamp, Start: f(0)}, devicelink.TagTypeInteger, "rate is required"},
		{"foreign field", PatternSpec{Kind: KindConstant, Value: scalar("1"), Rate: f(1)}, devicelink.TagTypeInteger, "rate does not apply"},
		{"sine on boolean", PatternSpec{Kind: KindSine, Offset: f(0), Amplitude: f(1), Period: dur(time.Second)}, devicelink.TagTypeBoolean, "does not apply to a boolean tag"},
		{"ramp on string", PatternSpec{Kind: KindRamp, Start: f(0), Rate: f(1)}, devicelink.TagTypeString, "does not apply to a string tag"},
		{"non-integer constant", PatternSpec{Kind: KindConstant, Value: scalar("1.5")}, devicelink.TagTypeInteger, "is not an integer"},
		{"non-boolean step", PatternSpec{Kind: KindStep, Values: []Scalar{"true", "yes"}, Hold: dur(time.Second)}, devicelink.TagTypeBoolean, `values[1]: "yes" is not a boolean`},
		{"empty step", PatternSpec{Kind: KindStep, Values: []Scalar{}, Hold: dur(time.Second)}, devicelink.TagTypeString, "values must not be empty"},
		{"zero hold", PatternSpec{Kind: KindStep, Values: []Scalar{"a"}, Hold: dur(0)}, devicelink.TagTypeString, "hold must be positive"},
		{"zero period", PatternSpec{Kind: KindSine, Offset: f(0), Amplitude: f(1), Period: dur(0)}, devicelink.TagTypeInteger, "period must be positive"},
		{"ramp max below start", PatternSpec{Kind: KindRamp, Start: f(10), Rate: f(1), Max: f(5)}, devicelink.TagTypeInteger, "max greater than start"},
		{"ramp max with falling rate", PatternSpec{Kind: KindRamp, Start: f(0), Rate: f(-1), Max: f(5)}, devicelink.TagTypeInteger, "rate must be positive"},
		{"walk start out of bounds", PatternSpec{Kind: KindRandomWalk, Start: f(20), Step: f(1), Min: f(0), Max: f(10)}, devicelink.TagTypeInteger, "min <= start <= max"},
		{"walk zero step", PatternSpec{Kind: KindRandomWalk, Start: f(0), Step: f(0), Min: f(0), Max: f(10)}, devicelink.TagTypeInteger, "step must be positive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newGenerator(tt.pattern, tt.tagType, rand.New(rand.NewPCG(1, 2)))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}
