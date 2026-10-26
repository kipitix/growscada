// Package eventlog renders the global status bar (current time + "Events"
// button) and the collapsible server event log panel that it toggles. The
// panel subscribes to the server's SSE event stream for the lifetime of the
// app, independently of whether it is currently visible: one connection
// carries the server's own events and those of the selected project
// (SetProject), so a tab holds a single connection.
package eventlog

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
)

// maxEntries is the cap on how many events are kept in the client-side
// buffer. Oldest entries are dropped once the cap is reached, regardless of
// the active filter (the filter only affects what is displayed).
const maxEntries = 500

const filtersStorageKey = "eventlog:filters"

// ActionServerEvent is the go-app action the Bar fires for every event received
// from the server's SSE streams; its value is the apiv0.Event. Its Tag, if
// any, is the state of the Tag the event is about (apiclient.ParseEvent drops
// one that is not). Components that show server data handle it (ctx.Handle) to refresh themselves, so the app
// keeps a single SSE connection. An event of a project's content is of the
// project selected when it arrives: the connection of a project no longer
// selected is closed, and what it still delivers is dropped.
const ActionServerEvent = "eventlog.server-event"

// EventResync is the Type of the synthetic apiv0.Event the Bar fires each time
// the connection to the server's events (projects, WidgetTypes) is
// (re)established: the first time, after a loss, and after a switch of
// project made while the server's events were not coming in. The server
// subscribes a client before it answers, so from this point on no event is
// missed; but events published before it (while disconnected, or before the
// first connection was up) are not replayed. A component that shows the
// server's data therefore reloads it on EventResync. A switch of project
// while connected keeps the old connection until the new one is open, so
// the server's events go on and only EventProjectResync is fired.
const EventResync = "resync"

// EventProjectResync is EventResync of the events of the selected project
// (its Scenes, Widgets and Tags): fired (after EventResync, if that is fired
// too) each time the connection is (re)established while a project is
// selected, a switch of project included.
const EventProjectResync = "project_resync"

// StateDisconnected is the go-app state (ctx.ObserveState) holding, as a
// bool, whether the SSE connection to the server is lost. It turns true on
// the loss and false again once the browser has reconnected. It drives the
// "stale data" indication only: the reload after a reconnection is triggered
// by EventResync and EventProjectResync. Unset (false) until the first loss.
const StateDisconnected = "eventlog.disconnected"

type logEntry struct {
	Time     string   `json:"time"`
	Category category `json:"category"`
	Text     string   `json:"text"`
}

// Bar is the global status bar + event log panel component.
type Bar struct {
	app.Compo

	apiServerURL string
	// compoCtx is the component's own context, set with mounted in OnMount.
	compoCtx app.Context
	mounted  bool

	now       string
	panelOpen bool
	entries   []logEntry
	filters   map[category]bool

	// stream carries the server's events and, unless projectID is uuid.Nil,
	// those of the project projectID.
	stream             *eventStream
	projectID          uuid.UUID
	connectionDegraded bool
}

// eventStream is one SSE connection, kept open by the Bar until closed.
type eventStream struct {
	url string
	// resyncs are the Types of the synthetic events fired when it opens.
	resyncs []string
	// connected is whether it is open now: from its open to its next error.
	connected bool
	// predecessor is the connection of the project selected before, kept
	// while this one is not open yet, so the server's events go on; it is
	// closed once this one opens, which then fires no EventResync
	// (skipServerResync) unless the predecessor was lost meanwhile.
	predecessor      *eventStream
	skipServerResync bool
	// successor is set on a predecessor: superseded by it, it delivers only
	// the server's events, no longer those of its project.
	successor          *eventStream
	source             app.Value
	onMessage          app.Func
	onError            app.Func
	onOpen             app.Func
	reconnectScheduled bool
	closed             bool
}

func NewBar(apiServerURL string) *Bar {
	return &Bar{apiServerURL: apiServerURL}
}

func (b *Bar) OnMount(ctx app.Context) {
	b.compoCtx, b.mounted = ctx, true
	b.now = time.Now().Format("15:04:05")

	b.filters = defaultFilters()
	var saved map[category]bool
	if err := ctx.LocalStorage().Get(filtersStorageKey, &saved); err == nil {
		for _, c := range allCategories {
			if v, ok := saved[c]; ok {
				b.filters[c] = v
			}
		}
	}

	ctx.After(time.Second, b.tick)
	b.stream = b.connectStream(ctx)
}

