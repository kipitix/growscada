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

// WidgetTypeResponse is the HTTP DTO for representing a widget type of a
// library's Draft in API responses.
type WidgetTypeResponse struct {
	ID             uuid.UUID   `json:"id"`
	LibraryID      uuid.UUID   `json:"library_id"`
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
// Version is the new widget type's version, the one its first update states.
type CreateWidgetTypeResponse struct {
	ID      uuid.UUID `json:"id"`
	Version int       `json:"version"`
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

// ReleasedWidgetTypeResponse is the HTTP DTO of the copy of a WidgetType a
// Library Release holds: its definition under the WidgetType's ID, with no
// version — a release never changes. A Widget's type_id names one.
type ReleasedWidgetTypeResponse struct {
	ID             uuid.UUID   `json:"id"`
	LibraryID      uuid.UUID   `json:"library_id"`
	Name           string      `json:"name"`
	HtmlTemplate   string      `json:"html_template"`
	Script         string      `json:"script"`
	ScriptLanguage string      `json:"script_language"`
	DefaultWidth   int         `json:"default_width"`
	DefaultHeight  int         `json:"default_height"`
	InputPorts     []InputPort `json:"input_ports"`
}

// GetPinnedWidgetTypesResponse is the HTTP DTO of GET
// /projects/{id}/widget-types: the WidgetTypes of every Library Release the
// project pins, grouped by library in the order the pins were added.
type GetPinnedWidgetTypesResponse struct {
	WidgetTypes []ReleasedWidgetTypeResponse `json:"widget_types"`
}
