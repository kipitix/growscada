package jsonschema

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type base struct {
	ID uuid.UUID `json:"id"`
}

type shape struct {
	base
	Name    string         `json:"name"`
	Note    string         `json:"note,omitempty"`
	Points  []point        `json:"points"`
	Center  *point         `json:"center"`
	Tags    map[string]int `json:"tags"`
	Forced  *int           `json:"forced" schema:"required"`
	Ignored string         `json:"-"`
	hidden  string         //nolint:unused
	Meta    struct {
		Label string `json:"label"`
	} `json:"meta"`
}

type yamlHeader struct {
	APIVersion string `yaml:"apiVersion"`
}

type yamlDoc struct {
	yamlHeader `yaml:",inline"`
	Spec       struct {
		Type *string `yaml:"type" schema:"required"`
	} `yaml:"spec"`
}

func generate(t *testing.T, tag string, roots ...any) map[string]any {
	t.Helper()
	data, err := Generate(Document{ID: "id", Title: "title", Tag: tag, Roots: roots})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func def(t *testing.T, doc map[string]any, name string) map[string]any {
	t.Helper()
	d, ok := doc["$defs"].(map[string]any)[name].(map[string]any)
	if !ok {
		t.Fatalf("no $defs/%s in %v", name, doc["$defs"])
	}
	return d
}

func TestGenerateJSON(t *testing.T) {
	doc := generate(t, "json", shape{})
	s := def(t, doc, "shape")
	props := s["properties"].(map[string]any)

	for _, name := range []string{"id", "name", "note", "points", "center", "tags", "forced", "meta"} {
		if _, ok := props[name]; !ok {
			t.Errorf("property %q missing", name)
		}
	}
	for _, name := range []string{"Ignored", "-", "hidden", "base"} {
		if _, ok := props[name]; ok {
			t.Errorf("property %q must not be generated", name)
		}
	}
	if got := props["id"].(map[string]any)["format"]; got != "uuid" {
		t.Errorf("id format = %v, want uuid", got)
	}
	if got := props["points"].(map[string]any)["items"].(map[string]any)["$ref"]; got != "#/$defs/point" {
		t.Errorf("points items = %v", got)
	}
	if got := props["center"].(map[string]any)["$ref"]; got != "#/$defs/point" {
		t.Errorf("center = %v", got)
	}
	if _, ok := props["meta"].(map[string]any)["properties"]; !ok {
		t.Errorf("anonymous struct must be inlined: %v", props["meta"])
	}
	def(t, doc, "point")

	required := strings.Join(stringList(s["required"]), ",")
	if required != "id,name,points,tags,forced,meta" {
		t.Errorf("required = %s", required)
	}
}

func TestGenerateYAMLInline(t *testing.T) {
	doc := generate(t, "yaml", yamlDoc{})
	s := def(t, doc, "yamlDoc")
	props := s["properties"].(map[string]any)
	if _, ok := props["apiVersion"]; !ok {
		t.Errorf("inline header not flattened: %v", props)
	}
	spec := props["spec"].(map[string]any)
	if got := strings.Join(stringList(spec["required"]), ","); got != "type" {
		t.Errorf("spec required = %s", got)
	}
}

func TestGenerateIsStable(t *testing.T) {
	a, err := Generate(Document{Tag: "json", Roots: []any{shape{}}})
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		b, _ := Generate(Document{Tag: "json", Roots: []any{shape{}}})
		if string(a) != string(b) {
			t.Fatal("output differs between runs")
		}
	}
}

func TestGenerateRejectsNameClash(t *testing.T) {
	type point struct{ Z int }
	if _, err := Generate(Document{Tag: "json", Roots: []any{shape{}, point{}}}); err == nil {
		t.Fatal("two types named point: want error")
	}
}

func schemaOf(t *testing.T, s string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(s), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestMinorIncompatibilities(t *testing.T) {
	const older = `{"$defs": {
		"Tag": {"type": "object", "required": ["id"], "properties": {
			"id": {"type": "string"},
			"tags": {"type": "array", "items": {"type": "object", "properties": {"n": {"type": "integer"}}}}
		}}
	}}`
	tests := []struct {
		name   string
		newer  string
		broken bool
	}{
		{"identical", older, false},
		{"optional property added", `{"$defs": {"Tag": {"type": "object", "required": ["id"], "properties": {
			"id": {"type": "string"}, "unit": {"type": "string"},
			"tags": {"type": "array", "items": {"type": "object", "properties": {"n": {"type": "integer"}}}}}}}}`, false},
		{"type added", `{"$defs": {"Tag": {"type": "object", "required": ["id"], "properties": {
			"id": {"type": "string"},
			"tags": {"type": "array", "items": {"type": "object", "properties": {"n": {"type": "integer"}}}}}},
			"Scene": {"type": "object"}}}`, false},
		{"required property added", `{"$defs": {"Tag": {"type": "object", "required": ["id", "unit"], "properties": {
			"id": {"type": "string"}, "unit": {"type": "string"},
			"tags": {"type": "array", "items": {"type": "object", "properties": {"n": {"type": "integer"}}}}}}}}`, true},
		{"property removed", `{"$defs": {"Tag": {"type": "object", "required": ["id"], "properties": {
			"id": {"type": "string"}}}}}`, true},
		{"property type changed", `{"$defs": {"Tag": {"type": "object", "required": ["id"], "properties": {
			"id": {"type": "integer"},
			"tags": {"type": "array", "items": {"type": "object", "properties": {"n": {"type": "integer"}}}}}}}}`, true},
		{"nested property removed", `{"$defs": {"Tag": {"type": "object", "required": ["id"], "properties": {
			"id": {"type": "string"},
			"tags": {"type": "array", "items": {"type": "object", "properties": {}}}}}}}`, true},
		{"type removed", `{"$defs": {}}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			problems := MinorIncompatibilities(schemaOf(t, older), schemaOf(t, tt.newer))
			if (len(problems) > 0) != tt.broken {
				t.Fatalf("problems = %v, want broken = %v", problems, tt.broken)
			}
		})
	}
}
