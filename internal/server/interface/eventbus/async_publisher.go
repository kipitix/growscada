package eventbus

import (
	"context"

	"github.com/kipitix/growscada/internal/server/domain/event"
)

// AsyncPublisher hands events to an EventBus from one goroutine of its own
// (Run), in the order they were published: the publisher only queues an
// event, and the delivery to the subscribers (the SSE fan-out) runs apart
// from it. The Runtime node publishes tag_updated through it, so a Device's
// write does not wait for the fan-out (ADR 0011).
type AsyncPublisher struct {
	bus     EventBus
	queue   chan event.Event
	stopped chan struct{}
}

// NewAsyncPublisher returns a publisher to aBus queuing up to size events.
func NewAsyncPublisher(aBus EventBus, size int) *AsyncPublisher {
	return &AsyncPublisher{bus: aBus, queue: make(chan event.Event, size), stopped: make(chan struct{})}
}

// Publish queues the event. With the queue full it waits for room, so no
// event is dropped while subscribers keep up; once Run has returned it drops
// the event instead of waiting.
func (p *AsyncPublisher) Publish(e event.Event) {
	select {
	case p.queue <- e:
	case <-p.stopped:
	}
}

// Run delivers the queued events to the bus until ctx is done, then the
// events still queued, and returns. An event published after that is
// dropped.
func (p *AsyncPublisher) Run(ctx context.Context) {
	defer close(p.stopped)
	for {
		select {
		case e := <-p.queue:
			p.bus.Publish(e)
		case <-ctx.Done():
			for {
				select {
				case e := <-p.queue:
					p.bus.Publish(e)
				default:
					return
				}
			}
		}
	}
}
