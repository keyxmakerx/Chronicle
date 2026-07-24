package calendar

import "time"

// First-class RSVPs on calendar events (C-CAL-RSVP-P1). A calendar event is NOT
// a session — sessions carry their own attendee/token tables in the sessions
// plugin. This model is the calendar plugin's OWN aggregate (migration 013),
// deliberately disjoint so RSVP data never rides the campaign/AI export egress
// (see rsvp_egress_test.go). It supersedes the old drawer ruling that RSVPs
// needed an event↔session link (see .ai/decisions.md).

// RSVP status values (the in-app trichotomy the Director sees counts of).
const (
	RSVPStatusYes   = "yes"
	RSVPStatusMaybe = "maybe"
	RSVPStatusNo    = "no"
)

// Emailed-token actions. The first three mirror the RSVP statuses; the last two
// are email-only verbs the operator asked for (requirements §1 item 6):
//   - out_week: RSVP "no" AND mark the responder's real week unavailable via the
//     scheduler availability exceptions (self-write only).
//   - suggest:  a free-text "recommend a better time" page → stored as the RSVP
//     note + a bell notification to the event owner. NOT a slot proposal (that
//     is Scribe+ by ruling; a player email click must never mint one).
const (
	RSVPActionYes     = "yes"
	RSVPActionMaybe   = "maybe"
	RSVPActionNo      = "no"
	RSVPActionOutWeek = "out_week"
	RSVPActionSuggest = "suggest"
)

// rsvpTokenTTLDays is how long an emailed RSVP link stays valid before it
// expires (mirrors the sessions RSVP + slot-proposal token TTL).
const rsvpTokenTTLDays = 7

// NotifCalendarRSVP is the notification type constant for calendar-event RSVP
// bell notifications (collection enabled → members; response received → owner).
// Distinct from the sessions/scheduler notification types so the two features
// never collide in the shared, generic notifications store (T-B2).
const NotifCalendarRSVP = "calendar_rsvp"

// EventRSVP is one member's RSVP to a calendar event. One row per (event,user);
// re-responding UPSERTs (UNIQUE(event_id,user_id)). Note carries the free-text
// from a "suggest another time" response.
type EventRSVP struct {
	ID        int       `json:"-"`
	EventID   string    `json:"event_id"`
	UserID    string    `json:"user_id"`
	Status    string    `json:"status"` // yes | maybe | no
	Note      *string   `json:"note,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`

	// Joined for the per-person breakdown (Owner/co-DM only — detail gating).
	DisplayName string  `json:"display_name,omitempty"`
	AvatarPath  *string `json:"avatar_path,omitempty"`
}

// EventRSVPToken is a single-use, expiring emailed RSVP link. The token itself
// is a DB-stored OPAQUE credential (crypto/rand hex; the house pattern, NOT
// HMAC-signed), encoding a single (event, user, action).
type EventRSVPToken struct {
	ID        int
	Token     string
	EventID   string
	UserID    string
	Action    string // yes | maybe | no | out_week | suggest
	UsedAt    *time.Time
	ExpiresAt time.Time
	CreatedAt time.Time
}

// RSVPCounts is the aggregate everyone who can view the event may see.
type RSVPCounts struct {
	Yes   int `json:"yes"`
	Maybe int `json:"maybe"`
	No    int `json:"no"`
}

// Total returns the number of members who have responded (any status).
func (c RSVPCounts) Total() int { return c.Yes + c.Maybe + c.No }

// RSVPSummary is the read model for the counts endpoint + the in-app panel.
// Counts + MyStatus are for everyone with view access; People (the per-user
// breakdown) is populated ONLY for Owner/co-DM (mirrors the scheduler overlay's
// detail gating).
type RSVPSummary struct {
	EventID  string      `json:"event_id"`
	Counts   RSVPCounts  `json:"counts"`
	MyStatus string      `json:"my_status,omitempty"`
	People   []EventRSVP `json:"people,omitempty"`
}

// ValidRSVPStatus reports whether s is one of the three RSVP statuses.
func ValidRSVPStatus(s string) bool {
	return s == RSVPStatusYes || s == RSVPStatusMaybe || s == RSVPStatusNo
}

// validRSVPAction reports whether a is one of the five token/in-app actions.
func validRSVPAction(a string) bool {
	switch a {
	case RSVPActionYes, RSVPActionMaybe, RSVPActionNo, RSVPActionOutWeek, RSVPActionSuggest:
		return true
	default:
		return false
	}
}

// rsvpStatusForAction maps a token action to the RSVP status it records. The
// two email-only verbs both record "no" (out_week is an explicit decline;
// suggest declines-with-a-note), so the counts always reflect a real decision.
func rsvpStatusForAction(action string) string {
	switch action {
	case RSVPActionYes:
		return RSVPStatusYes
	case RSVPActionMaybe:
		return RSVPStatusMaybe
	default: // no, out_week, suggest
		return RSVPStatusNo
	}
}
