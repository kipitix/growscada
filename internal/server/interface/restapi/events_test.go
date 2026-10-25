package restapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/version"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
	"github.com/kipitix/growscada/internal/server/interface/restapi"
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

// runEventsHandler opens the server's event stream.
func runEventsHandler(t *testing.T, h *restapi.EventsHandlers, w http.ResponseWriter) (cancel func(), done <-chan struct{}) {
	t.Helper()
	return serveStream(httptest.NewRequest(http.MethodGet, "/api/v0/events", nil), h.GetEvents, w)
}

// runProjectEventsHandler opens the event stream of the project projectID.
func runProjectEventsHandler(t *testing.T, h *restapi.EventsHandlers, w http.ResponseWriter, projectID uuid.UUID) (cancel func(), done <-chan struct{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v0/projects/"+projectID.String()+"/events", nil)
	req.SetPathValue("project_id", projectID.String())
	return serveStream(req, h.GetProjectEvents, w)
}

func serveStream(req *http.Request, handler http.HandlerFunc, w http.ResponseWriter) (cancel func(), done <-chan struct{}) {
	ctx, cancelFn := context.WithCancel(req.Context())
	req = req.WithContext(ctx)

	doneCh := make(chan struct{})
	go func() {
		handler(w, req)
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
	bus := eventbus.NewEventBus()
	h := restapi.NewEventsHandler(bus, nil, 100)

	rec := newSyncRecorder()
	cancel, done := runEventsHandler(t, h, rec)
	waitFor(t, time.Second, func() bool { return strings.Contains(rec.body(), ": connected") })
	cancel()
	<-done

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}
}

func TestEventsHandlers_GetProjectEvents_StreamsPublishedEvent(t *testing.T) {
	bus := eventbus.NewEventBus()
	h := restapi.NewEventsHandler(bus, anyProjects{}, 100)

	rec := newSyncRecorder()
	cancel, done := runProjectEventsHandler(t, h, rec, mainProject)
	waitFor(t, time.Second, func() bool { return strings.Contains(rec.body(), ": connected") })

	tagID := id.NewID[tag.Tag]()
	bus.Publish(tag.NewTagCreatedEvent(tagID, mainProjectID))

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

// sseEventMessages parses every `data:` line of an SSE body as JSON.
func sseEventMessages(t *testing.T, body string) []map[string]any {
	t.Helper()
	var msgs []map[string]any
	for _, line := range strings.Split(body, "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Fatalf("invalid event JSON %q: %v", data, err)
		}
		msgs = append(msgs, m)
	}
	return msgs
}

func TestEventsHandlers_GetProjectEvents_TagUpdatedCarriesTagState(t *testing.T) {
	bus := eventbus.NewEventBus()
	h := restapi.NewEventsHandler(bus, anyProjects{}, 100)

	rec := newSyncRecorder()
	cancel, done := runProjectEventsHandler(t, h, rec, mainProject)
	waitFor(t, time.Second, func() bool { return strings.Contains(rec.body(), ": connected") })

	name, _ := tag.NewTagName("pressure")
	value, _ := tag.TagTypeInteger.NewTagValue(int64(42))
	aVersion, _ := version.New(version.WithNumber[tag.Tag](7))
	aTag, err := tag.ReconstituteTag(id.NewID[tag.Tag](), mainProjectID, name, tag.TagTypeInteger, value, tag.TagQualityUncertain, aVersion)
	if err != nil {
		t.Fatalf("NewTag: %v", err)
	}
	bus.Publish(tag.NewTagCreatedEvent(aTag.ID(), mainProjectID))
	bus.Publish(tag.NewTagUpdatedEvent(aTag))

	waitFor(t, time.Second, func() bool { return strings.Contains(rec.body(), "tag_updated") })
	cancel()
	<-done

	var created, updated map[string]any
	for _, m := range sseEventMessages(t, rec.body()) {
		switch m["type"] {
		case "tag_created":
			created = m
		case "tag_updated":
			updated = m
		}
	}
	if created == nil || updated == nil {
		t.Fatalf("expected tag_created and tag_updated events, got %q", rec.body())
	}
	if _, ok := created["tag"]; ok {
		t.Errorf("tag_created must not carry a tag, got %v", created)
	}

	want := map[string]any{
		"id":      aTag.ID().String(),
		"name":    "pressure",
		"type":    "integer",
		"value":   "42",
		"quality": "uncertain",
		"version": float64(7),
	}
	got, ok := updated["tag"].(map[string]any)
	if !ok {
		t.Fatalf("tag_updated: expected tag object, got %v", updated["tag"])
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("tag_updated tag.%s: expected %v, got %v", k, v, got[k])
		}
	}
}

