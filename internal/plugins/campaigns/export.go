// Package campaigns — export.go defines the JSON structure for campaign exports.
// A campaign export is a self-contained JSON document that captures all
// campaign data: metadata, entity types, entities, tags, relations, calendar,
// timelines, sessions, maps, notes, and addon configuration.
//
// Media files are not embedded in the JSON itself; the export includes a
// media manifest with file metadata, and ?include_media=1 bundles the actual
// bytes alongside it in a zip (see export_handler.go).
package campaigns

import (
	"encoding/json"
	"time"
)

// ExportFormat is the format identifier for Chronicle campaign exports.
const ExportFormat = "chronicle-campaign-v1"

// ExportVersion is the current schema version.
const ExportVersion = 1

// CampaignExport is the top-level JSON envelope for a campaign export.
type CampaignExport struct {
	Format  string `json:"format"`  // "chronicle-campaign-v1"
	Version int    `json:"version"` // schema version

	Campaign    ExportCampaignMeta  `json:"campaign"`
	EntityTypes []ExportEntityType  `json:"entity_types"`
	Entities    []ExportEntity      `json:"entities"`
	Tags        []ExportTag         `json:"tags"`
	EntityTags  []ExportEntityTag   `json:"entity_tags"`
	Relations   []ExportRelation    `json:"relations"`
	Groups      []ExportGroup       `json:"groups,omitempty"`
	Posts       []ExportPost        `json:"posts,omitempty"`
	Calendar    *ExportCalendarData `json:"calendar,omitempty"`
	Timelines   []ExportTimeline    `json:"timelines,omitempty"`
	Sessions    []ExportSession     `json:"sessions,omitempty"`
	Maps        []ExportMap         `json:"maps,omitempty"`
	Notes       []ExportNote        `json:"notes,omitempty"`
	Addons      []ExportAddon       `json:"addons,omitempty"`
	Media       []ExportMediaFile   `json:"media,omitempty"`

	// ExportedAt records when this export was generated.
	ExportedAt time.Time `json:"exported_at"`
}

// --- Campaign Metadata ---

// ExportCampaignMeta holds the campaign-level configuration.
type ExportCampaignMeta struct {
	Name            string          `json:"name"`
	Description     *string         `json:"description,omitempty"`
	IsPublic        bool            `json:"is_public"`
	Settings        json.RawMessage `json:"settings,omitempty"`
	SidebarConfig   json.RawMessage `json:"sidebar_config,omitempty"`
	DashboardLayout json.RawMessage `json:"dashboard_layout,omitempty"`
}

// --- Entity Types ---

// ExportEntityType captures an entity type template with fields and layout.
type ExportEntityType struct {
	// OriginalID is the entity type's original ID, used to remap entity references.
	OriginalID      int             `json:"original_id"`
	Slug            string          `json:"slug"`
	Name            string          `json:"name"`
	NamePlural      string          `json:"name_plural"`
	Icon            string          `json:"icon"`
	Color           string          `json:"color"`
	Description     *string         `json:"description,omitempty"`
	PinnedEntityIDs []string        `json:"pinned_entity_ids,omitempty"`
	DashboardLayout *string         `json:"dashboard_layout,omitempty"`
	Fields          json.RawMessage `json:"fields"`
	Layout          json.RawMessage `json:"layout"`
	SortOrder       int             `json:"sort_order"`
	IsDefault       bool            `json:"is_default"`
	Enabled         bool            `json:"enabled"`
}

// --- Entities ---

// ExportEntity captures a single entity with all its data.
type ExportEntity struct {
	OriginalID     string                   `json:"original_id"`
	EntityTypeSlug string                   `json:"entity_type_slug"`
	Name           string                   `json:"name"`
	Slug           string                   `json:"slug"`
	Entry          *string                  `json:"entry,omitempty"`
	EntryHTML      *string                  `json:"entry_html,omitempty"`
	ImagePath      *string                  `json:"image_path,omitempty"`
	ParentSlug     *string                  `json:"parent_slug,omitempty"`
	TypeLabel      *string                  `json:"type_label,omitempty"`
	IsPrivate      bool                     `json:"is_private"`
	IsTemplate     bool                     `json:"is_template"`
	Visibility     string                   `json:"visibility,omitempty"`
	Permissions    []ExportEntityPermission `json:"permissions,omitempty"`
	FieldsData     json.RawMessage          `json:"fields_data,omitempty"`
	FieldOverrides json.RawMessage          `json:"field_overrides,omitempty"`
	PopupConfig    json.RawMessage          `json:"popup_config,omitempty"`
}

