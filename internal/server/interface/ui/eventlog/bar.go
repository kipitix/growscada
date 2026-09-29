// Package eventlog renders the global status bar (current time + "Events"
// button) and the collapsible server event log panel that it toggles. The
// panel subscribes to the server's SSE event stream for the lifetime of the
// app, independently of whether it is currently visible.
package eventlog

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
)

// maxEntries is the cap on how many events are kept in the client-side
// buffer. Oldest entries are dropped once the cap is reached, regardless of
// the active filter (the filter only affects what is displayed).
const maxEntries = 500

const filtersStorageKey = "eventlog:filters"

// ActionServerEvent is the go-app action the Bar fires for every event received
// from the server's SSE stream; its value is a ServerEvent. Components that
// show server data handle it (ctx.Handle) to refresh themselves, so the app
// keeps a single SSE connection.
const ActionServerEvent = "eventlog.server-event"

// StateDisconnected is the go-app state (ctx.ObserveState) holding, as a
// bool, whether the SSE connection to the server is lost. It turns true on
// the loss and false again once the browser has reconnected; events published
// meanwhile are not replayed, so a component that shows server data reloads it
// when the state turns false. Unset (false) until the first loss.
const StateDisconnected = "eventlog.disconnected"

// ServerEvent is a domain event received from the server's SSE stream.
type ServerEvent struct {
	Type string // EventType name, e.g. "tag_created"
	ID   string // ID of the affected aggregate/client, may be empty
	// Tag is the Tag's full new state (the JSON of GET /api/v1/tags/{id}),
	// carried by tag_updated only; nil otherwise.
	Tag json.RawMessage
}

type logEntry struct {
	Time     string   `json:"time"`
	Category category `json:"category"`
	Text     string   `json:"text"`
}

type wireMessage struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	ID        string          `json:"id"`
	Tag       json.RawMessage `json:"tag"`
}

// Bar is the global status bar + event log panel component.
type Bar struct {
	app.Compo

	apiServerURL string

	now       string
	panelOpen bool
	entries   []logEntry
	filters   map[category]bool

	eventSource        app.Value
	onMessage          app.Func
	onError            app.Func
	onOpen             app.Func
	reconnectScheduled bool
	connectionDegraded bool
}

func NewBar(apiServerURL string) *Bar {
	return &Bar{apiServerURL: apiServerURL}
}

func (b *Bar) OnMount(ctx app.Context) {
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
	b.connect(ctx)
}

func (b *Bar) OnDismount() {
	if b.onMessage != nil {
		b.onMessage.Release()
	}
	if b.onError != nil {
		b.onError.Release()
	}
	if b.onOpen != nil {
		b.onOpen.Release()
	}
	if b.eventSource != nil {
		b.eventSource.Call("close")
	}
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

// connect opens the SSE connection and keeps it for the component's whole
// lifetime, with no history replay on either side. The native EventSource
// retries a dropped connection by itself, but gives up (CLOSED) when the
// server goes away mid-stream; handleError then reopens it.
func (b *Bar) connect(ctx app.Context) {
	b.onMessage = app.FuncOf(func(this app.Value, args []app.Value) any {
		data := args[0].Get("data").String()
		ctx.Dispatch(func(ctx app.Context) {
			b.handleMessage(ctx, data)
		})
		return nil
	})

	b.onError = app.FuncOf(func(this app.Value, args []app.Value) any {
		ctx.Dispatch(func(ctx app.Context) {
			b.handleError(ctx)
		})
		return nil
	})

	b.onOpen = app.FuncOf(func(this app.Value, args []app.Value) any {
		ctx.Dispatch(func(ctx app.Context) {
			b.handleOpen(ctx)
		})
		return nil
	})

	b.openEventSource()
}

func (b *Bar) openEventSource() {
	es := app.Window().Get("EventSource").New(b.apiServerURL + "/api/v1/events")
	es.Set("onmessage", b.onMessage)
	es.Set("onerror", b.onError)
	es.Set("onopen", b.onOpen)
	b.eventSource = es
}

// reopenIfClosed schedules a new connection when the browser has stopped
// retrying the current one, and keeps retrying while the server is down.
func (b *Bar) reopenIfClosed(ctx app.Context) {
	if b.reconnectScheduled || b.eventSource.Get("readyState").Int() != eventSourceClosed {
		return
	}
	b.reconnectScheduled = true
	ctx.After(reconnectDelay, func(ctx app.Context) {
		b.reconnectScheduled = false
		b.eventSource.Call("close")
		b.openEventSource()
	})
}

func (b *Bar) handleMessage(ctx app.Context, data string) {
	// A message proves the stream is live, so a prior error is stale.
	b.markConnected(ctx)

	var msg wireMessage
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		return
	}

	ctx.NewActionWithValue(ActionServerEvent, ServerEvent{Type: msg.Type, ID: msg.ID, Tag: msg.Tag})

	cat := categoryOther
	text := msg.Type
	if info, ok := eventTypeInfoByType[msg.Type]; ok {
		cat = info.category
		text = fmt.Sprintf(info.format, msg.ID)
	} else if msg.ID != "" {
		text = fmt.Sprintf("%s %s", msg.Type, msg.ID)
	}

	entry := logEntry{
		Time:     displayTime(msg.Timestamp),
		Category: cat,
		Text:     text,
	}

	b.entries = append([]logEntry{entry}, b.entries...)
	if len(b.entries) > maxEntries {
		b.entries = b.entries[:maxEntries]
	}
}

// handleError responds to the SSE connection's "error" event (dropped
// connection, unreachable server, ...). The connection is retried — by the
// browser, or by reopenIfClosed once the browser gives up — and the event
// re-fires on every failed attempt, so shouldNotifyError throttles it to one
// toast per outage instead of one per retry.
func (b *Bar) handleError(ctx app.Context) {
	b.reopenIfClosed(ctx)
	notify, degraded := shouldNotifyError(b.connectionDegraded)
	b.connectionDegraded = degraded
	if notify {
		ctx.SetState(StateDisconnected, true)
		ctx.NewActionWithValue(toast.ActionAdd, toast.NetworkError(
			errors.New("lost connection to the event stream; retrying automatically"),
		))
	}
}

// handleOpen responds to the SSE connection being (re)established.
func (b *Bar) handleOpen(ctx app.Context) {
	b.markConnected(ctx)
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
