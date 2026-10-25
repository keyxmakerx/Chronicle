// Package connector lets an AI app (Claude, ChatGPT, Open WebUI, LM Studio)
// connect straight to one campaign over the Model Context Protocol, look
// things up and ask for changes, with nothing to copy and paste. No tool
// changes the campaign: a request for changes waits on the owner's Your AI
// page until they review it on the same screen a paste uses, so an AI app set
// to allow everything still can't apply anything.
//
// A connection is a key the campaign's owner makes on the "Your AI" page. The
// key is shown once, sent by the AI app as a request header, kept only as its
// SHA-256, expires on its own and can be revoked. It is scoped to one owner,
// one campaign, and the privacy mode and Change setting chosen when it was
// made, and dies as soon as its maker is no longer the campaign's owner.
// Every tool call is logged for the owner as "What it did".
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
	// CanAsk is the Change setting, fixed when the key was made: true lets
	// the AI ask for changes, false ("Only look") only lets it read.
	CanAsk bool
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

// Request is what an AI asked for, waiting for the owner's review.
type Request struct {
	ID           int64
	CampaignID   string
	ConnectionID int64
	// AppName is the connected app's name when it asked (untrusted, cleaned).
	AppName string
	// Summary is the AI's own one line about the request (untrusted, shown
	// in quotes); Counts is Chronicle's ("2 to add, 1 to change").
	Summary string
	Counts  string
	Blocks  int
	// Body is the blocks, in the copy-and-paste format. Only Request reads
	// it; lists leave it empty.
	Body      string
	Status    string
	Outcome   string
	CreatedAt time.Time
	DecidedAt *time.Time
}

// A request's status.
const (
	RequestWaiting    = "waiting"
	RequestAnswered   = "answered"
	RequestTurnedDown = "turned_down"
)

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
	// changesMax caps one request's blocks; requestsWaiting caps how many
	// wait for the owner at once, so an AI can't bury them; requestsKept
	// is how many answered ones are kept for the page.
	changesMax      = 256 * 1024
	requestsWaiting = 20
	requestsKept    = 100
)
