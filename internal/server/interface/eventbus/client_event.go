package eventbus

import (
	"github.com/kipitix/growscada/internal/server/domain/client"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// ClientEvent - base interface for all client connection-lifecycle events.
// They are events of delivery, not of the domain: the SSE handler publishes
// them straight to the EventBus, and they never reach the outbox.
type ClientEvent interface {
	event.Event
	ClientID() id.ID[client.Client]
}

type clientEventImpl struct {
	event.Base
	clientID id.ID[client.Client]
}

var _ ClientEvent = (*clientEventImpl)(nil)

// NO FABRIC METHOD

func (e clientEventImpl) ClientID() id.ID[client.Client] {
	return e.clientID
}
