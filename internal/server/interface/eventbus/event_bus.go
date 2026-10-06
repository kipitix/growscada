// Package eventbus delivers events in process to the live views of the
// system (the SSE stream): the domain events the outbox dispatcher hands over
// after commit, and the connection-lifecycle events of delivery itself.
package eventbus

import (
	"sync"

	"github.com/kipitix/growscada/internal/server/domain/event"
)

type EventBus interface {
	Publish(e event.Event)
	Subscribe(eventType event.EventType, handler EventHandler) Subscription
	Unsubscribe(subscription Subscription)
}

type EventHandler func(event.Event)

// Subscription - opaque handle returned by Subscribe, used to remove the
// associated handler via Unsubscribe. EventHandler funcs are not comparable,
// so this handle is how a subscription is identified for removal.
type Subscription struct {
	eventType event.EventType
	id        uint64
}

type eventBusImpl struct {
	mu       sync.RWMutex
	nextID   uint64
	handlers map[event.EventType]map[uint64]EventHandler
}

var _ EventBus = (*eventBusImpl)(nil)

func NewEventBus() EventBus {
	return &eventBusImpl{
		handlers: make(map[event.EventType]map[uint64]EventHandler),
	}
}

func (eb *eventBusImpl) Publish(e event.Event) {
	eb.mu.RLock()
	byID := eb.handlers[e.Type()]
	handlers := make([]EventHandler, 0, len(byID))
	for _, h := range byID {
		handlers = append(handlers, h)
	}
	eb.mu.RUnlock()

	for _, h := range handlers {
		h(e)
	}
}

func (eb *eventBusImpl) Subscribe(eventType event.EventType, handler EventHandler) Subscription {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.nextID++
	id := eb.nextID

	if eb.handlers[eventType] == nil {
		eb.handlers[eventType] = make(map[uint64]EventHandler)
	}
	eb.handlers[eventType][id] = handler

	return Subscription{eventType: eventType, id: id}
}

func (eb *eventBusImpl) Unsubscribe(subscription Subscription) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	delete(eb.handlers[subscription.eventType], subscription.id)
}
