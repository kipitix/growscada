package event

import "github.com/kipitix/growscada/internal/domain/indicator_type"

type IndicatorTypeDeletedEvent interface {
	IndicatorTypeEvent
}

type indicatorTypeDeletedEventImpl struct {
	indicatorTypeEventImpl
}

var _ IndicatorTypeDeletedEvent = (*indicatorTypeDeletedEventImpl)(nil)

func NewIndicatorTypeDeletedEvent(anID indicator_type.IndicatorTypeID, opts ...EventOption) IndicatorTypeDeletedEvent {
	ev := &indicatorTypeDeletedEventImpl{
		indicatorTypeEventImpl: indicatorTypeEventImpl{
			indicatorTypeID: anID,
			eventImpl: eventImpl{
				eventType: EventTypeIndicatorTypeDeleted,
				timestamp: NewEventTimestamp(),
			},
		},
	}

	ev.applyOptions(opts...)

	return ev
}