// --- Tags ---

// ExportTag captures a tag definition.
type ExportTag struct {
	OriginalID int    `json:"original_id"`
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	Color      string `json:"color"`
	DmOnly     bool   `json:"dm_only"`
}

// ExportEntityTag captures a tag assignment to an entity.
type ExportEntityTag struct {
	EntitySlug string `json:"entity_slug"`
	TagSlug    string `json:"tag_slug"`
}

// --- Relations ---

// ExportRelation captures a bidirectional relationship between two entities.
type ExportRelation struct {
	SourceEntitySlug    string          `json:"source_entity_slug"`
	TargetEntitySlug    string          `json:"target_entity_slug"`
	RelationType        string          `json:"relation_type"`
	ReverseRelationType string          `json:"reverse_relation_type"`
	Metadata            json.RawMessage `json:"metadata,omitempty"`
	DmOnly              bool            `json:"dm_only,omitempty"`
}

// --- Calendar ---

// ExportCalendarData captures the full calendar configuration and events.
// Reuses the existing calendar export types for sub-resources.
//
// Visibility/VisibilityRules are additive fields (V5, #778): a calendar can
// itself be dm_only (see calendar.Calendar), independent of any individual
// event's visibility. Omitting these on export would mean every restored
// calendar comes back as "everyone" regardless of how it was set before the
// backup — see calendarImportAdapter.ImportCalendar's doc comment for how a
// pre-V5 backup (which never had this field) is handled on import.
type ExportCalendarData struct {
	Name             string                  `json:"name"`
	Description      *string                 `json:"description,omitempty"`
	Mode             string                  `json:"mode"`
	Visibility       string                  `json:"visibility,omitempty"`
	VisibilityRules  *string                 `json:"visibility_rules,omitempty"`
	EpochName        *string                 `json:"epoch_name,omitempty"`
	CurrentYear      int                     `json:"current_year"`
	CurrentMonth     int                     `json:"current_month"`
	CurrentDay       int                     `json:"current_day"`
	CurrentHour      int                     `json:"current_hour"`
	CurrentMinute    int                     `json:"current_minute"`
	HoursPerDay      int                     `json:"hours_per_day"`
	MinutesPerHour   int                     `json:"minutes_per_hour"`
	SecondsPerMinute int                     `json:"seconds_per_minute"`
	LeapYearEvery    int                     `json:"leap_year_every"`
	LeapYearOffset   int                     `json:"leap_year_offset"`
	Months           []ExportCalendarMonth   `json:"months"`
	Weekdays         []ExportCalendarWeekday `json:"weekdays"`
	Moons            []ExportCalendarMoon    `json:"moons,omitempty"`
	Seasons          []ExportCalendarSeason  `json:"seasons,omitempty"`
	Eras             []ExportCalendarEra     `json:"eras,omitempty"`
	EventCategories  []ExportEventCategory   `json:"event_categories,omitempty"`
	Events           []ExportCalendarEvent   `json:"events,omitempty"`
}

// ExportCalendarMonth is a month definition for export.
type ExportCalendarMonth struct {
	Name          string `json:"name"`
	Days          int    `json:"days"`
	SortOrder     int    `json:"sort_order"`
	IsIntercalary bool   `json:"is_intercalary"`
	LeapYearDays  int    `json:"leap_year_days"`
}

// ExportCalendarWeekday is a weekday definition for export.
type ExportCalendarWeekday struct {
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}

