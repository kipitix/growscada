package library

import (
	"fmt"
	"slices"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// WidgetType - aggregate representing a visual widget type.
// Defines how a SCADA widget element is rendered via an HTML template and a script.
// It belongs to the Draft of exactly one Library, fixed at creation; its name
// is unique within that Library. Projects never see it, only the copies the
// Library Releases they pin hold (ReleasedWidgetType).
//
// WidgetType is immutable: every change returns a new WidgetType that also
// records the change's event. A change keeps the Version; the repository
// raises it once when it saves the result.
type WidgetType interface {
	ID() id.ID[WidgetType]
	// LibraryID is the Library whose Draft the widget type belongs to; it
	// never changes.
	LibraryID() id.ID[Library]
	Name() WidgetTypeName
	HtmlTemplate() HtmlTemplate
	Script() Script
	ScriptLanguage() ScriptLanguage
	DefaultSize() Size
	InputPorts() []InputPort
	Version() version.Version[WidgetType]

	// CheckVersion reports an edit conflict: the caller changes the widget
	// type having seen it at a Version other than the current one.
	// Returns ErrWidgetTypeConflict if expected is not the Version.
	CheckVersion(expected version.Version[WidgetType]) error

	// Update replaces the widget type's definition and records a
	// WidgetTypeUpdatedEvent.
	// Returns ErrWidgetTypeConflict if expected is not the Version, or
	// ErrInvalidWidgetType if the definition is invalid: two InputPorts share
	// a name (also ErrDuplicateInputPortName) or the script language is the
	// invalid zero value.
	Update(
		expected version.Version[WidgetType],
		aName WidgetTypeName,
		anHtmlTemplate HtmlTemplate,
		aScript Script,
		aScriptLanguage ScriptLanguage,
		aDefaultSize Size,
		someInputPorts []InputPort,
	) (WidgetType, error)

	// ResetTo replaces the widget type's definition by the one released, a
	// copy of the same WidgetType in a Library Release, and records a
	// WidgetTypeUpdatedEvent: the change of a Discard, which rests on no
	// Version a client saw. A released copy of another WidgetType is an
	// error.
	ResetTo(released ReleasedWidgetType) (WidgetType, error)

	// Delete records a WidgetTypeDeletedEvent; the repository's Delete removes
	// the widget type. Like removing any aggregate, it expects no Version.
	Delete() WidgetType

	event.Recorder
	fmt.Stringer
}

// widgetTypeImpl - WidgetType implementation struct.
type widgetTypeImpl struct {
	id             id.ID[WidgetType]
	libraryID      id.ID[Library]
	name           WidgetTypeName
	htmlTemplate   HtmlTemplate
	script         Script
	scriptLanguage ScriptLanguage
	defaultSize    Size
	inputPorts     []InputPort
	version        version.Version[WidgetType]
	pending        []event.Event
}

var _ WidgetType = (*widgetTypeImpl)(nil)

// CreateWidgetType creates a new, not yet saved WidgetType of the Draft of the
// Library aLibraryID and records a WidgetTypeCreatedEvent.
// Returns ErrDuplicateInputPortName if any two InputPorts share the same name,
// and an error if the script language is the invalid zero value.
func CreateWidgetType(
	anID id.ID[WidgetType],
	aLibraryID id.ID[Library],
	aName WidgetTypeName,
	anHtmlTemplate HtmlTemplate,
	aScript Script,
	aScriptLanguage ScriptLanguage,
	aDefaultSize Size,
	someInputPorts []InputPort,
) (WidgetType, error) {
	wt, err := ReconstituteWidgetType(anID, aLibraryID, aName, anHtmlTemplate, aScript, aScriptLanguage, aDefaultSize, someInputPorts, version.Initial[WidgetType]())
	if err != nil {
		return nil, err
	}
	impl := wt.(*widgetTypeImpl)
	impl.pending = []event.Event{NewWidgetTypeCreatedEvent(anID, aLibraryID)}
	return impl, nil
}

// RecreateWidgetType creates anew, not yet saved, the WidgetType a Library
// Release holds, under its ID and in its Library, and records a
// WidgetTypeCreatedEvent: a Discard bringing back a WidgetType removed from
// the Draft since the release.
func RecreateWidgetType(released ReleasedWidgetType) (WidgetType, error) {
	return CreateWidgetType(released.ID(), released.LibraryID(), released.Name(), released.HtmlTemplate(),
		released.Script(), released.ScriptLanguage(), released.DefaultSize(), released.InputPorts())
}

// ReconstituteWidgetType rebuilds a WidgetType as stored, at the given
// version; it records no event. For repositories.
// Returns ErrDuplicateInputPortName if any two InputPorts share the same name,
// and an error if the script language is the invalid zero value.
func ReconstituteWidgetType(
	anID id.ID[WidgetType],
	aLibraryID id.ID[Library],
	aName WidgetTypeName,
	anHtmlTemplate HtmlTemplate,
	aScript Script,
	aScriptLanguage ScriptLanguage,
	aDefaultSize Size,
	someInputPorts []InputPort,
	aVersion version.Version[WidgetType],
) (WidgetType, error) {
	if err := checkDefinition(aScriptLanguage, someInputPorts); err != nil {
		return nil, err
	}

	ports := make([]InputPort, len(someInputPorts))
	copy(ports, someInputPorts)

	return &widgetTypeImpl{
		id:             anID,
		libraryID:      aLibraryID,
		name:           aName,
		htmlTemplate:   anHtmlTemplate,
		script:         aScript,
		scriptLanguage: aScriptLanguage,
		defaultSize:    aDefaultSize,
		inputPorts:     ports,
		version:        aVersion,
	}, nil
}

// checkDefinition checks the rules of a WidgetType's definition as a whole:
// a valid script language, InputPorts with unique names.
func checkDefinition(aScriptLanguage ScriptLanguage, someInputPorts []InputPort) error {
	if !aScriptLanguage.IsValid() {
		return fmt.Errorf("cannot create widget type: invalid script language")
	}
	seen := make(map[string]struct{}, len(someInputPorts))
	for _, p := range someInputPorts {
		key := p.Name().String()
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%w: %q", ErrDuplicateInputPortName, key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (wt widgetTypeImpl) ID() id.ID[WidgetType]                { return wt.id }
func (wt widgetTypeImpl) LibraryID() id.ID[Library]            { return wt.libraryID }
func (wt widgetTypeImpl) Name() WidgetTypeName                 { return wt.name }
func (wt widgetTypeImpl) HtmlTemplate() HtmlTemplate           { return wt.htmlTemplate }
func (wt widgetTypeImpl) Script() Script                       { return wt.script }
func (wt widgetTypeImpl) ScriptLanguage() ScriptLanguage       { return wt.scriptLanguage }
func (wt widgetTypeImpl) DefaultSize() Size                    { return wt.defaultSize }
func (wt widgetTypeImpl) Version() version.Version[WidgetType] { return wt.version }

func (wt *widgetTypeImpl) CheckVersion(expected version.Version[WidgetType]) error {
	if expected != wt.version {
		return fmt.Errorf("%w: expected version %s, current %s", ErrWidgetTypeConflict, expected, wt.version)
	}
	return nil
}

func (wt *widgetTypeImpl) Update(
	expected version.Version[WidgetType],
	aName WidgetTypeName,
	anHtmlTemplate HtmlTemplate,
	aScript Script,
	aScriptLanguage ScriptLanguage,
	aDefaultSize Size,
	someInputPorts []InputPort,
) (WidgetType, error) {
	if err := wt.CheckVersion(expected); err != nil {
		return nil, err
	}
	updated, err := ReconstituteWidgetType(wt.id, wt.libraryID, aName, anHtmlTemplate, aScript, aScriptLanguage, aDefaultSize, someInputPorts, wt.version)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidWidgetType, err)
	}
	impl := updated.(*widgetTypeImpl)
	impl.pending = append(slices.Clone(wt.pending), NewWidgetTypeUpdatedEvent(wt.id, wt.libraryID))
	return impl, nil
}

func (wt *widgetTypeImpl) ResetTo(released ReleasedWidgetType) (WidgetType, error) {
	if released.ID() != wt.id || released.LibraryID() != wt.libraryID {
		return nil, fmt.Errorf("cannot reset widget type %s of library %s to %s of library %s",
			wt.id, wt.libraryID, released.ID(), released.LibraryID())
	}
	return wt.Update(wt.version, released.Name(), released.HtmlTemplate(), released.Script(),
		released.ScriptLanguage(), released.DefaultSize(), released.InputPorts())
}

func (wt *widgetTypeImpl) Delete() WidgetType {
	deleted := *wt
	deleted.pending = append(slices.Clone(wt.pending), NewWidgetTypeDeletedEvent(wt.id, wt.libraryID))
	return &deleted
}

func (wt widgetTypeImpl) PendingEvents() []event.Event {
	return slices.Clone(wt.pending)
}

func (wt widgetTypeImpl) InputPorts() []InputPort {
	out := make([]InputPort, len(wt.inputPorts))
	copy(out, wt.inputPorts)
	return out
}

// String implements [fmt.Stringer].
func (wt widgetTypeImpl) String() string {
	return fmt.Sprintf("WidgetType: %s, Library: %s, Language: %s, DefaultSize: %s", wt.name, wt.libraryID, wt.scriptLanguage, wt.defaultSize)
}
