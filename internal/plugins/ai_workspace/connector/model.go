// Package connector lets an AI app (Claude, ChatGPT, Open WebUI, LM Studio)
// connect straight to one campaign over the Model Context Protocol and look
// things up, with nothing to copy and paste. In this step every tool only
// reads: none can change the campaign.
//
// A connection is a key the campaign's owner makes on the "Your AI" page. The
// key is shown once, sent by the AI app as a request header, kept only as its
// SHA-256, expires on its own and can be revoked. It is scoped to one owner,
// one campaign and the privacy mode chosen when it was made, and dies as soon
// as its maker is no longer the campaign's owner. Every tool call is logged
// for the owner as "What it did".
package connector

import (
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
)

// Connection is one key an owner made for their AI app.
type Connection struct {
	ID         int64
	CampaignID string
	// UserID is the owner who made the key. Answers are read as them.
	UserID string
	// KeyHash is the hex SHA-256 of the key; the key itself is never kept.
	KeyHash string
	// KeyHint is the key's last four characters, to tell keys apart.
	KeyHint string
	// Privacy is what the AI may see, fixed when the key was made.
	Privacy aiexport.PrivacyMode
	// ClientName is what the AI app called itself when it connected
	// (untrusted, cleaned and cut to clientNameMax).
	ClientName string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

// Expired reports whether the key has run out at now.
func (c *Connection) Expired(now time.Time) bool { return !now.Before(c.ExpiresAt) }

// Live reports whether the key still opens the campaign at now.
func (c *Connection) Live(now time.Time) bool { return c.RevokedAt == nil && !c.Expired(now) }

// Call is one tool call, as "What it did" lists it.
type Call struct {
	ID           int64
	ConnectionID int64
	CampaignID   string
	At           time.Time
	// Tool is the tool's name; Summary says in plain words what was asked.
	Tool    string
	Summary string
	// Note is the answer's size ("3 pages") or why it wasn't answered.
	Note string
	OK   bool
}

// Limits on what a connection may do and keep.
const (
	// KeyLifetime is how long a new key works before it runs out.
	KeyLifetime = 90 * 24 * time.Hour
	// callsPerMinute and callsPerDay cap one connection's tool calls.
	callsPerMinute = 30
	callsPerDay    = 2000
	// callsKept is how many calls per campaign "What it did" keeps.
	callsKept = 500
	// CallsShown is how many calls the page lists.
	CallsShown = 50
	// clientNameMax caps the name an AI app gives itself.
	clientNameMax = 64
	// summaryMax and noteMax match the call log's columns.
	summaryMax = 200
	noteMax    = 120
)
