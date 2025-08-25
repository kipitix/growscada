package value

import "time"

type Quality int

const (
	Good Quality = iota
	Bad
	Uncertain
)

type Value struct {
	Data      any
	Quality   Quality
	Timestamp time.Time
}

func NewValue(data any, quality Quality) Value {
	return Value{
		Data:      data,
		Quality:   quality,
		Timestamp: time.Now().UTC(),
	}
}
