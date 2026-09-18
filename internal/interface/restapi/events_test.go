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

// syncRecorder wraps httptest.ResponseRecorder with a mutex. The handler
// under test writes to it from its own goroutine (started by
// runEventsHandler) while the test goroutine polls its body/headers via
// waitFor; httptest.ResponseRecorder gives no such guarantee on its own,
// since its Write/Body/Header aren't synchronized against concurrent use.
type syncRecorder struct {
	mu  sync.Mutex
	rec *httptest.ResponseRecorder
}

func newSyncRecorder() *syncRecorder {
	return &syncRecorder{rec: httptest.NewRecorder()}
}

func (s *syncRecorder) Header() http.Header {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rec.Header()
}

func (s *syncRecorder) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rec.Write(p)
}

func (s *syncRecorder) WriteHeader(code int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec.WriteHeader(code)
}

func (s *syncRecorder) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec.Flush()
}

// body returns the response body accumulated so far; safe to call
// concurrently with the handler still writing.
func (s *syncRecorder) body() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rec.Body.String()
}

func (s *syncRecorder) code() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rec.Code
}

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
	h := restapi.NewEventsHandler(bus, 100)

	rec := newSyncRecorder()
	cancel, done := runEventsHandler(t, h, rec)
	waitFor(t, time.Second, func() bool { return strings.Contains(rec.body(), ": connected") })
	cancel()
	<-done

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}
}

func TestEventsHandlers_GetEvents_StreamsPublishedEvent(t *testing.T) {
	bus := event.NewEventBus()
	h := restapi.NewEventsHandler(bus, 100)

	rec := newSyncRecorder()
	cancel, done := runEventsHandler(t, h, rec)
	waitFor(t, time.Second, func() bool { return strings.Contains(rec.body(), ": connected") })

	tagID := id.NewID[tag.Tag]()
	bus.Publish(event.NewTagCreatedEvent(tagID))

	waitFor(t, time.Second, func() bool { return strings.Contains(rec.body(), "tag_created") })
	cancel()
	<-done

	body := rec.body()
	if !strings.Contains(body, `"type":"tag_created"`) {
		t.Errorf("expected body to contain tag_created event, got %q", body)
	}
	if !strings.Contains(body, tagID.String()) {
		t.Errorf("expected body to contain tag id %s, got %q", tagID, body)
	}
}

func TestEventsHandlers_GetEvents_PublishesConnectAndDisconnectEvents(t *testing.T) {
	bus := event.NewEventBus()
	h := restapi.NewEventsHandler(bus, 100)

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

	rec := newSyncRecorder()
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
	h := restapi.NewEventsHandler(bus, 100)

	rec1 := newSyncRecorder()
	cancel1, done1 := runEventsHandler(t, h, rec1)
	rec2 := newSyncRecorder()
	cancel2, done2 := runEventsHandler(t, h, rec2)

	waitFor(t, time.Second, func() bool {
		return strings.Contains(rec1.body(), ": connected") && strings.Contains(rec2.body(), ": connected")
	})

	bus.Publish(event.NewSceneCreatedEvent(id.NewID[scene.Scene]()))

	waitFor(t, time.Second, func() bool {
		return strings.Contains(rec1.body(), "scene_created") && strings.Contains(rec2.body(), "scene_created")
	})

	cancel1()
	cancel2()
	<-done1
	<-done2
}

// TestEventsHandlers_Publish_NoClientsDoesNotAllocate guards the fix for the
// CODE_REVIEW.md finding that eventHub.broadcast used to marshal every
// domain event to JSON synchronously in the publishing goroutine (i.e. the
// REST request path) even with zero SSE tabs open. No GetEvents call is
// made here, so the hub never gains a client.
func TestEventsHandlers_Publish_NoClientsDoesNotAllocate(t *testing.T) {
	bus := event.NewEventBus()
	_ = restapi.NewEventsHandler(bus, 100) // subscribes the hub to bus; no client connects

	e := event.NewTagCreatedEvent(id.NewID[tag.Tag]())

	allocs := testing.AllocsPerRun(200, func() {
		bus.Publish(e)
	})

	if allocs > 0 {
		t.Errorf("expected zero allocations broadcasting with no SSE clients, got %.2f allocs/op", allocs)
	}
}

