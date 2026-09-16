// Package eventlog renders the global status bar (current time + "Events"
// button) and the collapsible server event log panel that it toggles. The
// panel subscribes to the server's SSE event stream for the lifetime of the
// app, independently of whether it is currently visible.
package eventlog

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

// maxEntries is the cap on how many events are kept in the client-side
// buffer. Oldest entries are dropped once the cap is reached, regardless of
// the active filter (the filter only affects what is displayed).
const maxEntries = 500

const filtersStorageKey = "eventlog:filters"

type logEntry struct {
	Time     string   `json:"time"`
	Category category `json:"category"`
	Text     string   `json:"text"`
}

type wireMessage struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	ID        string `json:"id"`
}

// Bar is the global status bar + event log panel component.
type Bar struct {
	app.Compo

	apiServerURL string

	now       string
	panelOpen bool
	entries   []logEntry
	filters   map[category]bool

	eventSource app.Value
	onMessage   app.Func
	onError     app.Func
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
	if b.eventSource != nil {
		b.eventSource.Call("close")
	}
}

func (b *Bar) tick(ctx app.Context) {
	b.now = time.Now().Format("15:04:05")
	ctx.After(time.Second, b.tick)
}

// connect opens the SSE connection and keeps it for the component's whole
// lifetime; the native EventSource auto-reconnects on its own after a drop,
// with no history replay on either side.
func (b *Bar) connect(ctx app.Context) {
	url := b.apiServerURL + "/api/v1/events"

	b.onMessage = app.FuncOf(func(this app.Value, args []app.Value) any {
		data := args[0].Get("data").String()
		ctx.Dispatch(func(ctx app.Context) {
			b.handleMessage(data)
		})
		return nil
	})

	es := app.Window().Get("EventSource").New(url)
	es.Set("onmessage", b.onMessage)
	b.eventSource = es
}

func (b *Bar) handleMessage(data string) {
	var msg wireMessage
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		return
	}

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
