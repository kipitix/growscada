package library

import (
	"fmt"
	"slices"

	"github.com/kipitix/growscada/internal/server/domain/id"
)

// ReleasedWidgetType - the copy of a WidgetType held by a Library Release:
// its definition under the WidgetType's ID, with no Version — a release
// never changes. It is what a Project sees of a WidgetType, through the
// release it pins (ADR 0007), and what the Widgets of its Scenes are checked
// against.
// Value Object.
type ReleasedWidgetType struct {
	id             id.ID[WidgetType]
	libraryID      id.ID[Library]
	name           WidgetTypeName
	htmlTemplate   HtmlTemplate
	script         Script
	scriptLanguage ScriptLanguage
	defaultSize    Size
	inputPorts     []InputPort
}

// NewReleasedWidgetType rebuilds a released WidgetType as stored. For
// repositories, and for tests.
// Returns ErrDuplicateInputPortName if any two InputPorts share the same name,
// and an error if the script language is the invalid zero value.
func NewReleasedWidgetType(
	anID id.ID[WidgetType],
	aLibraryID id.ID[Library],
	aName WidgetTypeName,
	anHtmlTemplate HtmlTemplate,
	aScript Script,
	aScriptLanguage ScriptLanguage,
	aDefaultSize Size,
	someInputPorts []InputPort,
) (ReleasedWidgetType, error) {
	if err := checkDefinition(aScriptLanguage, someInputPorts); err != nil {
		return ReleasedWidgetType{}, err
	}
	return ReleasedWidgetType{
		id:             anID,
		libraryID:      aLibraryID,
		name:           aName,
		htmlTemplate:   anHtmlTemplate,
		script:         aScript,
		scriptLanguage: aScriptLanguage,
		defaultSize:    aDefaultSize,
		inputPorts:     slices.Clone(someInputPorts),
	}, nil
}

// ReleasedFrom returns the copy of the WidgetType wt a release holds.
func ReleasedFrom(wt WidgetType) ReleasedWidgetType {
	return ReleasedWidgetType{
		id:             wt.ID(),
		libraryID:      wt.LibraryID(),
		name:           wt.Name(),
		htmlTemplate:   wt.HtmlTemplate(),
		script:         wt.Script(),
		scriptLanguage: wt.ScriptLanguage(),
		defaultSize:    wt.DefaultSize(),
		inputPorts:     wt.InputPorts(),
	}
}

func (r ReleasedWidgetType) ID() id.ID[WidgetType]          { return r.id }
func (r ReleasedWidgetType) LibraryID() id.ID[Library]      { return r.libraryID }
func (r ReleasedWidgetType) Name() WidgetTypeName           { return r.name }
func (r ReleasedWidgetType) HtmlTemplate() HtmlTemplate     { return r.htmlTemplate }
func (r ReleasedWidgetType) Script() Script                 { return r.script }
func (r ReleasedWidgetType) ScriptLanguage() ScriptLanguage { return r.scriptLanguage }
func (r ReleasedWidgetType) DefaultSize() Size              { return r.defaultSize }

// InputPorts returns a copy of the type's InputPorts.
func (r ReleasedWidgetType) InputPorts() []InputPort { return slices.Clone(r.inputPorts) }

// Matches reports whether wt, a WidgetType of the Draft, is the same
// WidgetType with the same definition: what a release of the Draft now
// would hold.
func (r ReleasedWidgetType) Matches(wt WidgetType) bool {
	return r.id == wt.ID() &&
		r.libraryID == wt.LibraryID() &&
		r.name == wt.Name() &&
		r.htmlTemplate == wt.HtmlTemplate() &&
		r.script == wt.Script() &&
		r.scriptLanguage == wt.ScriptLanguage() &&
		r.defaultSize == wt.DefaultSize() &&
		slices.Equal(r.inputPorts, wt.InputPorts())
}

// String implements [fmt.Stringer].
func (r ReleasedWidgetType) String() string {
	return fmt.Sprintf("ReleasedWidgetType: %s, Library: %s", r.name, r.libraryID)
}
