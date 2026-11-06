package restapi

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/client"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
)

// eventWriteTimeout bounds one write to an SSE client. A client that stops
// reading (a dead TCP connection, a full send buffer) fails the write instead
// of holding its handler, and so http.Server.Shutdown, forever.
const eventWriteTimeout = 5 * time.Second

// EventsHandlers handles the SSE streams of domain events: the server's,
// optionally joined by one project's, one per project, and the Device
// Gateway's.
type EventsHandlers struct {
	hub      *eventHub
	projects application.ProjectService
	tokens   runtimeapp.DeviceTokenService
	locks    application.EditLockService
}

// NewEventsHandler creates the SSE event handler; a project's stream is
// opened only for a project projects finds. maxClients caps how many
// concurrent event streams the hub will accept — see eventHub.addClient.
func NewEventsHandler(bus eventbus.EventBus, projects application.ProjectService, maxClients int, opts ...EventsOption) *EventsHandlers {
	h := &EventsHandlers{hub: newEventHub(bus, maxClients), projects: projects}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// EventsOption configures the SSE event handler.
type EventsOption func(*EventsHandlers)

// WithDeviceTokens serves the Device Gateway's stream: a token authenticates
// it, and its Project's events are what it carries. Without it the stream
// answers 401: the handler serves no Runtime node.
func WithDeviceTokens(tokens runtimeapp.DeviceTokenService) EventsOption {
	return func(h *EventsHandlers) { h.tokens = tokens }
}

// WithEditLocks counts a stream that names its client (the client parameter)
// as presence of that client: while the stream is open, its EditLock is alive
// (ADR 0015). Without it no stream is presence.
func WithEditLocks(locks application.EditLockService) EventsOption {
	return func(h *EventsHandlers) { h.locks = locks }
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

// GetDeviceEvents handles GET /api/v0/device/events: the event stream of a
// Device Gateway, authenticated by its token. The token names the Device and
// so the Project; the stream carries that Project's configuration events —
// device_updated, environment_device_updated and their creations and
// deletions — on which the Gateway rereads its configuration, and not the Tag
// values. A token that is unknown or revoked is 401, so a Gateway stops
// instead of waiting. The stream is opened at once and never ends by itself:
// a Gateway rereads the configuration on every (re)connection, which is also
// how it learns of a rotation.
func (h *EventsHandlers) GetDeviceEvents(w http.ResponseWriter, r *http.Request) {
	token, ok := authenticateDevice(w, r, h.tokens)
	if !ok {
		return
	}
	h.stream(w, r, eventStreams{project: token.ProjectID, gateway: true})
}

// stream serves an SSE stream of streams (see eventHub.addClient).
func (h *EventsHandlers) stream(w http.ResponseWriter, r *http.Request, streams eventStreams) {
	// The client that follows the stream is presence: its EditLock survives
	// while the stream is open (ADR 0015). The parameter is optional.
	key, follows, ok := h.streamClient(w, r)
	if !ok {
		return
	}
	if follows {
		h.locks.Attach(key)
		defer h.locks.Detach(key)
	}

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

// streamClient is the client named by the stream's query parameter. A stream
// that names none, or a server without EditLocks, is no presence: follows is
// false. A parameter that is not a UUID has already been answered with a 400,
// so the caller returns on ok == false.
func (h *EventsHandlers) streamClient(w http.ResponseWriter, r *http.Request) (key editlock.Key, follows, ok bool) {
	raw := r.URL.Query().Get(apiv0.EventsQueryClient)
	if raw == "" || h.locks == nil {
		return editlock.Key{}, false, true
	}
	key, err := editlock.ParseKey(raw)
	if err != nil {
		sendProblem(w, NewBadRequest(fmt.Sprintf("%s: '%s' is not a valid UUID", apiv0.EventsQueryClient, raw), r.URL.Path))
		return editlock.Key{}, false, false
	}
	return key, true, true
}

// writeEvent writes one SSE message within eventWriteTimeout. A writer that
// cannot take a deadline is written to without one.
func writeEvent(w http.ResponseWriter, rc *http.ResponseController, payload []byte) error {
	return writeSSEMessage(w, rc, "", payload)
}

// writeSSEMessage writes one SSE message within eventWriteTimeout: the id line
// first when the message has an id (a stream that resumes by position carries
// one), then the data line and the blank line that ends the message. A writer
// that cannot take a deadline is written to without one.
func writeSSEMessage(w http.ResponseWriter, rc *http.ResponseController, messageID string, payload []byte) error {
	if err := rc.SetWriteDeadline(time.Now().Add(eventWriteTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	field := "data: "
	if messageID != "" {
		field = "id: " + messageID + "\ndata: "
	}
	if _, err := fmt.Fprintf(w, "%s%s\n\n", field, payload); err != nil {
		return err
	}
	return rc.Flush()
}
