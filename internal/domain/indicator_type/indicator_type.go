package indicator_type

import "fmt"

const (
	IndicatorTypeVersionInitial   IndicatorTypeVersion = 0
	IndicatorTypeVersionCommitted IndicatorTypeVersion = 1
)

// IndicatorType - aggregate representing a visual indicator type.
// Defines how a SCADA indicator element is rendered via an SVG template and a script.
type IndicatorType interface {
	ID() IndicatorTypeID
	Name() IndicatorTypeName
	SvgTemplate() SvgTemplate
	Script() Script
	ScriptLanguage() ScriptLanguage
	Version() IndicatorTypeVersion

	Update(name IndicatorTypeName, svgTemplate SvgTemplate, script Script, lang ScriptLanguage)
	IncrementVersion()

	fmt.Stringer
}

// indicatorTypeImpl - IndicatorType implementation struct.
type indicatorTypeImpl struct {
	id             IndicatorTypeID
	name           IndicatorTypeName
	svgTemplate    SvgTemplate
	script         Script
	scriptLanguage ScriptLanguage
	version        IndicatorTypeVersion
}

var _ IndicatorType = (*indicatorTypeImpl)(nil)

// NewIndicatorType creates a new IndicatorType aggregate.
func NewIndicatorType(
	anID IndicatorTypeID,
	aName IndicatorTypeName,
	aSvgTemplate SvgTemplate,
	aScript Script,
	aScriptLanguage ScriptLanguage,
	aVersion IndicatorTypeVersion,
) (IndicatorType, error) {
	return &indicatorTypeImpl{
		id:             anID,
		name:           aName,
		svgTemplate:    aSvgTemplate,
		script:         aScript,
		scriptLanguage: aScriptLanguage,
		version:        aVersion,
	}, nil
}

// ID returns the indicator type identifier.
func (it indicatorTypeImpl) ID() IndicatorTypeID {
	return it.id
}

// Name returns the indicator type name.
func (it indicatorTypeImpl) Name() IndicatorTypeName {
	return it.name
}

// SvgTemplate returns the SVG template.
func (it indicatorTypeImpl) SvgTemplate() SvgTemplate {
	return it.svgTemplate
}

// Script returns the rendering script.
func (it indicatorTypeImpl) Script() Script {
	return it.script
}

// ScriptLanguage returns the scripting language.
func (it indicatorTypeImpl) ScriptLanguage() ScriptLanguage {
	return it.scriptLanguage
}

// Version returns the current version of the indicator type.
func (it indicatorTypeImpl) Version() IndicatorTypeVersion {
	return it.version
}

// Update replaces all mutable fields of the indicator type.
func (it *indicatorTypeImpl) Update(name IndicatorTypeName, svgTemplate SvgTemplate, script Script, lang ScriptLanguage) {
	it.name = name
	it.svgTemplate = svgTemplate
	it.script = script
	it.scriptLanguage = lang
}

// IncrementVersion increments the version by one.
func (it *indicatorTypeImpl) IncrementVersion() {
	it.version++
}

// String implements [fmt.Stringer].
func (it indicatorTypeImpl) String() string {
	return fmt.Sprintf(
		"IndicatorType: %s, Language: %s",
		it.name, it.scriptLanguage,
	)
}
