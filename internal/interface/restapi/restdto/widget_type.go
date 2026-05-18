package restdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/application/appdto"
)

// WidgetTypeResponse is the HTTP DTO for representing a widget type in API responses.
type WidgetTypeResponse struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	HtmlTemplate   string    `json:"html_template"`
	Script         string    `json:"script"`
	ScriptLanguage string    `json:"script_language"`
	DefaultWidth   int       `json:"default_width"`
	DefaultHeight  int       `json:"default_height"`
	Version        int       `json:"version"`
}

// GetWidgetTypesResponse is the HTTP DTO for a list of widget types.
type GetWidgetTypesResponse struct {
	WidgetTypes []WidgetTypeResponse `json:"widget_types"`
}

// CreateWidgetTypeRequest is the HTTP DTO for creating a widget type.
type CreateWidgetTypeRequest struct {
	Name           string `json:"name"`
	HtmlTemplate   string `json:"html_template"`
	Script         string `json:"script"`
	ScriptLanguage string `json:"script_language"`
	DefaultWidth   int    `json:"default_width"`
	DefaultHeight  int    `json:"default_height"`
}

// CreateWidgetTypeResponse is the HTTP DTO for a creation response.
type CreateWidgetTypeResponse struct {
	ID uuid.UUID `json:"id"`
}

// UpdateWidgetTypeRequest is the HTTP DTO for updating a widget type.
type UpdateWidgetTypeRequest struct {
	Name           string `json:"name"`
	HtmlTemplate   string `json:"html_template"`
	Script         string `json:"script"`
	ScriptLanguage string `json:"script_language"`
	DefaultWidth   int    `json:"default_width"`
	DefaultHeight  int    `json:"default_height"`
}

// UpdateWidgetTypeResponse is the HTTP DTO for an update response.
type UpdateWidgetTypeResponse struct {
	Version int `json:"version"`
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
	}
}

func NewUpdateWidgetTypeInput(r UpdateWidgetTypeRequest, id uuid.UUID) appdto.UpdateWidgetTypeInput {
	return appdto.UpdateWidgetTypeInput{
		ID:             id,
		Name:           r.Name,
		HtmlTemplate:   r.HtmlTemplate,
		Script:         r.Script,
		ScriptLanguage: r.ScriptLanguage,
		DefaultWidth:   r.DefaultWidth,
		DefaultHeight:  r.DefaultHeight,
	}
}
