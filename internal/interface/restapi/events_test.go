package restapi_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/domain/event"
	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/scene"
	"github.com/kipitix/growscada/internal/domain/tag"
	"github.com/kipitix/growscada/internal/interface/restapi"
)

// blockingResponseWriter lets a test simulate an SSE client whose second
// write (its first *event*, after the initial ": connected" comment) stalls
// forever, e.g. a dead TCP connection — so the handler's backpressure
// handling can be exercised without a real network client.
type blockingResponseWriter struct {
	header     http.Header
	mu         sync.Mutex
	buf        bytes.Buffer
	writeCount int
	startOnce  sync.Once
	started    chan struct{}
	unblock    chan struct{}
}

func newBlockingResponseWriter() *blockingResponseWriter {
	return &blockingResponseWriter{
		header:  make(http.Header),
		started: make(chan struct{}),
		unblock: make(chan struct{}),
	}
}

func (w *blockingResponseWriter) Header() http.Header { return w.header }
func (w *blockingResponseWriter) WriteHeader(int)     {}

func (w *blockingResponseWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.writeCount++
	n := w.writeCount
	w.mu.Unlock()

	if n == 2 {
		w.startOnce.Do(func() { close(w.started) })
		<-w.unblock
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *blockingResponseWriter) Flush() {}

func runEventsHandler(t *testing.T, h *restapi.EventsHandlers, w http.ResponseWriter) (cancel func(), done <-chan struct{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	ctx, cancelFn := context.WithCancel(req.Context())
	req = req.WithContext(ctx)

	doneCh := make(chan struct{})
	go func() {
		h.GetEvents(w, req)
		close(doneCh)
	}()
	return cancelFn, doneCh
}

func waitFor(t *testing.T, d time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}

func TestEventsHandlers_GetEvents_SetsSSEHeaders(t *testing.T) {
	bus := event.NewEventBus()
	h := restapi.NewEventsHandler(bus)

	rec := httptest.NewRecorder()
	cancel, done := runEventsHandler(t, h, rec)
	waitFor(t, time.Second, func() bool { return strings.Contains(rec.Body.String(), ": connected") })
	cancel()
	<-done

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}
}

func TestEventsHandlers_GetEvents_StreamsPublishedEvent(t *testing.T) {
	bus := event.NewEventBus()
	h := restapi.NewEventsHandler(bus)

	rec := httptest.NewRecorder()
	cancel, done := runEventsHandler(t, h, rec)
	waitFor(t, time.Second, func() bool { return strings.Contains(rec.Body.String(), ": connected") })

	tagID := id.NewID[tag.Tag]()
	bus.Publish(event.NewTagCreatedEvent(tagID))

	waitFor(t, time.Second, func() bool { return strings.Contains(rec.Body.String(), "tag_created") })
	cancel()
	<-done

	body := rec.Body.String()
	if !strings.Contains(body, `"type":"tag_created"`) {
		t.Errorf("expected body to contain tag_created event, got %q", body)
	}
	if !strings.Contains(body, tagID.String()) {
		t.Errorf("expected body to contain tag id %s, got %q", tagID, body)
	}
}

func TestEventsHandlers_GetEvents_PublishesConnectAndDisconnectEvents(t *testing.T) {
	bus := event.NewEventBus()
	h := restapi.NewEventsHandler(bus)

	var mu sync.Mutex
	var seen []event.EventType
	bus.Subscribe(event.EventTypeClientConnected, func(e event.Event) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, e.Type())
	})
	bus.Subscribe(event.EventTypeClientDisconnected, func(e event.Event) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, e.Type())
	})

	rec := httptest.NewRecorder()
	cancel, done := runEventsHandler(t, h, rec)
	waitFor(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(seen) >= 1
	})
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 || seen[0] != event.EventTypeClientConnected || seen[1] != event.EventTypeClientDisconnected {
		t.Errorf("expected [connected, disconnected], got %v", seen)
	}
}

func TestEventsHandlers_GetEvents_MultipleClientsEachReceiveBroadcast(t *testing.T) {
	bus := event.NewEventBus()
	h := restapi.NewEventsHandler(bus)

	rec1 := httptest.NewRecorder()
	cancel1, done1 := runEventsHandler(t, h, rec1)
	rec2 := httptest.NewRecorder()
	cancel2, done2 := runEventsHandler(t, h, rec2)

	waitFor(t, time.Second, func() bool {
		return strings.Contains(rec1.Body.String(), ": connected") && strings.Contains(rec2.Body.String(), ": connected")
	})

	bus.Publish(event.NewSceneCreatedEvent(id.NewID[scene.Scene]()))

	waitFor(t, time.Second, func() bool {
		return strings.Contains(rec1.Body.String(), "scene_created") && strings.Contains(rec2.Body.String(), "scene_created")
	})

	cancel1()
	cancel2()
	<-done1
	<-done2
}

// TestEventsHandlers_GetEvents_SlowClientIsDroppedWithoutBlockingPublish is
// the key architectural guarantee: a client that never drains its buffer
// must not block EventBus.Publish for the rest of the application, and must
// eventually be disconnected once its buffer overflows.
func TestEventsHandlers_GetEvents_SlowClientIsDroppedWithoutBlockingPublish(t *testing.T) {
	bus := event.NewEventBus()
	h := restapi.NewEventsHandler(bus)

	w := newBlockingResponseWriter()
	cancel, done := runEventsHandler(t, h, w)
	defer cancel()

	// Registering the client (hub.addClient) races with this goroutine, so
	// keep publishing single events until one actually reaches the client's
	// Write call (proving it was registered) and stalls there.
	deadline := time.After(time.Second)
publishUntilStarted:
	for {
		bus.Publish(event.NewTagCreatedEvent(id.NewID[tag.Tag]()))
		select {
		case <-w.started:
			break publishUntilStarted
		case <-deadline:
			t.Fatal("handler never reached its first event write")
		case <-time.After(5 * time.Millisecond):
		}
	}

	// Flood well past the per-client buffer capacity. Publish must not block
	// even though the client's consumer is stuck in Write.
	publishDone := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			bus.Publish(event.NewTagCreatedEvent(id.NewID[tag.Tag]()))
		}
		close(publishDone)
	}()

	select {
	case <-publishDone:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on a slow/stalled SSE client")
	}

	// Release the stalled write; the handler should drain and exit because
	// the hub closed its channel once the buffer overflowed.
	close(w.unblock)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler did not exit after its client was dropped for overflow")
	}
}
