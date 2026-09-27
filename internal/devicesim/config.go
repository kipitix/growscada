package devicesim

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/kipitix/growscada/internal/devicelink"
)

// Config defaults.
const (
	DefaultServer   = "http://localhost:9090"
	DefaultControl  = ":9191"
	DefaultInterval = time.Second
)

// DefaultQuality is the quality of tags that set none.
var DefaultQuality = devicelink.QualityGood

// Config is the simulator's YAML config: one virtual Device.
type Config struct {
	// Server is the GrowSCADA REST API URL.
	Server string `yaml:"server"`
	// Control is the listen address of the control API.
	Control string `yaml:"control"`
	// Autostart starts generating right away; otherwise on POST /api/v1/device/start.
	Autostart *bool `yaml:"autostart"`
	// DefaultInterval is the write interval of tags that set none.
	DefaultInterval *Duration   `yaml:"defaultInterval"`
	Tags            []TagConfig `yaml:"tags"`
}

// TagConfig is one simulated tag.
type TagConfig struct {
	// Tag is the tag's unique name on the server.
	Tag      string             `yaml:"tag"`
	Interval *Duration          `yaml:"interval"`
	Quality  devicelink.Quality `yaml:"quality"`
	Pattern  PatternSpec        `yaml:"pattern"`
}

// ReadConfig parses and validates a config; unknown fields are errors.
// Defaults are filled in.
func ReadConfig(r io.Reader) (Config, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return Config{}, errors.New("config is empty")
		}
		return Config{}, err
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server == "" {
		c.Server = DefaultServer
	}
	if c.Control == "" {
		c.Control = DefaultControl
	}
	if c.Autostart == nil {
		autostart := true
		c.Autostart = &autostart
	}
	if c.DefaultInterval == nil {
		c.DefaultInterval = &Duration{D: DefaultInterval}
	}
	for i := range c.Tags {
		t := &c.Tags[i]
		if t.Interval == nil {
			t.Interval = c.DefaultInterval
		}
		if !t.Quality.IsValid() { // not set: an empty one fails to parse
			t.Quality = DefaultQuality
		}
	}
}

// Validate checks the config after defaults are filled in. The tag types are
// not known yet: the pattern/type compatibility is checked when the tags are
// resolved on the server.
func (c Config) Validate() error {
	if c.DefaultInterval.D <= 0 {
		return errors.New("defaultInterval must be positive")
	}
	if len(c.Tags) == 0 {
		return errors.New("tags: at least one tag is required")
	}
	seen := make(map[string]bool, len(c.Tags))
	for i, t := range c.Tags {
		if t.Tag == "" {
			return fmt.Errorf("tags[%d]: tag (the tag name) is required", i)
		}
		if seen[t.Tag] {
			return fmt.Errorf("tags[%d]: tag %q is listed twice", i, t.Tag)
		}
		seen[t.Tag] = true
		if t.Interval.D <= 0 {
			return fmt.Errorf("tag %q: interval must be positive", t.Tag)
		}
		if !t.Quality.IsValid() {
			return fmt.Errorf("tag %q: quality is required", t.Tag)
		}
		if err := t.Pattern.Validate(); err != nil {
			return fmt.Errorf("tag %q: %w", t.Tag, err)
		}
	}
	return nil
}
