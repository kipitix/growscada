package restdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/application/appdto"
	"github.com/kipitix/growscada/internal/interface/ui/uidto"
)

// InputPortDTO is the HTTP DTO for a WidgetType input port.
// Type alias for uidto.InputPortDTO — both packages share the same wire format.
type InputPortDTO = uidto.InputPortDTO

// WidgetTypeResponse is the HTTP DTO for representing a widget type in API responses.
type WidgetTypeResponse struct {
	ID             uuid.UUID      `json:"id"`
	Name           string         `json:"name"`
	HtmlTemplate   string         `json:"html_template"`
	Script         string         `json:"script"`
	ScriptLanguage string         `json:"script_language"`
	DefaultWidth   int            `json:"default_width"`
	DefaultHeight  int            `json:"default_height"`
	InputPorts     []InputPortDTO `json:"input_ports"`
	Version        int            `json:"version"`
}

// GetWidgetTypesResponse is the HTTP DTO for a list of widget types.
type GetWidgetTypesResponse struct {
	WidgetTypes []WidgetTypeResponse `json:"widget_types"`
}

// CreateWidgetTypeRequest is the HTTP DTO for creating a widget type.
type CreateWidgetTypeRequest struct {
	Name           string         `json:"name"`
	HtmlTemplate   string         `json:"html_template"`
	Script         string         `json:"script"`
	ScriptLanguage string         `json:"script_language"`
	DefaultWidth   int            `json:"default_width"`
	DefaultHeight  int            `json:"default_height"`
	InputPorts     []InputPortDTO `json:"input_ports"`
}

// CreateWidgetTypeResponse is the HTTP DTO for a creation response.
type CreateWidgetTypeResponse struct {
	ID uuid.UUID `json:"id"`
}

// UpdateWidgetTypeRequest is the HTTP DTO for updating a widget type.
// Version must match the current persisted version for optimistic locking.
type UpdateWidgetTypeRequest struct {
	Name           string         `json:"name"`
	HtmlTemplate   string         `json:"html_template"`
	Script         string         `json:"script"`
	ScriptLanguage string         `json:"script_language"`
	DefaultWidth   int            `json:"default_width"`
	DefaultHeight  int            `json:"default_height"`
	InputPorts     []InputPortDTO `json:"input_ports"`
	Version        int            `json:"version"`
}

// UpdateWidgetTypeResponse is the HTTP DTO for an update response.
type UpdateWidgetTypeResponse struct {
	Version int `json:"version"`
}

func inputPortDTOsToAppDTOs(ports []InputPortDTO) []appdto.InputPort {
	result := make([]appdto.InputPort, len(ports))
	for i, p := range ports {
		result[i] = appdto.InputPort{
			Name:        p.Name,
			Description: p.Description,
			TypeHint:    p.TypeHint,
		}
	}
	return result
}

func appInputPortDTOsToRest(ports []appdto.InputPort) []InputPortDTO {
	result := make([]InputPortDTO, len(ports))
	for i, p := range ports {
		result[i] = InputPortDTO{
			Name:        p.Name,
			Description: p.Description,
			TypeHint:    p.TypeHint,
		}
	}
	return result
}

func NewWidgetTypeResponse(wt appdto.WidgetType) WidgetTypeResponse {
	return WidgetTypeResponse{
		ID:             wt.ID,
		Name:           wt.Name,
		HtmlTemplate:   wt.HtmlTemplate,
		Script:         wt.Script,
		ScriptLanguage: wt.ScriptLanguage,
		DefaultWidth:   wt.DefaultWidth,
		DefaultHeight:  wt.DefaultHeight,
		InputPorts:     appInputPortDTOsToRest(wt.InputPorts),
		Version:        wt.Version,
	}
}

func NewGetWidgetTypesResponse(list []appdto.WidgetType) GetWidgetTypesResponse {
	items := make([]WidgetTypeResponse, len(list))
	for i, wt := range list {
		items[i] = NewWidgetTypeResponse(wt)
	}
	return GetWidgetTypesResponse{WidgetTypes: items}
}

func NewCreateWidgetTypeResponse(wt appdto.WidgetType) CreateWidgetTypeResponse {
	return CreateWidgetTypeResponse{ID: wt.ID}
}

func NewUpdateWidgetTypeResponse(wt appdto.WidgetType) UpdateWidgetTypeResponse {
	return UpdateWidgetTypeResponse{Version: wt.Version}
}

func NewCreateWidgetTypeInput(r CreateWidgetTypeRequest) appdto.CreateWidgetTypeInput {
	return appdto.CreateWidgetTypeInput{
		Name:           r.Name,
		HtmlTemplate:   r.HtmlTemplate,
		Script:         r.Script,
		ScriptLanguage: r.ScriptLanguage,
		DefaultWidth:   r.DefaultWidth,
		DefaultHeight:  r.DefaultHeight,
		InputPorts:     inputPortDTOsToAppDTOs(r.InputPorts),
	}
}

func NewUpdateWidgetTypeInput(r UpdateWidgetTypeRequest, anID uuid.UUID) appdto.UpdateWidgetTypeInput {
	return appdto.UpdateWidgetTypeInput{
		ID:             anID,
		Name:           r.Name,
		HtmlTemplate:   r.HtmlTemplate,
		Script:         r.Script,
		ScriptLanguage: r.ScriptLanguage,
		DefaultWidth:   r.DefaultWidth,
		DefaultHeight:  r.DefaultHeight,
		InputPorts:     inputPortDTOsToAppDTOs(r.InputPorts),
		Version:        r.Version,
	}
}
