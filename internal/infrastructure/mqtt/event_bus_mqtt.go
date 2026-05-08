package mqtt

import (
	"fmt"
	"log/slog"

	"github.com/kipitix/growscada/internal/domain/event"
)

// MQTTClient is a port for an MQTT broker connection.
// Implement this interface with any MQTT library (e.g. paho).
type MQTTClient interface {
	Publish(topic string, payload []byte) error
}

type mqttEventBusImpl struct {
	inner  event.EventBus
	client MQTTClient
}

var _ event.EventBus = (*mqttEventBusImpl)(nil)

func NewMQTTEventBus(inner event.EventBus, client MQTTClient) event.EventBus {
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
		slog.Error(fmt.Errorf("cannot publish event: %w", err).Error())
	}
}

func (m *mqttEventBusImpl) Subscribe(eventType event.EventType, handler event.EventHandler) {
	m.inner.Subscribe(eventType, handler)
}
