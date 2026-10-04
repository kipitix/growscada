package uidto

import apiv0 "github.com/kipitix/growscada/contract/api/v0"

// InputPortDTO is the shared UI DTO for a widget type input port, the
// contract's own type so the UI cannot drift from what the server decodes.
// Used by both the library and project packages.
type InputPortDTO = apiv0.InputPort

// TypeHintLabel returns the display label for a type hint string.
// An empty string means the port accepts any tag type.
func TypeHintLabel(typeHint string) string {
	if typeHint == "" {
		return "any"
	}
	return typeHint
}
