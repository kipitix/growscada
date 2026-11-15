package label

import (
	"github.com/kipitix/growscada/internal/server/domain/draft"
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

// LabelAssignmentEvent is recorded by Assign and Unassign: the label marks a
// saved state, or stops marking it. It names the Draft's scope and the saved
// state's number, so a client can reload the history of the right Library or
// Project (task 85_3; ADR 0025).
type LabelAssignmentEvent interface {
	LabelEvent
	Scope() draft.Scope
	SavedStateNumber() int
}

type labelEventImpl struct {
	event.Base
	labelID          id.ID[Label]
	scope            draft.Scope
	savedStateNumber int
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

// LabelAssignedEvent is recorded by Assign: the label marks a saved state.
type LabelAssignedEvent interface{ LabelAssignmentEvent }

// LabelUnassignedEvent is recorded by Unassign: the label no longer marks a
// saved state.
type LabelUnassignedEvent interface{ LabelAssignmentEvent }

type labelCreatedEventImpl struct{ labelEventImpl }
type labelUpdatedEventImpl struct{ labelEventImpl }
type labelDeletedEventImpl struct{ labelEventImpl }

type labelAssignedEventImpl struct{ labelEventImpl }

var _ LabelAssignedEvent = (*labelAssignedEventImpl)(nil)

func (e labelAssignedEventImpl) Scope() draft.Scope    { return e.scope }
func (e labelAssignedEventImpl) SavedStateNumber() int { return e.savedStateNumber }

type labelUnassignedEventImpl struct{ labelEventImpl }

var _ LabelUnassignedEvent = (*labelUnassignedEventImpl)(nil)

func (e labelUnassignedEventImpl) Scope() draft.Scope    { return e.scope }
func (e labelUnassignedEventImpl) SavedStateNumber() int { return e.savedStateNumber }

func NewLabelCreatedEvent(anID id.ID[Label], opts ...event.EventOption) LabelCreatedEvent {
	return &labelCreatedEventImpl{newLabelEvent(event.EventTypeLabelCreated, anID, opts...)}
}

func NewLabelUpdatedEvent(anID id.ID[Label], opts ...event.EventOption) LabelUpdatedEvent {
	return &labelUpdatedEventImpl{newLabelEvent(event.EventTypeLabelUpdated, anID, opts...)}
}

func NewLabelDeletedEvent(anID id.ID[Label], opts ...event.EventOption) LabelDeletedEvent {
	return &labelDeletedEventImpl{newLabelEvent(event.EventTypeLabelDeleted, anID, opts...)}
}

// NewLabelAssignedEvent records the mark of the saved state savedStateNumber of
// scope.
func NewLabelAssignedEvent(anID id.ID[Label], scope draft.Scope, savedStateNumber int, opts ...event.EventOption) LabelAssignedEvent {
	e := newLabelEvent(event.EventTypeLabelAssigned, anID, opts...)
	e.scope, e.savedStateNumber = scope, savedStateNumber
	return &labelAssignedEventImpl{e}
}

// NewLabelUnassignedEvent records that the label no longer marks the saved
// state savedStateNumber of scope.
func NewLabelUnassignedEvent(anID id.ID[Label], scope draft.Scope, savedStateNumber int, opts ...event.EventOption) LabelUnassignedEvent {
	e := newLabelEvent(event.EventTypeLabelUnassigned, anID, opts...)
	e.scope, e.savedStateNumber = scope, savedStateNumber
	return &labelUnassignedEventImpl{e}
}
