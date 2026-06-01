package widget

import (
	"fmt"

	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/version"
)

// WidgetType - aggregate representing a visual widget type.
// Defines how a SCADA widget element is rendered via an HTML template and a script.
type WidgetType interface {
	ID() id.ID[WidgetType]
	Name() WidgetTypeName
	HtmlTemplate() HtmlTemplate
	Script() Script
	ScriptLanguage() ScriptLanguage
	DefaultSize() Size
	InputPorts() []InputPort
	Version() version.Version[WidgetType]

	fmt.Stringer
}

// widgetTypeImpl - WidgetType implementation struct.
type widgetTypeImpl struct {
	id             id.ID[WidgetType]
	name           WidgetTypeName
	htmlTemplate   HtmlTemplate
	script         Script
	scriptLanguage ScriptLanguage
	defaultSize    Size
	inputPorts     []InputPort
	version        version.Version[WidgetType]
}

var _ WidgetType = (*widgetTypeImpl)(nil)

// NewWidgetType creates a new WidgetType aggregate.
// Returns ErrDuplicateInputPortName if any two InputPorts share the same name.
func NewWidgetType(
	anID id.ID[WidgetType],
	aName WidgetTypeName,
	anHtmlTemplate HtmlTemplate,
	aScript Script,
	aScriptLanguage ScriptLanguage,
	aDefaultSize Size,
	someInputPorts []InputPort,
	aVersion version.Version[WidgetType],
) (WidgetType, error) {
	seen := make(map[string]struct{}, len(someInputPorts))
	for _, p := range someInputPorts {
		key := p.Name().String()
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateInputPortName, key)
		}
		seen[key] = struct{}{}
	}

	ports := make([]InputPort, len(someInputPorts))
	copy(ports, someInputPorts)

	return &widgetTypeImpl{
		id:             anID,
		name:           aName,
		htmlTemplate:   anHtmlTemplate,
		script:         aScript,
		scriptLanguage: aScriptLanguage,
		defaultSize:    aDefaultSize,
		inputPorts:     ports,
		version:        aVersion,
	}, nil
}

func (wt widgetTypeImpl) ID() id.ID[WidgetType]                { return wt.id }
func (wt widgetTypeImpl) Name() WidgetTypeName                 { return wt.name }
func (wt widgetTypeImpl) HtmlTemplate() HtmlTemplate           { return wt.htmlTemplate }
func (wt widgetTypeImpl) Script() Script                       { return wt.script }
func (wt widgetTypeImpl) ScriptLanguage() ScriptLanguage       { return wt.scriptLanguage }
func (wt widgetTypeImpl) DefaultSize() Size                    { return wt.defaultSize }
func (wt widgetTypeImpl) Version() version.Version[WidgetType] { return wt.version }

func (wt widgetTypeImpl) InputPorts() []InputPort {
	out := make([]InputPort, len(wt.inputPorts))
	copy(out, wt.inputPorts)
	return out
}

// String implements [fmt.Stringer].
func (wt widgetTypeImpl) String() string {
	return fmt.Sprintf("WidgetType: %s, Language: %s, DefaultSize: %s", wt.name, wt.scriptLanguage, wt.defaultSize)
}