// ExportCalendarMoon is a moon definition for export.
//
// HiddenFromPlayers is an additive field (V5, #778): dropping it on export
// would mean an imported campaign always shows a previously GM-only moon to
// players, quietly reversing a secrecy choice the Director made — the same
// class of mistake CLAUDE.md's partial-update warning names ("a rename push
// once bound is_private=false and published a hidden character").
type ExportCalendarMoon struct {
	Name              string  `json:"name"`
	CycleDays         float64 `json:"cycle_days"`
	PhaseOffset       float64 `json:"phase_offset"`
	Color             string  `json:"color"`
	HiddenFromPlayers bool    `json:"hidden_from_players,omitempty"`
}

// ExportCalendarSeason is a season definition for export.
type ExportCalendarSeason struct {
	Name          string  `json:"name"`
	StartMonth    int     `json:"start_month"`
	StartDay      int     `json:"start_day"`
	EndMonth      int     `json:"end_month"`
	EndDay        int     `json:"end_day"`
	Description   *string `json:"description,omitempty"`
	Color         string  `json:"color"`
	WeatherEffect *string `json:"weather_effect,omitempty"`
}

// ExportCalendarEra is an era definition for export.
//
// StartMonth/StartDay/EndMonth/EndDay are additive fields (V5, calendar-v5
// seams, #778): the calendar plugin's Era became day-granular under V5 (see
// calendar.Era), but this struct predates that and originally carried only
// year boundaries. A backup written before this change (or by any importer
// that doesn't know these fields) omits them, which unmarshals as zero —
// calendarImportAdapter treats a zero StartMonth/StartDay as "day-level
// precision unknown" and defaults to month 1 day 1 of the year (and month 12
// day 31 of EndYear when set), the same "whole year" reading
// Era.ContainsDate already gives an end year with no end month/day.
type ExportCalendarEra struct {
	Name        string  `json:"name"`
	StartYear   int     `json:"start_year"`
	StartMonth  int     `json:"start_month,omitempty"`
	StartDay    int     `json:"start_day,omitempty"`
	EndYear     *int    `json:"end_year,omitempty"`
	EndMonth    *int    `json:"end_month,omitempty"`
	EndDay      *int    `json:"end_day,omitempty"`
	Description *string `json:"description,omitempty"`
	Color       string  `json:"color"`
	SortOrder   int     `json:"sort_order"`
}

// ExportEventCategory is an event category definition for export.
type ExportEventCategory struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
}

// ExportCalendarEvent captures a single calendar event.
//
// RecurrenceInterval/RecurrenceEndYear/RecurrenceEndMonth/RecurrenceEndDay/
// RecurrenceMaxOccurrences/VisibilityRules/Announced/Tier/Color/Icon/AllDay/
// Payload are additive fields (V5, #778): calendar.Event grew all of these
// after this struct was written, and dropping them silently on export would
// have been exactly the "backup that looks complete but silently lost data"
// failure this restoration exists to fix — a dm_only event's per-user
// VisibilityRules allow/deny list and a moon-night Payload are both content a
// Director would reasonably expect a backup to keep.
type ExportCalendarEvent struct {
	Name            string  `json:"name"`
	Description     *string `json:"description,omitempty"`
	DescriptionHTML *string `json:"description_html,omitempty"`
	EntitySlug      *string `json:"entity_slug,omitempty"`
	Year            int     `json:"year"`
	Month           int     `json:"month"`
	Day             int     `json:"day"`
	StartHour       *int    `json:"start_hour,omitempty"`
	StartMinute     *int    `json:"start_minute,omitempty"`
	EndYear         *int    `json:"end_year,omitempty"`
	EndMonth        *int    `json:"end_month,omitempty"`
	EndDay          *int    `json:"end_day,omitempty"`
	EndHour         *int    `json:"end_hour,omitempty"`
	EndMinute       *int    `json:"end_minute,omitempty"`
	IsRecurring     bool    `json:"is_recurring"`
	RecurrenceType  *string `json:"recurrence_type,omitempty"`
	// RecurrenceInterval/RecurrenceEnd*/RecurrenceMaxOccurrences mirror
	// calendar.Event's own recurrence fields — see that struct's doc comment.
	RecurrenceInterval       *int   `json:"recurrence_interval,omitempty"`
	RecurrenceEndYear        *int   `json:"recurrence_end_year,omitempty"`
	RecurrenceEndMonth       *int   `json:"recurrence_end_month,omitempty"`
	RecurrenceEndDay         *int   `json:"recurrence_end_day,omitempty"`
	RecurrenceMaxOccurrences *int   `json:"recurrence_max_occurrences,omitempty"`
	Visibility               string `json:"visibility"`
	// VisibilityRules is the optional {allowed_users,denied_users} JSON
	// blob (see calendar.VisibilityRules) — raw, not re-typed, so it
	// round-trips exactly.
	VisibilityRules *string `json:"visibility_rules,omitempty"`
	Category        *string `json:"category,omitempty"`
	// Announced overrides the event's kind's default announced setting; nil
	// means "inherit" (see calendar.Event.EffectiveAnnounced).
	Announced *string `json:"announced,omitempty"`
	Tier      *string `json:"tier,omitempty"`
	Color     *string `json:"color,omitempty"`
	Icon      *string `json:"icon,omitempty"`
	AllDay    bool    `json:"all_day,omitempty"`
	// Payload is the raw JSON extras blob (e.g. a moon-night marker) — see
	// calendar.Event.Payload / calendar.MoonNightPayload.
	Payload *string `json:"payload,omitempty"`
}

