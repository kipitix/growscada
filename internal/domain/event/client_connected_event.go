package event

import (
	"github.com/kipitix/growscada/internal/domain/client"
	"github.com/kipitix/growscada/internal/domain/id"
)

// ClientConnectedEvent - event published when an SSE client connects.
type ClientConnectedEvent interface {
	ClientEvent
}

type clientConnectedEventImpl struct {
	clientEventImpl
}

var _ ClientConnectedEvent = (*clientConnectedEventImpl)(nil)

func NewClientConnectedEvent(anID id.ID[client.Client], opts ...EventOption) ClientConnectedEvent {
	ev := &clientConnectedEventImpl{
		clientEventImpl: clientEventImpl{
			clientID: anID,
			eventImpl: eventImpl{
				eventType: EventTypeClientConnected,
				timestamp: NewEventTimestamp(),
			},
		},
	}
	ev.applyOptions(opts...)
	return ev
}
