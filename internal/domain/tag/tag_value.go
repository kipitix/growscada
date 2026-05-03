package tag

// TagValue represents the value of a tag.
// Value Object
type TagValue interface {
	Value() any
	String() string
}