// BenchmarkEventsHandlers_Publish_NoClients measures the same no-client
// broadcast path for manual profiling; run with `make bench`.
func BenchmarkEventsHandlers_Publish_NoClients(b *testing.B) {
	bus := event.NewEventBus()
	_ = restapi.NewEventsHandler(bus, 100)

	e := event.NewTagCreatedEvent(id.NewID[tag.Tag]())

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Publish(e)
	}
}

// TestEventsHandlers_Close_StopsForwardingNewEvents guards the fix for the
// CODE_REVIEW.md finding that eventHub.Close() was never wired into the
// server's graceful shutdown. It asserts Close()'s actual effect: once
// called, the hub is unsubscribed from the EventBus, so events published
// afterwards never reach an already-connected client — even though, per
// eventHub.Close's contract, the connection itself is left open until its
// own request context is canceled.
func TestEventsHandlers_Close_StopsForwardingNewEvents(t *testing.T) {
	bus := event.NewEventBus()
	h := restapi.NewEventsHandler(bus, 100)

	rec := newSyncRecorder()
	cancel, done := runEventsHandler(t, h, rec)
	waitFor(t, time.Second, func() bool { return strings.Contains(rec.body(), ": connected") })

	tagID := id.NewID[tag.Tag]()
	bus.Publish(event.NewTagCreatedEvent(tagID))
	waitFor(t, time.Second, func() bool { return strings.Contains(rec.body(), "tag_created") })

	h.Close()

	sceneID := id.NewID[scene.Scene]()
	bus.Publish(event.NewSceneCreatedEvent(sceneID))

	// There is no positive signal to wait for here — Close() means this
	// event should never arrive. EventBus.Publish calls subscribers
	// synchronously, so if the hub were still subscribed the client's
	// buffered channel would already hold the message by the time Publish
	// returns, and the handler's read loop would drain it into rec within
	// microseconds; a short fixed wait is enough to catch a regression.
	time.Sleep(50 * time.Millisecond)
	if strings.Contains(rec.body(), "scene_created") {
		t.Error("expected no events to be forwarded after Close(), but scene_created was received")
	}

	cancel()
	<-done
}

// TestEventsHandlers_GetEvents_RejectsBeyondMaxClients guards the fix for
// the CODE_REVIEW.md finding that GET /api/v1/events accepted an unbounded
// number of concurrent SSE connections. A small maxClients (2) keeps the
// test from needing to open 100 real connections.
func TestEventsHandlers_GetEvents_RejectsBeyondMaxClients(t *testing.T) {
	bus := event.NewEventBus()
	h := restapi.NewEventsHandler(bus, 2)

	rec1 := newSyncRecorder()
	cancel1, done1 := runEventsHandler(t, h, rec1)
	rec2 := newSyncRecorder()
	cancel2, done2 := runEventsHandler(t, h, rec2)
	waitFor(t, time.Second, func() bool {
		return strings.Contains(rec1.body(), ": connected") && strings.Contains(rec2.body(), ": connected")
	})

	rec3 := newSyncRecorder()
	cancel3, done3 := runEventsHandler(t, h, rec3)
	<-done3
	cancel3()

	if rec3.code() != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when over the connection limit, got %d", rec3.code())
	}
	if ct := rec3.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected a JSON problem response, got Content-Type %q, body %q", ct, rec3.body())
	}

	// Freeing a slot must let a new connection through.
	cancel1()
	<-done1

	rec4 := newSyncRecorder()
	cancel4, done4 := runEventsHandler(t, h, rec4)
	waitFor(t, time.Second, func() bool { return strings.Contains(rec4.body(), ": connected") })
	cancel4()
	<-done4

	cancel2()
	<-done2
}

// TestEventsHandlers_GetEvents_SlowClientIsDroppedWithoutBlockingPublish is
// the key architectural guarantee: a client that never drains its buffer
// must not block EventBus.Publish for the rest of the application, and must
// eventually be disconnected once its buffer overflows.
func TestEventsHandlers_GetEvents_SlowClientIsDroppedWithoutBlockingPublish(t *testing.T) {
	bus := event.NewEventBus()
	h := restapi.NewEventsHandler(bus, 100)

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
