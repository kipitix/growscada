package event

import "github.com/kipitix/growscada/internal/domain/indicator_type"

type IndicatorTypeCreatedEvent interface {
	IndicatorTypeEvent
}

type indicatorTypeCreatedEventImpl struct {
	indicatorTypeEventImpl
}

var _ IndicatorTypeCreatedEvent = (*indicatorTypeCreatedEventImpl)(nil)

func NewIndicatorTypeCreatedEvent(anID indicator_type.IndicatorTypeID, opts ...EventOption) IndicatorTypeCreatedEvent {
	ev := &indicatorTypeCreatedEventImpl{
		indicatorTypeEventImpl: indicatorTypeEventImpl{
			indicatorTypeID: anID,
			eventImpl: eventImpl{
				eventType: EventTypeIndicatorTypeCreated,
				timestamp: NewEventTimestamp(),
			},
		},
	}

	ev.applyOptions(opts...)

	return ev
}
