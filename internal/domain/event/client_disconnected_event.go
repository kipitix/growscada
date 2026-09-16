package event

import (
	"github.com/kipitix/growscada/internal/domain/client"
	"github.com/kipitix/growscada/internal/domain/id"
)

// ClientDisconnectedEvent - event published when an SSE client disconnects.
type ClientDisconnectedEvent interface {
	ClientEvent
}

type clientDisconnectedEventImpl struct {
	clientEventImpl
}

var _ ClientDisconnectedEvent = (*clientDisconnectedEventImpl)(nil)

func NewClientDisconnectedEvent(anID id.ID[client.Client], opts ...EventOption) ClientDisconnectedEvent {
	ev := &clientDisconnectedEventImpl{
		clientEventImpl: clientEventImpl{
			clientID: anID,
			eventImpl: eventImpl{
				eventType: EventTypeClientDisconnected,
				timestamp: NewEventTimestamp(),
			},
		},
	}
	ev.applyOptions(opts...)
	return ev
}
