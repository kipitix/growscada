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
//
// WidgetType is immutable: every change returns a new WidgetType that also
// records the change's event. A change keeps the Version; the repository
// raises it once when it saves the result.
type WidgetType interface {
	ID() id.ID[WidgetType]
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

	// Delete records a WidgetTypeDeletedEvent; the repository's Delete removes
	// the widget type. Like removing any aggregate, it expects no Version.
	Delete() WidgetType

	event.Recorder
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
	pending        []event.Event
}

var _ WidgetType = (*widgetTypeImpl)(nil)

// CreateWidgetType creates a new, not yet saved WidgetType and records a
// WidgetTypeCreatedEvent.
// Returns ErrDuplicateInputPortName if any two InputPorts share the same name,
// and an error if the script language is the invalid zero value.
func CreateWidgetType(
	anID id.ID[WidgetType],
	aName WidgetTypeName,
	anHtmlTemplate HtmlTemplate,
	aScript Script,
	aScriptLanguage ScriptLanguage,
	aDefaultSize Size,
	someInputPorts []InputPort,
) (WidgetType, error) {
	wt, err := ReconstituteWidgetType(anID, aName, anHtmlTemplate, aScript, aScriptLanguage, aDefaultSize, someInputPorts, version.Initial[WidgetType]())
	if err != nil {
		return nil, err
	}
	impl := wt.(*widgetTypeImpl)
	impl.pending = []event.Event{NewWidgetTypeCreatedEvent(anID)}
	return impl, nil
}

// ReconstituteWidgetType rebuilds a WidgetType as stored, at the given
// version; it records no event. For repositories.
// Returns ErrDuplicateInputPortName if any two InputPorts share the same name,
// and an error if the script language is the invalid zero value.
func ReconstituteWidgetType(
	anID id.ID[WidgetType],
	aName WidgetTypeName,
	anHtmlTemplate HtmlTemplate,
	aScript Script,
	aScriptLanguage ScriptLanguage,
	aDefaultSize Size,
	someInputPorts []InputPort,
	aVersion version.Version[WidgetType],
) (WidgetType, error) {
	if !aScriptLanguage.IsValid() {
		return nil, fmt.Errorf("cannot create widget type: invalid script language")
	}

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
	updated, err := ReconstituteWidgetType(wt.id, aName, anHtmlTemplate, aScript, aScriptLanguage, aDefaultSize, someInputPorts, wt.version)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidWidgetType, err)
	}
	impl := updated.(*widgetTypeImpl)
	impl.pending = append(slices.Clone(wt.pending), NewWidgetTypeUpdatedEvent(wt.id))
	return impl, nil
}

func (wt *widgetTypeImpl) Delete() WidgetType {
	deleted := *wt
	deleted.pending = append(slices.Clone(wt.pending), NewWidgetTypeDeletedEvent(wt.id))
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
	return fmt.Sprintf("WidgetType: %s, Language: %s, DefaultSize: %s", wt.name, wt.scriptLanguage, wt.defaultSize)
}
