package event

import "github.com/kipitix/growscada/internal/domain/indicator_type"

type IndicatorTypeUpdatedEvent interface {
	IndicatorTypeEvent
}

type indicatorTypeUpdatedEventImpl struct {
	indicatorTypeEventImpl
}

var _ IndicatorTypeUpdatedEvent = (*indicatorTypeUpdatedEventImpl)(nil)

func NewIndicatorTypeUpdatedEvent(anID indicator_type.IndicatorTypeID, opts ...EventOption) IndicatorTypeUpdatedEvent {
	ev := &indicatorTypeUpdatedEventImpl{
		indicatorTypeEventImpl: indicatorTypeEventImpl{
			indicatorTypeID: anID,
			eventImpl: eventImpl{
				eventType: EventTypeIndicatorTypeUpdated,
				timestamp: NewEventTimestamp(),
			},
		},
	}

	ev.applyOptions(opts...)

	return ev
}
