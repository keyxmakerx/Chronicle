package sessions

import "time"

// This file holds the slot-proposal + scheduler-notification domain types
// (C-SCHED-P2). A proposal is a DM's set of 1..5 candidate slots; each slot
// (option) is a UTC INSTANT (RC-12.5), rendered into the viewer's zone at read
// time. Per-option responses live in their OWN table — never session_attendees.

// Proposal status values.
const (
	ProposalOpen   = "open"
	ProposalClosed = "closed"
)

// Per-option response values (mirror RSVP accepted/declined/tentative without
// reusing that table).
const (
	ResponseYes   = "yes"
	ResponseNo    = "no"
	ResponseMaybe = "maybe"
)

// Scheduler notification types (the only writers this slice).
const (
	NotifProposalCreated   = "proposal_created"
	NotifProposalResponse  = "proposal_response"
	NotifProposalConfirmed = "proposal_confirmed" // C-SCHED-P3: winner picked → session created.
)

// maxProposalOptions caps a proposal at 5 candidate slots (design: 1..5).
const maxProposalOptions = 5

// SlotProposal is a DM's scheduling proposal header.
type SlotProposal struct {
	ID         string
	CampaignID string
	CreatedBy  string
	Title      string
	Note       *string
	Status     string // open | closed
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// SlotProposalOption is one candidate slot, stored as a UTC instant range.
type SlotProposalOption struct {
	ID          string
	ProposalID  string
	StartsAtUTC time.Time
	EndsAtUTC   time.Time
	Ordinal     int
	IsWinner    bool
}

// SlotProposalResponse is a member's response to one option. One row per
// (option, user); re-responding upserts.
type SlotProposalResponse struct {
	ID        string
	OptionID  string
	UserID    string
	Response  string // yes | no | maybe
	UpdatedAt time.Time
}

// SlotProposalToken is a one-click email response token (option + user +
// response). Single-use, expiring. Mirrors RSVPToken as a pattern.
type SlotProposalToken struct {
	ID        int
	Token     string
	OptionID  string
	UserID    string
	Response  string
	UsedAt    *time.Time
	ExpiresAt time.Time
	CreatedAt time.Time
}

// Notification is one in-app notification row. Generic + removable (T-B2); the
// scheduler feature is its only writer this slice.
type Notification struct {
	ID         string
	UserID     string
	CampaignID *string
	Type       string
	Payload    *string // JSON blob (render context)
	Link       *string
	ReadAt     *time.Time
	CreatedAt  time.Time
}

// --- API request DTOs (camelCase JSON) ---

// CreateProposalRequest is the DM slot-builder submission: a title/note plus
// 1..5 candidate slots. Slots are submitted as viewer-zone wall-clocks (date +
// minute range in TZ); the service resolves them to UTC instants via timeutil,
// which is the DST-correct path — the same discipline the availability overlay
// uses. Storage is UTC (RC-12.5); only the API input is wall-clock.
type CreateProposalRequest struct {
	Title   string                `json:"title"`
	Note    string                `json:"note"`
	TZ      string                `json:"tz"` // IANA zone the wall-clocks are in
	Options []ProposalOptionInput `json:"options"`
}

// ProposalOptionInput is one candidate slot as a viewer-zone wall-clock: a date
// (YYYY-MM-DD in TZ) plus a [start,end) minute range from local midnight.
type ProposalOptionInput struct {
	Date        string `json:"date"` // YYYY-MM-DD (wall-clock date in the request's TZ)
	StartMinute int    `json:"startMinute"`
	EndMinute   int    `json:"endMinute"`
}

// RespondRequest records a member's response to a single option. Bound from
// either a JSON body or an HTMX form post (hx-vals), so it carries both tags.
type RespondRequest struct {
	Response string `json:"response" form:"response"` // yes | no | maybe
}

// --- View types (projected to the viewer's zone for render) ---

// LocalSlot is a UTC option range projected into a viewer zone for display.
type LocalSlot struct {
	StartsAtUTC string `json:"startsAtUtc"` // RFC-3339 UTC (stable key)
	EndsAtUTC   string `json:"endsAtUtc"`
	DateLabel   string `json:"dateLabel"` // e.g. "Sat, Jul 18"
	TimeLabel   string `json:"timeLabel"` // e.g. "7:00 PM – 11:00 PM"
	TZLabel     string `json:"tzLabel"`   // the viewer zone the labels are in
}

// ProposalView is the full proposal detail for one viewer: options with tallies,
// the viewer's own response per option, and — for the owner (includeDetail) —
// who responded what.
type ProposalView struct {
	Proposal      SlotProposal
	ViewerTZ      string
	IncludeDetail bool
	Options       []ProposalOptionView
}

// ProposalOptionView is one option with its response tally + the viewer's own
// choice, plus (detail only) the per-responder breakdown.
type ProposalOptionView struct {
	Option     SlotProposalOption
	Local      LocalSlot
	YesCount   int
	NoCount    int
	MaybeCount int
	MyResponse string                  // "", yes, no, maybe
	Responders []ProposalResponderView // detail only (owner)
}

// ProposalResponderView is one member's response, shown to the owner only.
type ProposalResponderView struct {
	UserID   string
	Name     string
	Response string
}

// ProposalSummary is one row in the proposals list.
type ProposalSummary struct {
	Proposal    SlotProposal
	OptionCount int
	ResponderN  int  // distinct members who responded to any option
	MyResponded bool // whether the viewer has responded to any option
}
