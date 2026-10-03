// Package jsonschema generates the JSON schemas of the contracts from their
// Go types and checks that a MINOR only adds what old readers can ignore.
// It covers exactly the Go shapes the contracts use — structs, scalars,
// slices, maps, pointers and text-marshalled values — not reflection in
// general.
package jsonschema

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Draft is the JSON Schema dialect of every generated document.
const Draft = "https://json-schema.org/draft/2020-12/schema"

// Describer lets a type state its own schema instead of the reflected one.
type Describer interface {
	JSONSchema() map[string]any
}

// Document describes one version of a contract.
type Document struct {
	// ID is the document's $id.
	ID string
	// Title names the contract and version.
	Title string
	// Tag is the struct tag that names the fields: "json" or "yaml".
	Tag string
	// Roots are values of the top-level types; every named struct type
	// reachable from them becomes a $defs entry.
	Roots []any
}

// Generate renders the document as indented JSON with sorted keys, so the
// output is stable and diffs well.
//
// A field is required unless it is a pointer or has omitempty; the struct
// tag schema:"required" or schema:"optional" overrides that.
func Generate(doc Document) ([]byte, error) {
	g := generator{tag: doc.Tag, defs: map[string]any{}, types: map[string]reflect.Type{}}
	for _, root := range doc.Roots {
		if _, err := g.schemaOf(reflect.TypeOf(root)); err != nil {
			return nil, err
		}
	}
	out := map[string]any{
		"$schema": Draft,
		"$id":     doc.ID,
		"title":   doc.Title,
		"$defs":   g.defs,
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type generator struct {
	tag   string
	defs  map[string]any
	types map[string]reflect.Type
}

var (
	describerType     = reflect.TypeFor[Describer]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
	uuidType          = reflect.TypeFor[uuid.UUID]()
	timeType          = reflect.TypeFor[time.Time]()
)

func (g *generator) schemaOf(t reflect.Type) (map[string]any, error) {
	switch {
	case t.Implements(describerType):
		return reflect.Zero(t).Interface().(Describer).JSONSchema(), nil
	case t == uuidType:
		return map[string]any{"type": "string", "format": "uuid"}, nil
	case t == timeType:
		return map[string]any{"type": "string", "format": "date-time"}, nil
	case t.Implements(textMarshalerType):
		return map[string]any{"type": "string"}, nil
	}

	switch t.Kind() {
	case reflect.Pointer:
		return g.schemaOf(t.Elem())
	case reflect.String:
		return map[string]any{"type": "string"}, nil
	case reflect.Bool:
		return map[string]any{"type": "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer"}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer", "minimum": 0}, nil
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}, nil
	case reflect.Interface:
		return map[string]any{}, nil
	case reflect.Slice, reflect.Array:
		items, err := g.schemaOf(t.Elem())
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "array", "items": items}, nil
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("%v: map keys must be strings", t)
		}
		values, err := g.schemaOf(t.Elem())
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "object", "additionalProperties": values}, nil
	case reflect.Struct:
		return g.structRef(t)
	}
	return nil, fmt.Errorf("%v: unsupported kind %v", t, t.Kind())
}

// structRef inlines an anonymous struct and turns a named one into a $ref to
// its $defs entry, generating the entry on first use.
func (g *generator) structRef(t reflect.Type) (map[string]any, error) {
	if t.Name() == "" {
		return g.structSchema(t)
	}
	name := t.Name()
	ref := map[string]any{"$ref": "#/$defs/" + name}
	if seen, ok := g.types[name]; ok {
		if seen != t {
			return nil, fmt.Errorf("two types are named %s: %v and %v", name, seen, t)
		}
		return ref, nil
	}
	g.types[name] = t // before recursing, so recursive types terminate
	schema, err := g.structSchema(t)
	if err != nil {
		return nil, err
	}
	g.defs[name] = schema
	return ref, nil
}

func (g *generator) structSchema(t reflect.Type) (map[string]any, error) {
	properties := map[string]any{}
	required := []string{}
	if err := g.addFields(t, properties, &required); err != nil {
		return nil, err
	}
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema, nil
}

// addFields collects t's fields, flattening embedded structs (json) and
// ",inline" ones (yaml) into the same object.
func (g *generator) addFields(t reflect.Type, properties map[string]any, required *[]string) error {
	for i := range t.NumField() {
		field := t.Field(i)
		name, opts, tagged := g.fieldName(field)
		if name == "-" || (!field.IsExported() && !field.Anonymous) {
			continue
		}
		inline := opts["inline"] || (field.Anonymous && !tagged)
		if inline {
			ft := field.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() != reflect.Struct {
				return fmt.Errorf("%v.%s: only structs can be inlined", t, field.Name)
			}
			if err := g.addFields(ft, properties, required); err != nil {
				return err
			}
			continue
		}
		if !field.IsExported() {
			continue
		}
		if _, dup := properties[name]; dup {
			return fmt.Errorf("%v: two fields are named %q", t, name)
		}
		schema, err := g.schemaOf(field.Type)
		if err != nil {
			return fmt.Errorf("%v.%s: %w", t, field.Name, err)
		}
		properties[name] = schema
		if g.isRequired(field, opts) {
			*required = append(*required, name)
		}
	}
	return nil
}

func (g *generator) fieldName(field reflect.StructField) (string, map[string]bool, bool) {
	tag, tagged := field.Tag.Lookup(g.tag)
	name, rest, _ := strings.Cut(tag, ",")
	opts := map[string]bool{}
	for opt := range strings.SplitSeq(rest, ",") {
		if opt != "" {
			opts[opt] = true
		}
	}
	if name == "" {
		name = field.Name
		if g.tag == "yaml" {
			name = strings.ToLower(name) // yaml.v3's default field name
		}
	}
	return name, opts, tagged && tag != ""
}

func (g *generator) isRequired(field reflect.StructField, opts map[string]bool) bool {
	switch field.Tag.Get("schema") {
	case "required":
		return true
	case "optional":
		return false
	}
	return field.Type.Kind() != reflect.Pointer && !opts["omitempty"]
}
