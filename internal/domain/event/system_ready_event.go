package event

import "time"

type SystemReadyEvent interface {
	Event
}

type systemReadyEventImpl struct {
}

var _ SystemReadyEvent = (*systemReadyEventImpl)(nil)

func NewSystemReadyEvent() SystemReadyEvent {
	return &systemReadyEventImpl{}
}

func (e systemReadyEventImpl) Type() EventType {
	return EventTypeSystemReady
}

func (e systemReadyEventImpl) Timestamp() EventTimestamp {
	return EventTimestamp(time.Now())
}

func (e systemReadyEventImpl) String() string {
	return "EventSystemReady"
}
