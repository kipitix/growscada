package history

import "gitverse.ru/kipitix/growscada/internal/domain/value"

type History struct {
	values   []value.Value
	capacity int
}

func NewHistory(capacity int) *History {
	return &History{
		capacity: capacity,
		values:   make([]value.Value, 0, capacity),
	}
}

func (h *History) Add(v value.Value) {
	if len(h.values) >= h.capacity {
		h.values = append(h.values[1:], v)
	} else {
		h.values = append(h.values, v)
	}
}
