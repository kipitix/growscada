package restapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	recordv0 "github.com/kipitix/growscada/contract/record/v0"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
)

// journalStreamSafetyInterval is how often the live Journal stream reads the
// table even without a wake-up: a tag_updated or revision_created that the
// stream missed (published before its subscription, or dropped with AtMostOnce
// delivery) must not leave the stream behind forever. Wake-ups normally make
// the wait shorter (task 42).
const journalStreamSafetyInterval = 5 * time.Second

// lastEventIDHeader is the SSE header a reconnecting client sends: the id of
// the last message it received, which for the Journal is the position to
// read after. The stream's start is ?from= first, then this header, then the
// Project's current position (task 42).
const lastEventIDHeader = "Last-Event-ID"

// JournalHandlers serves a Project's Journal on the Runtime node (task 42,
// ADR 0019): a page of entries and a live stream of them. Both are read-only
// and touch no Draft, so the stream names no client and holds no EditLock.
type JournalHandlers struct {
	journal runtimeapp.JournalService
	bus     eventbus.EventBus

	closeOnce sync.Once
	closed    chan struct{}
}

// NewJournalHandler creates the handlers over the Runtime node's Journal and
// the EventBus whose tag_updated and revision_created wake the live stream.
func NewJournalHandler(journal runtimeapp.JournalService, bus eventbus.EventBus) *JournalHandlers {
	return &JournalHandlers{journal: journal, bus: bus, closed: make(chan struct{})}
}

// Close ends every open Journal stream, so http.Server.Shutdown does not wait
// for them. Safe to call more than once.
func (h *JournalHandlers) Close() {
	h.closeOnce.Do(func() { close(h.closed) })
}

// GetJournal handles GET /projects/{project_id}/journal: a page of the
// Project's Journal entries, in the order of their positions. from is
// exclusive and defaults to 0; limit defaults to 100 and never exceeds 1000.
// A Project that is not live is 404, a position or page size the Journal
// cannot serve is 400.
func (h *JournalHandlers) GetJournal(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	from, ok := journalQueryPosition(w, r, apiv0.JournalQueryFrom, 0)
	if !ok {
		return
	}
	rawLimit := r.URL.Query().Get(apiv0.JournalQueryLimit)
	limit := runtimeapp.JournalPageSize
	if rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			sendProblem(w, NewBadRequest(fmt.Sprintf("%s: '%s' is not a number", apiv0.JournalQueryLimit, rawLimit), r.URL.Path))
			return
		}
		limit = parsed
	}
	entries, err := h.journal.Read(r.Context(), projectID, from, limit)
	if err != nil {
		sendError(w, r, err)
		return
	}
	if entries == nil {
		entries = []recordv0.Entry{}
	}
	sendJSONResponse(w, http.StatusOK, apiv0.JournalResponse{Entries: entries})
}

// GetJournalEvents handles GET /projects/{project_id}/journal/events: the
// Project's Journal as an SSE stream. The start is ?from= (which wins), else
// Last-Event-ID, else the Project's current position, so a stream opened
// without either carries new entries alone. Each message is the entry's
// position in id and the record contract's document in data, so a reconnect
// with Last-Event-ID catches up exactly by reading the table; the live tail is
// at-most-once, so two racing commits may reach the stream out of order or one
// may be missed (ADR 0019). A project that is not live is 404 and a bad
// position is 400, both before the stream opens.
func (h *JournalHandlers) GetJournalEvents(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	from, given, ok := journalStreamStart(w, r)
	if !ok {
		return
	}

	// The wake-ups are registered before the position is read: a commit
	// between the two must not be lost. They carry no data; the stream reads
	// the table.
	wake := make(chan struct{}, 1)
	notify := func(e event.Event) {
		content, ok := e.(project.ContentEvent)
		if !ok || content.ProjectID().UUID() != projectID {
			return
		}
		select {
		case wake <- struct{}{}:
		default: // a pending wake-up is enough: the stream catches up by position
		}
	}
	subscriptions := []eventbus.Subscription{
		h.bus.Subscribe(event.EventTypeTagUpdated, notify),
		h.bus.Subscribe(event.EventTypeRevisionCreated, notify),
	}
	defer func() {
		for _, subscription := range subscriptions {
			h.bus.Unsubscribe(subscription)
		}
	}()

	last := from
	if !given {
		position, err := h.journal.Position(r.Context(), projectID)
		if err != nil {
			sendError(w, r, err)
			return
		}
		last = position
	}

	// One page is read before the stream opens: a Project that is not live
	// (or a position it cannot serve) is still a Problem Details response,
	// not a stream that starts and ends at once.
	page, err := h.journal.Read(r.Context(), projectID, last, runtimeapp.JournalPageSize)
	if err != nil {
		sendError(w, r, err)
		return
	}

	if _, ok := w.(http.Flusher); !ok {
		sendProblem(w, NewInternalError("streaming not supported", r.URL.Path))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
		return
	}
	if err := http.NewResponseController(w).Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return
	}

	if err := h.writeJournalEntries(w, page); err != nil {
		return
	}
	if len(page) > 0 {
		last = page[len(page)-1].Seq
	}
	last, err = h.catchUp(r.Context(), w, projectID, last)
	if err != nil {
		return
	}

	ticker := time.NewTicker(journalStreamSafetyInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-h.closed:
			return
		case <-wake:
		case <-ticker.C:
		}
		last, err = h.catchUp(r.Context(), w, projectID, last)
		if err != nil {
			return
		}
	}
}

