package eventbus

import (
	"github.com/kipitix/growscada/internal/server/domain/client"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// ClientConnectedEvent - event published when an SSE client connects.
type ClientConnectedEvent interface {
	ClientEvent
}

type clientConnectedEventImpl struct {
	clientEventImpl
}

var _ ClientConnectedEvent = (*clientConnectedEventImpl)(nil)

func NewClientConnectedEvent(anID id.ID[client.Client], opts ...event.EventOption) ClientConnectedEvent {
	return &clientConnectedEventImpl{
		clientEventImpl: clientEventImpl{
			Base:     event.NewBase(event.EventTypeClientConnected, opts...),
			clientID: anID,
		},
	}
}
