package widget

import "fmt"

// WidgetType - aggregate representing a visual widget type.
// Defines how a SCADA widget element is rendered via an HTML template and a script.
type WidgetType interface {
	ID() WidgetTypeID
	Name() WidgetTypeName
	HtmlTemplate() HtmlTemplate
	Script() Script
	ScriptLanguage() ScriptLanguage
	Version() WidgetTypeVersion

	Update(name WidgetTypeName, htmlTemplate HtmlTemplate, script Script, lang ScriptLanguage)
	IncrementVersion()

	fmt.Stringer
}

// widgetTypeImpl - WidgetType implementation struct.
type widgetTypeImpl struct {
	id             WidgetTypeID
	name           WidgetTypeName
	htmlTemplate   HtmlTemplate
	script         Script
	scriptLanguage ScriptLanguage
	version        WidgetTypeVersion
}

var _ WidgetType = (*widgetTypeImpl)(nil)

// NewWidgetType creates a new WidgetType aggregate.
func NewWidgetType(
	anID WidgetTypeID,
	aName WidgetTypeName,
	anHtmlTemplate HtmlTemplate,
	aScript Script,
	aScriptLanguage ScriptLanguage,
	aVersion WidgetTypeVersion,
) (WidgetType, error) {
	return &widgetTypeImpl{
		id:             anID,
		name:           aName,
		htmlTemplate:   anHtmlTemplate,
		script:         aScript,
		scriptLanguage: aScriptLanguage,
		version:        aVersion,
	}, nil
}

// ID returns the widget type identifier.
func (wt widgetTypeImpl) ID() WidgetTypeID {
	return wt.id
}

// Name returns the widget type name.
func (wt widgetTypeImpl) Name() WidgetTypeName {
	return wt.name
}

// HtmlTemplate returns the HTML template.
func (wt widgetTypeImpl) HtmlTemplate() HtmlTemplate {
	return wt.htmlTemplate
}

// Script returns the rendering script.
func (wt widgetTypeImpl) Script() Script {
	return wt.script
}

// ScriptLanguage returns the scripting language.
func (wt widgetTypeImpl) ScriptLanguage() ScriptLanguage {
	return wt.scriptLanguage
}

// Version returns the current version of the widget type.
func (wt widgetTypeImpl) Version() WidgetTypeVersion {
	return wt.version
}

// Update replaces all mutable fields of the widget type.
func (wt *widgetTypeImpl) Update(name WidgetTypeName, htmlTemplate HtmlTemplate, script Script, lang ScriptLanguage) {
	wt.name = name
	wt.htmlTemplate = htmlTemplate
	wt.script = script
	wt.scriptLanguage = lang
}

// IncrementVersion increments the version by one.
func (wt *widgetTypeImpl) IncrementVersion() {
	wt.version = wt.version.Next()
}

// String implements [fmt.Stringer].
func (wt widgetTypeImpl) String() string {
	return fmt.Sprintf(
		"WidgetType: %s, Language: %s",
		wt.name, wt.scriptLanguage,
	)
}
