package websocket

import (
	"log/slog"
	"sync"

	"github.com/google/uuid"
	gorillaWs "github.com/gorilla/websocket"
)

// Hub manages all WebSocket connections across campaigns. It routes messages
// to clients in the same campaign and handles connection lifecycle.
//
// The hub is safe for concurrent use. Services call Broadcast() to push
// domain events to connected clients without importing the WebSocket library.
type Hub struct {
	// clients tracks all connected clients, keyed by campaign ID then client ID.
	clients map[string]map[string]*Client

	// broadcast receives messages from clients and services for fan-out.
	broadcast chan *Message

	// register adds a new client to the hub.
	register chan *Client

	// unregister removes a client from the hub.
	unregister chan *Client

	mu sync.RWMutex
}

// NewHub creates a new WebSocket hub. Call Run() to start processing.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]map[string]*Client),
		broadcast:  make(chan *Message, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run starts the hub's event loop. It should be called in a goroutine.
// The hub processes register, unregister, and broadcast events sequentially
// to avoid race conditions on the clients map.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			campaign := h.clients[client.CampaignID]
			if campaign == nil {
				campaign = make(map[string]*Client)
				h.clients[client.CampaignID] = campaign
			}
			campaign[client.ID] = client
			h.mu.Unlock()

			slog.Info("ws: client connected",
				slog.String("client", client.ID),
				slog.String("campaign", client.CampaignID),
				slog.String("source", client.Source),
				slog.Int("campaign_clients", len(campaign)),
			)

		case client := <-h.unregister:
			h.mu.Lock()
			if campaign, ok := h.clients[client.CampaignID]; ok {
				if _, exists := campaign[client.ID]; exists {
					delete(campaign, client.ID)
					close(client.send)
					if len(campaign) == 0 {
						delete(h.clients, client.CampaignID)
					}
				}
			}
			h.mu.Unlock()

			slog.Info("ws: client disconnected",
				slog.String("client", client.ID),
				slog.String("campaign", client.CampaignID),
			)

		case msg := <-h.broadcast:
			h.mu.RLock()
			campaign := h.clients[msg.CampaignID]
			h.mu.RUnlock()

			if campaign == nil {
				continue
			}

			data, err := msg.Encode()
			if err != nil {
				slog.Error("ws: failed to encode message",
					slog.Any("error", err),
					slog.String("type", string(msg.Type)),
				)
				continue
			}

			h.mu.RLock()
			for id, client := range campaign {
				// Don't echo back to sender.
				if id == msg.SenderID {
					continue
				}

				select {
				case client.send <- data:
				default:
					// Client's send buffer is full; disconnect it.
					slog.Warn("ws: client send buffer full, disconnecting",
						slog.String("client", id),
					)
					go func(c *Client) {
						h.unregister <- c
					}(client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends a message to all clients in the specified campaign.
// This is the primary method services use to push domain events.
// It is safe for concurrent use from any goroutine.
func (h *Hub) Broadcast(msg *Message) {
	h.broadcast <- msg
}

// BroadcastToAll sends a message to all connected clients across all campaigns.
// Used for system-wide announcements (e.g., server shutdown notice).
func (h *Hub) BroadcastToAll(msg *Message) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	data, err := msg.Encode()
	if err != nil {
		return
	}

	for _, campaign := range h.clients {
		for _, client := range campaign {
			select {
			case client.send <- data:
			default:
			}
		}
	}
}

// CampaignClientCount returns the number of connected clients for a campaign.
func (h *Hub) CampaignClientCount(campaignID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[campaignID])
}

// TotalClientCount returns the total number of connected clients.
func (h *Hub) TotalClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	total := 0
	for _, campaign := range h.clients {
		total += len(campaign)
	}
	return total
}

// RegisterClient creates a new client and starts its read/write pumps.
// Returns the client for external reference (e.g., to track in tests).
func (h *Hub) RegisterClient(conn WSConn, campaignID, userID, source string, role int) *Client {
	client := &Client{
		ID:         uuid.New().String(),
		CampaignID: campaignID,
		UserID:     userID,
		Source:     source,
		Role:       role,
		hub:        h,
		conn:       conn.(*gorillaWs.Conn),
		send:       make(chan []byte, sendBufferSize),
		done:       make(chan struct{}),
	}

	h.register <- client
	go client.writePump()
	go client.readPump()

	return client
}

// WSConn is an interface satisfied by *websocket.Conn, used for testability.
type WSConn interface{}
