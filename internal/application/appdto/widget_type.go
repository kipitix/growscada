package appdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/domain/widget"
)

// WidgetType is the application-layer DTO for widget type data.
type WidgetType struct {
	ID             uuid.UUID
	Name           string
	HtmlTemplate   string
	Script         string
	ScriptLanguage string
	Version        int
}

// CreateWidgetTypeInput holds the input data for creating a widget type.
type CreateWidgetTypeInput struct {
	Name           string
	HtmlTemplate   string
	Script         string
	ScriptLanguage string
}

// UpdateWidgetTypeInput holds the input data for updating a widget type.
type UpdateWidgetTypeInput struct {
	ID             uuid.UUID
	Name           string
	HtmlTemplate   string
	Script         string
	ScriptLanguage string
}

// NewWidgetType creates a WidgetType DTO from the domain aggregate.
func NewWidgetType(wt widget.WidgetType) WidgetType {
	return WidgetType{
		ID:             wt.ID().UUID(),
		Name:           wt.Name().String(),
		HtmlTemplate:   wt.HtmlTemplate().String(),
		Script:         wt.Script().String(),
		ScriptLanguage: wt.ScriptLanguage().String(),
		Version:        wt.Version().Number(),
	}
}

// NewWidgetTypeList creates a slice of WidgetType DTOs from domain aggregates.
func NewWidgetTypeList(list []widget.WidgetType) []WidgetType {
	result := make([]WidgetType, len(list))
	for i, wt := range list {
		result[i] = NewWidgetType(wt)
	}
	return result
}