func TestEventsHandlers_GetEvents_PublishesConnectAndDisconnectEvents(t *testing.T) {
	bus := eventbus.NewEventBus()
	h := restapi.NewEventsHandler(bus, nil, 100)

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
	bus := eventbus.NewEventBus()
	h := restapi.NewEventsHandler(bus, nil, 100)

	rec1 := newSyncRecorder()
	cancel1, done1 := runEventsHandler(t, h, rec1)
	rec2 := newSyncRecorder()
	cancel2, done2 := runEventsHandler(t, h, rec2)

	waitFor(t, time.Second, func() bool {
		return strings.Contains(rec1.body(), ": connected") && strings.Contains(rec2.body(), ": connected")
	})

	bus.Publish(library.NewWidgetTypeCreatedEvent(id.NewID[library.WidgetType]()))

	waitFor(t, time.Second, func() bool {
		return strings.Contains(rec1.body(), "widget_type_created") && strings.Contains(rec2.body(), "widget_type_created")
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
	bus := eventbus.NewEventBus()
	_ = restapi.NewEventsHandler(bus, nil, 100) // subscribes the hub to bus; no client connects

	e := library.NewWidgetTypeCreatedEvent(id.NewID[library.WidgetType]())

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
	bus := eventbus.NewEventBus()
	_ = restapi.NewEventsHandler(bus, nil, 100)

	e := library.NewWidgetTypeCreatedEvent(id.NewID[library.WidgetType]())

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Publish(e)
	}
}

// countingBus is an EventBus that counts the subscriptions still active, so
// a test can see the hub detach from the bus.
type countingBus struct {
	eventbus.EventBus
	mu     sync.Mutex
	active int
}

func (b *countingBus) Subscribe(et event.EventType, handler eventbus.EventHandler) eventbus.Subscription {
	b.mu.Lock()
	b.active++
	b.mu.Unlock()
	return b.EventBus.Subscribe(et, handler)
}

func (b *countingBus) Unsubscribe(sub eventbus.Subscription) {
	b.mu.Lock()
	b.active--
	b.mu.Unlock()
	b.EventBus.Unsubscribe(sub)
}

func (b *countingBus) activeSubscriptions() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.active
}

// TestEventsHandlers_Close_EndsOpenStream: Close ends an open stream by
// itself, without its request context being canceled — otherwise
// http.Server.Shutdown waits for the SSE handler until its timeout — and
// detaches the hub from the EventBus.
func TestEventsHandlers_Close_EndsOpenStream(t *testing.T) {
	bus := &countingBus{EventBus: eventbus.NewEventBus()}
	h := restapi.NewEventsHandler(bus, nil, 100)
	if bus.activeSubscriptions() == 0 {
		t.Fatal("expected the hub to subscribe to the EventBus")
	}

	rec := newSyncRecorder()
	cancel, done := runEventsHandler(t, h, rec)
	defer cancel()
	waitFor(t, time.Second, func() bool { return strings.Contains(rec.body(), ": connected") })

	h.Close()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("GetEvents did not return after Close")
	}

	if n := bus.activeSubscriptions(); n != 0 {
		t.Errorf("expected the hub to unsubscribe from the EventBus on Close, %d subscriptions left", n)
	}
}

