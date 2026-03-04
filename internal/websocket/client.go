package websocket

import (
	"log/slog"
	"sync"
	"time"

	gorillaWs "github.com/gorilla/websocket"
)

const (
	// writeWait is the time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// pongWait is the time allowed to read the next pong from the peer.
	pongWait = 60 * time.Second

	// pingPeriod sends pings at this interval. Must be less than pongWait.
	pingPeriod = 30 * time.Second

	// maxMessageSize is the maximum size of an incoming message (64KB).
	maxMessageSize = 64 * 1024

	// sendBufferSize is the channel buffer for outgoing messages.
	sendBufferSize = 256
)

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

	hub  *Hub
	conn *gorillaWs.Conn
	send chan []byte
	done chan struct{}
	once sync.Once
}

// readPump reads messages from the WebSocket connection and forwards them
// to the hub for broadcast. It runs in its own goroutine per client.
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
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, data, err := c.conn.ReadMessage()
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

		msg, err := DecodeMessage(data)
		if err != nil {
			slog.Warn("ws: invalid message",
				slog.String("client", c.ID),
				slog.Any("error", err),
			)
			continue
		}

		// Enforce campaign scope: clients can only send messages for their campaign.
		msg.CampaignID = c.CampaignID
		msg.SenderID = c.ID

		c.hub.broadcast <- msg
	}
}

// writePump sends messages from the hub to the WebSocket connection.
// It runs in its own goroutine per client.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
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
