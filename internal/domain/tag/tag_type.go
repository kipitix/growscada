package tag

// go-enum tool
// ENUM(Boolean,Integer,Float,String)
type TagTypeEnum int

// TagType is a value object for tag type
type TagType interface {
	Type() TagTypeEnum
	Equals(other TagType) bool
	String() string

	IsBoolean() bool
	IsInteger() bool
	IsFloat() bool
	IsString() bool
}

type tagType struct {
	tagType TagTypeEnum
}

var _ TagType = (*tagType)(nil)

func NewTagType(aTagType TagTypeEnum) TagType {
	return &tagType{tagType: aTagType}
}

func (t tagType) Type() TagTypeEnum {
	return t.tagType
}

func (t tagType) Equals(other TagType) bool {
	return t.tagType == other.Type()
}

func (t tagType) String() string {
	return t.tagType.String()
}

func (t tagType) IsBoolean() bool {
	return t.tagType == TagTypeEnumBoolean
}

func (t tagType) IsInteger() bool {
	return t.tagType == TagTypeEnumInteger
}

func (t tagType) IsFloat() bool {
	return t.tagType == TagTypeEnumFloat
}

func (t tagType) IsString() bool {
	return t.tagType == TagTypeEnumString
}