// stalledConnWriter simulates a client that stopped reading: after the
// ": connected" comment every write stalls until its write deadline, which
// the test fires by closing expire.
type stalledConnWriter struct {
	header   http.Header
	mu       sync.Mutex
	writes   int
	deadline time.Time
	stalled  chan struct{}
	expire   chan struct{}
}

func newStalledConnWriter() *stalledConnWriter {
	return &stalledConnWriter{
		header:  make(http.Header),
		stalled: make(chan struct{}),
		expire:  make(chan struct{}),
	}
}

func (w *stalledConnWriter) Header() http.Header { return w.header }
func (w *stalledConnWriter) WriteHeader(int)     {}
func (w *stalledConnWriter) Flush()              {}

func (w *stalledConnWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.writes++
	n, deadline := w.writes, w.deadline
	w.mu.Unlock()
	if n == 1 {
		return len(p), nil
	}
	if deadline.IsZero() {
		select {} // no deadline: stalls forever, as a dead connection does
	}
	if n == 2 {
		close(w.stalled)
	}
	<-w.expire
	return 0, os.ErrDeadlineExceeded
}

func (w *stalledConnWriter) SetWriteDeadline(d time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.deadline = d
	return nil
}

func (w *stalledConnWriter) writeDeadline() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.deadline
}

// TestEventsHandlers_GetEvents_StalledWriteEndsOnDeadline: a handler stuck
// writing to a client that stopped reading is not freed by Close (it does
// not read its channel), so each write carries a deadline, and the handler
// returns once the write fails — otherwise http.Server.Shutdown waits for it
// until its timeout.
func TestEventsHandlers_GetEvents_StalledWriteEndsOnDeadline(t *testing.T) {
	bus := eventbus.NewEventBus()
	h := restapi.NewEventsHandler(bus, nil, 100)

	w := newStalledConnWriter()
	cancel, done := runEventsHandler(t, h, w)
	defer cancel()
	waitFor(t, time.Second, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.writes >= 1
	})

	sent := time.Now()
	bus.Publish(library.NewWidgetTypeCreatedEvent(id.NewID[library.WidgetType]()))
	select {
	case <-w.stalled:
	case <-time.After(time.Second):
		t.Fatal("the event write did not start, or started without a write deadline")
	}
	if d := w.writeDeadline(); d.Before(sent) || d.After(sent.Add(15*time.Second)) {
		t.Errorf("expected a write deadline within the 15 s shutdown budget, got %v after the publish", d.Sub(sent))
	}

	h.Close()
	select {
	case <-done:
		t.Fatal("GetEvents returned while its write was still stalled")
	case <-time.After(50 * time.Millisecond):
	}

	close(w.expire)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("GetEvents did not return after its write failed on the deadline")
	}
}

// TestEventsHandlers_GetEvents_RejectedAfterClose: a stream opened after
// Close is refused at once with a Problem saying why, instead of hanging.
func TestEventsHandlers_GetEvents_RejectedAfterClose(t *testing.T) {
	bus := eventbus.NewEventBus()
	h := restapi.NewEventsHandler(bus, nil, 100)
	h.Close()

	rec := newSyncRecorder()
	cancel, done := runEventsHandler(t, h, rec)
	defer cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("GetEvents did not return after Close")
	}

	if rec.code() != http.StatusServiceUnavailable {
		t.Errorf("expected 503 after Close, got %d", rec.code())
	}
	if !strings.Contains(rec.body(), "server is shutting down") {
		t.Errorf("expected the shutdown reason in the problem, got %q", rec.body())
	}
}

