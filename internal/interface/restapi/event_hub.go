package restapi

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/kipitix/growscada/internal/domain/event"
)

// eventClientBufferSize is the per-connection outgoing buffer. A client that
// cannot drain it fast enough is disconnected rather than blocking
// EventBus.Publish for the rest of the application.
const eventClientBufferSize = 64

// eventMessage is the wire payload sent as SSE `data:` for every domain
// event, regardless of its concrete type.
type eventMessage struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	ID        string `json:"id,omitempty"`
}

// eventHub subscribes to every EventType on the domain EventBus exactly once
// and fans each published event out to all currently connected SSE clients.
type eventHub struct {
	bus           event.EventBus
	subscriptions []event.Subscription
	maxClients    int

	mu      sync.Mutex
	clients map[*eventClient]struct{}
}

type eventClient struct {
	messages chan []byte
	closed   bool
}

func newEventHub(bus event.EventBus, maxClients int) *eventHub {
	h := &eventHub{
		bus:        bus,
		maxClients: maxClients,
		clients:    make(map[*eventClient]struct{}),
	}

	for _, et := range event.AllEventTypes() {
		h.subscriptions = append(h.subscriptions, bus.Subscribe(et, h.broadcast))
	}

	return h
}

// Publish forwards to the underlying EventBus, so callers that only hold a
// reference to the hub (e.g. the SSE handler, for connection-lifecycle
// events) don't need a separate EventBus reference.
func (h *eventHub) Publish(e event.Event) {
	h.bus.Publish(e)
}

// Close detaches the hub from the EventBus. Connected clients are left to be
// disconnected by their own request context cancellation.
func (h *eventHub) Close() {
	for _, sub := range h.subscriptions {
		h.bus.Unsubscribe(sub)
	}
}

func (h *eventHub) broadcast(e event.Event) {
	h.mu.Lock()
	if len(h.clients) == 0 {
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()

	payload, err := json.Marshal(eventMessage{
		Type:      e.Type().String(),
		Timestamp: e.Timestamp().Time().Format(time.RFC3339),
		ID:        eventSourceID(e),
	})
	if err != nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		if c.closed {
			continue
		}
		select {
		case c.messages <- payload:
		default:
			// Slow client: drop it instead of blocking the publisher.
			c.closed = true
			close(c.messages)
		}
	}
}

// addClient registers a new SSE connection and returns its message channel.
// It refuses to register beyond maxClients, returning ok=false so the
// caller can reject the connection before committing to any response.
func (h *eventHub) addClient() (c *eventClient, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.clients) >= h.maxClients {
		return nil, false
	}

	c = &eventClient{messages: make(chan []byte, eventClientBufferSize)}
	h.clients[c] = struct{}{}
	return c, true
}

// removeClient unregisters an SSE connection. Safe to call more than once.
func (h *eventHub) removeClient(c *eventClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c]; !ok {
		return
	}
	delete(h.clients, c)
	if !c.closed {
		c.closed = true
		close(c.messages)
	}
}

// eventSourceID extracts the identifying UUID carried by a domain event
// (aggregate ID for domain aggregates, connection ID for client-lifecycle
// events), if any.
func eventSourceID(e event.Event) string {
	switch ev := e.(type) {
	case event.TagEvent:
		return ev.TagID().String()
	case event.WidgetEvent:
		return ev.WidgetID().String()
	case event.WidgetTypeEvent:
		return ev.WidgetTypeID().String()
	case event.SceneEvent:
		return ev.SceneID().String()
	case event.ClientEvent:
		return ev.ClientID().String()
	default:
		return ""
	}
}
