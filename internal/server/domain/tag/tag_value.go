package tag

import "fmt"

// TagValue represents the value of a tag.
// Value Object
type TagValue interface {
	Value() any

	fmt.Stringer
}
