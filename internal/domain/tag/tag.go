package tag

type Tag interface {
	ID() TagID
	Name() TagName
	Type() TagType
	Quality() TagQuality
}

type tag struct {
	id      TagID
	name    TagName
	theType TagType
	quality TagQuality
}

var _ Tag = (*tag)(nil)

func NewTag(id TagID, name TagName) Tag {
	return &tag{
		id:   id,
		name: name,
	}
}

func (t tag) ID() TagID {
	return t.id
}

func (t tag) Name() TagName {
	return t.name
}

func (t tag) Type() TagType {
	return t.theType
}

func (t tag) Quality() TagQuality {
	return t.quality
}
