package label

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// LabelEvent - base interface for the events a Label records about itself.
// They are server events, like those of Projects and Libraries: a label is not
// content of any Draft (task 85_2; ADR 0025).
type LabelEvent interface {
	event.Event
	LabelID() id.ID[Label]
	labelEvent()
}

type labelEventImpl struct {
	event.Base
	labelID id.ID[Label]
}

var _ LabelEvent = (*labelEventImpl)(nil)

func (e labelEventImpl) LabelID() id.ID[Label] { return e.labelID }
func (labelEventImpl) labelEvent()             {}

func newLabelEvent(anEventType event.EventType, anID id.ID[Label], opts ...event.EventOption) labelEventImpl {
	return labelEventImpl{Base: event.NewBase(anEventType, opts...), labelID: anID}
}

// LabelCreatedEvent is recorded by CreateLabel.
type LabelCreatedEvent interface{ LabelEvent }

// LabelUpdatedEvent is recorded by Rename and Recolor: the name and/or the
// color changed.
type LabelUpdatedEvent interface{ LabelEvent }

// LabelDeletedEvent is recorded by Delete.
type LabelDeletedEvent interface{ LabelEvent }

type labelCreatedEventImpl struct{ labelEventImpl }
type labelUpdatedEventImpl struct{ labelEventImpl }
type labelDeletedEventImpl struct{ labelEventImpl }

func NewLabelCreatedEvent(anID id.ID[Label], opts ...event.EventOption) LabelCreatedEvent {
	return &labelCreatedEventImpl{newLabelEvent(event.EventTypeLabelCreated, anID, opts...)}
}

func NewLabelUpdatedEvent(anID id.ID[Label], opts ...event.EventOption) LabelUpdatedEvent {
	return &labelUpdatedEventImpl{newLabelEvent(event.EventTypeLabelUpdated, anID, opts...)}
}

func NewLabelDeletedEvent(anID id.ID[Label], opts ...event.EventOption) LabelDeletedEvent {
	return &labelDeletedEventImpl{newLabelEvent(event.EventTypeLabelDeleted, anID, opts...)}
}
