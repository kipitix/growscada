package library

import (
	"fmt"
	"slices"

	"github.com/kipitix/growscada/internal/server/domain/id"
)

// WidgetTypeCopy - the copy of a WidgetType held by a saved state
// or by a Project's ProjectFile: its definition under the WidgetType's ID, with
// no Version, because a saved state never changes. It is what a Project sees of
// a WidgetType, through the saved state it imports (ADR 0007), and what the
// Widgets of its Scenes are checked against (task 85_7).
// Value Object.
type WidgetTypeCopy struct {
	id             id.ID[WidgetType]
	libraryID      id.ID[Library]
	name           WidgetTypeName
	htmlTemplate   HtmlTemplate
	script         Script
	scriptLanguage ScriptLanguage
	defaultSize    Size
	inputPorts     []InputPort
}

// NewWidgetTypeCopy rebuilds a copied WidgetType as stored. For repositories,
// and for tests.
// Returns ErrDuplicateInputPortName if any two InputPorts share the same name,
// and an error if the script language is the invalid zero value.
func NewWidgetTypeCopy(
	anID id.ID[WidgetType],
	aLibraryID id.ID[Library],
	aName WidgetTypeName,
	anHtmlTemplate HtmlTemplate,
	aScript Script,
	aScriptLanguage ScriptLanguage,
	aDefaultSize Size,
	someInputPorts []InputPort,
) (WidgetTypeCopy, error) {
	if err := checkDefinition(aScriptLanguage, someInputPorts); err != nil {
		return WidgetTypeCopy{}, err
	}
	return WidgetTypeCopy{
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

// CopyOf returns the copy of the WidgetType wt a saved state holds.
func CopyOf(wt WidgetType) WidgetTypeCopy {
	return WidgetTypeCopy{
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

func (r WidgetTypeCopy) ID() id.ID[WidgetType]          { return r.id }
func (r WidgetTypeCopy) LibraryID() id.ID[Library]      { return r.libraryID }
func (r WidgetTypeCopy) Name() WidgetTypeName           { return r.name }
func (r WidgetTypeCopy) HtmlTemplate() HtmlTemplate     { return r.htmlTemplate }
func (r WidgetTypeCopy) Script() Script                 { return r.script }
func (r WidgetTypeCopy) ScriptLanguage() ScriptLanguage { return r.scriptLanguage }
func (r WidgetTypeCopy) DefaultSize() Size              { return r.defaultSize }

// InputPorts returns a copy of the type's InputPorts.
func (r WidgetTypeCopy) InputPorts() []InputPort { return slices.Clone(r.inputPorts) }

// SameAs reports whether other is this copy with the same definition: what a
// load of a saved state compares before it calls the Draft clean (task 74).
func (r WidgetTypeCopy) SameAs(other WidgetTypeCopy) bool {
	return r.id == other.id &&
		r.libraryID == other.libraryID &&
		r.name == other.name &&
		r.htmlTemplate == other.htmlTemplate &&
		r.script == other.script &&
		r.scriptLanguage == other.scriptLanguage &&
		r.defaultSize == other.defaultSize &&
		slices.Equal(r.inputPorts, other.inputPorts)
}

// Matches reports whether wt, a WidgetType of the Draft, is the same
// WidgetType with the same definition: what a saved state of the Draft now
// would hold.
func (r WidgetTypeCopy) Matches(wt WidgetType) bool {
	return r.SameAs(CopyOf(wt))
}

// String implements [fmt.Stringer].
func (r WidgetTypeCopy) String() string {
	return fmt.Sprintf("WidgetTypeCopy: %s, Library: %s", r.name, r.libraryID)
}
