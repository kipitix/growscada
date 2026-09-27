package event

type SystemReadyEvent interface {
	Event
}

type systemReadyEventImpl struct {
	eventImpl
}

var _ SystemReadyEvent = (*systemReadyEventImpl)(nil)

func NewSystemReadyEvent(opts ...EventOption) SystemReadyEvent {
	ev := &systemReadyEventImpl{
		eventImpl: eventImpl{
			eventType: EventTypeSystemReady,
			timestamp: NewEventTimestamp(),
		},
	}

	ev.applyOptions(opts...)

	return ev
}
