package appdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/server/domain/library"
)

// InputPort is the application-layer DTO for a WidgetType input port.
type InputPort struct {
	Name        string
	Description string
	TypeHint    string
}

// WidgetType is the application-layer DTO for widget type data: a
// WidgetType of a Library's Draft, at its Version.
type WidgetType struct {
	ID             uuid.UUID
	LibraryID      uuid.UUID
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
func NewWidgetType(wt library.WidgetType) WidgetType {
	return WidgetType{
		ID:             wt.ID().UUID(),
		LibraryID:      wt.LibraryID().UUID(),
		Name:           wt.Name().String(),
		HtmlTemplate:   wt.HtmlTemplate().String(),
		Script:         wt.Script().String(),
		ScriptLanguage: wt.ScriptLanguage().String(),
		DefaultWidth:   wt.DefaultSize().Width(),
		DefaultHeight:  wt.DefaultSize().Height(),
		InputPorts:     newInputPorts(wt.InputPorts()),
		Version:        wt.Version().Number(),
	}
}

func newInputPorts(domainPorts []library.InputPort) []InputPort {
	ports := make([]InputPort, len(domainPorts))
	for i, p := range domainPorts {
		ports[i] = InputPort{
			Name:        p.Name().String(),
			Description: p.Description(),
			TypeHint:    p.TypeHint().String(),
		}
	}
	return ports
}

// ReleasedWidgetType is the application-layer DTO for the copy of a
// WidgetType a Library Release holds: no Version, a release never changes.
type ReleasedWidgetType struct {
	ID             uuid.UUID
	LibraryID      uuid.UUID
	Name           string
	HtmlTemplate   string
	Script         string
	ScriptLanguage string
	DefaultWidth   int
	DefaultHeight  int
	InputPorts     []InputPort
}

// NewReleasedWidgetType creates a ReleasedWidgetType DTO from the domain
// value object.
func NewReleasedWidgetType(wt library.ReleasedWidgetType) ReleasedWidgetType {
	return ReleasedWidgetType{
		ID:             wt.ID().UUID(),
		LibraryID:      wt.LibraryID().UUID(),
		Name:           wt.Name().String(),
		HtmlTemplate:   wt.HtmlTemplate().String(),
		Script:         wt.Script().String(),
		ScriptLanguage: wt.ScriptLanguage().String(),
		DefaultWidth:   wt.DefaultSize().Width(),
		DefaultHeight:  wt.DefaultSize().Height(),
		InputPorts:     newInputPorts(wt.InputPorts()),
	}
}

// NewReleasedWidgetTypeList creates a slice of ReleasedWidgetType DTOs.
func NewReleasedWidgetTypeList(list []library.ReleasedWidgetType) []ReleasedWidgetType {
	result := make([]ReleasedWidgetType, len(list))
	for i, wt := range list {
		result[i] = NewReleasedWidgetType(wt)
	}
	return result
}

// NewWidgetTypeList creates a slice of WidgetType DTOs from domain aggregates.
func NewWidgetTypeList(list []library.WidgetType) []WidgetType {
	result := make([]WidgetType, len(list))
	for i, wt := range list {
		result[i] = NewWidgetType(wt)
	}
	return result
}
