package event

import (
	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/tag"
)

type TagEvent interface {
	Event
	TagID() id.ID[tag.Tag]
}

type tagEventImpl struct {
	eventImpl
	tagID id.ID[tag.Tag]
}

var _ TagEvent = (*tagEventImpl)(nil)

// NO FABRIC METHOD

func (e tagEventImpl) TagID() id.ID[tag.Tag] {
	return e.tagID
}
