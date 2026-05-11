package event

import "github.com/kipitix/growscada/internal/domain/indicator_type"

type IndicatorTypeEvent interface {
	Event
	IndicatorTypeID() indicator_type.IndicatorTypeID
}

type indicatorTypeEventImpl struct {
	eventImpl
	indicatorTypeID indicator_type.IndicatorTypeID
}

var _ IndicatorTypeEvent = (*indicatorTypeEventImpl)(nil)

// NO FABRIC METHOD

func (e indicatorTypeEventImpl) IndicatorTypeID() indicator_type.IndicatorTypeID {
	return e.indicatorTypeID
}
