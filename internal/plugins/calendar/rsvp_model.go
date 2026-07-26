// Event RSVPs (C-CAL-RSVP-P1) — the calendar's own attendance surface.
//
// Deliberately disjoint from the sessions plugin's session_attendees: a calendar
// event is not a session, and the operator's ask is "who's coming" ON THE
// CALENDAR. Every type here is calendar-owned; nothing in this file is exported
// through the campaign export or the AI export (pinned by rsvp_egress_test.go).
package calendar

import "time"

// RSVP status values. These are the exact ENUM members of
// calendar_event_rsvps.status — the DB rejects anything else, and
// ValidRSVPStatus is the write-path gate so a bad value never reaches SQL.
const (
	RSVPYes   = "yes"
	RSVPMaybe = "maybe"
	RSVPNo    = "no"
)

// Token action values — the ENUM members of calendar_event_rsvp_tokens.action.
//
// The three status actions map 1:1 onto an RSVP write. The two richer actions
// are the operator's emailed asks (requirements §1 item 6):
//
//	RSVPActionOutWeek — "out just this week": records a `no` AND blocks the
//	                    member's scheduler availability for the event's real week.
//	RSVPActionSuggest — "recommend better times": a free-text note on the RSVP
//	                    plus a bell notification to the event's owner. It does NOT
//	                    mint a slot proposal — proposal creation is Scribe+ by
//	                    ruling (sessions/routes.go:53) and an email click must not
//	                    escalate a Player into that capability.
const (
	RSVPActionYes     = "yes"
	RSVPActionMaybe   = "maybe"
	RSVPActionNo      = "no"
	RSVPActionOutWeek = "out_week"
	RSVPActionSuggest = "suggest"
)

// rsvpTokenTTL is how long an emailed action link stays redeemable. Matches the
// sessions RSVP token window so the two emails make the same promise.
const rsvpTokenTTL = 7 * 24 * time.Hour

// maxRSVPNoteLen bounds the "suggest another time" free text. Generous enough
// for a couple of sentences, small enough that the note can ride a notification
// message and an owner-facing list without truncation logic elsewhere.
const maxRSVPNoteLen = 500

// ValidRSVPStatus reports whether s is a storable RSVP status.
func ValidRSVPStatus(s string) bool {
	return s == RSVPYes || s == RSVPMaybe || s == RSVPNo
}

// ValidRSVPAction reports whether a is a mintable token action.
func ValidRSVPAction(a string) bool {
	switch a {
	case RSVPActionYes, RSVPActionMaybe, RSVPActionNo, RSVPActionOutWeek, RSVPActionSuggest:
		return true
	}
	return false
}

// statusForAction maps a token action onto the RSVP status it records.
// "Out this week" is a decline that additionally writes availability; "suggest"
// leaves the member's existing answer alone (it only attaches a note), so it has
// no status of its own and returns "".
func statusForAction(action string) string {
	switch action {
	case RSVPActionYes:
		return RSVPYes
	case RSVPActionMaybe:
		return RSVPMaybe
	case RSVPActionNo, RSVPActionOutWeek:
		return RSVPNo
	default:
		return ""
	}
}

// EventRSVP is one member's answer to one event.
type EventRSVP struct {
	ID        string    `json:"id"`
	EventID   string    `json:"event_id"`
	UserID    string    `json:"user_id"`
	Status    string    `json:"status"`
	Note      *string   `json:"note,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// EventRSVPToken is one emailed single-use action link.
type EventRSVPToken struct {
	ID        int        `json:"-"`
	Token     string     `json:"-"`
	EventID   string     `json:"event_id"`
	UserID    string     `json:"user_id"`
	Action    string     `json:"action"`
	UsedAt    *time.Time `json:"-"`
	ExpiresAt time.Time  `json:"-"`
	CreatedAt time.Time  `json:"-"`
}

// RSVPCounts is the aggregate every campaign member may see.
type RSVPCounts struct {
	Yes   int `json:"yes"`
	Maybe int `json:"maybe"`
	No    int `json:"no"`
}

// RSVPResponder is one named response. Populated ONLY when the viewer is
// Owner/co-DM — the same detail gate the scheduler's week overlay uses
// (sessions/availability_model.go WeekOverlay.IncludeDetail): members see the
// aggregate, the Director sees who.
type RSVPResponder struct {
	UserID      string  `json:"user_id"`
	DisplayName string  `json:"display_name"`
	Status      string  `json:"status"`
	Note        *string `json:"note,omitempty"`
}

// EventRSVPSummary is the wire shape of GET …/events/:eid/rsvps.
//
// Counts + CollectRSVPs are visible to every member who can VIEW the event.
// Responders is non-nil only for Owner/co-DM (IncludeDetail). MyStatus is the
// caller's own answer, which they may always see — it is their own data.
type EventRSVPSummary struct {
	EventID       string          `json:"event_id"`
	CollectRSVPs  bool            `json:"collect_rsvps"`
	Counts        RSVPCounts      `json:"counts"`
	MyStatus      string          `json:"my_status,omitempty"`
	MyNote        *string         `json:"my_note,omitempty"`
	IncludeDetail bool            `json:"include_detail"`
	Responders    []RSVPResponder `json:"responders,omitempty"`
}

// SetRSVPRequest is the body of POST …/events/:eid/rsvp (camelCase per house
// JSON convention; every field here is single-word so the two spellings
// coincide).
//
// Action is OPTIONAL and defaults to the plain status write. It exists so the
// in-app buttons can reach the same two richer actions the emailed links offer
// ("out this week", "suggest another time") without a second endpoint per
// action — the alternative was three near-identical routes. Omitting it makes
// the body exactly {status, note?}.
type SetRSVPRequest struct {
	Status string  `json:"status"`
	Note   *string `json:"note,omitempty"`
	Action string  `json:"action,omitempty"` // "", "out_week", or "suggest"
}
