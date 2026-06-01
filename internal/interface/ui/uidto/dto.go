package uidto

// InputPortDTO is the shared UI DTO for a widget type input port.
// Used by both the library and project packages when decoding API responses.
type InputPortDTO struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	TypeHint    string `json:"type_hint"`
}

// TypeHintLabel returns the display label for a type hint string.
// Empty string and "unknown" both normalize to "any".
func TypeHintLabel(typeHint string) string {
	if typeHint == "" || typeHint == "unknown" {
		return "any"
	}
	return typeHint
}
