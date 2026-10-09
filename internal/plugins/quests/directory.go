package quests

import "context"

// The interfaces below are owned by this plugin and implemented in
// internal/app over the entities, maps and campaigns services, so quests
// never imports another plugin (plugin isolation).

// EntityInfo is the little this plugin needs to know about a page.
type EntityInfo struct {
	ID        string
	Name      string
	TypeName  string
	TypeSlug  string
	ImagePath string
}

// EntityDirectory answers questions about pages.
type EntityDirectory interface {
	// Entities returns the requested pages that exist IN the campaign; a
	// missing id or one in another campaign is simply absent.
	Entities(ctx context.Context, campaignID string, ids []string) (map[string]EntityInfo, error)
	// FilterViewable returns the subset of ids the viewer may open, using
	// the entities plugin's own visibility rules.
	FilterViewable(ctx context.Context, campaignID string, ids []string, role int, userID string) (map[string]bool, error)
	// Search finds pages by name for the picker, already limited to what
	// the viewer may see.
	Search(ctx context.Context, campaignID, query string, role int, userID string, limit int) ([]EntityInfo, error)
}

// CharacterInfo is a character page and the display name of the member who
// claimed it ("" when unclaimed).
type CharacterInfo struct {
	ID     string
	Name   string
	Player string
}

// CharacterDirectory lists the campaign's characters for handing out rewards.
type CharacterDirectory interface {
	// ListCharacters returns the character pages the viewer may see.
	ListCharacters(ctx context.Context, campaignID string, role int, userID string) ([]CharacterInfo, error)
}

// MapInfo is a map's id and name.
type MapInfo struct {
	ID   string
	Name string
}

// MapDirectory answers questions about maps.
type MapDirectory interface {
	// Maps returns the requested maps that exist in the campaign.
	Maps(ctx context.Context, campaignID string, ids []string) (map[string]MapInfo, error)
	// ListMaps returns every map of the campaign.
	ListMaps(ctx context.Context, campaignID string) ([]MapInfo, error)
	// Enabled reports whether the campaign has the maps addon on. When it is
	// off, Maps and ListMaps find nothing, so map pins and links are refused.
	Enabled(ctx context.Context, campaignID string) (bool, error)
}

// MemberNames resolves campaign members' display names.
type MemberNames interface {
	DisplayNames(ctx context.Context, campaignID string, userIDs []string) (map[string]string, error)
}

// TypeDirectory answers whether a category (entity type) belongs to a campaign,
// so a category id from another campaign can never be used as a board home.
type TypeDirectory interface {
	// TypeInCampaign is false for a missing type and for one of another campaign.
	TypeInCampaign(ctx context.Context, campaignID string, typeID int) (bool, error)
}

// DueDay is a day on the campaign calendar, as stored on a quest sheet.
type DueDay struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

// CalendarMonth is one month of the calendar, as the due-date picker needs it.
type CalendarMonth struct {
	Name     string `json:"name"`
	Days     int    `json:"days"`
	LeapDays int    `json:"leapDays"`
}

// QuestCalendar is the campaign calendar reduced to what a due date needs. All
// date arithmetic lives behind it, in the calendar plugin's own methods, so
// the quests plugin never second-guesses leap rules or month lengths.
type QuestCalendar interface {
	ID() string
	Name() string
	// Today is the calendar's current in-world date.
	Today() DueDay
	Months() []CalendarMonth
	// Leap is the leap-year interval and offset (0 every = no leap years).
	Leap() (every, offset int)
	// Valid reports whether d is a real day of this calendar.
	Valid(d DueDay) bool
	// Label formats d in the calendar's own style.
	Label(d DueDay) string
	// DaysFromToday is d minus today in days; negative when d is past.
	DaysFromToday(d DueDay) int
}

// DueEvent is the calendar event that mirrors a quest's due date.
type DueEvent struct {
	EntityID string
	Title    string
	Day      DueDay
	// DMOnly keeps the event off players' calendars; it is set whenever a
	// plain player may not open the quest page, so a hidden quest's title
	// never reaches them.
	DMOnly bool
	// CreatedBy is the acting user; only used when the event is created.
	CreatedBy string
}

// CalendarDirectory is the quests plugin's view of the campaign calendar. It
// is implemented in internal/app over the calendar service.
type CalendarDirectory interface {
	// Calendar returns the campaign's primary calendar, or nil (and no
	// error) when there is none or the calendar addon is off.
	Calendar(ctx context.Context, campaignID string) (QuestCalendar, error)
	// SaveDueEvent updates the event eventID, or creates one when eventID is
	// empty or the event is gone, and returns the event's id.
	SaveDueEvent(ctx context.Context, campaignID string, cal QuestCalendar, eventID string, ev DueEvent) (string, error)
	// SetDueEventVisibility changes only the event's visibility, and only
	// when it differs. A missing event is not an error.
	SetDueEventVisibility(ctx context.Context, campaignID string, cal QuestCalendar, eventID string, dmOnly bool) error
	// DeleteDueEvent removes the event; a missing event is not an error.
	DeleteDueEvent(ctx context.Context, campaignID string, cal QuestCalendar, eventID string) error
}