// --- Timelines ---

// ExportTimeline captures a timeline with its events, entity groups, and connections.
type ExportTimeline struct {
	Name            string                  `json:"name"`
	Description     *string                 `json:"description,omitempty"`
	DescriptionHTML *string                 `json:"description_html,omitempty"`
	Color           string                  `json:"color"`
	Icon            string                  `json:"icon"`
	Visibility      string                  `json:"visibility"`
	SortOrder       int                     `json:"sort_order"`
	ZoomDefault     string                  `json:"zoom_default"`
	Events          []ExportTimelineEvent   `json:"events,omitempty"`
	EntityGroups    []ExportEntityGroup     `json:"entity_groups,omitempty"`
	Connections     []ExportEventConnection `json:"connections,omitempty"`
}

// ExportTimelineEvent captures a standalone timeline event.
type ExportTimelineEvent struct {
	Name            string  `json:"name"`
	Description     *string `json:"description,omitempty"`
	DescriptionHTML *string `json:"description_html,omitempty"`
	EntitySlug      *string `json:"entity_slug,omitempty"`
	Year            int     `json:"year"`
	Month           int     `json:"month"`
	Day             int     `json:"day"`
	StartHour       *int    `json:"start_hour,omitempty"`
	StartMinute     *int    `json:"start_minute,omitempty"`
	EndYear         *int    `json:"end_year,omitempty"`
	EndMonth        *int    `json:"end_month,omitempty"`
	EndDay          *int    `json:"end_day,omitempty"`
	EndHour         *int    `json:"end_hour,omitempty"`
	EndMinute       *int    `json:"end_minute,omitempty"`
	IsRecurring     bool    `json:"is_recurring"`
	RecurrenceType  *string `json:"recurrence_type,omitempty"`
	Category        *string `json:"category,omitempty"`
	Visibility      string  `json:"visibility"`
	DisplayOrder    int     `json:"display_order"`
	Label           *string `json:"label,omitempty"`
	Color           *string `json:"color,omitempty"`
}

// ExportEntityGroup captures a timeline entity group (swim lane).
type ExportEntityGroup struct {
	Name      string   `json:"name"`
	Color     string   `json:"color"`
	SortOrder int      `json:"sort_order"`
	Members   []string `json:"members,omitempty"` // Entity slugs.
}

// ExportEventConnection captures a connection between two timeline events.
// Source and target are identified by event index within the same timeline export.
type ExportEventConnection struct {
	SourceIndex int     `json:"source_index"` // Index into the timeline's Events slice.
	TargetIndex int     `json:"target_index"` // Index into the timeline's Events slice.
	Label       *string `json:"label,omitempty"`
	Color       *string `json:"color,omitempty"`
	Style       string  `json:"style"`
}

