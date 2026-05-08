package event

import "github.com/kipitix/growscada/internal/domain/tag"

type TagEvent interface {
	Event
	TagID() tag.TagID
}

type tagEventImpl struct {
	eventImpl
	tagID tag.TagID
}

var _ TagEvent = (*tagEventImpl)(nil)

// NO FABRIC METHOD

func (e tagEventImpl) TagID() tag.TagID {
	return e.tagID
}
