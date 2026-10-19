package restapi

import (
	"encoding/json"
	"errors"
	"sync"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
)

// eventClientBufferSize is the per-connection outgoing buffer. A client that
// cannot drain it fast enough is disconnected rather than blocking
// EventBus.Publish (and so the outbox dispatcher) for the rest of the application.
const eventClientBufferSize = 64

// eventHub subscribes to every EventType on the EventBus exactly once
// and fans each published event out to all currently connected SSE clients.
type eventHub struct {
	bus           eventbus.EventBus
	subscriptions []eventbus.Subscription
	maxClients    int

	closeOnce sync.Once

	mu      sync.Mutex
	clients map[*eventClient]struct{}
	closed  bool
}

// Reasons addClient refuses a connection. The texts are the problem details
// the client gets.
var (
	errHubClosed      = errors.New("server is shutting down")
	errTooManyClients = errors.New("too many concurrent event stream connections")
)

type eventClient struct {
	messages chan []byte
	closed   bool
}

// close ends the client's stream by closing its channel. Called with the
// hub's mu held; safe to call more than once.
func (c *eventClient) close() {
	if !c.closed {
		c.closed = true
		close(c.messages)
	}
}

func newEventHub(bus eventbus.EventBus, maxClients int) *eventHub {
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

// Close detaches the hub from the EventBus and ends every connected client's
// stream by closing its channel, so the SSE handlers return without waiting
// for their request context. Later connections are refused. Safe to call
// more than once: every call returns once the hub is detached.
func (h *eventHub) Close() {
	h.closeOnce.Do(func() {
		h.mu.Lock()
		h.closed = true
		for c := range h.clients {
			c.close()
		}
		// Forgotten here, so a broadcast before the unsubscription below
		// finds no client and marshals nothing; removeClient of a
		// forgotten client does nothing.
		clear(h.clients)
		h.mu.Unlock()

		// Outside mu: the bus is not called with the hub's lock held.
		for _, sub := range h.subscriptions {
			h.bus.Unsubscribe(sub)
		}
	})
}

func (h *eventHub) broadcast(e event.Event) {
	h.mu.Lock()
	if len(h.clients) == 0 {
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()

	ev := newAPIEvent(e)
	if ev.Type == "" {
		return // not published: no name in the API
	}
	payload, err := json.Marshal(ev)
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
			c.close()
		}
	}
}

// addClient registers a new SSE connection and returns its message channel.
// It refuses to register once the hub is closed (errHubClosed) or beyond
// maxClients (errTooManyClients), so the caller can reject the connection
// before committing to any response.
func (h *eventHub) addClient() (*eventClient, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return nil, errHubClosed
	}
	if len(h.clients) >= h.maxClients {
		return nil, errTooManyClients
	}

	c := &eventClient{messages: make(chan []byte, eventClientBufferSize)}
	h.clients[c] = struct{}{}
	return c, nil
}

// removeClient unregisters an SSE connection. Safe to call more than once.
func (h *eventHub) removeClient(c *eventClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c]; !ok {
		return
	}
	delete(h.clients, c)
	c.close()
}
