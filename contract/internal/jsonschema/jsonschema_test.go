package jsonschema

import (
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"

	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
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

type described struct{ v int }

func (described) JSONSchema() map[string]any {
	return map[string]any{"type": "string", "pattern": "^x$"}
}

type pointers struct {
	Described *described `json:"described"`
	ID        *uuid.UUID `json:"id"`
	At        *time.Time `json:"at"`
	Text      *textValue `json:"text"`
}

type textValue struct{ s string }

func (*textValue) MarshalText() ([]byte, error) { return nil, nil }

// A pointer field has the schema of its element, whatever the element's
// special handling, and never calls a method on a nil pointer.
func TestGeneratePointers(t *testing.T) {
	props := def(t, generate(t, "json", pointers{}), "pointers")["properties"].(map[string]any)
	if got := props["described"].(map[string]any)["pattern"]; got != "^x$" {
		t.Errorf("described = %v", props["described"])
	}
	if got := props["id"].(map[string]any)["format"]; got != "uuid" {
		t.Errorf("id = %v", props["id"])
	}
	if got := props["at"].(map[string]any)["format"]; got != "date-time" {
		t.Errorf("at = %v", props["at"])
	}
	if got := props["text"].(map[string]any)["type"]; got != "string" {
		t.Errorf("text = %v", props["text"])
	}
}

type EmbeddedPart struct {
	Inner string `json:"inner" yaml:"inner"`
}

type NamedPart struct {
	Other string `json:"other" yaml:"other"`
}

// Each field exercises a flattening rule on which encoding/json and yaml.v3
// differ: the schema must follow the encoder it is generated for.
type embedding struct {
	EmbeddedPart `json:",omitempty"` // json: no name, flattened; yaml: nested under "embeddedpart"
	*NamedPart   `json:"named" yaml:",inline"`
	Dash         string `json:"-," yaml:"-"` // json: a field named "-"; yaml: skipped
}

// keysOf returns the top-level keys the encoder writes for v.
func keysOf(t *testing.T, tag string, v any) []string {
	t.Helper()
	var data []byte
	var err error
	m := map[string]any{}
	if tag == "yaml" {
		if data, err = yaml.Marshal(v); err == nil {
			err = yaml.Unmarshal(data, &m)
		}
	} else {
		if data, err = json.Marshal(v); err == nil {
			err = json.Unmarshal(data, &m)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	return slices.Sorted(maps.Keys(m))
}

// encoding/json writes an embedded unexported struct named by its tag.
type unexportedNamed struct {
	*namedHidden `json:"hidden"`
}

type namedHidden struct {
	X int `json:"x"`
}

func TestGenerateJSONEmbeddedUnexportedNamed(t *testing.T) {
	props := def(t, generate(t, "json", unexportedNamed{}), "unexportedNamed")["properties"].(map[string]any)
	got := slices.Sorted(maps.Keys(props))
	if enc := keysOf(t, "json", unexportedNamed{&namedHidden{}}); !slices.Equal(got, enc) {
		t.Errorf("properties = %v, the encoder writes %v", got, enc)
	}
}

func TestGenerateFlattensAsTheEncoder(t *testing.T) {
	v := embedding{NamedPart: &NamedPart{}}
	for _, tc := range []struct {
		tag  string
		want []string
	}{
		{"json", []string{"-", "inner", "named"}}, // "named" holds NamedPart: tagged, not flattened
		{"yaml", []string{"embeddedpart", "other"}},
	} {
		props := def(t, generate(t, tc.tag, embedding{}), "embedding")["properties"].(map[string]any)
		got := slices.Sorted(maps.Keys(props))
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: properties = %v, want %v", tc.tag, got, tc.want)
		}
		if enc := keysOf(t, tc.tag, v); !slices.Equal(got, enc) {
			t.Errorf("%s: properties = %v, the encoder writes %v", tc.tag, got, enc)
		}
	}
}

type options struct {
	Zero    int               `json:"zero,omitzero"`
	Count   int               `json:"count,string"`
	Size    uint              `json:"size,string"`
	On      bool              `json:"on,string"`
	Ratio   *float64          `json:"ratio,string"`
	Label   string            `json:"label,string"`
	Points  []point           `json:"points,string"` // ignored on a slice
	Plain   int               `json:"plain"`
	Labels  map[string]string `json:"labels,omitzero"`
	Missing int               `json:"missing,omitempty,string"`
}

func TestGenerateJSONOptions(t *testing.T) {
	s := def(t, generate(t, "json", options{}), "options")
	props := s["properties"].(map[string]any)

	if got := strings.Join(stringList(s["required"]), ","); got != "count,size,on,label,points,plain" {
		t.Errorf("required = %s", got)
	}
	for name, want := range map[string]string{
		"count": "string", "size": "string", "on": "string", "ratio": "string", "label": "string",
		"points": "array", "plain": "integer", "missing": "string",
	} {
		if got := props[name].(map[string]any)["type"]; got != want {
			t.Errorf("%s type = %v, want %s", name, got, want)
		}
	}

	// The encoder's own output must satisfy the patterns of quoted numbers.
	data, _ := json.Marshal(options{Count: -42, Size: 7})
	var enc map[string]any
	if err := json.Unmarshal(data, &enc); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"count", "size"} {
		pattern := props[name].(map[string]any)["pattern"].(string)
		if !regexp.MustCompile(pattern).MatchString(enc[name].(string)) {
			t.Errorf("%s: encoder wrote %q, not matching %s", name, enc[name], pattern)
		}
	}
	if _, ok := enc["zero"]; ok {
		t.Error("encoder wrote zero despite omitzero: the test no longer checks what it claims")
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
