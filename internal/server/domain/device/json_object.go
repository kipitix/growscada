package device

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// newJSONObject reads a JSON object from raw input: no input at all is the
// empty object, anything that is not a JSON object is an error. what names
// the value in the error.
func newJSONObject(raw []byte, what string) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return json.RawMessage("{}"), nil
	}
	if !json.Valid(trimmed) {
		return nil, fmt.Errorf("%s must be valid JSON", what)
	}
	if trimmed[0] != '{' {
		return nil, fmt.Errorf("%s must be a JSON object", what)
	}
	return json.RawMessage(bytes.Clone(trimmed)), nil
}
