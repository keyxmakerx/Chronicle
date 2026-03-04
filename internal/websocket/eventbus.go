package websocket

// EventBus is the interface services use to emit domain events to WebSocket
// clients. Services depend on this interface (not the Hub directly) to avoid
// tight coupling and simplify testing.
type EventBus interface {
	// Publish sends a message to all clients in the specified campaign,
	// excluding the sender if SenderID is set.
	Publish(msg *Message)
}

// hubEventBus wraps the Hub to implement EventBus.
type hubEventBus struct {
	hub *Hub
}

// NewEventBus creates an EventBus backed by the given Hub.
func NewEventBus(hub *Hub) EventBus {
	return &hubEventBus{hub: hub}
}

// Publish delegates to the hub's Broadcast method.
func (b *hubEventBus) Publish(msg *Message) {
	if b.hub != nil {
		b.hub.Broadcast(msg)
	}
}

// NoopEventBus is a no-op implementation for use when WebSocket is disabled
// or in tests that don't need event broadcasting.
type NoopEventBus struct{}

// Publish does nothing.
func (NoopEventBus) Publish(*Message) {}
