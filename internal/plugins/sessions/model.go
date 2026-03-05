// Package sessions manages game session scheduling, linked entities, and RSVP
// tracking for Chronicle campaigns. Sessions bridge worldbuilding and actual
// play by recording when games happen, who attended, and which entities were
// involved.
package sessions

import (
	"fmt"
	"time"
)

// Session status constants.
const (
	StatusPlanned   = "planned"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"
)

// Recurrence type constants for repeating sessions.
const (
	RecurrenceWeekly    = "weekly"     // Every week on the same day.
	RecurrenceBiWeekly  = "biweekly"   // Every 2 weeks on the same day.
	RecurrenceMonthly   = "monthly"    // Same day-of-month each month.
	RecurrenceCustom    = "custom"     // Every N weeks (recurrence_interval).
)

// Attendee RSVP status constants.
const (
	RSVPInvited   = "invited"
	RSVPAccepted  = "accepted"
	RSVPDeclined  = "declined"
	RSVPTentative = "tentative"
)

// Session entity role constants.
const (
	EntityRoleMentioned   = "mentioned"
	EntityRoleEncountered = "encountered"
	EntityRoleKey         = "key"
)

// Session represents a game session for a campaign.
type Session struct {
	ID            string     `json:"id"`
	CampaignID    string     `json:"campaign_id"`
	Name          string     `json:"name"`
	Summary       *string    `json:"summary,omitempty"`
	Notes         *string    `json:"-"`         // ProseMirror JSON, GM-only.
	NotesHTML     *string    `json:"notes_html,omitempty"` // Pre-rendered HTML.
	ScheduledDate *string    `json:"scheduled_date,omitempty"` // YYYY-MM-DD format.
	CalendarYear  *int       `json:"calendar_year,omitempty"`
	CalendarMonth *int       `json:"calendar_month,omitempty"`
	CalendarDay   *int       `json:"calendar_day,omitempty"`
	Status        string     `json:"status"`

	// Recurrence fields for repeating sessions (e.g. "every other Saturday").
	IsRecurring        bool    `json:"is_recurring"`
	RecurrenceType     *string `json:"recurrence_type,omitempty"`      // weekly, biweekly, monthly, custom
	RecurrenceInterval int     `json:"recurrence_interval,omitempty"`  // N for "every N weeks" (custom type)
	RecurrenceDayOfWeek *int   `json:"recurrence_day_of_week,omitempty"` // 0=Sun, 1=Mon, ..., 6=Sat
	RecurrenceEndDate  *string `json:"recurrence_end_date,omitempty"`  // YYYY-MM-DD when recurrence stops

	SortOrder     int        `json:"sort_order"`
	CreatedBy     string     `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	// Joined data (not always populated).
	Attendees []Attendee     `json:"attendees,omitempty"`
	Entities  []SessionEntity `json:"entities,omitempty"`
	CreatorName string       `json:"creator_name,omitempty"`
}

// GetCampaignID returns the campaign this session belongs to. Implements
// middleware.CampaignScoped for generic IDOR protection.
func (s *Session) GetCampaignID() string { return s.CampaignID }

// IsPlanned returns true if the session hasn't happened yet.
func (s *Session) IsPlanned() bool {
	return s.Status == StatusPlanned
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
	Status      string     `json:"status"` // invited, accepted, declined, tentative
	RespondedAt *time.Time `json:"responded_at,omitempty"`

	// Joined data.
	DisplayName string  `json:"display_name,omitempty"`
	AvatarPath  *string `json:"avatar_path,omitempty"`
}

// SessionEntity represents an entity linked to a session.
type SessionEntity struct {
	ID       int    `json:"id"`
	SessionID string `json:"session_id"`
	EntityID string `json:"entity_id"`
	Role     string `json:"role"` // mentioned, encountered, key

	// Joined data.
	EntityName string `json:"entity_name,omitempty"`
	EntitySlug string `json:"entity_slug,omitempty"`
}

// --- DTOs ---

// CreateSessionInput is the validated input for creating a session.
type CreateSessionInput struct {
	Name                string
	Summary             *string
	ScheduledDate       *string
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
type UpdateSessionInput struct {
	Name                string
	Summary             *string
	ScheduledDate       *string
	CalendarYear        *int
	CalendarMonth       *int
	CalendarDay         *int
	Status              string
	IsRecurring         bool
	RecurrenceType      *string
	RecurrenceInterval  int
	RecurrenceDayOfWeek *int
	RecurrenceEndDate   *string
}

// SessionListData holds data for the session list page.
type SessionListData struct {
	Sessions []Session
	Campaign interface{} // *campaigns.Campaign, avoid import cycle.
}

// FormatScheduledDate returns a human-readable date string like "Sat, Mar 8, 2028"
// from the YYYY-MM-DD scheduled_date field. Returns empty string if not set.
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
	return t.Format("Mon, Jan 2, 2006")
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
