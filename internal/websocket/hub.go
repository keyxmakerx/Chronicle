package websocket

import (
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	gorillaWs "github.com/gorilla/websocket"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
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

	// pingEvery is fixed before any client connects; tests shorten it per hub.
	pingEvery time.Duration
}

// NewHub creates a new WebSocket hub. Call Run() to start processing.
func NewHub() *Hub {
	return &Hub{
		clients:         make(map[string]map[string]*Client),
		broadcast:       make(chan *Message, 256),
		register:        make(chan *Client),
		unregister:      make(chan *Client),
		foundryLastSeen: make(map[string]time.Time),
		pingEvery:       pingPeriod,
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

			if client.Source == foundry_vtt.ModuleSource {
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

				// An expired key's socket may still be open (writePump
				// hasn't ticked yet); stop delivering to it immediately
				// rather than waiting for that tick to close it.
				if client.ExpiresAt != nil && time.Now().After(*client.ExpiresAt) {
					continue
				}

				// Audience gate: messages flagged RequiresDM only go to
				// clients with Owner role or IsDmGranted=true — server-side
				// defense pairing with client-side visibility filters, so
				// clients can't receive what we never send.
				if msg.RequiresDM && !permissions.CanSeeDmOnly(client.Role, client.IsDmGranted) {
					continue
				}

				// Per-user visibility_rules gate: RequiresDM above only
				// covers the binary dm_only case; a "specific" visibility
				// marker/drawing needs its own allowed_users/denied_users
				// check via msg.AudienceAllows, which must mirror the HTTP
				// list path's SQL predicate (maps' ListMarkers/ListDrawings)
				// or a marker/drawing leaks more or less visibility over the
				// wire than the HTTP list shows. DM-equivalent clients
				// bypass this too, matching HTTP.
				if !permissions.CanSeeDmOnly(client.Role, client.IsDmGranted) && !msg.AudienceAllows(client.UserID) {
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

// AudienceAllows reports whether userID is in m's audience per its
// AllowedUsers/DeniedUsers lists. Callers must apply this only to
// non-DM-equivalent clients — Owners/DM-granted users bypass it entirely,
// same as the HTTP list path.
//
// Mirrors maps.VisibilityRules' SQL predicate (ListMarkers, ListDrawings): an
// explicit deny always excludes (and, per permissions.DeniesAnonymous /
// ADR-049, so does a non-empty deny list against an anonymous userID); a
// non-empty AllowedUsers is a strict allowlist; an empty (or absent) rule set
// means "everyone". List-membership logic is deliberately duplicated rather
// than imported from maps, since this package is generic transport
// infrastructure — but the anonymous-deny rule is shared via
// permissions.DeniesAnonymous so the two can't drift apart on that point.
// Exported so the duplication can be pinned: internal/app/map_audience_parity_test.go
// runs one table of cases through this and through maps.VisibilityRules.Allows,
// asserting they agree.
func (m *Message) AudienceAllows(userID string) bool {
	if permissions.DeniesAnonymous(m.DeniedUsers, userID) {
		return false
	}
	for _, id := range m.DeniedUsers {
		if id == userID {
			return false
		}
	}
	if len(m.AllowedUsers) == 0 {
		return true
	}
	for _, id := range m.AllowedUsers {
		if id == userID {
			return true
		}
	}
	return false
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
// isDmGranted and expiresAt are both resolved once at auth time and cached
// on the client rather than looked up per message or per tick.
func (h *Hub) RegisterClient(conn WSConn, campaignID, userID, source string, role int, isDmGranted bool, expiresAt *time.Time) *Client {
	client := &Client{
		ID:          uuid.New().String(),
		CampaignID:  campaignID,
		UserID:      userID,
		Source:      source,
		Role:        role,
		IsDmGranted: isDmGranted,
		ExpiresAt:   expiresAt,
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

// RevokeAPIKeyClients force-disconnects every Foundry-sourced client in
// the campaign. Call this when a sync key is revoked or Sync API is
// switched off, so a socket from before the change can't keep receiving.
func (h *Hub) RevokeAPIKeyClients(campaignID string) {
	h.revokeMatching(campaignID, func(c *Client) bool {
		return c.Source == "foundry" || c.Source == foundry_vtt.ModuleSource
	})
}

// RevokeUser force-disconnects every client belonging to userID in the
// campaign, regardless of source. Call this whenever that user's access
// in the campaign is lowered, so a cached role or grant can't outlive it.
func (h *Hub) RevokeUser(campaignID, userID string) {
	h.revokeMatching(campaignID, func(c *Client) bool {
		return c.UserID == userID
	})
}

// RevokeUserEverywhere force-disconnects userID's sockets across ALL
// campaigns. Call this when an account is disabled: unlike RevokeUser,
// there's no single campaign to scope to, so every client map is swept.
func (h *Hub) RevokeUserEverywhere(userID string) {
	h.mu.RLock()
	var matched []*Client
	for _, clients := range h.clients {
		for _, c := range clients {
			if c.UserID == userID {
				matched = append(matched, c)
			}
		}
	}
	h.mu.RUnlock()

	for _, c := range matched {
		c.close()
	}
}

// RevokeCampaign force-disconnects every client in campaignID, regardless
// of user or source. Call this once a campaign is deleted — its members and
// addon state are gone, so nothing should keep receiving on that socket.
func (h *Hub) RevokeCampaign(campaignID string) {
	h.revokeMatching(campaignID, func(*Client) bool { return true })
}

// revokeMatching closes every client in campaignID for which match returns
// true. Only needs a read lock: closing makes the conn error out, and
// readPump's own cleanup removes it from the map via the normal event loop.
func (h *Hub) revokeMatching(campaignID string, match func(*Client) bool) {
	h.mu.RLock()
	campaign := h.clients[campaignID]
	matched := make([]*Client, 0, len(campaign))
	for _, c := range campaign {
		if match(c) {
			matched = append(matched, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range matched {
		c.close()
	}
}
