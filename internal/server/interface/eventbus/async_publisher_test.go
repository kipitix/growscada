package eventbus_test

import (
	"context"
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
)

// TestAsyncPublisher_DeliversInOrderApartFromThePublisher: a subscriber that
// blocks does not hold the publisher while the queue has room, and the
// events reach it in the order published.
func TestAsyncPublisher_DeliversInOrderApartFromThePublisher(t *testing.T) {
	bus := eventbus.NewEventBus()
	release := make(chan struct{})
	received := make(chan event.Event, 3)
	bus.Subscribe(event.EventTypeClientConnected, func(e event.Event) {
		<-release
		received <- e
	})
	p := eventbus.NewAsyncPublisher(bus, 3)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go p.Run(ctx)

	published := []event.Event{newClientConnected(), newClientConnected(), newClientConnected()}
	done := make(chan struct{})
	go func() {
		for _, e := range published {
			p.Publish(e)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish waited for the subscriber")
	}
	close(release)
	for i, want := range published {
		select {
		case got := <-received:
			if got != want {
				t.Errorf("event %d out of order", i)
			}
		case <-time.After(time.Second):
			t.Fatalf("event %d not delivered", i)
		}
	}
}

// TestAsyncPublisher_StopDeliversTheQueuedAndDropsTheLater: Run delivers what
// is queued when stopped; a Publish after that returns at once.
func TestAsyncPublisher_StopDeliversTheQueuedAndDropsTheLater(t *testing.T) {
	bus := eventbus.NewEventBus()
	var delivered int
	bus.Subscribe(event.EventTypeClientConnected, func(event.Event) { delivered++ })
	p := eventbus.NewAsyncPublisher(bus, 2)
	p.Publish(newClientConnected())
	p.Publish(newClientConnected())

	ctx, stop := context.WithCancel(context.Background())
	stop()
	p.Run(ctx)
	if delivered != 2 {
		t.Errorf("delivered %d of the 2 queued", delivered)
	}

	done := make(chan struct{})
	go func() {
		for range 3 {
			p.Publish(newClientConnected())
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish after Run returned waited")
	}
}
