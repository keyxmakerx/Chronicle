// Package sessions manages game session scheduling, linked entities, and RSVP
// tracking for Chronicle campaigns. Sessions bridge worldbuilding and actual
// play by recording when games happen, who attended, and which entities were
// involved.
package sessions

import (
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

// Session status constants.
const (
	StatusPlanned   = "planned"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"
)

// Recurrence type constants for repeating sessions.
const (
	RecurrenceWeekly   = "weekly"   // Every week on the same day.
	RecurrenceBiWeekly = "biweekly" // Every 2 weeks on the same day.
	RecurrenceMonthly  = "monthly"  // Same day-of-month each month.
	RecurrenceCustom   = "custom"   // Every N weeks (recurrence_interval).
)

// Attendee RSVP status constants.
const (
	RSVPInvited   = "invited"
	RSVPAccepted  = "accepted"
	RSVPDeclined  = "declined"
	RSVPTentative = "tentative"
	// RSVPCarriedYes is a proposal-confirmed "yes" vote that still needs the
	// member's own explicit confirmation before it counts as Going (operator
	// server-change #3: a yes vote on the winning option is not automatically
	// a Going). A member carries this status until they submit their own
	// accepted/declined/tentative answer through the normal RSVP flow, which
	// simply overwrites it — there is no separate "confirm" action.
	RSVPCarriedYes = "carried_yes"
)

// RSVP-token action constants for session_rsvp_tokens.action, beyond the
// accept/decline/tentative actions that reuse the RSVP status constants
// above. RSVPActionSuggest marks an emailed "suggest another time" link,
// whose token does not resolve to one of those three fixed outcomes.
const RSVPActionSuggest = "suggest"

// Session entity role constants.
const (
	EntityRoleMentioned   = "mentioned"
	EntityRoleEncountered = "encountered"
	EntityRoleKey         = "key"
)

// Session represents a game session for a campaign.
type Session struct {
	ID            string  `json:"id"`
	CampaignID    string  `json:"campaign_id"`
	Name          string  `json:"name"`
	Summary       *string `json:"summary,omitempty"`
	Notes         *string `json:"-"`                        // ProseMirror JSON, GM-only.
	NotesHTML     *string `json:"notes_html,omitempty"`     // Pre-rendered HTML.
	Recap         *string `json:"-"`                        // ProseMirror JSON, visible to all members.
	RecapHTML     *string `json:"recap_html,omitempty"`     // Pre-rendered HTML.
	ScheduledDate *string `json:"scheduled_date,omitempty"` // YYYY-MM-DD format.
	// ScheduledTime is the wall-clock start time as "HH:MM" (24-hour), zone-less
	// like ScheduledDate. Set from a confirmed proposal's winning UTC instant
	// (converted to the confirmer's zone) or the create/edit modal. nil means
	// no time set.
	ScheduledTime *string `json:"scheduled_time,omitempty"`
	// ScheduledTZ is the IANA zone ScheduledTime was set in — the organizer's
	// zone at confirm/create time (game-night server change #1). Nil for
	// every row that predates it and for a manual session whose creator's
	// zone was never captured; a viewer with no zone of their own falls back
	// to THIS zone for display, never UTC and never a calendar's own zone
	// (a session is not necessarily tied to one calendar row).
	ScheduledTZ   *string `json:"scheduled_tz,omitempty"`
	CalendarYear  *int    `json:"calendar_year,omitempty"`
	CalendarMonth *int    `json:"calendar_month,omitempty"`
	CalendarDay   *int    `json:"calendar_day,omitempty"`
	Status        string  `json:"status"`

	// Recurrence fields for repeating sessions (e.g. "every other Saturday").
	IsRecurring         bool    `json:"is_recurring"`
	RecurrenceType      *string `json:"recurrence_type,omitempty"`        // weekly, biweekly, monthly, custom
	RecurrenceInterval  int     `json:"recurrence_interval,omitempty"`    // N for "every N weeks" (custom type)
	RecurrenceDayOfWeek *int    `json:"recurrence_day_of_week,omitempty"` // 0=Sun, 1=Mon, ..., 6=Sat
	RecurrenceEndDate   *string `json:"recurrence_end_date,omitempty"`    // YYYY-MM-DD when recurrence stops

	SortOrder int       `json:"sort_order"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// DeletedAt marks a soft-deleted (cancelled/removed) session, restorable
	// by a co-Director/Owner. Nil means live. Every repository read filters
	// deleted_at IS NULL except the restore path.
	DeletedAt *time.Time `json:"-"`

	// Joined data (not always populated).
	Attendees   []Attendee      `json:"attendees,omitempty"`
	Entities    []SessionEntity `json:"entities,omitempty"`
	CreatorName string          `json:"creator_name,omitempty"`
}

// GetCampaignID returns the campaign this session belongs to. Implements
// middleware.CampaignScoped for generic IDOR protection.
func (s *Session) GetCampaignID() string { return s.CampaignID }

// IsPlanned returns true if the session hasn't happened yet.
func (s *Session) IsPlanned() bool {
	return s.Status == StatusPlanned
}

// WorldDate is an in-world calendar date as stored on a session. It carries
// no calendar id: a session's date is a bare year/month/day triple.
type WorldDate struct {
	Year, Month, Day int
}

// HasCalendarDate returns true if the session has an in-game date set.
func (s *Session) HasCalendarDate() bool {
	return s.CalendarYear != nil && s.CalendarMonth != nil && s.CalendarDay != nil
}

// Attendee represents a campaign member's RSVP status for a session.
type Attendee struct {
	ID          int        `json:"id"`
	SessionID   string     `json:"session_id"`
	UserID      string     `json:"user_id"`
	Status      string     `json:"status"` // invited, accepted, declined, tentative, carried_yes
	RespondedAt *time.Time `json:"responded_at,omitempty"`
	// Note is a short player-set note on their own RSVP. Nil means none —
	// stored as SQL NULL, and an explicit empty-string write also clears it
	// (see sessionRepository.SetAttendeeNote). json:"-": nothing serializes
	// Attendee to JSON today (every read renders Templ/HTML); a future JSON
	// endpoint must opt back in deliberately rather than hand every
	// attendee's note to every viewer by default.
	Note *string `json:"-"`
	// ExcludedFromCount is the Director's own "leave myself out of the N/M
	// tally" switch (operator answer #3). Only ever true on the row of the
	// user who set it on themselves; excludes the row from both the
	// numerator and denominator of any going/total count while leaving the
	// individual answer visible to them and to the Director.
	ExcludedFromCount bool `json:"excluded_from_count"`
	// NeedsRecheck is set when the session moved or was cancelled AFTER this
	// member had already answered. The answer itself is never cleared — this
	// only flags "look again".
	NeedsRecheck bool `json:"needs_recheck"`

	// Joined data.
	DisplayName string  `json:"display_name,omitempty"`
	AvatarPath  *string `json:"avatar_path,omitempty"`
}

// OccurrenceRSVP is one member's answer to a SPECIFIC night of a repeating
// session — the per-occurrence twin of Attendee. See session_occurrence_rsvps.
type OccurrenceRSVP struct {
	ID                int
	SessionID         string
	UserID            string
	OccurrenceDate    string // YYYY-MM-DD
	Status            string
	Note              *string
	ExcludedFromCount bool
	NeedsRecheck      bool
	RespondedAt       *time.Time

	// Joined data, populated by ListOccurrenceRSVPs for rendering a roster.
	DisplayName string
	AvatarPath  *string
}

// RescheduleSuggestion is a member's "suggest another time" reply, recorded
// from the emailed suggest-token flow (session_reschedule_suggestions).
type RescheduleSuggestion struct {
	ID             int
	SessionID      string
	UserID         string
	OccurrenceDate *string // set only when suggesting against one night of a series
	SuggestedDate  string
	SuggestedTime  *string
	Note           *string
	CreatedAt      time.Time
}

// CalendarFeedToken is one member's private, replaceable game-night calendar
// feed credential (session_calendar_feed_tokens) — operator answer #1.
type CalendarFeedToken struct {
	ID         int
	CampaignID string
	UserID     string
	Token      string
	CreatedAt  time.Time
}

// SessionEntity represents an entity linked to a session.
type SessionEntity struct {
	ID        int    `json:"id"`
	SessionID string `json:"session_id"`
	EntityID  string `json:"entity_id"`
	Role      string `json:"role"` // mentioned, encountered, key

	// Joined data.
	EntityName string `json:"entity_name,omitempty"`
	EntitySlug string `json:"entity_slug,omitempty"`
}

// --- DTOs ---

// CreateSessionInput is the validated input for creating a session.
type CreateSessionInput struct {
	Name          string
	Summary       *string
	ScheduledDate *string
	ScheduledTime *string
	// ScheduledTZ is the organizer's IANA zone ScheduledTime was set in (see
	// Session.ScheduledTZ). Nil for a manual session whose creator's zone
	// wasn't captured.
	ScheduledTZ         *string
	CalendarYear        *int
	CalendarMonth       *int
	CalendarDay         *int
	IsRecurring         bool
	RecurrenceType      *string
	RecurrenceInterval  int
	RecurrenceDayOfWeek *int
	RecurrenceEndDate   *string
	CreatedBy           string
}

// UpdateSessionInput is the validated input for updating a session.
//
// Every field is a patch.Field: this is a PARTIAL update — an absent key
// preserves the stored value, an explicit null clears it, a present value
// replaces it. Do not re-introduce a value-typed field here;
// partial_update_test.go reddens if you do.
type UpdateSessionInput struct {
	Name                patch.Field[string]
	Summary             patch.Field[string]
	ScheduledDate       patch.Field[string]
	ScheduledTime       patch.Field[string]
	CalendarYear        patch.Field[int]
	CalendarMonth       patch.Field[int]
	CalendarDay         patch.Field[int]
	Status              patch.Field[string]
	IsRecurring         patch.Field[bool]
	RecurrenceType      patch.Field[string]
	RecurrenceInterval  patch.Field[int]
	RecurrenceDayOfWeek patch.Field[int]
	RecurrenceEndDate   patch.Field[string]
}

// SessionListData holds data for the session list page.
type SessionListData struct {
	Sessions []Session
	Campaign interface{} // *campaigns.Campaign, avoid import cycle.
}

// FormatScheduledDate returns a human-readable date string like "Sat, Mar 8, 2028"
// from the YYYY-MM-DD scheduled_date field, with the wall-clock time appended
// ("Sat, Mar 8, 2028 · 7:00 PM") when scheduled_time is set. Returns empty
// string if no date is set.
func (s *Session) FormatScheduledDate() string {
	if s.ScheduledDate == nil || *s.ScheduledDate == "" {
		return ""
	}
	t, err := time.Parse("2006-01-02", *s.ScheduledDate)
	if err != nil {
		// Try parsing with time component in case DB returns datetime format.
		t, err = time.Parse(time.RFC3339, *s.ScheduledDate)
		if err != nil {
			return *s.ScheduledDate
		}
	}
	out := t.Format("Mon, Jan 2, 2006")
	if tl := s.FormatScheduledTime(); tl != "" {
		out += " · " + tl
	}
	return out
}

// FormatScheduledWhen is FormatScheduledDate plus the zone the time was set
// in ("Sat, Oct 10, 2026 · 7:00 PM CDT"), for places like the invite email
// that a reader sees outside Chronicle, with no page converting the time to
// theirs. Without a stored zone or time it is FormatScheduledDate unchanged.
func (s *Session) FormatScheduledWhen() string {
	out := s.FormatScheduledDate()
	if out == "" || s.ScheduledTZ == nil || s.ScheduledTime == nil || *s.ScheduledTime == "" {
		return out
	}
	loc, err := time.LoadLocation(*s.ScheduledTZ)
	if err != nil {
		return out
	}
	at, err := time.ParseInLocation("2006-01-02 15:04", *s.ScheduledDate+" "+*s.ScheduledTime, loc)
	if err != nil {
		return out
	}
	if abbr := timeutil.ZoneAbbrevOrEmpty(*s.ScheduledTZ, at); abbr != "" {
		out += " " + abbr
	}
	return out
}

// FormatScheduledTime renders the "HH:MM" (24-hour) scheduled_time as a friendly
// 12-hour clock ("7:00 PM"), or "" if unset/unparseable. Zone-less, matching the
// zone-less scheduled_date (the confirmed slot's wall-clock for the group).
func (s *Session) FormatScheduledTime() string {
	if s.ScheduledTime == nil || *s.ScheduledTime == "" {
		return ""
	}
	t, err := time.Parse("15:04", *s.ScheduledTime)
	if err != nil {
		return *s.ScheduledTime
	}
	return t.Format("3:04 PM")
}

// RecurrenceLabel returns a human-readable label for the recurrence pattern.
// e.g. "Every week", "Every 2 weeks", "Monthly".
func (s *Session) RecurrenceLabel() string {
	if !s.IsRecurring || s.RecurrenceType == nil {
		return ""
	}
	switch *s.RecurrenceType {
	case RecurrenceWeekly:
		return "Every week"
	case RecurrenceBiWeekly:
		return "Every 2 weeks"
	case RecurrenceMonthly:
		return "Monthly"
	case RecurrenceCustom:
		if s.RecurrenceInterval > 1 {
			return fmt.Sprintf("Every %d weeks", s.RecurrenceInterval)
		}
		return "Every week"
	default:
		return ""
	}
}

// RSVPToken holds a one-time-use RSVP token for email-based responses.
type RSVPToken struct {
	ID        int        `json:"id"`
	Token     string     `json:"token"`
	SessionID string     `json:"session_id"`
	UserID    string     `json:"user_id"`
	Action    string     `json:"action"` // accepted, declined, tentative
	UsedAt    *time.Time `json:"used_at,omitempty"`
	ExpiresAt time.Time  `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
}
