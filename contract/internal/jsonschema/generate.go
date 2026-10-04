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
	// A pointer has the schema of what it points to. Dereferencing first keeps
	// the interface checks below off nil pointers: *T of a T with a value
	// method would otherwise call it on nil and panic.
	if t.Kind() == reflect.Pointer {
		return g.schemaOf(t.Elem())
	}
	pt := reflect.PointerTo(t)
	switch {
	case t.Implements(describerType) || pt.Implements(describerType):
		return reflect.New(t).Interface().(Describer).JSONSchema(), nil
	case t == uuidType:
		return map[string]any{"type": "string", "format": "uuid"}, nil
	case t == timeType:
		return map[string]any{"type": "string", "format": "date-time"}, nil
	case t.Implements(textMarshalerType) || pt.Implements(textMarshalerType):
		return map[string]any{"type": "string"}, nil
	}

	switch t.Kind() {
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

// addFields collects t's fields, flattening the ones the encoder flattens into
// the same object: for json an embedded struct whose tag gives no name, for
// yaml (yaml.v3) only a field marked ",inline".
func (g *generator) addFields(t reflect.Type, properties map[string]any, required *[]string) error {
	for i := range t.NumField() {
		field := t.Field(i)
		name, opts, named, skip := g.fieldName(field)
		// An unexported field is invisible to both encoders, except an embedded
		// struct, whose exported fields are promoted.
		if skip || (!field.IsExported() && !(field.Anonymous && isStruct(field.Type))) {
			continue
		}
		if g.inlines(field, opts, named) {
			if !isStruct(field.Type) {
				return fmt.Errorf("%v.%s: only structs can be inlined", t, field.Name)
			}
			if err := g.addFields(deref(field.Type), properties, required); err != nil {
				return err
			}
			continue
		}
		// encoding/json writes an embedded unexported struct that its tag names
		// as an ordinary field; yaml.v3 cannot encode one.
		if !field.IsExported() && g.tag == "yaml" {
			continue
		}
		if _, dup := properties[name]; dup {
			return fmt.Errorf("%v: two fields are named %q", t, name)
		}
		schema, err := g.fieldSchema(field, opts)
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

// inlines reports whether the encoder flattens field into its parent object.
// encoding/json flattens an embedded struct (or pointer to one) whose tag
// names nothing, even with options such as ",omitempty", and has no
// ",inline" option; yaml.v3 flattens only ",inline" and nests an embedded
// field without it under its name.
func (g *generator) inlines(field reflect.StructField, opts map[string]bool, named bool) bool {
	if g.tag == "yaml" {
		return opts["inline"]
	}
	return field.Anonymous && !named && isStruct(field.Type)
}

func deref(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Pointer {
		return t.Elem()
	}
	return t
}

// isStruct reports whether t is a struct or a pointer to one.
func isStruct(t reflect.Type) bool {
	return deref(t).Kind() == reflect.Struct
}

// fieldName returns the field's property name, its tag options, whether the
// tag gave the name and whether the encoder skips the field (tag "-"; "-,"
// names a field "-").
func (g *generator) fieldName(field reflect.StructField) (name string, opts map[string]bool, named, skip bool) {
	tag := field.Tag.Get(g.tag)
	if tag == "-" {
		return "", nil, false, true
	}
	name, rest, _ := strings.Cut(tag, ",")
	opts = map[string]bool{}
	for opt := range strings.SplitSeq(rest, ",") {
		if opt != "" {
			opts[opt] = true
		}
	}
	if name != "" {
		return name, opts, true, false
	}
	name = field.Name
	if g.tag == "yaml" {
		name = strings.ToLower(name) // yaml.v3's default field name
	}
	return name, opts, false, false
}

func (g *generator) isRequired(field reflect.StructField, opts map[string]bool) bool {
	switch field.Tag.Get("schema") {
	case "required":
		return true
	case "optional":
		return false
	}
	if field.Type.Kind() == reflect.Pointer || opts["omitempty"] {
		return false
	}
	// encoding/json leaves out a zero value with ",omitzero" (yaml.v3 has no
	// such option).
	return g.tag != "json" || !opts["omitzero"]
}

// fieldSchema is the schema of field's value as the encoder writes it.
func (g *generator) fieldSchema(field reflect.StructField, opts map[string]bool) (map[string]any, error) {
	if g.tag == "json" && opts["string"] {
		if schema, ok := quotedSchema(deref(field.Type)); ok {
			return schema, nil
		}
	}
	return g.schemaOf(field.Type)
}

// quotedSchema is the schema of a ",string" field: encoding/json writes a
// string, number or bool as a JSON string holding its JSON encoding, and
// ignores the option on any other kind.
func quotedSchema(t reflect.Type) (map[string]any, bool) {
	switch t.Kind() {
	case reflect.Bool:
		return map[string]any{"type": "string", "enum": []any{"true", "false"}}, true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "string", "pattern": "^-?(0|[1-9][0-9]*)$"}, true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "string", "pattern": "^(0|[1-9][0-9]*)$"}, true
	case reflect.Float32, reflect.Float64, reflect.String:
		return map[string]any{"type": "string"}, true
	}
	return nil, false
}
