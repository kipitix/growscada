package devicesim

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Pattern kinds.
const (
	KindConstant   = "constant"
	KindRamp       = "ramp"
	KindSine       = "sine"
	KindRandomWalk = "random_walk"
	KindStep       = "step"
)

// Tag types (the server's TagType strings).
const (
	TypeString  = "string"
	TypeBoolean = "boolean"
	TypeInteger = "integer"
)

// kindsByType is the compatibility matrix of tag types and pattern kinds.
var kindsByType = map[string][]string{
	TypeInteger: {KindConstant, KindRamp, KindSine, KindRandomWalk, KindStep},
	TypeBoolean: {KindConstant, KindStep},
	TypeString:  {KindConstant, KindStep},
}

// PatternSpec is how a tag's values are generated: the `pattern` object of the
// config and the body of POST /api/v1/device/tags/{name}/pattern. Which fields apply
// depends on Kind; the others must be absent.
type PatternSpec struct {
	Kind string `yaml:"kind" json:"kind"`

	// constant
	Value *Scalar `yaml:"value,omitempty" json:"value,omitempty"`
	// ramp, random_walk
	Start *float64 `yaml:"start,omitempty" json:"start,omitempty"`
	// ramp: units per second; the value goes back to Start on reaching Max
	Rate *float64 `yaml:"rate,omitempty" json:"rate,omitempty"`
	Max  *float64 `yaml:"max,omitempty" json:"max,omitempty"`
	// sine
	Offset    *float64  `yaml:"offset,omitempty" json:"offset,omitempty"`
	Amplitude *float64  `yaml:"amplitude,omitempty" json:"amplitude,omitempty"`
	Period    *Duration `yaml:"period,omitempty" json:"period,omitempty"`
	// random_walk: at most ±Step per tick, kept within [Min, Max]
	Step *float64 `yaml:"step,omitempty" json:"step,omitempty"`
	Min  *float64 `yaml:"min,omitempty" json:"min,omitempty"`
	// step: Values in turn, each held for Hold
	Values []Scalar  `yaml:"values,omitempty" json:"values,omitempty"`
	Hold   *Duration `yaml:"hold,omitempty" json:"hold,omitempty"`
}

// field is one PatternSpec field, for checking which ones a kind uses.
type field struct {
	name string
	set  bool
}

func (p PatternSpec) fields() []field {
	return []field{
		{"value", p.Value != nil},
		{"start", p.Start != nil},
		{"rate", p.Rate != nil},
		{"max", p.Max != nil},
		{"offset", p.Offset != nil},
		{"amplitude", p.Amplitude != nil},
		{"period", p.Period != nil},
		{"step", p.Step != nil},
		{"min", p.Min != nil},
		{"values", p.Values != nil},
		{"hold", p.Hold != nil},
	}
}

// Validate checks the spec on its own, without knowing the tag type.
func (p PatternSpec) Validate() error {
	var required, optional []string
	switch p.Kind {
	case KindConstant:
		required = []string{"value"}
	case KindRamp:
		required, optional = []string{"start", "rate"}, []string{"max"}
	case KindSine:
		required = []string{"offset", "amplitude", "period"}
	case KindRandomWalk:
		required = []string{"start", "step", "min", "max"}
	case KindStep:
		required = []string{"values", "hold"}
	case "":
		return errors.New("pattern: kind is required")
	default:
		return fmt.Errorf("pattern: unknown kind %q (want %s, %s, %s, %s or %s)",
			p.Kind, KindConstant, KindRamp, KindSine, KindRandomWalk, KindStep)
	}
	for _, f := range p.fields() {
		switch {
		case slices.Contains(required, f.name) && !f.set:
			return fmt.Errorf("pattern %s: %s is required", p.Kind, f.name)
		case f.set && !slices.Contains(required, f.name) && !slices.Contains(optional, f.name):
			return fmt.Errorf("pattern %s: %s does not apply", p.Kind, f.name)
		}
	}

	switch p.Kind {
	case KindRamp:
		if p.Max != nil && (*p.Rate <= 0 || *p.Max <= *p.Start) {
			return fmt.Errorf("pattern %s: with max, rate must be positive and max greater than start", p.Kind)
		}
	case KindSine:
		if p.Period.D <= 0 {
			return fmt.Errorf("pattern %s: period must be positive", p.Kind)
		}
	case KindRandomWalk:
		if *p.Step <= 0 {
			return fmt.Errorf("pattern %s: step must be positive", p.Kind)
		}
		if *p.Min > *p.Start || *p.Start > *p.Max {
			return fmt.Errorf("pattern %s: want min <= start <= max", p.Kind)
		}
	case KindStep:
		if len(p.Values) == 0 {
			return fmt.Errorf("pattern %s: values must not be empty", p.Kind)
		}
		if p.Hold.D <= 0 {
			return fmt.Errorf("pattern %s: hold must be positive", p.Kind)
		}
	}
	return nil
}

