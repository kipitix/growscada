package tagstate

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// TagUpdatedEvent is published by the Runtime node once a write of a Tag's
// state has committed (ADR 0011): never stored in the outbox, delivered
// at-most-once. It carries the full new state, its Seq included, so
// subscribers need not read it back. It is a tag.TagEvent.
type TagUpdatedEvent interface {
	tag.TagEvent
	State() TagState
}

type tagUpdatedEventImpl struct {
	event.Base
	project.Content
	state TagState
}

var _ TagUpdatedEvent = (*tagUpdatedEventImpl)(nil)

// NewTagUpdatedEvent returns the event carrying aState as the Tag's new
// state.
func NewTagUpdatedEvent(aState TagState, opts ...event.EventOption) TagUpdatedEvent {
	return &tagUpdatedEventImpl{
		Base:    event.NewBase(event.EventTypeTagUpdated, opts...),
		Content: project.NewContent(aState.ProjectID()),
		state:   aState,
	}
}

func (e tagUpdatedEventImpl) TagID() id.ID[tag.Tag] { return e.state.TagID() }
func (e tagUpdatedEventImpl) State() TagState       { return e.state }
