package restapi

import (
	"fmt"
	"net/http"

	"github.com/kipitix/growscada/internal/domain/client"
	"github.com/kipitix/growscada/internal/domain/event"
	"github.com/kipitix/growscada/internal/domain/id"
)

// EventsHandlers handles the SSE stream of domain events.
type EventsHandlers struct {
	hub *eventHub
}

// NewEventsHandler creates the SSE event handler. maxClients caps how many
// concurrent event streams the hub will accept — see eventHub.addClient.
func NewEventsHandler(bus event.EventBus, maxClients int) *EventsHandlers {
	return &EventsHandlers{hub: newEventHub(bus, maxClients)}
}

// Close detaches the hub from the EventBus. Connected clients are left to be
// disconnected by their own request context cancellation.
func (h *EventsHandlers) Close() {
	h.hub.Close()
}

// GetEvents handles GET /api/v1/events. It upgrades the connection to an SSE
// stream and forwards every domain event published after this point — no
// history is replayed.
func (h *EventsHandlers) GetEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		sendJSONResponse(w, http.StatusInternalServerError, NewInternalError("streaming not supported", r.URL.Path))
		return
	}

	// Checked before writing any response so a rejection can still be a
	// normal JSON error instead of an already-committed SSE stream.
	c, ok := h.hub.addClient()
	if !ok {
		sendJSONResponse(w, http.StatusServiceUnavailable, NewServiceUnavailableError("too many concurrent event stream connections", r.URL.Path))
		return
	}
	defer h.hub.removeClient(c)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	// SSE comment line: establishes the stream is live before any event is
	// due, and gives clients (and tests) something to observe immediately.
	fmt.Fprint(w, ": connected\n\n") //nolint:errcheck
	flusher.Flush()

	connectionID := id.NewID[client.Client]()
	h.hub.Publish(event.NewClientConnectedEvent(connectionID))
	defer h.hub.Publish(event.NewClientDisconnectedEvent(connectionID))

	for {
		select {
		case <-r.Context().Done():
			return
		case payload, ok := <-c.messages:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", payload) //nolint:errcheck
			flusher.Flush()
		}
	}
}
