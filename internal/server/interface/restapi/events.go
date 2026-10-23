package restapi

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/client"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
)

// eventWriteTimeout bounds one write to an SSE client. A client that stops
// reading (a dead TCP connection, a full send buffer) fails the write instead
// of holding its handler, and so http.Server.Shutdown, forever.
const eventWriteTimeout = 5 * time.Second

// EventsHandlers handles the SSE stream of domain events.
type EventsHandlers struct {
	hub *eventHub
}

// NewEventsHandler creates the SSE event handler. maxClients caps how many
// concurrent event streams the hub will accept — see eventHub.addClient.
func NewEventsHandler(bus eventbus.EventBus, maxClients int) *EventsHandlers {
	return &EventsHandlers{hub: newEventHub(bus, maxClients)}
}

// Close detaches the hub from the EventBus and ends every open event stream,
// so http.Server.Shutdown does not wait for them; later streams get 503.
func (h *EventsHandlers) Close() {
	h.hub.Close()
}

// GetEvents handles GET /api/v0/events. It upgrades the connection to an SSE
// stream and forwards every domain event published after this point — no
// history is replayed.
func (h *EventsHandlers) GetEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		sendProblem(w, NewInternalError("streaming not supported", r.URL.Path))
		return
	}

	// Checked before writing any response so a rejection can still be a
	// Problem Details response instead of an already-committed SSE stream.
	c, err := h.hub.addClient()
	if err != nil {
		sendProblem(w, NewServiceUnavailableError(err.Error(), r.URL.Path))
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
	h.hub.Publish(eventbus.NewClientConnectedEvent(connectionID))
	defer h.hub.Publish(eventbus.NewClientDisconnectedEvent(connectionID))

	rc := http.NewResponseController(w)
	for {
		select {
		case <-r.Context().Done():
			return
		case payload, ok := <-c.messages:
			if !ok {
				return
			}
			if err := writeEvent(w, rc, payload); err != nil {
				return
			}
		}
	}
}

// writeEvent writes one SSE message within eventWriteTimeout. A writer that
// cannot take a deadline is written to without one.
func writeEvent(w http.ResponseWriter, rc *http.ResponseController, payload []byte) error {
	if err := rc.SetWriteDeadline(time.Now().Add(eventWriteTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return err
	}
	return rc.Flush()
}
