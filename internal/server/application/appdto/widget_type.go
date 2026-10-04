package appdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/server/domain/widget"
)

// InputPort is the application-layer DTO for a WidgetType input port.
type InputPort struct {
	Name        string
	Description string
	TypeHint    string
}

// WidgetType is the application-layer DTO for widget type data.
type WidgetType struct {
	ID             uuid.UUID
	Name           string
	HtmlTemplate   string
	Script         string
	ScriptLanguage string
	DefaultWidth   int
	DefaultHeight  int
	InputPorts     []InputPort
	Version        int
}

// WidgetTypeInput holds the fields of a widget type a client creates or
// updates. The widget type's ID and the version an update expects are passed
// separately.
type WidgetTypeInput struct {
	Name           string
	HtmlTemplate   string
	Script         string
	ScriptLanguage string
	DefaultWidth   int
	DefaultHeight  int
	InputPorts     []InputPort
}

// NewWidgetType creates a WidgetType DTO from the domain aggregate.
func NewWidgetType(wt widget.WidgetType) WidgetType {
	domainPorts := wt.InputPorts()
	ports := make([]InputPort, len(domainPorts))
	for i, p := range domainPorts {
		ports[i] = InputPort{
			Name:        p.Name().String(),
			Description: p.Description(),
			TypeHint:    p.TypeHint().String(),
		}
	}
	return WidgetType{
		ID:             wt.ID().UUID(),
		Name:           wt.Name().String(),
		HtmlTemplate:   wt.HtmlTemplate().String(),
		Script:         wt.Script().String(),
		ScriptLanguage: wt.ScriptLanguage().String(),
		DefaultWidth:   wt.DefaultSize().Width(),
		DefaultHeight:  wt.DefaultSize().Height(),
		InputPorts:     ports,
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
