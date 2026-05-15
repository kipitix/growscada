package widget

import (
	"fmt"

	"github.com/kipitix/growscada/internal/domain/version"
)

// WidgetType - aggregate representing a visual widget type.
// Defines how a SCADA widget element is rendered via an HTML template and a script.
type WidgetType interface {
	ID() WidgetTypeID
	Name() WidgetTypeName
	HtmlTemplate() HtmlTemplate
	Script() Script
	ScriptLanguage() ScriptLanguage
	Version() version.Version

	fmt.Stringer
}

// widgetTypeImpl - WidgetType implementation struct.
type widgetTypeImpl struct {
	id             WidgetTypeID
	name           WidgetTypeName
	htmlTemplate   HtmlTemplate
	script         Script
	scriptLanguage ScriptLanguage
	version        version.Version
}

var _ WidgetType = (*widgetTypeImpl)(nil)

// NewWidgetType creates a new WidgetType aggregate.
func NewWidgetType(
	anID WidgetTypeID,
	aName WidgetTypeName,
	anHtmlTemplate HtmlTemplate,
	aScript Script,
	aScriptLanguage ScriptLanguage,
	aVersion version.Version,
) WidgetType {
	return &widgetTypeImpl{
		id:             anID,
		name:           aName,
		htmlTemplate:   anHtmlTemplate,
		script:         aScript,
		scriptLanguage: aScriptLanguage,
		version:        aVersion,
	}
}

func (wt widgetTypeImpl) ID() WidgetTypeID           { return wt.id }
func (wt widgetTypeImpl) Name() WidgetTypeName        { return wt.name }
func (wt widgetTypeImpl) HtmlTemplate() HtmlTemplate  { return wt.htmlTemplate }
func (wt widgetTypeImpl) Script() Script              { return wt.script }
func (wt widgetTypeImpl) ScriptLanguage() ScriptLanguage { return wt.scriptLanguage }
func (wt widgetTypeImpl) Version() version.Version    { return wt.version }

// String implements [fmt.Stringer].
func (wt widgetTypeImpl) String() string {
	return fmt.Sprintf("WidgetType: %s, Language: %s", wt.name, wt.scriptLanguage)
}
