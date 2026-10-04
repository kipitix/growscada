package devicesim

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kipitix/growscada/internal/devicelink"
)

// PatternKind names how a tag's values are generated.
type PatternKind string

// Pattern kinds.
const (
	KindConstant   PatternKind = "constant"
	KindRamp       PatternKind = "ramp"
	KindSine       PatternKind = "sine"
	KindRandomWalk PatternKind = "random_walk"
	KindStep       PatternKind = "step"
)

// kindSpec is everything a pattern kind decides: which PatternSpec fields it
// uses, which tag types it fits, its invariants and its generator.
type kindSpec struct {
	required, optional []string
	types              []devicelink.TagType
	// check verifies the invariants once the required fields are known to be
	// set; nil if there are none.
	check func(p PatternSpec) error
	build func(p PatternSpec, tagType devicelink.TagType, rnd *rand.Rand) (generator, error)
}

var (
	anyType     = []devicelink.TagType{devicelink.TagTypeInteger, devicelink.TagTypeBoolean, devicelink.TagTypeString}
	integerOnly = []devicelink.TagType{devicelink.TagTypeInteger}
)

// kinds is the table of pattern kinds; its types form the compatibility matrix
// of tag types and pattern kinds.
var kinds = map[PatternKind]kindSpec{
	KindConstant: {
		required: []string{"value"},
		types:    anyType,
		build: func(p PatternSpec, tagType devicelink.TagType, _ *rand.Rand) (generator, error) {
			v, err := formatScalar(*p.Value, tagType)
			if err != nil {
				return nil, fmt.Errorf("value: %w", err)
			}
			return constantGen(v), nil
		},
	},
	KindRamp: {
		required: []string{"start", "rate"},
		optional: []string{"max"},
		types:    integerOnly,
		check: func(p PatternSpec) error {
			if p.Max != nil && (*p.Rate <= 0 || *p.Max <= *p.Start) {
				return errors.New("with max, rate must be positive and max greater than start")
			}
			return nil
		},
		build: func(p PatternSpec, _ devicelink.TagType, _ *rand.Rand) (generator, error) {
			g := rampGen{start: *p.Start, rate: *p.Rate}
			if p.Max != nil {
				g.max, g.wraps = *p.Max, true
			}
			return numeric(g.at), nil
		},
	},
	KindSine: {
		required: []string{"offset", "amplitude", "period"},
		types:    integerOnly,
		check: func(p PatternSpec) error {
			if p.Period.D <= 0 {
				return errors.New("period must be positive")
			}
			return nil
		},
		build: func(p PatternSpec, _ devicelink.TagType, _ *rand.Rand) (generator, error) {
			g := sineGen{offset: *p.Offset, amplitude: *p.Amplitude, period: p.Period.D}
			return numeric(g.at), nil
		},
	},
	KindRandomWalk: {
		required: []string{"start", "step", "min", "max"},
		types:    integerOnly,
		check: func(p PatternSpec) error {
			if *p.Step <= 0 {
				return errors.New("step must be positive")
			}
			if *p.Min > *p.Start || *p.Start > *p.Max {
				return errors.New("want min <= start <= max")
			}
			return nil
		},
		build: func(p PatternSpec, _ devicelink.TagType, rnd *rand.Rand) (generator, error) {
			return &randomWalkGen{cur: *p.Start, step: *p.Step, min: *p.Min, max: *p.Max, rnd: rnd}, nil
		},
	},
	KindStep: {
		required: []string{"values", "hold"},
		types:    anyType,
		check: func(p PatternSpec) error {
			if len(p.Values) == 0 {
				return errors.New("values must not be empty")
			}
			if p.Hold.D <= 0 {
				return errors.New("hold must be positive")
			}
			return nil
		},
		build: func(p PatternSpec, tagType devicelink.TagType, _ *rand.Rand) (generator, error) {
			values := make([]string, len(p.Values))
			for i, s := range p.Values {
				v, err := formatScalar(s, tagType)
				if err != nil {
					return nil, fmt.Errorf("values[%d]: %w", i, err)
				}
				values[i] = v
			}
			return stepGen{values: values, hold: p.Hold.D}, nil
		},
	},
}

// kindNames lists the kinds that fit the filter, sorted, for error messages.
func kindNames(fits func(kindSpec) bool) string {
	var names []string
	for _, k := range slices.Sorted(maps.Keys(kinds)) {
		if fits(kinds[k]) {
			names = append(names, string(k))
		}
	}
	return strings.Join(names, ", ")
}

// PatternSpec is how a tag's values are generated: the `pattern` object of the
// config and the body of POST /control/device/tags/{name}/pattern. Which fields apply
// depends on Kind; the others must be absent.
type PatternSpec struct {
	Kind PatternKind `yaml:"kind" json:"kind"`

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
	if p.Kind == "" {
		return errors.New("pattern: kind is required")
	}
	spec, ok := kinds[p.Kind]
	if !ok {
		return fmt.Errorf("pattern: unknown kind %q (want %s)", p.Kind, kindNames(func(kindSpec) bool { return true }))
	}
	for _, f := range p.fields() {
		switch {
		case slices.Contains(spec.required, f.name) && !f.set:
			return fmt.Errorf("pattern %s: %s is required", p.Kind, f.name)
		case f.set && !slices.Contains(spec.required, f.name) && !slices.Contains(spec.optional, f.name):
			return fmt.Errorf("pattern %s: %s does not apply", p.Kind, f.name)
		}
	}
	if spec.check != nil {
		if err := spec.check(p); err != nil {
			return fmt.Errorf("pattern %s: %w", p.Kind, err)
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
func newGenerator(p PatternSpec, tagType devicelink.TagType, rnd *rand.Rand) (generator, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	spec := kinds[p.Kind]
	if !slices.Contains(spec.types, tagType) {
		return nil, fmt.Errorf("pattern %s does not apply to a %s tag (want %s)", p.Kind, tagType,
			kindNames(func(k kindSpec) bool { return slices.Contains(k.types, tagType) }))
	}
	g, err := spec.build(p, tagType, rnd)
	if err != nil {
		return nil, fmt.Errorf("pattern %s: %w", p.Kind, err)
	}
	return g, nil
}

// formatScalar checks a configured value against the tag type and returns it
// in the server's form. An integer tag takes only an integer literal: 42.5 is
// more likely a typo than a wish to round, which applies only to generated
// numbers.
func formatScalar(s Scalar, tagType devicelink.TagType) (string, error) {
	v := string(s)
	switch tagType {
	case devicelink.TagTypeInteger:
		if _, err := strconv.ParseInt(v, 10, 64); err != nil {
			return "", fmt.Errorf("%q is not an integer", v)
		}
	case devicelink.TagTypeBoolean:
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
