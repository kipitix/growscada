package event

import "sync"

type EventBus interface {
	Publish(event Event)
	Subscribe(eventType EventType, handler EventHandler)
}

type EventHandler func(Event)

type eventBusImpl struct {
	mu       sync.RWMutex
	handlers map[EventType][]EventHandler
}

var _ EventBus = (*eventBusImpl)(nil)

func NewEventBus() EventBus {
	return &eventBusImpl{
		handlers: make(map[EventType][]EventHandler),
	}
}

func (eb *eventBusImpl) Publish(event Event) {
	eb.mu.RLock()
	handlers := make([]EventHandler, len(eb.handlers[event.Type()]))
	copy(handlers, eb.handlers[event.Type()])
	eb.mu.RUnlock()

	for _, h := range handlers {
		h(event)
	}
}

func (eb *eventBusImpl) Subscribe(eventType EventType, handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.handlers[eventType] = append(eb.handlers[eventType], handler)
}
