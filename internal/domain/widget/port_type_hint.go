package widget

import (
	"fmt"

	"github.com/kipitix/growscada/internal/domain/tag"
)

// PortTypeHint - the TagType an InputPort accepts: either one concrete TagType
// or any TagType. "Any" is a property of the hint, not a TagType.
// The zero value means "any".
// Value Object.
type PortTypeHint struct {
	tagType tag.TagType
}

var _ fmt.Stringer = PortTypeHint{}

// AnyTagType returns a hint that accepts any TagType.
func AnyTagType() PortTypeHint {
	return PortTypeHint{}
}

// OnlyTagType returns a hint that accepts only the given TagType.
// Returns an error if the TagType is the invalid zero value.
func OnlyTagType(t tag.TagType) (PortTypeHint, error) {
	if !t.IsValid() {
		return PortTypeHint{}, fmt.Errorf("port type hint: invalid tag type")
	}
	return PortTypeHint{tagType: t}, nil
}

// NewPortTypeHint parses a hint from its string form: "" means any TagType,
// otherwise the string must be a known TagType.
func NewPortTypeHint(s string) (PortTypeHint, error) {
	if s == "" {
		return AnyTagType(), nil
	}
	t, err := tag.NewTagType(s)
	if err != nil {
		return PortTypeHint{}, fmt.Errorf("port type hint: %w", err)
	}
	return PortTypeHint{tagType: t}, nil
}

// IsAny reports whether the hint accepts any TagType.
func (h PortTypeHint) IsAny() bool {
	return !h.tagType.IsValid()
}

// TagType returns the concrete TagType and true, or false if the hint accepts any TagType.
func (h PortTypeHint) TagType() (tag.TagType, bool) {
	return h.tagType, !h.IsAny()
}

// Accepts reports whether a Tag of the given type may be bound to the port.
func (h PortTypeHint) Accepts(t tag.TagType) bool {
	return h.IsAny() || h.tagType == t
}

// String returns "" for any TagType, otherwise the TagType's string form.
func (h PortTypeHint) String() string {
	if h.IsAny() {
		return ""
	}
	return h.tagType.String()
}
