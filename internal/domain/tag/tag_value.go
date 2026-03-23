package tag

type TagValue any

// tagValue is a value object for tag value
// type tagValue interface {
// 	Value() TagValue
// 	Type() TagType
// 	Equals(other tagValue) bool
// }

// // tagValueImpl is the implementation of tagValue
// type tagValueImpl struct {
// 	value     TagValue
// 	valueType TagType
// }

// // Ensure tagValueImpl implements tagValue interface
// var _ tagValue = (*tagValueImpl)(nil)

// // NewTagValue creates a new tagValue
// func NewTagValue(value TagValue, valueType TagType) tagValue {
// 	return &tagValueImpl{value: value, valueType: valueType}
// }

// // Type returns the type of the tag value
// func (t tagValueImpl) Value() TagValue {
// 	return t.value
// }

// // Type returns the type of the tag value
// func (t tagValueImpl) Type() TagType {
// 	return t.valueType
// }

// // Equals checks if the tag value is equal to another tag value
// func (t tagValueImpl) Equals(other tagValue) bool {
// 	return t.value == other.Value() && t.valueType == other.Type()
// }
