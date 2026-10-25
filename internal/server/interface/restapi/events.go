package restapi

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/domain/client"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
)

// eventWriteTimeout bounds one write to an SSE client. A client that stops
// reading (a dead TCP connection, a full send buffer) fails the write instead
// of holding its handler, and so http.Server.Shutdown, forever.
const eventWriteTimeout = 5 * time.Second

// EventsHandlers handles the SSE streams of domain events: the server's,
// optionally joined by one project's, and one per project.
type EventsHandlers struct {
	hub      *eventHub
	projects application.ProjectService
}

// NewEventsHandler creates the SSE event handler; a project's stream is
// opened only for a project projects finds. maxClients caps how many
// concurrent event streams the hub will accept — see eventHub.addClient.
func NewEventsHandler(bus eventbus.EventBus, projects application.ProjectService, maxClients int) *EventsHandlers {
	return &EventsHandlers{hub: newEventHub(bus, maxClients), projects: projects}
}

// Close detaches the hub from the EventBus and ends every open event stream,
// so http.Server.Shutdown does not wait for them; later streams get 503.
func (h *EventsHandlers) Close() {
	h.hub.Close()
}

// GetEvents handles GET /api/v0/events: the server's stream, of projects,
// WidgetTypes and connections, joined by the stream of the project
// ?project= names, so one connection carries both. It upgrades the
// connection to an SSE stream and forwards every such event published after
// this point — no history is replayed. A project not found (deleted
// meanwhile) leaves the server's stream alone: its client learns of the
// deletion from that stream.
func (h *EventsHandlers) GetEvents(w http.ResponseWriter, r *http.Request) {
	streams := eventStreams{server: true}
	if raw := r.URL.Query().Get(apiv0.EventsQueryProject); raw != "" {
		projectID, err := uuid.Parse(raw)
		if err != nil {
			sendProblem(w, NewBadRequest(fmt.Sprintf("%s: '%s' is not a valid UUID", apiv0.EventsQueryProject, raw), r.URL.Path))
			return
		}
		switch _, err := h.projects.FindProjectByID(r.Context(), projectID); {
		case err == nil:
			streams.project = projectID
		case !errors.Is(err, project.ErrProjectNotFound):
			sendError(w, r, err)
			return
		}
	}
	h.stream(w, r, streams)
}

// GetProjectEvents handles GET /api/v0/projects/{project_id}/events: the
// stream of the project's content (Tags, Scenes, Widgets), like GetEvents.
// The project must exist when the stream opens; once it is deleted the
// stream stays open, with nothing more to carry.
func (h *EventsHandlers) GetProjectEvents(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	if _, err := h.projects.FindProjectByID(r.Context(), projectID); err != nil {
		sendError(w, r, err)
		return
	}
	h.stream(w, r, eventStreams{project: projectID})
}

// stream serves an SSE stream of streams (see eventHub.addClient).
func (h *EventsHandlers) stream(w http.ResponseWriter, r *http.Request, streams eventStreams) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		sendProblem(w, NewInternalError("streaming not supported", r.URL.Path))
		return
	}

	// Checked before writing any response so a rejection can still be a
	// Problem Details response instead of an already-committed SSE stream.
	c, err := h.hub.addClient(streams)
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
