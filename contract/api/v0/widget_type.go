package apiv0

import (
	"github.com/google/uuid"
)

// InputPort is a WidgetType's input port. An empty TypeHint accepts any
// TagType.
type InputPort struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	TypeHint    string `json:"type_hint"`
}

// WidgetTypeResponse is the HTTP DTO for representing a widget type in API responses.
type WidgetTypeResponse struct {
	ID             uuid.UUID   `json:"id"`
	Name           string      `json:"name"`
	HtmlTemplate   string      `json:"html_template"`
	Script         string      `json:"script"`
	ScriptLanguage string      `json:"script_language"`
	DefaultWidth   int         `json:"default_width"`
	DefaultHeight  int         `json:"default_height"`
	InputPorts     []InputPort `json:"input_ports"`
	Version        int         `json:"version"`
}

// GetWidgetTypesResponse is the HTTP DTO for a list of widget types.
type GetWidgetTypesResponse struct {
	WidgetTypes []WidgetTypeResponse `json:"widget_types"`
}

// CreateWidgetTypeRequest is the HTTP DTO for creating a widget type.
type CreateWidgetTypeRequest struct {
	Name           string      `json:"name"`
	HtmlTemplate   string      `json:"html_template"`
	Script         string      `json:"script"`
	ScriptLanguage string      `json:"script_language"`
	DefaultWidth   int         `json:"default_width"`
	DefaultHeight  int         `json:"default_height"`
	InputPorts     []InputPort `json:"input_ports"`
}

// CreateWidgetTypeResponse is the HTTP DTO for a creation response.
type CreateWidgetTypeResponse struct {
	ID uuid.UUID `json:"id"`
}

// UpdateWidgetTypeRequest is the HTTP DTO for updating a widget type.
// Version must match the current persisted version for optimistic locking.
type UpdateWidgetTypeRequest struct {
	Name           string      `json:"name"`
	HtmlTemplate   string      `json:"html_template"`
	Script         string      `json:"script"`
	ScriptLanguage string      `json:"script_language"`
	DefaultWidth   int         `json:"default_width"`
	DefaultHeight  int         `json:"default_height"`
	InputPorts     []InputPort `json:"input_ports"`
	Version        int         `json:"version"`
}

// UpdateWidgetTypeResponse is the HTTP DTO for an update response.
type UpdateWidgetTypeResponse struct {
	Version int `json:"version"`
}
