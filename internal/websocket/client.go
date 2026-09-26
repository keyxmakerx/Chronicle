package websocket

import (
	"log/slog"
	"sync"
	"time"

	gorillaWs "github.com/gorilla/websocket"

	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
)

const (
	// writeWait is the time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// pongWait is the time allowed to read the next pong from the peer.
	pongWait = 60 * time.Second

	// maxMessageSize is the maximum size of an incoming message (64KB).
	maxMessageSize = 64 * 1024

	// sendBufferSize is the channel buffer for outgoing messages.
	sendBufferSize = 256
)

// pingPeriod is the default ping interval (must stay under pongWait). It
// also sets how often writePump rechecks a client's expiry.
const pingPeriod = 30 * time.Second

// Client represents a single WebSocket connection to the hub.
// Each client belongs to one campaign and has an optional user/API key identity.
type Client struct {
	// ID is a unique identifier for this connection.
	ID string

	// CampaignID scopes this client to a specific campaign's message stream.
	CampaignID string

	// UserID is the authenticated user (from session or API key owner).
	UserID string

	// Source identifies the client type ("browser" or "foundry").
	Source string

	// Role is the user's campaign role (for permission filtering).
	Role int

	// IsDmGranted mirrors campaigns.CampaignContext.IsDmGranted —
	// non-Owner members the campaign Owner has granted dm_only
	// visibility to via CampaignSettings.DmGrantIDs. The hub's
	// broadcast gate (see hub.go) treats Role>=Owner OR IsDmGranted as
	// "may receive RequiresDM messages." Resolved once at registration
	// from the auth path so the gate is a per-message constant-time
	// check; revoking a grant requires the user to reconnect.
	IsDmGranted bool

	// ExpiresAt is the API key's expiry, if any (nil for a session or an
	// unexpiring key). Rechecked after connect so an expiry crossed
	// mid-connection still cuts the socket off.
	ExpiresAt *time.Time

	hub  *Hub
	conn *gorillaWs.Conn
	send chan []byte
	done chan struct{}
	once sync.Once
}

// readPump reads WebSocket frames to keep pongs, close frames and read
// deadlines working. It never forwards a client's message: only the
// server broadcasts.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	if err := c.conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		return
	}
	c.conn.SetPongHandler(func(string) error {
		// Foundry-module pong receipts refresh the presence window so
		// the /foundry-presence pill stays "connected" as long as the
		// WS heartbeat is alive. Browser clients aren't tracked.
		if c.Source == foundry_vtt.ModuleSource {
			c.hub.MarkFoundrySeen(c.CampaignID)
		}
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if gorillaWs.IsUnexpectedCloseError(err,
				gorillaWs.CloseGoingAway,
				gorillaWs.CloseNormalClosure,
			) {
				slog.Warn("ws: unexpected close",
					slog.String("client", c.ID),
					slog.Any("error", err),
				)
			}
			return
		}

		// Every frame is discarded — see the doc comment above. Debug only,
		// so a chatty or misbehaving client can't fill logs at a louder level.
		slog.Debug("ws: dropped client-sent message", slog.String("client", c.ID))
	}
}

// writePump sends messages from the hub to the WebSocket connection.
// It runs in its own goroutine per client.
func (c *Client) writePump() {
	ticker := time.NewTicker(c.hub.pingEvery)
	defer func() {
		ticker.Stop()
		c.close()
	}()

	for {
		select {
		case data, ok := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return
			}
			if !ok {
				// Hub closed the channel.
				_ = c.conn.WriteMessage(gorillaWs.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(gorillaWs.TextMessage, data); err != nil {
				return
			}

		case <-ticker.C:
			// Reuses the ping cadence to recheck expiry, rather than a
			// separate timer: an expired key closes here instead of
			// getting pinged.
			if c.ExpiresAt != nil && time.Now().After(*c.ExpiresAt) {
				return
			}
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return
			}
			if err := c.conn.WriteMessage(gorillaWs.PingMessage, nil); err != nil {
				return
			}

		case <-c.done:
			return
		}
	}
}

// close cleanly shuts down the client connection exactly once.
func (c *Client) close() {
	c.once.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}
