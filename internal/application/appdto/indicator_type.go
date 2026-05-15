package appdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/domain/indicator_type"
)

// IndicatorType is the application-layer DTO for indicator type data.
type IndicatorType struct {
	ID             uuid.UUID
	Name           string
	SvgTemplate    string
	Script         string
	ScriptLanguage string
	Version        int
}

// CreateIndicatorTypeInput holds the input data for creating an indicator type.
type CreateIndicatorTypeInput struct {
	Name           string
	SvgTemplate    string
	Script         string
	ScriptLanguage string
}

// UpdateIndicatorTypeInput holds the input data for updating an indicator type.
type UpdateIndicatorTypeInput struct {
	ID             uuid.UUID
	Name           string
	SvgTemplate    string
	Script         string
	ScriptLanguage string
}

// NewIndicatorType creates an IndicatorType DTO from the domain aggregate.
func NewIndicatorType(it indicator_type.IndicatorType) IndicatorType {
	return IndicatorType{
		ID:             it.ID().UUID(),
		Name:           it.Name().String(),
		SvgTemplate:    it.SvgTemplate().String(),
		Script:         it.Script().String(),
		ScriptLanguage: it.ScriptLanguage().String(),
		Version:        it.Version().Number(),
	}
}

// NewIndicatorTypeList creates a slice of IndicatorType DTOs from domain aggregates.
func NewIndicatorTypeList(list []indicator_type.IndicatorType) []IndicatorType {
	result := make([]IndicatorType, len(list))
	for i, it := range list {
		result[i] = NewIndicatorType(it)
	}
	return result
}