// generator produces a tag's successive values, already formatted for its
// type. elapsed is the running time since the pattern was set; next is called
// once per tick.
type generator interface {
	next(elapsed time.Duration) string
}

// newGenerator builds the generator for a tag of the given type, checking the
// compatibility matrix and the values against the type.
func newGenerator(p PatternSpec, tagType string, rnd *rand.Rand) (generator, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	kinds, ok := kindsByType[tagType]
	if !ok {
		return nil, fmt.Errorf("unsupported tag type %q", tagType)
	}
	if !slices.Contains(kinds, p.Kind) {
		return nil, fmt.Errorf("pattern %s does not apply to a %s tag (want %s)", p.Kind, tagType, strings.Join(kinds, ", "))
	}

	switch p.Kind {
	case KindConstant:
		v, err := formatScalar(*p.Value, tagType)
		if err != nil {
			return nil, fmt.Errorf("pattern %s: value: %w", p.Kind, err)
		}
		return constantGen(v), nil
	case KindStep:
		values := make([]string, len(p.Values))
		for i, s := range p.Values {
			v, err := formatScalar(s, tagType)
			if err != nil {
				return nil, fmt.Errorf("pattern %s: values[%d]: %w", p.Kind, i, err)
			}
			values[i] = v
		}
		return stepGen{values: values, hold: p.Hold.D}, nil
	case KindRamp:
		g := rampGen{start: *p.Start, rate: *p.Rate}
		if p.Max != nil {
			g.max, g.wraps = *p.Max, true
		}
		return numeric(g.at), nil
	case KindSine:
		g := sineGen{offset: *p.Offset, amplitude: *p.Amplitude, period: p.Period.D}
		return numeric(g.at), nil
	default: // KindRandomWalk
		return &randomWalkGen{cur: *p.Start, step: *p.Step, min: *p.Min, max: *p.Max, rnd: rnd}, nil
	}
}

// formatScalar checks a configured value against the tag type and returns it
// in the server's form.
func formatScalar(s Scalar, tagType string) (string, error) {
	v := string(s)
	switch tagType {
	case TypeInteger:
		if _, err := strconv.ParseInt(v, 10, 64); err != nil {
			return "", fmt.Errorf("%q is not an integer", v)
		}
	case TypeBoolean:
		if v != "true" && v != "false" {
			return "", fmt.Errorf("%q is not a boolean (want true or false)", v)
		}
	}
	return v, nil
}

// formatInteger rounds a generated number to the nearest integer.
func formatInteger(v float64) string {
	return strconv.FormatInt(int64(math.Round(v)), 10)
}

type constantGen string

func (g constantGen) next(time.Duration) string { return string(g) }

type stepGen struct {
	values []string
	hold   time.Duration
}

func (g stepGen) next(elapsed time.Duration) string {
	return g.values[int(elapsed/g.hold)%len(g.values)]
}

// numeric adapts a function of time to a generator for an integer tag.
type numeric func(elapsed time.Duration) float64

func (f numeric) next(elapsed time.Duration) string { return formatInteger(f(elapsed)) }

type rampGen struct {
	start, rate, max float64
	wraps            bool
}

func (g rampGen) at(elapsed time.Duration) float64 {
	grown := g.rate * elapsed.Seconds()
	if g.wraps {
		grown = math.Mod(grown, g.max-g.start)
	}
	return g.start + grown
}

type sineGen struct {
	offset, amplitude float64
	period            time.Duration
}

func (g sineGen) at(elapsed time.Duration) float64 {
	return g.offset + g.amplitude*math.Sin(2*math.Pi*elapsed.Seconds()/g.period.Seconds())
}

// randomWalkGen starts at start and moves by at most ±step per tick.
type randomWalkGen struct {
	cur, step, min, max float64
	started             bool
	rnd                 *rand.Rand
}

func (g *randomWalkGen) next(time.Duration) string {
	if g.started {
		g.cur = min(g.max, max(g.min, g.cur+(g.rnd.Float64()*2-1)*g.step))
	}
	g.started = true
	return formatInteger(g.cur)
}
