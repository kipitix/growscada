package event

import (
	"fmt"
	"time"
)

type EventTimestamp time.Time

var _ fmt.Stringer = EventTimestamp(time.Now())

func (et EventTimestamp) String() string {
	return time.Time(et).String()
}
