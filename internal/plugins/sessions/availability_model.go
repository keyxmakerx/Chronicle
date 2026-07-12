package sessions

import "time"

// Availability state constants. A recurring block is stored only when a member
// is available or actively prefers a time; the ABSENCE of a row means
// "unavailable" (the default wash in the overlay). Exceptions may carry an
// explicit "unavailable" to punch a hole in the recurring pattern for one date.
const (
	AvailAvailable   = "available"
	AvailPreferred   = "preferred"
	AvailUnavailable = "unavailable"
)

// AvailabilityBlock is one stored recurring weekly block, expressed as a
// ZONE-LOCAL wall-clock (RC-12.5): a weekday plus a [start,end) minute range in
// the member's own IANA zone. Never a UTC instant.
type AvailabilityBlock struct {
	ID          string
	CampaignID  string
	UserID      string
	DayOfWeek   int // 0=Sun..6=Sat
	StartMinute int // minutes from local midnight
	EndMinute   int
	State       string // available | preferred
	TZ          string // IANA zone the wall-clock is in
	UpdatedAt   time.Time
}

// AvailabilityException overrides the recurring pattern for one real-world
// (Gregorian) date — e.g. "this week I'm away Thursday". When a member has any
// exception rows for a date, those rows REPLACE the recurring pattern for that
// date in the overlay projection.
type AvailabilityException struct {
	ID          string
	CampaignID  string
	UserID      string
	OnDate      string // YYYY-MM-DD
	StartMinute int
	EndMinute   int
	State       string // available | preferred | unavailable
	TZ          string
	UpdatedAt   time.Time
}

// --- API request DTOs (camelCase JSON, consumed by availability.js) ---

// SaveAvailabilityRequest is the bulk save of a member's recurring pattern.
// The whole pattern is replaced atomically (replace-all), so the client always
// sends the complete current grid.
type SaveAvailabilityRequest struct {
	TZ     string                 `json:"tz"`
	Blocks []AvailabilityBlockDTO `json:"blocks"`
}

// AvailabilityBlockDTO is one block in a save request or a "my availability"
// response.
type AvailabilityBlockDTO struct {
	DayOfWeek   int    `json:"dayOfWeek"`   // 0=Sun..6=Sat
	StartMinute int    `json:"startMinute"` // minutes from local midnight
	EndMinute   int    `json:"endMinute"`
	State       string `json:"state"` // available | preferred
}

// MyAvailabilityResponse is the current user's own recurring pattern, returned
// to seed the paint grid.
type MyAvailabilityResponse struct {
	TZ     string                 `json:"tz"`
	Blocks []AvailabilityBlockDTO `json:"blocks"`
}

// AddExceptionRequest adds a per-date override to the current user's pattern.
type AddExceptionRequest struct {
	OnDate      string `json:"onDate"` // YYYY-MM-DD
	StartMinute int    `json:"startMinute"`
	EndMinute   int    `json:"endMinute"`
	State       string `json:"state"` // available | preferred | unavailable
	TZ          string `json:"tz"`
}

// --- Overlay result types (the DM heatmap payload, projected to viewer zone) ---

// WeekOverlay is the heatmap payload for one week, already projected into the
// viewer's zone. Per-hour density (Free/Prefer counts) is visible to ALL
// members; per-member identity (Members[].Lanes and OverlayHour.FreeIDs/
// PreferIDs) is populated ONLY when IncludeDetail is true — owner / DM-granted
// (design §5 / Q1: members see the anonymous aggregate, the DM sees who).
type WeekOverlay struct {
	WeekStart     string          `json:"weekStart"` // YYYY-MM-DD, first column (viewer zone)
	ViewerTZ      string          `json:"viewerTz"`  // IANA label rendered on the grid
	TotalMembers  int             `json:"totalMembers"`
	IncludeDetail bool            `json:"includeDetail"`
	Days          []OverlayDay    `json:"days"`    // exactly 7 columns
	Members       []OverlayMember `json:"members"` // roster (colors/names) — detail only
}

// OverlayDay is one column: a real date and its 24 per-hour aggregate cells.
type OverlayDay struct {
	Date    string        `json:"date"`    // YYYY-MM-DD
	Weekday int           `json:"weekday"` // 0=Sun..6=Sat
	Hours   []OverlayHour `json:"hours"`   // 24 entries; index == hour 0..23
}

// OverlayHour is the aggregate for one hour cell in the viewer's zone. Free
// counts members available (preferred implies available); Prefer is the subset
// who actively prefer it. The client derives the ★ full-house-and-keen marker
// from Free==TotalMembers && Prefer==TotalMembers.
type OverlayHour struct {
	Free      int      `json:"free"`
	Prefer    int      `json:"prefer"`
	FreeIDs   []string `json:"freeIds,omitempty"`   // detail only
	PreferIDs []string `json:"preferIds,omitempty"` // detail only
}

// OverlayMember is one member's roster entry: a stable color for their lane
// plus, for the DM, their projected per-day availability lanes.
type OverlayMember struct {
	UserID string        `json:"userId"`
	Name   string        `json:"name"`
	Color  string        `json:"color"`
	Avatar *string       `json:"avatar,omitempty"`
	Role   string        `json:"role"` // "DM" or "player"
	Lanes  []LaneSegment `json:"lanes,omitempty"`
}

// LaneSegment is one contiguous availability run for a member on one column,
// already converted to the viewer's zone.
type LaneSegment struct {
	DayIndex    int    `json:"day"`   // 0..6 (column offset from WeekStart)
	StartMinute int    `json:"start"` // viewer-zone minute-of-day
	EndMinute   int    `json:"end"`
	State       string `json:"state"` // available | preferred
}

// overlayMemberInput is the handler→service projection input, mapped from
// campaigns.CampaignMember so the pure builder stays free of the campaigns
// import and is trivially unit-testable.
type overlayMemberInput struct {
	UserID  string
	Name    string
	Avatar  *string
	IsOwner bool
}

// availabilityPalette is a colorblind-friendly (CVD-checked) set of lane
// colors, assigned to members by stable index so a member keeps one color
// across renders. Mirrors the signed mockup's member palette intent; the
// per-campaign owner-editable colors of later phases can override these.
var availabilityPalette = []string{
	"#2a78d6", // blue
	"#e34948", // red
	"#eb6834", // orange
	"#e87ba4", // pink
	"#eda100", // amber
	"#3fa672", // green
	"#8b5cf6", // violet
	"#0891b2", // cyan
	"#b45309", // brown
	"#64748b", // slate
}

// paletteColor returns a stable lane color for the member at index i.
func paletteColor(i int) string {
	return availabilityPalette[i%len(availabilityPalette)]
}
