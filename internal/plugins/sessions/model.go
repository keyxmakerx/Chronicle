// Package sessions manages game session scheduling, linked entities, and RSVP
// tracking for Chronicle campaigns. Sessions bridge worldbuilding and actual
// play by recording when games happen, who attended, and which entities were
// involved.
package sessions

import "time"

// Session status constants.
const (
	StatusPlanned   = "planned"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"
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
	SortOrder     int        `json:"sort_order"`
	CreatedBy     string     `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	// Joined data (not always populated).
	Attendees []Attendee     `json:"attendees,omitempty"`
	Entities  []SessionEntity `json:"entities,omitempty"`
	CreatorName string       `json:"creator_name,omitempty"`
}

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
	Name          string
	Summary       *string
	ScheduledDate *string
	CalendarYear  *int
	CalendarMonth *int
	CalendarDay   *int
	CreatedBy     string
}

// UpdateSessionInput is the validated input for updating a session.
type UpdateSessionInput struct {
	Name          string
	Summary       *string
	ScheduledDate *string
	CalendarYear  *int
	CalendarMonth *int
	CalendarDay   *int
	Status        string
}

// SessionListData holds data for the session list page.
type SessionListData struct {
	Sessions []Session
	Campaign interface{} // *campaigns.Campaign, avoid import cycle.
}