// OnDismount closes the connections; a SetProject until the next OnMount
// only remembers the project, which that OnMount connects to.
func (b *Bar) OnDismount() {
	b.mounted = false
	b.stream.close()
	if b.stream != nil {
		b.stream.predecessor.close()
	}
}

// SetProject switches the connection to the events of the project id
// (uuid.Nil: none) by opening a new one; before the Bar is mounted it only
// remembers it. The new connection fires EventProjectResync once it is open
// (while a project is selected): Project and Operation load the new
// project's content on it only. If the server's events are coming in, the
// open connection carrying them stays until the new one is open, which then
// fires no EventResync; otherwise it fires EventResync first.
func (b *Bar) SetProject(id uuid.UUID) {
	if id == b.projectID {
		return
	}
	b.projectID = id
	if !b.mounted {
		return
	}
	prev := b.stream
	b.stream = b.connectStream(b.compoCtx)
	carrier := prev
	if prev != nil && !prev.connected {
		// Never open (or lost): nothing comes through it. Its own
		// predecessor may still carry the server's events.
		carrier = prev.predecessor
		prev.predecessor = nil
		prev.close()
	}
	if carrier == nil || carrier.closed {
		return
	}
	if !carrier.connected {
		carrier.close()
		return
	}
	carrier.successor = b.stream
	b.stream.predecessor, b.stream.skipServerResync = carrier, true
}

// connectStream connects to the server's events joined by those of the
// project selected, if any.
func (b *Bar) connectStream(ctx app.Context) *eventStream {
	url := b.apiServerURL + apiv0.PathPrefix + "/events"
	resyncs := []string{EventResync}
	if b.projectID != uuid.Nil {
		url += "?" + apiv0.EventsQueryProject + "=" + b.projectID.String()
		resyncs = append(resyncs, EventProjectResync)
	}
	return b.connect(ctx, url, resyncs)
}

func (b *Bar) tick(ctx app.Context) {
	b.now = time.Now().Format("15:04:05")
	ctx.After(time.Second, b.tick)
}

// reconnectDelay is how long the Bar waits before reopening an SSE
// connection the browser has given up on.
const reconnectDelay = 2 * time.Second

// eventSourceClosed is EventSource.CLOSED: the browser will not reconnect.
const eventSourceClosed = 2

// connect opens an SSE connection to url and keeps it until it is closed,
// with no history replay on either side. The native EventSource retries a
// dropped connection by itself, but gives up (CLOSED) when the server goes
// away mid-stream; handleError then reopens it. Callbacks of a stream closed
// meanwhile are ignored.
func (b *Bar) connect(ctx app.Context, url string, resyncs []string) *eventStream {
	st := &eventStream{url: url, resyncs: resyncs}
	st.onMessage = app.FuncOf(func(this app.Value, args []app.Value) any {
		data := args[0].Get("data").String()
		ctx.Dispatch(func(ctx app.Context) {
			if !st.closed {
				b.handleMessage(ctx, st, data)
			}
		})
		return nil
	})

	st.onError = app.FuncOf(func(this app.Value, args []app.Value) any {
		ctx.Dispatch(func(ctx app.Context) {
			if !st.closed {
				b.handleError(ctx, st)
			}
		})
		return nil
	})

	st.onOpen = app.FuncOf(func(this app.Value, args []app.Value) any {
		ctx.Dispatch(func(ctx app.Context) {
			if !st.closed {
				b.handleOpen(ctx, st)
			}
		})
		return nil
	})

	st.open()
	return st
}

func (st *eventStream) open() {
	es := app.Window().Get("EventSource").New(st.url)
	es.Set("onmessage", st.onMessage)
	es.Set("onerror", st.onError)
	es.Set("onopen", st.onOpen)
	st.source = es
}

// close ends the stream for good; safe on nil and more than once.
func (st *eventStream) close() {
	if st == nil || st.closed {
		return
	}
	st.closed = true
	st.source.Call("close")
	st.onMessage.Release()
	st.onError.Release()
	st.onOpen.Release()
}

// reopenIfClosed schedules a new connection when the browser has stopped
// retrying the stream, and keeps retrying while the server is down.
func (b *Bar) reopenIfClosed(ctx app.Context, st *eventStream) {
	if st.reconnectScheduled || st.source.Get("readyState").Int() != eventSourceClosed {
		return
	}
	st.reconnectScheduled = true
	ctx.After(reconnectDelay, func(ctx app.Context) {
		st.reconnectScheduled = false
		if st.closed {
			return
		}
		st.source.Call("close")
		st.open()
	})
}

