package apiv0

import (
	"bytes"
	"encoding/json"
	"errors"
)

// JSONObject is a JSON object carried as it is: the protocol-specific
// configuration of a Device, the connection parameters of an Environment
// Device. It keeps the bytes it was read from, so a value a reader does not
// know survives a read and a write; the zero value is the empty object.
// A value that is not a JSON object is invalid input.
type JSONObject json.RawMessage

var _ json.Marshaler = JSONObject{}

// MarshalJSON returns the object as it is. The zero value is the empty
// object, so a response always carries one.
func (o JSONObject) MarshalJSON() ([]byte, error) {
	if len(o) == 0 {
		return []byte("{}"), nil
	}
	return o, nil
}

// UnmarshalJSON accepts a JSON object alone.
func (o *JSONObject) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) {
		return errors.New("json: must be a JSON object")
	}
	*o = append((*o)[:0], trimmed...)
	return nil
}

// JSONSchema is the schema of JSONObject: an object of any shape.
func (JSONObject) JSONSchema() map[string]any {
	return map[string]any{"type": "object"}
}