// --- Sessions ---

// ExportSession captures a game session.
type ExportSession struct {
	Name               string                `json:"name"`
	Summary            *string               `json:"summary,omitempty"`
	Notes              *string               `json:"notes,omitempty"`
	NotesHTML          *string               `json:"notes_html,omitempty"`
	Recap              *string               `json:"recap,omitempty"`
	RecapHTML          *string               `json:"recap_html,omitempty"`
	ScheduledDate      *string               `json:"scheduled_date,omitempty"`
	ScheduledTime      *string               `json:"scheduled_time,omitempty"`
	CalendarYear       *int                  `json:"calendar_year,omitempty"`
	CalendarMonth      *int                  `json:"calendar_month,omitempty"`
	CalendarDay        *int                  `json:"calendar_day,omitempty"`
	Status             string                `json:"status"`
	IsRecurring        bool                  `json:"is_recurring"`
	RecurrenceType     *string               `json:"recurrence_type,omitempty"`
	RecurrenceInterval int                   `json:"recurrence_interval,omitempty"`
	SortOrder          int                   `json:"sort_order"`
	Entities           []ExportSessionEntity `json:"entities,omitempty"`
	Attendees          []ExportAttendee      `json:"attendees,omitempty"`
}

// ExportSessionEntity captures an entity linked to a session.
type ExportSessionEntity struct {
	EntitySlug string `json:"entity_slug"`
	Role       string `json:"role"`
}

// ExportAttendee captures a session attendee's RSVP status.
type ExportAttendee struct {
	UserID string `json:"user_id"`
	Status string `json:"status"` // invited, accepted, declined, tentative
}

// --- Maps ---

// ExportMap captures a map with markers, drawings, layers, tokens, and fog.
type ExportMap struct {
	Name        string            `json:"name"`
	Description *string           `json:"description,omitempty"`
	ImageID     *string           `json:"image_id,omitempty"`
	ImageWidth  int               `json:"image_width"`
	ImageHeight int               `json:"image_height"`
	SortOrder   int               `json:"sort_order"`
	Markers     []ExportMarker    `json:"markers,omitempty"`
	Drawings    []ExportDrawing   `json:"drawings,omitempty"`
	Layers      []ExportLayer     `json:"layers,omitempty"`
	Tokens      []ExportToken     `json:"tokens,omitempty"`
	FogRegions  []ExportFogRegion `json:"fog_regions,omitempty"`
}

// ExportMarker captures a map marker.
type ExportMarker struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Icon        string  `json:"icon"`
	Color       string  `json:"color"`
	EntitySlug  *string `json:"entity_slug,omitempty"`
	Visibility  string  `json:"visibility"`
}

// ExportDrawing captures a map drawing.
type ExportDrawing struct {
	DrawingType string          `json:"drawing_type"`
	LayerName   *string         `json:"layer_name,omitempty"`
	Points      json.RawMessage `json:"points"`
	StrokeColor string          `json:"stroke_color"`
	StrokeWidth float64         `json:"stroke_width"`
	FillColor   *string         `json:"fill_color,omitempty"`
	FillAlpha   float64         `json:"fill_alpha"`
	TextContent *string         `json:"text_content,omitempty"`
	FontSize    *int            `json:"font_size,omitempty"`
	Rotation    float64         `json:"rotation"`
	Visibility  string          `json:"visibility"`
}

// ExportLayer captures a map layer.
type ExportLayer struct {
	Name      string  `json:"name"`
	LayerType string  `json:"layer_type"`
	Visible   bool    `json:"visible"`
	Locked    bool    `json:"locked"`
	Opacity   float64 `json:"opacity"`
	SortOrder int     `json:"sort_order"`
}