func (b *Bar) handleMessage(ctx app.Context, st *eventStream, data string) {
	// A message proves the stream is live, so a prior error is stale.
	b.markConnected(ctx)

	ev, ok := apiclient.ParseEvent(data)
	if !ok || !forwards(st.successor != nil, ev.Type) {
		return
	}
	b.addEntry(ev)
	ctx.NewActionWithValue(ActionServerEvent, ev)
}

// addEntry logs ev, its id as it came.
func (b *Bar) addEntry(ev apiv0.Event) {
	cat := categoryOther
	text := ev.Type
	if info, ok := eventTypeInfoByType[ev.Type]; ok {
		cat = info.category
		text = info.describe(ev.ID)
	} else if ev.ID != "" {
		text = fmt.Sprintf("%s %s", ev.Type, ev.ID)
	}

	entry := logEntry{
		Time:     displayTime(ev.Timestamp),
		Category: cat,
		Text:     text,
	}

	b.entries = append([]logEntry{entry}, b.entries...)
	if len(b.entries) > maxEntries {
		b.entries = b.entries[:maxEntries]
	}
}

// handleError responds to a stream's "error" event (dropped connection,
// unreachable server, ...). The connection is retried — by the browser, or
// by reopenIfClosed once the browser gives up — and the event re-fires on
// every failed attempt, so shouldNotifyError throttles it to one toast per
// outage instead of one per retry.
func (b *Bar) handleError(ctx app.Context, st *eventStream) {
	st.connected = false
	if next := st.successor; next != nil {
		// A superseded connection lost: its successor, once open, resyncs
		// the server's data too; it is not reopened.
		next.predecessor, next.skipServerResync = nil, false
		st.close()
		return
	}
	b.reopenIfClosed(ctx, st)
	notify, degraded := shouldNotifyError(b.connectionDegraded)
	b.connectionDegraded = degraded
	if notify {
		ctx.SetState(StateDisconnected, true)
		ctx.NewActionWithValue(toast.ActionAdd, toast.NetworkError(
			errors.New("lost connection to the event stream; retrying automatically"),
		))
	}
}

// handleOpen responds to a stream being (re)established: events published
// before it are lost, so the components showing its data are told to resync.
func (b *Bar) handleOpen(ctx app.Context, st *eventStream) {
	b.markConnected(ctx)
	st.connected = true
	if prev := st.predecessor; prev != nil {
		st.predecessor = nil
		prev.close()
	}
	for _, resync := range resyncsOnOpen(st.resyncs, st.skipServerResync) {
		ctx.NewActionWithValue(ActionServerEvent, apiv0.Event{Type: resync})
	}
	st.skipServerResync = false // a later reopen follows a loss
}

// resyncsOnOpen returns the resyncs a connection fires as it opens: all of
// them, but EventResync when the server's events went on meanwhile through
// the connection it took over from (skipServer).
func resyncsOnOpen(resyncs []string, skipServer bool) []string {
	if !skipServer {
		return resyncs
	}
	return slices.DeleteFunc(slices.Clone(resyncs), func(r string) bool { return r == EventResync })
}

// forwards reports whether an event of the given type a connection received
// is delivered: a superseded one (its project no longer selected) delivers
// only the server's events, not those of its project's content.
func forwards(superseded bool, eventType string) bool {
	return !superseded || !isProjectContent(eventType)
}

// markConnected leaves the degraded state, announcing the reconnection to
// the components observing StateDisconnected.
func (b *Bar) markConnected(ctx app.Context) {
	if !b.connectionDegraded {
		return
	}
	b.connectionDegraded = false
	ctx.SetState(StateDisconnected, false)
}

// shouldNotifyError decides whether an EventSource error should surface a
// toast, given whether the connection was already known to be degraded.
// Only the transition into "degraded" notifies.
func shouldNotifyError(wasDegraded bool) (notify, degraded bool) {
	return !wasDegraded, true
}

// displayTime renders the server's RFC3339 event timestamp in the browser's
// local timezone. Falls back to the receipt time if parsing fails.
func displayTime(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return time.Now().Format("15:04:05")
	}
	return t.Local().Format("15:04:05")
}

func (b *Bar) toggleFilter(ctx app.Context, c category) {
	b.filters[c] = !b.filters[c]
	ctx.LocalStorage().Set(filtersStorageKey, b.filters)
}

func (b *Bar) togglePanel() {
	b.panelOpen = !b.panelOpen
}
