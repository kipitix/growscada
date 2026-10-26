package tag

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// TagEvent - base interface for all events a Tag records. It is a
// project.ContentEvent: it names the Tag's Project.
type TagEvent interface {
	project.ContentEvent
	TagID() id.ID[Tag]
}

type tagEventImpl struct {
	event.Base
	tagID id.ID[Tag]
	project.Content
}

var _ TagEvent = (*tagEventImpl)(nil)

// NO FABRIC METHOD

func (e tagEventImpl) TagID() id.ID[Tag] {
	return e.tagID
}
