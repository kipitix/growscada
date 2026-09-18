package event

import (
	"github.com/kipitix/growscada/internal/domain/client"
	"github.com/kipitix/growscada/internal/domain/id"
)

// ClientEvent - base interface for all client connection-lifecycle events.
type ClientEvent interface {
	Event
	ClientID() id.ID[client.Client]
}

type clientEventImpl struct {
	eventImpl
	clientID id.ID[client.Client]
}

var _ ClientEvent = (*clientEventImpl)(nil)

// NO FABRIC METHOD

func (e clientEventImpl) ClientID() id.ID[client.Client] {
	return e.clientID
}
