package eventbus

import (
	"github.com/kipitix/growscada/internal/server/domain/client"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// ClientDisconnectedEvent - event published when an SSE client disconnects.
type ClientDisconnectedEvent interface {
	ClientEvent
}

type clientDisconnectedEventImpl struct {
	clientEventImpl
}

var _ ClientDisconnectedEvent = (*clientDisconnectedEventImpl)(nil)

func NewClientDisconnectedEvent(anID id.ID[client.Client], opts ...event.EventOption) ClientDisconnectedEvent {
	return &clientDisconnectedEventImpl{
		clientEventImpl: clientEventImpl{
			Base:     event.NewBase(event.EventTypeClientDisconnected, opts...),
			clientID: anID,
		},
	}
}
