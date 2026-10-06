package eventbus

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
)

type SystemReadyEvent interface {
	event.Event
}

type systemReadyEventImpl struct {
	event.Base
}

var _ SystemReadyEvent = (*systemReadyEventImpl)(nil)

func NewSystemReadyEvent(opts ...event.EventOption) SystemReadyEvent {
	return &systemReadyEventImpl{Base: event.NewBase(event.EventTypeSystemReady, opts...)}
}
