package websocket

import (
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	gorillaWs "github.com/gorilla/websocket"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// foundryPresenceTTL is the window inside which a Foundry-module WS
// connection counts as "connected" for the /foundry-presence pill.
// Slightly longer than 3× the WS pingPeriod (30s) so a missed pong
// or two doesn't flicker the pill to disconnected.
const foundryPresenceTTL = 90 * time.Second

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

	// foundryPresence tracks the most recent activity for each campaign's
	// Foundry-module WS connection (source=="foundry-module"). Updated on
	// connect and on pong receipt; read by the /foundry-presence endpoint.
	// In-memory by design — presence is a transient property, no migration
	// or persistence needed.
	foundryMu       sync.RWMutex
	foundryLastSeen map[string]time.Time
}

// NewHub creates a new WebSocket hub. Call Run() to start processing.
func NewHub() *Hub {
	return &Hub{
		clients:         make(map[string]map[string]*Client),
		broadcast:       make(chan *Message, 256),
		register:        make(chan *Client),
		unregister:      make(chan *Client),
		foundryLastSeen: make(map[string]time.Time),
	}
}

// MarkFoundrySeen records activity from a Foundry-module connection for
// the campaign. Called on connect and on pong receipt so the presence
// window slides with the live heartbeat.
func (h *Hub) MarkFoundrySeen(campaignID string) {
	if campaignID == "" {
		return
	}
	h.foundryMu.Lock()
	h.foundryLastSeen[campaignID] = time.Now()
	h.foundryMu.Unlock()
}

// FoundryPresence returns the last-seen timestamp and whether the
// Foundry-module connection is considered live for the given campaign.
// A nil lastSeen means we've never recorded a Foundry-module connection
// for this campaign; connected is true when lastSeen is within the
// foundryPresenceTTL window.
func (h *Hub) FoundryPresence(campaignID string) (lastSeen *time.Time, connected bool) {
	h.foundryMu.RLock()
	t, ok := h.foundryLastSeen[campaignID]
	h.foundryMu.RUnlock()
	if !ok {
		return nil, false
	}
	out := t
	return &out, time.Since(t) < foundryPresenceTTL
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

			if client.Source == "foundry-module" {
				h.MarkFoundrySeen(client.CampaignID)
			}

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

				// Audience gate: messages flagged RequiresDM only go to
				// clients with Owner role or IsDmGranted=true. This is
				// the server-side defense that pairs with client-side
				// visibility filters — clients can't receive what we
				// never send. permissions.CanSeeDmOnly takes the role
				// plus an optional dmGranted flag; passing both keeps
				// the predicate aligned with how HTTP handlers gate
				// dm_only content.
				if msg.RequiresDM && !permissions.CanSeeDmOnly(client.Role, client.IsDmGranted) {
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
//
// isDmGranted reflects CampaignContext.IsDmGranted resolved at auth time;
// it lets the broadcast loop deliver RequiresDM messages to non-Owner
// users that the campaign Owner has explicitly trusted with DM-only
// visibility, without a per-message DB hit.
func (h *Hub) RegisterClient(conn WSConn, campaignID, userID, source string, role int, isDmGranted bool) *Client {
	client := &Client{
		ID:          uuid.New().String(),
		CampaignID:  campaignID,
		UserID:      userID,
		Source:      source,
		Role:        role,
		IsDmGranted: isDmGranted,
		hub:         h,
		conn:        conn.(*gorillaWs.Conn),
		send:        make(chan []byte, sendBufferSize),
		done:        make(chan struct{}),
	}

	h.register <- client
	go client.writePump()
	go client.readPump()

	return client
}

// WSConn is an interface satisfied by *websocket.Conn, used for testability.
type WSConn interface{}
