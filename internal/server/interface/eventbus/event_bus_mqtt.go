package eventbus

import (
	"fmt"
	"log/slog"

	"github.com/kipitix/growscada/internal/server/domain/event"
)

// MQTTClient is a port for an MQTT broker connection.
// Implement this interface with any MQTT library (e.g. paho).
type MQTTClient interface {
	Publish(topic string, payload []byte) error
}

type mqttEventBusImpl struct {
	inner  EventBus
	client MQTTClient
}

var _ EventBus = (*mqttEventBusImpl)(nil)

func NewMQTTEventBus(inner EventBus, client MQTTClient) EventBus {
	return &mqttEventBusImpl{
		inner:  inner,
		client: client,
	}
}

func (m *mqttEventBusImpl) Publish(e event.Event) {
	m.inner.Publish(e)

	topic := fmt.Sprintf("events/%s", e.Type())
	payload := []byte(e.String())
	err := m.client.Publish(topic, payload)
	if err != nil {
		slog.Error("cannot publish event", "error", err)
	}
}

func (m *mqttEventBusImpl) Subscribe(eventType event.EventType, handler EventHandler) Subscription {
	return m.inner.Subscribe(eventType, handler)
}

func (m *mqttEventBusImpl) Unsubscribe(subscription Subscription) {
	m.inner.Unsubscribe(subscription)
}
