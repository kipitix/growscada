package restdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/application/appdto"
)

// IndicatorTypeResponse is the HTTP DTO for representing an indicator type in API responses.
type IndicatorTypeResponse struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	SvgTemplate    string    `json:"svg_template"`
	Script         string    `json:"script"`
	ScriptLanguage string    `json:"script_language"`
	Version        int       `json:"version"`
}

// GetIndicatorTypesResponse is the HTTP DTO for a list of indicator types.
type GetIndicatorTypesResponse struct {
	IndicatorTypes []IndicatorTypeResponse `json:"indicator_types"`
}

// CreateIndicatorTypeRequest is the HTTP DTO for creating an indicator type.
type CreateIndicatorTypeRequest struct {
	Name           string `json:"name"`
	SvgTemplate    string `json:"svg_template"`
	Script         string `json:"script"`
	ScriptLanguage string `json:"script_language"`
}

// CreateIndicatorTypeResponse is the HTTP DTO for a creation response.
type CreateIndicatorTypeResponse struct {
	ID uuid.UUID `json:"id"`
}

// UpdateIndicatorTypeRequest is the HTTP DTO for updating an indicator type.
type UpdateIndicatorTypeRequest struct {
	Name           string `json:"name"`
	SvgTemplate    string `json:"svg_template"`
	Script         string `json:"script"`
	ScriptLanguage string `json:"script_language"`
}

// UpdateIndicatorTypeResponse is the HTTP DTO for an update response.
type UpdateIndicatorTypeResponse struct {
	Version int `json:"version"`
}

func NewIndicatorTypeResponse(it appdto.IndicatorType) IndicatorTypeResponse {
	return IndicatorTypeResponse{
		ID:             it.ID,
		Name:           it.Name,
		SvgTemplate:    it.SvgTemplate,
		Script:         it.Script,
		ScriptLanguage: it.ScriptLanguage,
		Version:        it.Version,
	}
}

func NewGetIndicatorTypesResponse(list []appdto.IndicatorType) GetIndicatorTypesResponse {
	items := make([]IndicatorTypeResponse, len(list))
	for i, it := range list {
		items[i] = NewIndicatorTypeResponse(it)
	}
	return GetIndicatorTypesResponse{IndicatorTypes: items}
}

func NewCreateIndicatorTypeResponse(it appdto.IndicatorType) CreateIndicatorTypeResponse {
	return CreateIndicatorTypeResponse{ID: it.ID}
}

func NewUpdateIndicatorTypeResponse(it appdto.IndicatorType) UpdateIndicatorTypeResponse {
	return UpdateIndicatorTypeResponse{Version: it.Version}
}

func NewCreateIndicatorTypeInput(r CreateIndicatorTypeRequest) appdto.CreateIndicatorTypeInput {
	return appdto.CreateIndicatorTypeInput{
		Name:           r.Name,
		SvgTemplate:    r.SvgTemplate,
		Script:         r.Script,
		ScriptLanguage: r.ScriptLanguage,
	}
}

func NewUpdateIndicatorTypeInput(r UpdateIndicatorTypeRequest, id uuid.UUID) appdto.UpdateIndicatorTypeInput {
	return appdto.UpdateIndicatorTypeInput{
		ID:             id,
		Name:           r.Name,
		SvgTemplate:    r.SvgTemplate,
		Script:         r.Script,
		ScriptLanguage: r.ScriptLanguage,
	}
}
