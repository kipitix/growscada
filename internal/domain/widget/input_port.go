package widget

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/kipitix/growscada/internal/domain/tag"
)

// ErrDuplicateInputPortName is returned when a WidgetType is constructed with
// two or more InputPorts sharing the same name.
var ErrDuplicateInputPortName = errors.New("duplicate input port name")

var jsIdentifierRe = regexp.MustCompile(`^[a-zA-Z_$][a-zA-Z0-9_$]*$`)

// InputPortName is the name of an InputPort. It must be a valid JS identifier
// so that templates can reference it as input.<name>.
// Value Object.
type InputPortName struct {
	name string
}

var _ fmt.Stringer = InputPortName{}

// NewInputPortName creates an InputPortName, validating that the string is a
// non-empty valid JavaScript identifier.
func NewInputPortName(s string) (InputPortName, error) {
	if s == "" {
		return InputPortName{}, fmt.Errorf("input port name must not be empty")
	}
	if !jsIdentifierRe.MatchString(s) {
		return InputPortName{}, fmt.Errorf("input port name %q must be a valid JS identifier", s)
	}
	return InputPortName{name: s}, nil
}

// String implements [fmt.Stringer].
func (n InputPortName) String() string { return n.name }

// InputPort is a named, typed input slot defined on a WidgetType.
// The JS template accesses it as input.<name>.
// typeHint == tag.TagTypeUnknown means any tag type is accepted.
// Value Object.
type InputPort struct {
	name        InputPortName
	description string
	typeHint    tag.TagType
}

// NewInputPort creates an InputPort.
func NewInputPort(name InputPortName, description string, typeHint tag.TagType) InputPort {
	return InputPort{name: name, description: description, typeHint: typeHint}
}

func (p InputPort) Name() InputPortName { return p.name }
func (p InputPort) Description() string { return p.description }
func (p InputPort) TypeHint() tag.TagType { return p.typeHint }
