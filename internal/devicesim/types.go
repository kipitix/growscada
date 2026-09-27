package devicesim

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a time.Duration written as a Go duration string ("500ms", "10s")
// in YAML and JSON.
type Duration struct {
	D time.Duration
}

func (d *Duration) parse(s string) error {
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.D = v
	return nil
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: want a duration such as 500ms or 10s", n.Line)
	}
	if err := d.parse(n.Value); err != nil {
		return fmt.Errorf("line %d: %w", n.Line, err)
	}
	return nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("want a duration string such as \"500ms\" or \"10s\"")
	}
	return d.parse(s)
}

// MarshalJSON implements json.Marshaler.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.D.String())
}

// Scalar is a tag value as written in the config: a string, number or boolean,
// kept as its text ("42", "true", "auto").
type Scalar string

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *Scalar) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: want a string, number or boolean", n.Line)
	}
	*s = Scalar(n.Value)
	return nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (s *Scalar) UnmarshalJSON(b []byte) error {
	var str string
	if err := json.Unmarshal(b, &str); err == nil {
		*s = Scalar(str)
		return nil
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch v.(type) {
	case float64, bool:
		*s = Scalar(bytes.TrimSpace(b))
		return nil
	}
	return fmt.Errorf("want a string, number or boolean")
}