// ExportToken captures a map token.
type ExportToken struct {
	Name           string          `json:"name"`
	EntitySlug     *string         `json:"entity_slug,omitempty"`
	ImagePath      *string         `json:"image_path,omitempty"`
	LayerName      *string         `json:"layer_name,omitempty"`
	X              float64         `json:"x"`
	Y              float64         `json:"y"`
	Width          float64         `json:"width"`
	Height         float64         `json:"height"`
	Rotation       float64         `json:"rotation"`
	Scale          float64         `json:"scale"`
	IsHidden       bool            `json:"is_hidden"`
	IsLocked       bool            `json:"is_locked"`
	Bar1Value      *int            `json:"bar1_value,omitempty"`
	Bar1Max        *int            `json:"bar1_max,omitempty"`
	Bar2Value      *int            `json:"bar2_value,omitempty"`
	Bar2Max        *int            `json:"bar2_max,omitempty"`
	AuraRadius     *float64        `json:"aura_radius,omitempty"`
	AuraColor      *string         `json:"aura_color,omitempty"`
	LightRadius    *float64        `json:"light_radius,omitempty"`
	LightDimRadius *float64        `json:"light_dim_radius,omitempty"`
	LightColor     *string         `json:"light_color,omitempty"`
	VisionEnabled  bool            `json:"vision_enabled"`
	VisionRange    *float64        `json:"vision_range,omitempty"`
	Elevation      int             `json:"elevation"`
	StatusEffects  json.RawMessage `json:"status_effects,omitempty"`
}

// ExportFogRegion captures a fog of war region.
type ExportFogRegion struct {
	Points     json.RawMessage `json:"points"`
	IsExplored bool            `json:"is_explored"`
}

// --- Notes ---

// ExportNote captures a note. Only shared notes are exported since
// personal notes are user-specific.
type ExportNote struct {
	Title      string  `json:"title"`
	Entry      *string `json:"entry,omitempty"`
	EntryHTML  *string `json:"entry_html,omitempty"`
	EntitySlug *string `json:"entity_slug,omitempty"`
	Color      string  `json:"color"`
	Pinned     bool    `json:"pinned"`

	// IsFolder marks a note that is a container for other notes rather
	// than content itself.
	IsFolder bool `json:"is_folder,omitempty"`

	// Content is the legacy block content (text + checklists). Checklists
	// live only here, never in Entry, so an export that drops this loses
	// every checkbox in the campaign.
	Content json.RawMessage `json:"content,omitempty"`

	// ParentIndex is the index into the export's Notes slice of this
	// note's containing folder, or nil for a top-level note. Index rather
	// than ID for the same reason ExportEventConnection uses indices: note
	// IDs are not stable across instances.
	ParentIndex *int `json:"parent_index,omitempty"`
}

// --- Addons ---

// ExportAddon captures a per-campaign addon configuration.
type ExportAddon struct {
	Slug    string          `json:"slug"`
	Enabled bool            `json:"enabled"`
	Config  json.RawMessage `json:"config,omitempty"`
}

// --- Groups ---

// ExportGroup captures a campaign group with its member user IDs.
// Member IDs are user UUIDs; they are only meaningful if the same users
// exist on the import target instance.
type ExportGroup struct {
	Name        string   `json:"name"`
	Description *string  `json:"description,omitempty"`
	MemberIDs   []string `json:"member_ids,omitempty"`
}

// --- Entity Permissions ---

// ExportEntityPermission captures a single permission grant on an entity.
type ExportEntityPermission struct {
	SubjectType string `json:"subject_type"` // "role", "user", "group"
	SubjectID   string `json:"subject_id"`
	Permission  string `json:"permission"` // "view", "edit"
}

// --- Posts ---

// ExportPost captures an entity sub-note (post).
type ExportPost struct {
	EntitySlug string          `json:"entity_slug"`
	Name       string          `json:"name"`
	Entry      json.RawMessage `json:"entry,omitempty"`
	EntryHTML  *string         `json:"entry_html,omitempty"`
	IsPrivate  bool            `json:"is_private"`
	SortOrder  int             `json:"sort_order"`
}

// --- Media Manifest ---

// ExportMediaFile captures media file metadata for reference remapping.
// Actual file bytes are not included in the JSON export.
type ExportMediaFile struct {
	OriginalID   string `json:"original_id"`
	OriginalName string `json:"original_name"`
	MimeType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
	UsageType    string `json:"usage_type"`
}