// TestEventsHandlers_Close_ConcurrentWithDisconnects: Close, called twice,
// races with clients disconnecting on their own and with events being
// published (the outbox drain publishes during shutdown); it must neither
// panic on a double channel close or a send on a closed channel nor race
// (make test runs with -race).
func TestEventsHandlers_Close_ConcurrentWithDisconnects(t *testing.T) {
	bus := eventbus.NewEventBus()
	h := restapi.NewEventsHandler(bus, nil, 100)

	const clients = 20
	cancels := make([]func(), clients)
	dones := make([]<-chan struct{}, clients)
	recs := make([]*syncRecorder, clients)
	for i := range clients {
		recs[i] = newSyncRecorder()
		cancels[i], dones[i] = runEventsHandler(t, h, recs[i])
	}
	waitFor(t, time.Second, func() bool {
		for _, rec := range recs {
			if !strings.Contains(rec.body(), ": connected") {
				return false
			}
		}
		return true
	})

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 50 {
				bus.Publish(library.NewWidgetTypeCreatedEvent(id.NewID[library.WidgetType]()))
			}
		})
	}
	for _, cancel := range cancels {
		wg.Go(cancel)
	}
	wg.Go(h.Close)
	wg.Go(h.Close)
	wg.Wait()

	for _, done := range dones {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("GetEvents did not return")
		}
	}
}

// TestEventsHandlers_Close_LetsServerShutdown reproduces the shutdown hang:
// with a stream open, Close and http.Server.Shutdown run concurrently, as
// gracedown runs the hooks of one layer. Shutdown must not wait for its
// timeout, and the client must see the stream end.
func TestEventsHandlers_Close_LetsServerShutdown(t *testing.T) {
	bus := eventbus.NewEventBus()
	h := restapi.NewEventsHandler(bus, nil, 100)
	srv := httptest.NewServer(http.HandlerFunc(h.GetEvents))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET events: %v", err)
	}
	defer resp.Body.Close()
	br := bufio.NewReader(resp.Body)
	if line, err := br.ReadString('\n'); err != nil || !strings.HasPrefix(line, ": connected") {
		t.Fatalf("expected the connected comment, got %q, %v", line, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	shutdownErr := make(chan error, 1)
	go func() { shutdownErr <- srv.Config.Shutdown(ctx) }()
	go h.Close()

	if err := <-shutdownErr; err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if rest, err := io.ReadAll(br); err != nil {
		t.Errorf("expected the stream to end cleanly, got %v (read %q)", err, rest)
	}
}

// TestEventsHandlers_GetEvents_RejectsBeyondMaxClients guards the fix for
// the CODE_REVIEW.md finding that GET /api/v0/events accepted an unbounded
// number of concurrent SSE connections. A small maxClients (2) keeps the
// test from needing to open 100 real connections.
func TestEventsHandlers_GetEvents_RejectsBeyondMaxClients(t *testing.T) {
	bus := eventbus.NewEventBus()
	h := restapi.NewEventsHandler(bus, nil, 2)

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
	if ct := rec3.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("expected a JSON problem response, got Content-Type %q, body %q", ct, rec3.body())
	}
	if !strings.Contains(rec3.body(), "too many concurrent event stream connections") {
		t.Errorf("expected the connection-limit reason in the problem, got %q", rec3.body())
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
	bus := eventbus.NewEventBus()
	h := restapi.NewEventsHandler(bus, nil, 100)

	w := newBlockingResponseWriter()
	cancel, done := runEventsHandler(t, h, w)
	defer cancel()

	// Registering the client (hub.addClient) races with this goroutine, so
	// keep publishing single events until one actually reaches the client's
	// Write call (proving it was registered) and stalls there.
	deadline := time.After(time.Second)
publishUntilStarted:
	for {
		bus.Publish(library.NewWidgetTypeCreatedEvent(id.NewID[library.WidgetType]()))
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
			bus.Publish(library.NewWidgetTypeCreatedEvent(id.NewID[library.WidgetType]()))
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
