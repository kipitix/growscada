package tag

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// TagEvent - base interface for all events a Tag records.
type TagEvent interface {
	event.Event
	TagID() id.ID[Tag]
}

type tagEventImpl struct {
	event.Base
	tagID id.ID[Tag]
}

var _ TagEvent = (*tagEventImpl)(nil)

// NO FABRIC METHOD

func (e tagEventImpl) TagID() id.ID[Tag] {
	return e.tagID
}