// catchUp reads the Project's Journal after last, page by page, until a page
// is empty, writing every entry, and returns the last position written. A
// failed read ends the stream: nothing can be written to it as a Problem
// Details any more.
func (h *JournalHandlers) catchUp(ctx context.Context, w http.ResponseWriter, projectID uuid.UUID, last int64) (int64, error) {
	for {
		page, err := h.journal.Read(ctx, projectID, last, runtimeapp.JournalPageSize)
		if err != nil {
			return last, err
		}
		if len(page) == 0 {
			return last, nil
		}
		if err := h.writeJournalEntries(w, page); err != nil {
			return last, err
		}
		last = page[len(page)-1].Seq
	}
}

// writeJournalEntries writes one SSE message per entry: its position as id and
// its record document as data. A message is written within eventWriteTimeout,
// so a client that stopped reading fails the stream instead of holding the
// handler.
func (h *JournalHandlers) writeJournalEntries(w http.ResponseWriter, entries []recordv0.Entry) error {
	rc := http.NewResponseController(w)
	for _, entry := range entries {
		payload, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		if err := rc.SetWriteDeadline(time.Now().Add(eventWriteTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return err
		}
		if _, err := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", entry.Seq, payload); err != nil {
			return err
		}
		if err := rc.Flush(); err != nil {
			return err
		}
	}
	return nil
}

// journalQueryPosition reads the position of a Journal read's query
// parameter: 0 when it is absent. A value that is not a non-negative number is
// answered with a 400, so the caller returns on ok == false.
func journalQueryPosition(w http.ResponseWriter, r *http.Request, name string, fallback int64) (int64, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, true
	}
	return parseJournalPosition(w, r, name, raw)
}

// journalStreamStart returns where the stream starts: ?from= wins over the
// Last-Event-ID header; without either the caller reads the Project's current
// position. A value that is not a non-negative number is a 400.
func journalStreamStart(w http.ResponseWriter, r *http.Request) (from int64, given, ok bool) {
	if raw := r.URL.Query().Get(apiv0.JournalQueryFrom); raw != "" {
		position, ok := parseJournalPosition(w, r, apiv0.JournalQueryFrom, raw)
		return position, true, ok
	}
	if raw := r.Header.Get(lastEventIDHeader); raw != "" {
		position, ok := parseJournalPosition(w, r, lastEventIDHeader, raw)
		return position, true, ok
	}
	return 0, false, true
}

// parseJournalPosition parses a Journal position; a negative or non-numeric
// one is answered with a 400.
func parseJournalPosition(w http.ResponseWriter, r *http.Request, name, raw string) (int64, bool) {
	position, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || position < 0 {
		sendProblem(w, NewBadRequest(fmt.Sprintf("%s: '%s' is not a Journal position", name, raw), r.URL.Path))
		return 0, false
	}
	return position, true
}
