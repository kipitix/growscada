package eventbus_test

import (
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/client"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
)

func mustClientID() id.ID[client.Client] {
	return id.NewID[client.Client]()
}

func newClientConnected() event.Event {
	return eventbus.NewClientConnectedEvent(mustClientID())
}

// --- EventBus ---

func TestEventBus_Publish_CallsSubscribedHandler(t *testing.T) {
	bus := eventbus.NewEventBus()
	var received []event.Event
	bus.Subscribe(event.EventTypeClientConnected, func(e event.Event) {
		received = append(received, e)
	})

	e := newClientConnected()
	bus.Publish(e)

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Type() != event.EventTypeClientConnected {
		t.Errorf("expected %s, got %s", event.EventTypeClientConnected, received[0].Type())
	}
}

func TestEventBus_Publish_DoesNotCallHandlerForDifferentType(t *testing.T) {
	bus := eventbus.NewEventBus()
	var received []event.Event
	bus.Subscribe(event.EventTypeClientDisconnected, func(e event.Event) {
		received = append(received, e)
	})

	bus.Publish(newClientConnected())

	if len(received) != 0 {
		t.Errorf("expected no events, got %d", len(received))
	}
}

func TestEventBus_Publish_CallsAllSubscribersForSameType(t *testing.T) {
	bus := eventbus.NewEventBus()
	count := 0
	bus.Subscribe(event.EventTypeClientConnected, func(e event.Event) { count++ })
	bus.Subscribe(event.EventTypeClientConnected, func(e event.Event) { count++ })

	bus.Publish(newClientConnected())

	if count != 2 {
		t.Errorf("expected 2 handler calls, got %d", count)
	}
}

func TestEventBus_Publish_NoSubscribers_DoesNotPanic(t *testing.T) {
	bus := eventbus.NewEventBus()
	bus.Publish(newClientConnected())
}

func TestEventBus_Unsubscribe_StopsReceivingEvents(t *testing.T) {
	bus := eventbus.NewEventBus()
	count := 0
	sub := bus.Subscribe(event.EventTypeClientConnected, func(e event.Event) { count++ })

	bus.Publish(newClientConnected())
	bus.Unsubscribe(sub)
	bus.Publish(newClientConnected())

	if count != 1 {
		t.Errorf("expected 1 handler call before unsubscribe, got %d", count)
	}
}

func TestEventBus_Unsubscribe_OnlyRemovesTargetedSubscription(t *testing.T) {
	bus := eventbus.NewEventBus()
	countA, countB := 0, 0
	subA := bus.Subscribe(event.EventTypeClientConnected, func(e event.Event) { countA++ })
	bus.Subscribe(event.EventTypeClientConnected, func(e event.Event) { countB++ })

	bus.Unsubscribe(subA)
	bus.Publish(newClientConnected())

	if countA != 0 {
		t.Errorf("expected unsubscribed handler to not be called, got %d calls", countA)
	}
	if countB != 1 {
		t.Errorf("expected remaining handler to be called once, got %d", countB)
	}
}

func TestEventBus_Unsubscribe_UnknownSubscription_DoesNotPanic(t *testing.T) {
	bus := eventbus.NewEventBus()
	sub := bus.Subscribe(event.EventTypeClientConnected, func(e event.Event) {})
	bus.Unsubscribe(sub)
	bus.Unsubscribe(sub) // double unsubscribe
}

// --- ClientConnectedEvent / ClientDisconnectedEvent ---

func TestNewClientConnectedEvent_Type_IsClientConnected(t *testing.T) {
	e := eventbus.NewClientConnectedEvent(mustClientID())
	if e.Type() != event.EventTypeClientConnected {
		t.Errorf("expected %s, got %s", event.EventTypeClientConnected, e.Type())
	}
}

func TestNewClientConnectedEvent_ClientID_MatchesProvided(t *testing.T) {
	id := mustClientID()
	e := eventbus.NewClientConnectedEvent(id)
	if e.ClientID() != id {
		t.Errorf("expected ClientID %v, got %v", id, e.ClientID())
	}
}

func TestNewClientDisconnectedEvent_Type_IsClientDisconnected(t *testing.T) {
	e := eventbus.NewClientDisconnectedEvent(mustClientID())
	if e.Type() != event.EventTypeClientDisconnected {
		t.Errorf("expected %s, got %s", event.EventTypeClientDisconnected, e.Type())
	}
}

func TestNewClientDisconnectedEvent_ClientID_MatchesProvided(t *testing.T) {
	id := mustClientID()
	e := eventbus.NewClientDisconnectedEvent(id)
	if e.ClientID() != id {
		t.Errorf("expected ClientID %v, got %v", id, e.ClientID())
	}
}

// --- SystemReadyEvent ---

func TestNewSystemReadyEvent_Type_IsSystemReady(t *testing.T) {
	e := eventbus.NewSystemReadyEvent()
	if e.Type() != event.EventTypeSystemReady {
		t.Errorf("expected %s, got %s", event.EventTypeSystemReady, e.Type())
	}
}

func TestNewSystemReadyEvent_Timestamp_IsNonZero(t *testing.T) {
	e := eventbus.NewSystemReadyEvent()
	if e.Timestamp().Time().IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestNewSystemReadyEvent_String_IsNonEmpty(t *testing.T) {
	e := eventbus.NewSystemReadyEvent()
	if e.String() == "" {
		t.Error("expected non-empty string representation")
	}
}

func TestNewSystemReadyEvent_WithTimestamp_SetsTimestamp(t *testing.T) {
	fixed := event.NewEventTimestamp(event.EventTimestampWithTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)))
	e := eventbus.NewSystemReadyEvent(event.WithTimestamp(fixed))

	if !e.Timestamp().Time().Equal(fixed.Time()) {
		t.Errorf("expected %v, got %v", fixed.Time(), e.Timestamp().Time())
	}
}
