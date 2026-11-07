package player

import (
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/internal/apiclient"
)

// eventSourceClosed is EventSource.CLOSED: the browser will not retry the
// stream, so the Player resyncs explicitly after a backoff (ADR 0020).
const eventSourceClosed = 2

// LiveSource is Operation's entry source: the browser's EventSource over the
// Project's Journal stream (GET …/journal/events?from=<position>, ADR 0020).
// It reports what the stream does and does not decide what the entries mean:
// the Player applies them and owns the resync. A browser retries a dropped
// stream by itself from the same position, which catches up exactly from the
// table (ADR 0019); once it gives up, the Player does a full resync.
//
// Outside a browser (the host engine of the UI tests) there is no
// EventSource, and Open follows nothing: a test supplies its own Source.
type LiveSource struct {
	project *apiclient.ProjectClient

	source    app.Value
	onMessage app.Func
	onOpen    app.Func
	onError   app.Func
	closed    bool
}

// NewLiveSource creates the live Journal source of one Project.
func NewLiveSource(project *apiclient.ProjectClient) *LiveSource {
	return &LiveSource{project: project}
}

// Open subscribes to the Project's Journal stream after position from,
// replacing a stream it had open.
func (s *LiveSource) Open(ctx app.Context, from int64, events Events) {
	s.Close()
	s.closed = false

	constructor := app.Window().Get("EventSource")
	if !constructor.Truthy() {
		return
	}

	s.onMessage = app.FuncOf(func(this app.Value, args []app.Value) any {
		data := args[0].Get("data").String()
		ctx.Dispatch(func(app.Context) {
			if s.closed || events.Entry == nil {
				return
			}
			if entry, ok := apiclient.ParseEntry(data); ok {
				events.Entry(entry)
			}
		})
		return nil
	})
	s.onOpen = app.FuncOf(func(app.Value, []app.Value) any {
		ctx.Dispatch(func(app.Context) {
			if !s.closed && events.Opened != nil {
				events.Opened()
			}
		})
		return nil
	})
	s.onError = app.FuncOf(func(app.Value, []app.Value) any {
		ctx.Dispatch(func(app.Context) {
			if s.closed || events.Lost == nil {
				return
			}
			// CONNECTING: the browser retries the same URL by itself.
			// CLOSED: it gave up, and the Player resyncs.
			events.Lost(s.source.Get("readyState").Int() == eventSourceClosed)
		})
		return nil
	})

	s.source = constructor.New(s.project.JournalEventsURL(from))
	s.source.Set("onmessage", s.onMessage)
	s.source.Set("onopen", s.onOpen)
	s.source.Set("onerror", s.onError)
}

// Close ends the stream; safe on nil and more than once.
func (s *LiveSource) Close() {
	if s == nil || s.closed {
		return
	}
	s.closed = true
	if s.source.Truthy() {
		s.source.Call("close")
	}
	if s.onMessage != nil {
		s.onMessage.Release()
		s.onOpen.Release()
		s.onError.Release()
	}
}
