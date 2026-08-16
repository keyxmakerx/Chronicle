package calendar

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// CalendarRepository defines persistence operations for calendars and events.
type CalendarRepository interface {
	// Calendar CRUD.
	Create(ctx context.Context, cal *Calendar) error
	GetByCampaignID(ctx context.Context, campaignID string) (*Calendar, error)
	GetDefaultByCampaignID(ctx context.Context, campaignID string) (*Calendar, error)
	GetByID(ctx context.Context, id string) (*Calendar, error)
	ListByCampaignID(ctx context.Context, campaignID string) ([]Calendar, error)
	SetDefault(ctx context.Context, campaignID, calendarID string) error
	Update(ctx context.Context, cal *Calendar) error
	Delete(ctx context.Context, id string) error

	// Active-calendar pointer (V2 Wave 1 PR 1 / C-CAL-V2-SHELL-FOUNDATION).
	// Returns the user's last-selected calendar ID for the campaign, or
	// "" if none has been recorded. Service layer resolves "" → campaign
	// default. Set writes the pointer; the caller is responsible for
	// validating that calendarID belongs to campaignID.
	GetActiveCalendarID(ctx context.Context, userID, campaignID string) (string, error)
	SetActiveCalendar(ctx context.Context, userID, campaignID, calendarID string) error

	// THE THREE PREFERENCE WRITERS BELOW ALL TAKE A calendarID, and it is not
	// optional. They upsert the SAME calendar_active row as the active-calendar
	// pointer, whose `calendar_id` column is NOT NULL and foreign-keyed to
	// `calendars(id)` (migration 006). Each of them used to insert the literal
	// empty string there as a first-write fallback; no calendar has that id,
	// InnoDB checks the FK on the attempted insert before the duplicate-key path
	// resolves, and so every one of these writes failed with errno 1452 and
	// surfaced as a 500. The caller must supply a calendar that exists in the
	// campaign — the service resolves it with the same active-or-default ladder
	// GetActiveCalendar walks. It seeds the row on first write and is NEVER
	// written over an existing pointer (the conflict clause names only the
	// preference column).

	// Sidebar pin preference (V2 Wave 1.7A §G). Per-user-per-campaign
	// boolean piggybacked on the calendar_active row; defaults TRUE
	// for new rows + backfilled rows per migration 007.
	GetSidebarPinned(ctx context.Context, userID, campaignID string) (bool, error)
	SetSidebarPinned(ctx context.Context, userID, campaignID, calendarID string, pinned bool) error

	// Per-viewer calendar-v4 Block layer set (C-CALV4-LAYERS-P9 [LYR-3]).
	// Piggybacked on the SAME calendar_active row as the two above, per
	// migration 007's coordinator decision (PR #368 stop-and-flag #3).
	//
	// THE nil RETURN IS THE POINT. Get answers (nil, nil) when the column is
	// NULL — "this viewer has never chosen" — and a NON-NIL, possibly EMPTY
	// slice when they have. The caller renders the host's seed for nil and the
	// viewer's own set for anything else, so "every layer off" is a reachable
	// state distinct from "no preference". A []string that collapsed both onto
	// len()==0 would make a bare month unreachable.
	GetBlockLayers(ctx context.Context, userID, campaignID string) ([]string, error)
	SetBlockLayers(ctx context.Context, userID, campaignID, calendarID string, keys []string) error
	GetBenchSections(ctx context.Context, userID, campaignID string) ([]string, error)
	SetBenchSections(ctx context.Context, userID, campaignID, calendarID string, keys []string) error

	// Months.
	SetMonths(ctx context.Context, calendarID string, months []MonthInput) error
	GetMonths(ctx context.Context, calendarID string) ([]Month, error)

	// Weekdays.
	SetWeekdays(ctx context.Context, calendarID string, weekdays []WeekdayInput) error
	GetWeekdays(ctx context.Context, calendarID string) ([]Weekday, error)

	// Moons.
	SetMoons(ctx context.Context, calendarID string, moons []MoonInput) error
	GetMoons(ctx context.Context, calendarID string) ([]Moon, error)

	// Seasons.
	SetSeasons(ctx context.Context, calendarID string, seasons []Season) error
	GetSeasons(ctx context.Context, calendarID string) ([]Season, error)

	// Eras.
	SetEras(ctx context.Context, calendarID string, eras []EraInput) error
	GetEras(ctx context.Context, calendarID string) ([]Era, error)
	// Per-era CRUD (V2 Wave 0 PR 2; complements bulk SetEras for AI Workspace
	// Wave 3 + UI per-card-edit use cases). All three publish
	// `structure.updated` at the service layer.
	CreateEra(ctx context.Context, calendarID string, input EraInput) (*Era, error)
	UpdateEra(ctx context.Context, eraID int, input EraInput) error
	// DeleteEra removes one era + returns the calendarID that owned it
	// so the service can publish a structure.updated event without an
	// extra round-trip. Returns "" if the era didn't exist.
	DeleteEra(ctx context.Context, eraID int) (calendarID string, err error)
	GetEraByID(ctx context.Context, eraID int) (*Era, error)

	// ApplyImport runs the entire calendar-import write workflow within
	// ONE transaction so a partial failure can't leave a calendar in a
	// mixed state (V1 tech debt addressed in V2 Wave 0 PR 2). Replaces
	// what the service-level ApplyImport used to do via 6 sequential
	// Set* calls (each opening its own transaction). The service
	// validates inputs upfront, mutates `cal` to reflect import-side
	// fields, then calls this method.
	ApplyImport(ctx context.Context, cal *Calendar, result *ImportResult) error

	// Event categories.
	SetEventCategories(ctx context.Context, calendarID string, cats []EventCategoryInput) error
	GetEventCategories(ctx context.Context, calendarID string) ([]EventCategory, error)

	// Weather.
	GetWeather(ctx context.Context, calendarID string) (*Weather, error)
	SetWeather(ctx context.Context, calendarID string, input WeatherInput) error

	// Weather zones (V2 Wave 0 PR 3 / C-CAL-WEATHER-ZONES). Zone
	// definitions are calendar-scoped; ApplyWeatherZones replaces the
	// full zone set in one transaction (delete-then-insert pattern
	// mirroring SetMonths / SetSeasons / etc.). SetActiveWeatherZone
	// updates the active-zone reference on the existing calendar_weather
	// row (columns added in migration 003; no schema change here).
	GetWeatherZones(ctx context.Context, calendarID string) ([]WeatherZone, error)
	ApplyWeatherZones(ctx context.Context, calendarID string, zones []WeatherZone) error
	SetActiveWeatherZone(ctx context.Context, calendarID string, zoneID, zoneName string) error

	// Cycles.
	SetCycles(ctx context.Context, calendarID string, cycles []CycleInput) error
	GetCycles(ctx context.Context, calendarID string) ([]Cycle, error)

	// Festivals.
	SetFestivals(ctx context.Context, calendarID string, festivals []FestivalInput) error
	GetFestivals(ctx context.Context, calendarID string) ([]Festival, error)

	// Events.
	CreateEvent(ctx context.Context, evt *Event) error
	GetEvent(ctx context.Context, id string) (*Event, error)
	UpdateEvent(ctx context.Context, evt *Event) error
	DeleteEvent(ctx context.Context, id string) error
	ListEventsForMonth(ctx context.Context, calendarID string, year, month int, role int) ([]Event, error)
	ListEventsForYear(ctx context.Context, calendarID string, year int, role int) ([]Event, error)
	ListEventsForDateRange(ctx context.Context, calendarID string, year, startMonth, startDay, endMonth, endDay int, role int) ([]Event, error)
	ListEventsForEntity(ctx context.Context, entityID string, role int) ([]Event, error)
	ListUpcomingEvents(ctx context.Context, calendarID string, year, month, day int, role int, limit int) ([]Event, error)
	SearchEvents(ctx context.Context, calendarID, query string, role int) ([]Event, error)
	// ListAllEvents returns every event for a calendar with no
	// role-based visibility filtering and no date constraint.
	// Used by the public Foundry-facing API (C-CALENDAR-ENDPOINTS):
	// that API gates by per-campaign signed token rather than user
	// role, and the wire response carries the `visibility` field
	// for the module to interpret. Sort order is calendar-stable
	// (year/month/day/start_hour/start_minute/name) so consumers
	// can rely on it for diffs.
	ListAllEvents(ctx context.Context, calendarID string) ([]Event, error)
	// StrandedEventCounts reports, per calendar in a campaign, how many events
	// point at a month position the calendar no longer has ([GR-18]). One
	// query for the whole campaign; calendars with none are absent.
	StrandedEventCounts(ctx context.Context, campaignID string) (map[string]int, error)

	// Event visibility.
	UpdateEventVisibility(ctx context.Context, eventID string, visibility string, visRules *string) error

	// Per-calendar visibility (C-CAL-DASHBOARD-W5b).
	UpdateCalendarVisibility(ctx context.Context, calendarID string, visibility string, visRules *string) error

	// Batch upcoming-events read for the dashboard (C-CAL-DASHBOARD-W5d) — the
	// next-event sort + the adaptive widget's agenda, in one query.
	EventDatesForCalendars(ctx context.Context, calIDs []string, role int) (map[string][]CalendarEventDate, error)

	// Entity ties (migration 009 / C-CAL-ENTITY-TIES-DATA-MODEL). Cascade
	// on entity/event/era delete is DB-enforced (ON DELETE CASCADE), so
	// there is no unlink-all method. Implementations in
	// entity_ties_repository.go.
	LinkEntityEvent(ctx context.Context, entityID, eventID, role string) error
	UnlinkEntityEvent(ctx context.Context, entityID, eventID string) error
	LinkEntityEra(ctx context.Context, entityID string, eraID int, role *string) error
	UnlinkEntityEra(ctx context.Context, entityID string, eraID int) error
	// EntitiesForEvent/EntitiesForEra take role + userID (viewer context) so
	// the repo can gate the returned entities via entityVisibilityFilter —
	// C-CAL-ENTITY-TIES-LEAK-FIX, mirroring EntitiesForCalendar below.
	EntitiesForEvent(ctx context.Context, eventID string, role int, userID string) ([]EntityTieRef, error)
	EntitiesForEra(ctx context.Context, eraID int, role int, userID string) ([]EntityTieRef, error)
	EntitiesForCalendar(ctx context.Context, calendarID string, role int, userID string) ([]EntityTieRef, error)
	EventsForEntity(ctx context.Context, entityID string) ([]EntityEventTie, error)
	ErasForEntity(ctx context.Context, entityID string) ([]EntityEraTie, error)
	// World-state model (migration 008 / C-CAL-WORLDSTATE-SERVER-MODEL).
	// All reads are scoped to a single date (year/month/day) except
	// GetMoonPhasesForCalendar which loads the named-phase vocab for every
	// moon of a calendar in one query. Implementations live in
	// worldstate_repository.go.
	GetDayWeather(ctx context.Context, calendarID string, year, month, day int) (*DayWeather, error)
	SetDayWeather(ctx context.Context, calendarID string, year, month, day int, weatherType string) error
	GetCelestialEvents(ctx context.Context, calendarID string, year, month, day int) ([]CelestialEvent, error)
	AddCelestialEvent(ctx context.Context, ce CelestialEvent) error
	// ClearCelestialEvents removes every celestial event on a calendar's given
	// date (C-CAL-GM-PANEL-REWORK B — the GM "clear world-events" off switch).
	ClearCelestialEvents(ctx context.Context, calendarID string, year, month, day int) error
	// ClearCelestialEventsByType removes only the given TYPE's events on the
	// date (the GM panel's per-event off switch; ClearCelestialEvents is the
	// clear-all).
	ClearCelestialEventsByType(ctx context.Context, calendarID string, year, month, day int, eventType string) error
	GetMoonPhasesForCalendar(ctx context.Context, calendarID string) (map[int][]MoonPhaseVocab, error)
	GetSpecialDays(ctx context.Context, calendarID string, year, month, day int) ([]SpecialDay, error)
	SetMoodTint(ctx context.Context, calendarID string, color *string, intensity *float64) error
	// SetRealDateAnchor writes migration 018's four anchor columns as ONE
	// value: pass a fully-populated anchor to set it, or nil to clear it.
	//
	// The signature is deliberately not four pointers. A partial anchor maps
	// nothing (real_date_anchor.go), so "three of the four" must be
	// unrepresentable at the boundary rather than validated behind it — the
	// column-level shape is the one that lets a caller write a half-anchor by
	// forgetting an argument.
	SetRealDateAnchor(ctx context.Context, calendarID string, a *RealDateAnchor) error
}

// calendarRepo is the MariaDB implementation of CalendarRepository.
type calendarRepo struct {
	db *sql.DB
}

// NewCalendarRepository creates a new MariaDB-backed calendar repository.
func NewCalendarRepository(db *sql.DB) CalendarRepository {
	return &calendarRepo{db: db}
}

// calendarCols is the column list for calendar queries. mood_tint_* are the
// persisted live-mood columns added in migration 008
// (C-CAL-WORLDSTATE-SERVER-MODEL); appended last so the column order of the
// pre-008 prefix is unchanged. The anchor_* four are migration 018's
// (C-CALV4-ANCHOR) and follow the same rule — appended, never interleaved,
// because scanCalendar reads this list POSITIONALLY and inserting a column
// mid-list silently shifts every field after it into the wrong destination.
const calendarCols = `id, campaign_id, mode, name, description, epoch_name, current_year,
        current_month, current_day, hours_per_day, minutes_per_hour, seconds_per_minute,
        current_hour, current_minute, leap_year_every, leap_year_offset,
        sort_order, is_default, created_at, updated_at,
        mood_tint_color, mood_tint_intensity, visibility, visibility_rules,
        tracks_real_time, real_time_zone,
        anchor_year, anchor_month, anchor_day, anchor_real_date`

// scanCalendar reads a row into a Calendar struct.
func scanCalendar(scanner interface{ Scan(...any) error }) (*Calendar, error) {
	cal := &Calendar{}
	err := scanner.Scan(&cal.ID, &cal.CampaignID, &cal.Mode,
		&cal.Name, &cal.Description, &cal.EpochName,
		&cal.CurrentYear, &cal.CurrentMonth, &cal.CurrentDay,
		&cal.HoursPerDay, &cal.MinutesPerHour, &cal.SecondsPerMinute,
		&cal.CurrentHour, &cal.CurrentMinute,
		&cal.LeapYearEvery, &cal.LeapYearOffset,
		&cal.SortOrder, &cal.IsDefault,
		&cal.CreatedAt, &cal.UpdatedAt,
		&cal.MoodTintColor, &cal.MoodTintIntensity,
		&cal.Visibility, &cal.VisibilityRules,
		&cal.TracksRealTime, &cal.RealTimeZone,
		&cal.AnchorYear, &cal.AnchorMonth, &cal.AnchorDay, &cal.AnchorRealDate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return cal, err
}

// Create inserts a new calendar.
func (r *calendarRepo) Create(ctx context.Context, cal *Calendar) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendars (id, campaign_id, mode, name, description, epoch_name,
		        current_year, current_month, current_day,
		        hours_per_day, minutes_per_hour, seconds_per_minute,
		        current_hour, current_minute,
		        leap_year_every, leap_year_offset,
		        sort_order, is_default,
		        tracks_real_time, real_time_zone)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cal.ID, cal.CampaignID, cal.Mode, cal.Name, cal.Description, cal.EpochName,
		cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay,
		cal.HoursPerDay, cal.MinutesPerHour, cal.SecondsPerMinute,
		cal.CurrentHour, cal.CurrentMinute,
		cal.LeapYearEvery, cal.LeapYearOffset,
		cal.SortOrder, cal.IsDefault,
		cal.TracksRealTime, cal.RealTimeZone,
	)
	return err
}

// GetByCampaignID returns the first calendar for a campaign (backward compat).
// Prefers the default calendar; falls back to oldest if none is marked default.
func (r *calendarRepo) GetByCampaignID(ctx context.Context, campaignID string) (*Calendar, error) {
	return scanCalendar(r.db.QueryRowContext(ctx,
		`SELECT `+calendarCols+` FROM calendars WHERE campaign_id = ? ORDER BY is_default DESC, sort_order ASC LIMIT 1`, campaignID))
}

// GetDefaultByCampaignID returns the default calendar for a campaign.
func (r *calendarRepo) GetDefaultByCampaignID(ctx context.Context, campaignID string) (*Calendar, error) {
	return scanCalendar(r.db.QueryRowContext(ctx,
		`SELECT `+calendarCols+` FROM calendars WHERE campaign_id = ? AND is_default = 1`, campaignID))
}

// GetByID returns a calendar by its ID.
func (r *calendarRepo) GetByID(ctx context.Context, id string) (*Calendar, error) {
	return scanCalendar(r.db.QueryRowContext(ctx,
		`SELECT `+calendarCols+` FROM calendars WHERE id = ?`, id))
}

// ListByCampaignID returns all calendars for a campaign, ordered by sort_order.
func (r *calendarRepo) ListByCampaignID(ctx context.Context, campaignID string) ([]Calendar, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+calendarCols+` FROM calendars WHERE campaign_id = ? ORDER BY sort_order, name`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var calendars []Calendar
	for rows.Next() {
		cal, err := scanCalendar(rows)
		if err != nil {
			return nil, err
		}
		if cal != nil {
			calendars = append(calendars, *cal)
		}
	}
	return calendars, rows.Err()
}

// SetDefault marks one calendar as the default for its campaign, unsetting
// the default flag on all other calendars in the same campaign.
func (r *calendarRepo) SetDefault(ctx context.Context, campaignID, calendarID string) error {
	// Unset all defaults in campaign.
	if _, err := r.db.ExecContext(ctx,
		`UPDATE calendars SET is_default = 0 WHERE campaign_id = ?`, campaignID); err != nil {
		return err
	}
	// Set the chosen one.
	_, err := r.db.ExecContext(ctx,
		`UPDATE calendars SET is_default = 1 WHERE id = ? AND campaign_id = ?`, calendarID, campaignID)
	return err
}

// GetActiveCalendarID returns the user's last-selected calendar ID
// for a campaign, or "" if no row exists yet. Service layer falls
// back to the campaign's default calendar when this returns "".
//
// A NULL POINTER READS AS "" — THE SAME ANSWER AS NO ROW, DELIBERATELY.
// Migration 017 made calendar_id nullable and moved the FK to ON DELETE SET
// NULL, so that deleting a calendar clears the pointer WITHOUT destroying the
// three unrelated per-viewer preferences that share this row (sidebar_pinned,
// block_layers, bench_sections). A viewer whose chosen calendar was deleted is
// in exactly the situation of a viewer who has never chosen one, and
// resolveActiveCalendar's ladder — pointer, then campaign default, then first
// by sort order — already handles that case, which is what makes 017 a schema
// change rather than a semantic one.
//
// The sql.NullString is load-bearing: scanning a NULL into a plain string is a
// driver error, so without it the read would 500 for every viewer in a campaign
// that had ever deleted a calendar. That is the whole reason this reader changes
// alongside the migration and not after it.
func (r *calendarRepo) GetActiveCalendarID(ctx context.Context, userID, campaignID string) (string, error) {
	var calendarID sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT calendar_id FROM calendar_active WHERE user_id = ? AND campaign_id = ?`,
		userID, campaignID).Scan(&calendarID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return calendarID.String, nil // "" when NULL — see the doc comment
}

// GetSidebarPinned returns the user-per-campaign sidebar pin
// preference. Defaults to TRUE when no row exists (matches the
// viewport-default pin behavior; operators on narrow viewports
// dismiss via toggle which writes FALSE).
func (r *calendarRepo) GetSidebarPinned(ctx context.Context, userID, campaignID string) (bool, error) {
	var pinned bool
	err := r.db.QueryRowContext(ctx,
		`SELECT sidebar_pinned FROM calendar_active WHERE user_id = ? AND campaign_id = ?`,
		userID, campaignID).Scan(&pinned)
	if err == sql.ErrNoRows {
		// No active-cal row yet → default to pinned per migration 007 default.
		return true, nil
	}
	if err != nil {
		return true, err
	}
	return pinned, nil
}

// SetSidebarPinned writes the pin preference. Upserts via the same
// calendar_active row used for active-cal pointers.
//
// calendarID SEEDS THE ROW AND IS NEVER WRITTEN OVER AN EXISTING ONE. The
// three preference writers on this table all used to insert an empty
// calendar_id as a "fallback on first write". `calendar_active.calendar_id`
// carries `fk_calendar_active_cal` referencing `calendars(id)` (migration
// 006:17,22-23), and no calendar carries the empty id — so the INSERT tripped
// MariaDB errno 1452 and the whole write 500'd. Migration 017 made the column
// NULLable (ON DELETE SET NULL, so a deleted calendar clears the pointer rather
// than destroying this row and the three preferences on it); that does NOT
// rescue the old behaviour, because "" is not NULL and the foreign key still
// rejects it. InnoDB checks
// the foreign key on the attempted insert, BEFORE the duplicate-key path
// resolves, so ON DUPLICATE KEY UPDATE never rescued it: the preference could
// not be saved by a first-time viewer or by a returning one. That is why the
// Block's layer switches "did nothing" — the POST fired, the server 500'd, no
// HX-Refresh came back, and the switch stayed exactly where it was.
//
// The service resolves a REAL calendar id (active pointer, else campaign
// default, else first by sort order — the same ladder GetActiveCalendar walks)
// and passes it here. The ON DUPLICATE KEY UPDATE clause deliberately names ONLY
// the preference column: a viewer who has chosen an active calendar must not
// have that choice silently rewritten to the default because they collapsed a
// section.
func (r *calendarRepo) SetSidebarPinned(ctx context.Context, userID, campaignID, calendarID string, pinned bool) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_active (user_id, campaign_id, calendar_id, sidebar_pinned)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE sidebar_pinned = VALUES(sidebar_pinned)`,
		userID, campaignID, calendarID, pinned)
	return err
}

// GetBlockLayers returns the viewer's stored calendar-v4 Block layer set for a
// campaign, or nil when they have never chosen one (C-CALV4-LAYERS-P9 [LYR-3]).
//
// NULL AND EMPTY ARE DIFFERENT ANSWERS and the sql.NullString is what keeps
// them apart:
//
//	no row / NULL  → (nil, nil)         the host's seed renders, as before
//	''             → ([]string{}, nil)  the viewer chose a bare month
//	'moons,eras'   → ([...], nil)       the viewer's own set
//
// No validation happens here. A repository owns SQL; the eight-key registry
// lives in the widget package and the filtering happens where it can log — see
// calendarService.GetBlockLayers.
func (r *calendarRepo) GetBlockLayers(ctx context.Context, userID, campaignID string) ([]string, error) {
	var stored sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT block_layers FROM calendar_active WHERE user_id = ? AND campaign_id = ?`,
		userID, campaignID).Scan(&stored)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !stored.Valid {
		return nil, nil
	}
	if stored.String == "" {
		// Deliberately non-nil and empty: "every layer off", not "unchosen".
		return []string{}, nil
	}
	return strings.Split(stored.String, ","), nil
}

// SetBlockLayers persists the viewer's layer set. Upserts through the same
// calendar_active row as the pin preference, and takes calendarID for the same
// reason — see SetSidebarPinned's comment for the FK that made the old
// empty-string seed a guaranteed 1452 on every call.
//
// A nil keys slice writes NULL — "forget my choice", the reset the switchboard
// does not expose yet but the store must be able to express, or a viewer could
// never get back to the host's seed. An empty non-nil slice writes '', which is
// the bare month.
func (r *calendarRepo) SetBlockLayers(ctx context.Context, userID, campaignID, calendarID string, keys []string) error {
	var val any
	if keys != nil {
		val = strings.Join(keys, ",")
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_active (user_id, campaign_id, calendar_id, block_layers)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE block_layers = VALUES(block_layers)`,
		userID, campaignID, calendarID, val)
	return err
}

// GetBenchSections returns the viewer's stored set of CLOSED Bench sections for
// a campaign, or nil when they have never chosen one (C-CALV4-BENCH-R2
// [BR2-5], migration 016).
//
// NULL AND EMPTY ARE DIFFERENT ANSWERS, exactly as they are for block_layers,
// and the sql.NullString is again what keeps them apart:
//
//	no row / NULL  → (nil, nil)         the ruled default: all four CLOSED
//	''             → ([]string{}, nil)  the viewer closed nothing: all four OPEN
//	'rsvp,rows'    → ([...], nil)       those two closed, the rest open
//
// The list is the CLOSED set rather than the open one because the ruled default
// is closed; storing the open set would make '' byte-identical to the default
// and there would be no way to record "I opened everything". See the migration
// header, which argues it at length.
//
// No validation happens here. A repository owns SQL; the four-key registry
// lives in bench_sections.go and the filtering happens where it can log — see
// calendarService.GetBenchSections.
func (r *calendarRepo) GetBenchSections(ctx context.Context, userID, campaignID string) ([]string, error) {
	var stored sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT bench_sections FROM calendar_active WHERE user_id = ? AND campaign_id = ?`,
		userID, campaignID).Scan(&stored)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !stored.Valid {
		return nil, nil
	}
	if stored.String == "" {
		// Deliberately non-nil and empty: "nothing is closed", not "unchosen".
		return []string{}, nil
	}
	return strings.Split(stored.String, ","), nil
}

// SetBenchSections persists the viewer's closed set. Upserts through the same
// calendar_active row as the pin and layer preferences, and takes calendarID for
// the same reason — see SetSidebarPinned's comment for the FK that made the old
// empty-string seed a guaranteed 1452 on every call.
//
// A nil keys slice writes NULL — "forget my choice", which returns the viewer
// to the ruled default. An empty non-nil slice writes '', which is all four
// open. The two are not interchangeable and this method must never collapse
// them.
func (r *calendarRepo) SetBenchSections(ctx context.Context, userID, campaignID, calendarID string, keys []string) error {
	var val any
	if keys != nil {
		val = strings.Join(keys, ",")
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_active (user_id, campaign_id, calendar_id, bench_sections)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE bench_sections = VALUES(bench_sections)`,
		userID, campaignID, calendarID, val)
	return err
}

// SetActiveCalendar writes the user-per-campaign active-calendar
// pointer. Upserts so the first switch creates the row and subsequent
// switches overwrite. Caller must validate calendarID belongs to
// campaignID before calling.
func (r *calendarRepo) SetActiveCalendar(ctx context.Context, userID, campaignID, calendarID string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_active (user_id, campaign_id, calendar_id)
		 VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE calendar_id = VALUES(calendar_id)`,
		userID, campaignID, calendarID)
	return err
}

// Update modifies an existing calendar's settings and current date/time.
func (r *calendarRepo) Update(ctx context.Context, cal *Calendar) error {
	// `mode` is written (C-REAL-CALENDAR-P3): the service's UpdateCalendar resolves
	// the mode (`cal.Mode = input.Mode` when non-empty) but the prior UPDATE omitted
	// the column, so a mode change routed through it (only the syncapi settings PUT
	// binds `mode`) was silently dropped on reload. Persisting it here also makes the
	// P3 0b real-time invariant load-bearing: a mode-walk on an RT calendar now would
	// persist, so validateRealTimeInvariant rejecting it is what actually prevents the
	// stranded flag. Every other caller forwards the loaded cal.Mode (or leaves it
	// unchanged), so this is a no-op write-back for them.
	_, err := r.db.ExecContext(ctx,
		`UPDATE calendars SET name = ?, description = ?, epoch_name = ?, mode = ?,
		        current_year = ?, current_month = ?, current_day = ?,
		        hours_per_day = ?, minutes_per_hour = ?, seconds_per_minute = ?,
		        current_hour = ?, current_minute = ?,
		        leap_year_every = ?, leap_year_offset = ?,
		        tracks_real_time = ?, real_time_zone = ?
		 WHERE id = ?`,
		cal.Name, cal.Description, cal.EpochName, cal.Mode,
		cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay,
		cal.HoursPerDay, cal.MinutesPerHour, cal.SecondsPerMinute,
		cal.CurrentHour, cal.CurrentMinute,
		cal.LeapYearEvery, cal.LeapYearOffset,
		cal.TracksRealTime, cal.RealTimeZone, cal.ID,
	)
	return err
}

// Delete removes a calendar and all child records (cascaded by FK).
func (r *calendarRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM calendars WHERE id = ?`, id)
	return err
}

// SetMonths replaces all months for a calendar (delete + bulk insert).
func (r *calendarRepo) SetMonths(ctx context.Context, calendarID string, months []MonthInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_months WHERE calendar_id = ?`, calendarID); err != nil {
		return err
	}
	for _, m := range months {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_months (calendar_id, name, days, sort_order, is_intercalary, leap_year_days)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			calendarID, m.Name, m.Days, m.SortOrder, m.IsIntercalary, m.LeapYearDays,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetMonths returns all months for a calendar ordered by sort_order.
func (r *calendarRepo) GetMonths(ctx context.Context, calendarID string) ([]Month, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, calendar_id, name, days, sort_order, is_intercalary, leap_year_days
		 FROM calendar_months WHERE calendar_id = ? ORDER BY sort_order`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var months []Month
	for rows.Next() {
		var m Month
		if err := rows.Scan(&m.ID, &m.CalendarID, &m.Name, &m.Days, &m.SortOrder, &m.IsIntercalary, &m.LeapYearDays); err != nil {
			return nil, err
		}
		months = append(months, m)
	}
	return months, rows.Err()
}

// SetWeekdays replaces all weekdays for a calendar.
func (r *calendarRepo) SetWeekdays(ctx context.Context, calendarID string, weekdays []WeekdayInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_weekdays WHERE calendar_id = ?`, calendarID); err != nil {
		return err
	}
	for _, w := range weekdays {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_weekdays (calendar_id, name, sort_order, is_rest_day)
			 VALUES (?, ?, ?, ?)`,
			calendarID, w.Name, w.SortOrder, w.IsRestDay,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetWeekdays returns all weekdays for a calendar ordered by sort_order.
func (r *calendarRepo) GetWeekdays(ctx context.Context, calendarID string) ([]Weekday, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, calendar_id, name, sort_order, is_rest_day
		 FROM calendar_weekdays WHERE calendar_id = ? ORDER BY sort_order`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var weekdays []Weekday
	for rows.Next() {
		var w Weekday
		if err := rows.Scan(&w.ID, &w.CalendarID, &w.Name, &w.SortOrder, &w.IsRestDay); err != nil {
			return nil, err
		}
		weekdays = append(weekdays, w)
	}
	return weekdays, rows.Err()
}

// SetMoons replaces all moons for a calendar.
func (r *calendarRepo) SetMoons(ctx context.Context, calendarID string, moons []MoonInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_moons WHERE calendar_id = ?`, calendarID); err != nil {
		return err
	}
	for _, m := range moons {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_moons (calendar_id, name, cycle_days, phase_offset, color)
			 VALUES (?, ?, ?, ?, ?)`,
			calendarID, m.Name, m.CycleDays, m.PhaseOffset, m.Color,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetMoons returns all moons for a calendar, including the moon-library
// render params added in migration 008 (base_design/tint/phase_source/size/
// orbit_speed). Existing moons read the column defaults.
func (r *calendarRepo) GetMoons(ctx context.Context, calendarID string) ([]Moon, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, calendar_id, name, cycle_days, phase_offset, color,
		        base_design, tint, phase_source, size, orbit_speed
		 FROM calendar_moons WHERE calendar_id = ?`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var moons []Moon
	for rows.Next() {
		var m Moon
		if err := rows.Scan(&m.ID, &m.CalendarID, &m.Name, &m.CycleDays, &m.PhaseOffset, &m.Color,
			&m.BaseDesign, &m.Tint, &m.PhaseSource, &m.Size, &m.OrbitSpeed); err != nil {
			return nil, err
		}
		moons = append(moons, m)
	}
	return moons, rows.Err()
}

// SetSeasons replaces all seasons for a calendar.
func (r *calendarRepo) SetSeasons(ctx context.Context, calendarID string, seasons []Season) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_seasons WHERE calendar_id = ?`, calendarID); err != nil {
		return err
	}
	for _, s := range seasons {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_seasons (calendar_id, name, start_month, start_day, end_month, end_day, description, color, weather_effect)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			calendarID, s.Name, s.StartMonth, s.StartDay, s.EndMonth, s.EndDay, s.Description, s.Color, s.WeatherEffect,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetSeasons returns all seasons for a calendar.
func (r *calendarRepo) GetSeasons(ctx context.Context, calendarID string) ([]Season, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, calendar_id, name, start_month, start_day, end_month, end_day, description, color, weather_effect
		 FROM calendar_seasons WHERE calendar_id = ?`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var seasons []Season
	for rows.Next() {
		var s Season
		if err := rows.Scan(&s.ID, &s.CalendarID, &s.Name, &s.StartMonth, &s.StartDay, &s.EndMonth, &s.EndDay, &s.Description, &s.Color, &s.WeatherEffect); err != nil {
			return nil, err
		}
		seasons = append(seasons, s)
	}
	return seasons, rows.Err()
}

// SetEras replaces all eras for a calendar.
func (r *calendarRepo) SetEras(ctx context.Context, calendarID string, eras []EraInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_eras WHERE calendar_id = ?`, calendarID); err != nil {
		return err
	}
	for _, e := range eras {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_eras (calendar_id, name, start_year, end_year, description, color, sort_order)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			calendarID, e.Name, e.StartYear, e.EndYear, e.Description, e.Color, e.SortOrder,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CreateEra inserts a single era and returns the persisted row (with ID).
// Auto-assigns sort_order to len(existing) so the new era sorts after
// existing ones. Per-era CRUD complements SetEras (replace-all); both
// shapes coexist.
func (r *calendarRepo) CreateEra(ctx context.Context, calendarID string, input EraInput) (*Era, error) {
	if input.SortOrder == 0 {
		var maxSort sql.NullInt64
		if err := r.db.QueryRowContext(ctx,
			`SELECT MAX(sort_order) FROM calendar_eras WHERE calendar_id = ?`,
			calendarID).Scan(&maxSort); err != nil {
			return nil, err
		}
		if maxSort.Valid {
			input.SortOrder = int(maxSort.Int64) + 1
		}
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_eras (calendar_id, name, start_year, end_year, description, color, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		calendarID, input.Name, input.StartYear, input.EndYear, input.Description, input.Color, input.SortOrder,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.GetEraByID(ctx, int(id))
}

// UpdateEra updates a single era by ID. All fields from EraInput are
// applied (no nil-preserve semantics — caller provides the full intended
// shape). Returns sql.ErrNoRows-equivalent (apperror.NotFound) if the
// era doesn't exist; the service layer maps.
func (r *calendarRepo) UpdateEra(ctx context.Context, eraID int, input EraInput) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE calendar_eras
		   SET name = ?, start_year = ?, end_year = ?, description = ?, color = ?, sort_order = ?
		 WHERE id = ?`,
		input.Name, input.StartYear, input.EndYear, input.Description, input.Color, input.SortOrder, eraID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteEra removes a single era + re-sorts remaining eras within the
// same calendar so sort_order stays contiguous (0..N-1). Done in one
// transaction so a partial reorder can't strand the calendar's eras
// in an inconsistent sort order.
func (r *calendarRepo) DeleteEra(ctx context.Context, eraID int) (string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var calendarID string
	if err := tx.QueryRowContext(ctx,
		`SELECT calendar_id FROM calendar_eras WHERE id = ?`, eraID,
	).Scan(&calendarID); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM calendar_eras WHERE id = ?`, eraID); err != nil {
		return "", err
	}

	// Re-sort remaining eras to be contiguous. SET sort_order = (
	// row_number() over (...) - 1) — emulated in MariaDB via a session
	// variable. Eras within a calendar are typically a small set
	// (handfuls), so the linear-scan reorder is cheap.
	if _, err := tx.ExecContext(ctx, `SET @i := -1`); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE calendar_eras SET sort_order = (@i := @i + 1)
		 WHERE calendar_id = ? ORDER BY sort_order, start_year`, calendarID); err != nil {
		return "", err
	}

	return calendarID, tx.Commit()
}

// GetEraByID returns one era by ID, or nil if not found.
func (r *calendarRepo) GetEraByID(ctx context.Context, eraID int) (*Era, error) {
	var e Era
	err := r.db.QueryRowContext(ctx,
		`SELECT id, calendar_id, name, start_year, end_year, description, color, sort_order
		 FROM calendar_eras WHERE id = ?`, eraID,
	).Scan(&e.ID, &e.CalendarID, &e.Name, &e.StartYear, &e.EndYear, &e.Description, &e.Color, &e.SortOrder)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// ApplyImport runs the entire calendar-import write workflow within one
// transaction. The caller (service layer) is responsible for validating
// inputs BEFORE this method is called — validation here is impossible
// because the tx is already open; rolling back due to validation defeats
// the point of upfront validation.
//
// Pre-Wave-0-PR-2 service.ApplyImport called s.SetMonths / SetWeekdays /
// SetMoons / SetSeasons / SetEras sequentially; each of those opened its
// own transaction; a failure midway through left the calendar with
// (some) replaced months + (untouched) old weekdays + etc. — partial
// state operators couldn't recover from cleanly. This method takes a
// single tx so on any error the whole import rolls back and the
// calendar stays at its pre-import state.
//
// SQL is duplicated from the corresponding Set* methods rather than
// extracted into shared helpers, keeping the existing Set* call sites
// untouched (zero risk to their tests + WS publish semantics). When
// Wave 3 AI Workspace lands an event-import path, the same pattern can
// extend with `applyEventsTx` etc. without touching Set*.
func (r *calendarRepo) ApplyImport(ctx context.Context, cal *Calendar, result *ImportResult) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin import tx: %w", err)
	}
	defer tx.Rollback()

	// 1. Calendar fields (UPDATE). Includes mode + real-time columns so a
	// Chronicle-native import that enables real-time tracking (C-REAL-CALENDAR-P3
	// 0c) actually persists reallife mode + the anchor zone; for every other
	// import these write back the target's unchanged mode with tracks_real_time
	// cleared (a no-op — the service already validated the result).
	if _, err := tx.ExecContext(ctx,
		`UPDATE calendars SET name = ?, description = ?, epoch_name = ?, mode = ?,
		        current_year = ?, current_month = ?, current_day = ?,
		        hours_per_day = ?, minutes_per_hour = ?, seconds_per_minute = ?,
		        current_hour = ?, current_minute = ?,
		        leap_year_every = ?, leap_year_offset = ?,
		        tracks_real_time = ?, real_time_zone = ?
		 WHERE id = ?`,
		cal.Name, cal.Description, cal.EpochName, cal.Mode,
		cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay,
		cal.HoursPerDay, cal.MinutesPerHour, cal.SecondsPerMinute,
		cal.CurrentHour, cal.CurrentMinute,
		cal.LeapYearEvery, cal.LeapYearOffset,
		cal.TracksRealTime, cal.RealTimeZone, cal.ID,
	); err != nil {
		return fmt.Errorf("update calendar: %w", err)
	}

	// 2. Months (optional — V1 ApplyImport skipped if empty; we do too).
	if len(result.Months) > 0 {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM calendar_months WHERE calendar_id = ?`, cal.ID); err != nil {
			return fmt.Errorf("delete months: %w", err)
		}
		for _, m := range result.Months {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO calendar_months (calendar_id, name, days, sort_order, is_intercalary, leap_year_days)
				 VALUES (?, ?, ?, ?, ?, ?)`,
				cal.ID, m.Name, m.Days, m.SortOrder, m.IsIntercalary, m.LeapYearDays,
			); err != nil {
				return fmt.Errorf("insert month %q: %w", m.Name, err)
			}
		}
	}

	// 3. Weekdays (optional).
	if len(result.Weekdays) > 0 {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM calendar_weekdays WHERE calendar_id = ?`, cal.ID); err != nil {
			return fmt.Errorf("delete weekdays: %w", err)
		}
		for _, w := range result.Weekdays {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO calendar_weekdays (calendar_id, name, sort_order, is_rest_day)
				 VALUES (?, ?, ?, ?)`,
				cal.ID, w.Name, w.SortOrder, w.IsRestDay,
			); err != nil {
				return fmt.Errorf("insert weekday %q: %w", w.Name, err)
			}
		}
	}

	// 4. Moons (always replace; result.Moons may be empty to clear).
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM calendar_moons WHERE calendar_id = ?`, cal.ID); err != nil {
		return fmt.Errorf("delete moons: %w", err)
	}
	for _, m := range result.Moons {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_moons (calendar_id, name, cycle_days, phase_offset, color)
			 VALUES (?, ?, ?, ?, ?)`,
			cal.ID, m.Name, m.CycleDays, m.PhaseOffset, m.Color,
		); err != nil {
			return fmt.Errorf("insert moon %q: %w", m.Name, err)
		}
	}

	// 5. Seasons.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM calendar_seasons WHERE calendar_id = ?`, cal.ID); err != nil {
		return fmt.Errorf("delete seasons: %w", err)
	}
	for _, s := range result.Seasons {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_seasons (calendar_id, name, start_month, start_day, end_month, end_day, description, color, weather_effect)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			cal.ID, s.Name, s.StartMonth, s.StartDay, s.EndMonth, s.EndDay, s.Description, s.Color, s.WeatherEffect,
		); err != nil {
			return fmt.Errorf("insert season %q: %w", s.Name, err)
		}
	}

	// 6. Eras.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM calendar_eras WHERE calendar_id = ?`, cal.ID); err != nil {
		return fmt.Errorf("delete eras: %w", err)
	}
	for _, e := range result.Eras {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_eras (calendar_id, name, start_year, end_year, description, color, sort_order)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			cal.ID, e.Name, e.StartYear, e.EndYear, e.Description, e.Color, e.SortOrder,
		); err != nil {
			return fmt.Errorf("insert era %q: %w", e.Name, err)
		}
	}

	return tx.Commit()
}

// GetEras returns all eras for a calendar ordered by sort_order.
func (r *calendarRepo) GetEras(ctx context.Context, calendarID string) ([]Era, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, calendar_id, name, start_year, end_year, description, color, sort_order
		 FROM calendar_eras WHERE calendar_id = ? ORDER BY sort_order, start_year`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var eras []Era
	for rows.Next() {
		var e Era
		if err := rows.Scan(&e.ID, &e.CalendarID, &e.Name, &e.StartYear, &e.EndYear, &e.Description, &e.Color, &e.SortOrder); err != nil {
			return nil, err
		}
		eras = append(eras, e)
	}
	return eras, rows.Err()
}

// SetEventCategories replaces all event categories for a calendar (delete + bulk insert).
func (r *calendarRepo) SetEventCategories(ctx context.Context, calendarID string, cats []EventCategoryInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_event_categories WHERE calendar_id = ?`, calendarID); err != nil {
		return err
	}
	for _, c := range cats {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_event_categories (calendar_id, slug, name, icon, color, sort_order)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			calendarID, c.Slug, c.Name, c.Icon, c.Color, c.SortOrder,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetEventCategories returns all event categories for a calendar ordered by sort_order.
func (r *calendarRepo) GetEventCategories(ctx context.Context, calendarID string) ([]EventCategory, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, calendar_id, slug, name, icon, color, sort_order
		 FROM calendar_event_categories WHERE calendar_id = ? ORDER BY sort_order`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []EventCategory
	for rows.Next() {
		var c EventCategory
		if err := rows.Scan(&c.ID, &c.CalendarID, &c.Slug, &c.Name, &c.Icon, &c.Color, &c.SortOrder); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

// eventCols is the column list for event queries (with entity join fields).
// Wave 1.6 added e.tier between e.category and e.color to close the
// PR #358 schema-only gap surfaced by PR #368 stop-and-flag #2.
// C-CAL-RSVP-P1 added e.collect_rsvps after e.all_day (migration 013).
//
// INVARIANT: this list and the Scan destination lists in GetEvent + scanEvents
// must stay the same length and order. TestEventColsMatchScanDestinations pins
// it — a mismatch is invisible to every existing test (nothing in the repo runs
// real SQL) but fails EVERY event query at runtime with
// "sql: expected N destination arguments in Scan".
const eventCols = `e.id, e.calendar_id, e.entity_id, e.name, e.description, e.description_html,
       e.year, e.month, e.day, e.start_hour, e.start_minute,
       e.end_year, e.end_month, e.end_day, e.end_hour, e.end_minute,
       e.is_recurring, e.recurrence_type,
       e.recurrence_interval, e.recurrence_end_year, e.recurrence_end_month,
       e.recurrence_end_day, e.recurrence_max_occurrences, e.recurrence_day_of_week,
       e.visibility, e.visibility_rules, e.category, e.tier,
       e.color, e.icon, e.all_day, e.collect_rsvps,
       e.created_by, e.created_at, e.updated_at,
       COALESCE(ent.name, ''), COALESCE(et.icon, ''), COALESCE(et.color, '')`

// eventJoins is the LEFT JOIN clause for entity display data.
const eventJoins = `LEFT JOIN entities ent ON ent.id = e.entity_id
     LEFT JOIN entity_types et ON et.id = ent.entity_type_id`

// recurringCandidateClause is the SQL that widens a date-bounded event query to
// include every RECURRING row whose base date sits outside the window, because
// Event.OccursOn — not the SQL — decides where an instance lands.
//
// IT IS DERIVED FROM THE Recurrence* CONSTANTS, NOT RE-TYPED, and that is the
// whole reason it exists. The literal `IN ('weekly','biweekly','monthly',
// 'custom')` was written out by hand in THREE places (here twice, and in
// block_repository.go's upcoming-events query), which made the accepted set a
// four-copy fact. When C-CALV4-GAMEREADY §6 added `yearly` to the engine, the
// predicate expanded it correctly and the ROW WAS NEVER LOADED: every yearly
// festival was invisible in every month but its own, and the only reason that
// was caught is that §6's guard runs against a REAL DATABASE. Against a fake
// repository the feature was perfectly green and completely dead.
//
// Adding a recurrence type is therefore a one-line change to the constant block
// and nothing else. The values are compile-time constants under this package's
// control and never user input, so this carries no injection surface; the
// apostrophe doubling is belt-and-braces against a future constant that
// contains one, which would otherwise break the query silently at runtime.
var recurringCandidateClause = buildRecurringCandidateClause()

// spanningCandidateClause widens a MONTH-bounded event query to include every
// multi-day row whose stored [start, end] window OVERLAPS that month, even
// though its stored month is a different one — C-CALV4-GAMEREADY §3, [GR-5].
//
// IT IS THE SECOND HALF OF THE SAME FIX, AND ONLY A REAL DATABASE FOUND IT.
// blockEventSpansDate makes the projection mark every day of a span, which is
// enough for a festival that begins and ends inside one month. A festival that
// runs from day 28 of one month to day 3 of the next has its stored `month` in
// the FIRST month only, so `WHERE e.year = ? AND e.month = ?` never returned it
// while the second month was being rendered — and the day card went on saying
// "No events on this day" for days 1..3 of a festival in progress, which is
// precisely the lie §3 exists to remove. The projection-level guard cannot see
// this; the MariaDB guard fails on it.
//
// THE COMPARISON IS A COMPOSITE, AND THE RADIX IS NOT 100. The house idiom
// elsewhere is `month * 100 + day`, which assumes no month exceeds 99 days —
// safe for Gregorian and for every shipped fantasy preset, but this is a
// user-authored month list and a 120-day month is expressible. The radix here
// is 10000, so month lengths up to 9999 days compare correctly, and the month
// bound uses day 0 / day 9999 rather than the month's real length so the clause
// needs no per-calendar geometry.
//
// The four placeholders are (year, month) twice: the row's END must be at or
// after the first of the asked-for month, and its START at or before the last.
const spanningCandidateClause = `(
		    e.end_year IS NOT NULL AND e.end_month IS NOT NULL AND e.end_day IS NOT NULL
		    AND (e.end_year * 100000000 + e.end_month * 10000 + e.end_day)
		        >= (? * 100000000 + ? * 10000 + 0)
		    AND (e.year * 100000000 + e.month * 10000 + e.day)
		        <= (? * 100000000 + ? * 10000 + 9999)
		  )`

func buildRecurringCandidateClause() string {
	quoted := make([]string, 0, len(RecurrenceTypes))
	for _, t := range RecurrenceTypes {
		quoted = append(quoted, "'"+strings.ReplaceAll(t, "'", "''")+"'")
	}
	return "(e.is_recurring = 1 AND e.recurrence_type IN (" + strings.Join(quoted, ",") + "))"
}

// CreateEvent inserts a new event.
func (r *calendarRepo) CreateEvent(ctx context.Context, evt *Event) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_events (id, calendar_id, entity_id, name, description, description_html,
		        year, month, day, start_hour, start_minute,
		        end_year, end_month, end_day, end_hour, end_minute,
		        is_recurring, recurrence_type,
		        recurrence_interval, recurrence_end_year, recurrence_end_month,
		        recurrence_end_day, recurrence_max_occurrences, recurrence_day_of_week,
		        visibility, visibility_rules, category, tier,
		        color, icon, all_day, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		evt.ID, evt.CalendarID, evt.EntityID, evt.Name, evt.Description, evt.DescriptionHTML,
		evt.Year, evt.Month, evt.Day, evt.StartHour, evt.StartMinute,
		evt.EndYear, evt.EndMonth, evt.EndDay, evt.EndHour, evt.EndMinute,
		evt.IsRecurring, evt.RecurrenceType,
		evt.RecurrenceInterval, evt.RecurrenceEndYear, evt.RecurrenceEndMonth,
		evt.RecurrenceEndDay, evt.RecurrenceMaxOccurrences, evt.RecurrenceDayOfWeek,
		evt.Visibility, evt.VisibilityRules, evt.Category, evt.Tier,
		evt.Color, evt.Icon, evt.AllDay, evt.CreatedBy,
	)
	return err
}

// GetEvent returns a single event by ID.
func (r *calendarRepo) GetEvent(ctx context.Context, id string) (*Event, error) {
	evt := &Event{}
	err := r.db.QueryRowContext(ctx,
		`SELECT `+eventCols+`
		 FROM calendar_events e `+eventJoins+`
		 WHERE e.id = ?`, id,
	).Scan(&evt.ID, &evt.CalendarID, &evt.EntityID, &evt.Name, &evt.Description, &evt.DescriptionHTML,
		&evt.Year, &evt.Month, &evt.Day, &evt.StartHour, &evt.StartMinute,
		&evt.EndYear, &evt.EndMonth, &evt.EndDay, &evt.EndHour, &evt.EndMinute,
		&evt.IsRecurring, &evt.RecurrenceType,
		&evt.RecurrenceInterval, &evt.RecurrenceEndYear, &evt.RecurrenceEndMonth,
		&evt.RecurrenceEndDay, &evt.RecurrenceMaxOccurrences, &evt.RecurrenceDayOfWeek,
		&evt.Visibility, &evt.VisibilityRules, &evt.Category, &evt.Tier,
		&evt.Color, &evt.Icon, &evt.AllDay, &evt.CollectRSVPs,
		&evt.CreatedBy, &evt.CreatedAt, &evt.UpdatedAt,
		&evt.EntityName, &evt.EntityIcon, &evt.EntityColor)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return evt, err
}

// UpdateEvent modifies an existing event.
func (r *calendarRepo) UpdateEvent(ctx context.Context, evt *Event) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE calendar_events
		 SET name = ?, description = ?, description_html = ?, entity_id = ?,
		     year = ?, month = ?, day = ?,
		     start_hour = ?, start_minute = ?,
		     end_year = ?, end_month = ?, end_day = ?, end_hour = ?, end_minute = ?,
		     is_recurring = ?, recurrence_type = ?,
		     recurrence_interval = ?, recurrence_end_year = ?, recurrence_end_month = ?,
		     recurrence_end_day = ?, recurrence_max_occurrences = ?, recurrence_day_of_week = ?,
		     visibility = ?, visibility_rules = ?, category = ?, tier = ?,
		     color = ?, icon = ?, all_day = ?
		 WHERE id = ?`,
		evt.Name, evt.Description, evt.DescriptionHTML, evt.EntityID,
		evt.Year, evt.Month, evt.Day,
		evt.StartHour, evt.StartMinute,
		evt.EndYear, evt.EndMonth, evt.EndDay, evt.EndHour, evt.EndMinute,
		evt.IsRecurring, evt.RecurrenceType,
		evt.RecurrenceInterval, evt.RecurrenceEndYear, evt.RecurrenceEndMonth,
		evt.RecurrenceEndDay, evt.RecurrenceMaxOccurrences, evt.RecurrenceDayOfWeek,
		evt.Visibility, evt.VisibilityRules, evt.Category, evt.Tier,
		evt.Color, evt.Icon, evt.AllDay, evt.ID,
	)
	return err
}

// DeleteEvent removes an event.
func (r *calendarRepo) DeleteEvent(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM calendar_events WHERE id = ?`, id)
	return err
}

// ListEventsForMonth returns all events for a specific month, filtered by role.
// Recurring events that match the month (any year) are included.
func (r *calendarRepo) ListEventsForMonth(ctx context.Context, calendarID string, year, month int, role int) ([]Event, error) {
	// Owners see all events including dm_only; others see only 'everyone'.
	visFilter := "AND e.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}

	// C-CAL-EDITOR-EXPANSION PR2: fetch this month's events PLUS every
	// recurring candidate (see recurringCandidateClause, which is DERIVED from
	// the Recurrence* constants — this used to be a hand-typed list and a
	// yearly festival was invisible because of it) for the calendar — the
	// recurring rows may have a base date in another month/year but project
	// into this month. The precise placement is decided in Go by
	// Event.OccursOn (the single expansion predicate), so the SQL just widens
	// the candidate set. The visibility filter still applies to every row, so a
	// dm_only recurring event is withheld entirely from non-DM viewers (its
	// instances never reach the projection).
	query := fmt.Sprintf(`
		SELECT `+eventCols+`
		FROM calendar_events e `+eventJoins+`
		WHERE e.calendar_id = ?
		  AND (
		    (e.year = ? AND e.month = ?)
		    OR `+recurringCandidateClause+`
		    OR `+spanningCandidateClause+`
		  )
		  %s
		ORDER BY e.day, COALESCE(e.start_hour, 99), COALESCE(e.start_minute, 99), e.name`, visFilter)

	rows, err := r.db.QueryContext(ctx, query, calendarID, year, month, year, month, year, month)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEvents(rows)
}

// ListEventsForYear returns all events for a specific year, filtered by role.
func (r *calendarRepo) ListEventsForYear(ctx context.Context, calendarID string, year int, role int) ([]Event, error) {
	visFilter := "AND e.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}

	// Recurring events surface only in their stored year. V3 will ship
	// unified recurring expansion; see Q-V2-6 resolution.
	query := fmt.Sprintf(`
		SELECT `+eventCols+`
		FROM calendar_events e `+eventJoins+`
		WHERE e.calendar_id = ?
		  AND e.year = ?
		  %s
		ORDER BY e.month, e.day, COALESCE(e.start_hour, 99), COALESCE(e.start_minute, 99), e.name`, visFilter)

	rows, err := r.db.QueryContext(ctx, query, calendarID, year)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEvents(rows)
}

// ListAllEvents returns every event for a calendar with no role
// filter and no date constraint. Public Foundry API consumer:
// see the CalendarRepository interface comment for why.
func (r *calendarRepo) ListAllEvents(ctx context.Context, calendarID string) ([]Event, error) {
	query := `
		SELECT ` + eventCols + `
		FROM calendar_events e ` + eventJoins + `
		WHERE e.calendar_id = ?
		ORDER BY e.year, e.month, e.day, COALESCE(e.start_hour, 99), COALESCE(e.start_minute, 99), e.name`

	rows, err := r.db.QueryContext(ctx, query, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEvents(rows)
}

// StrandedEventCounts returns, per calendar in a campaign, how many events
// point at a month POSITION that calendar no longer has
// (C-CALV4-GAMEREADY §9 [GR-18]).
//
// ONE QUERY FOR THE WHOLE CAMPAIGN, because its consumer is the Bench — a hot
// page that already renders every listed calendar, and a per-calendar read
// there would be N round trips on the surface a GM opens most.
//
// It reports the STRANDED state only, never the SHIFTED delta. Shift is
// meaningful only against the edit that caused it; a standing surface has no
// "before" to compare with, and inventing one would be the kind of number that
// looks authoritative and is not. The shift count is reported at the moment of
// the save, by the service, and nowhere else.
//
// Calendars with no stranded events are ABSENT from the map rather than
// present with a zero — the caller renders a row per entry, and a zero row is
// a row that says nothing.
func (r *calendarRepo) StrandedEventCounts(ctx context.Context, campaignID string) (map[string]int, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT e.calendar_id, COUNT(*)
		FROM calendar_events e
		JOIN calendars c ON c.id = e.calendar_id
		LEFT JOIN (
			SELECT calendar_id, COUNT(*) AS n
			FROM calendar_months
			GROUP BY calendar_id
		) m ON m.calendar_id = e.calendar_id
		WHERE c.campaign_id = ?
		  AND (e.month < 1 OR e.month > COALESCE(m.n, 0))
		GROUP BY e.calendar_id`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// ListEventsForDateRange returns events within a date range (same year).
// Handles single-month or cross-month ranges within the same year.
//
// Recurring events appear once at their stored (year, month, day); V3
// will ship unified recurring expansion. See Q-V2-6 resolution at
// decisions/2026-05-28-cal-timeline-v2-design.md.
func (r *calendarRepo) ListEventsForDateRange(ctx context.Context, calendarID string, year, startMonth, startDay, endMonth, endDay int, role int) ([]Event, error) {
	visFilter := "AND e.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}

	// Use composite date value (month*100 + day) for range comparison.
	// C-CAL-EDITOR-EXPANSION PR2: also pull recurring candidates (any base
	// date) so they can project into the week/day range; Event.OccursOn does
	// the precise placement in Go (single expansion predicate). Visibility
	// still filters every row.
	query := fmt.Sprintf(`
		SELECT `+eventCols+`
		FROM calendar_events e `+eventJoins+`
		WHERE e.calendar_id = ?
		  AND (
		    (e.year = ? AND (e.month * 100 + e.day) >= ? AND (e.month * 100 + e.day) <= ?)
		    OR `+recurringCandidateClause+`
		  )
		  %s
		ORDER BY e.month, e.day, COALESCE(e.start_hour, 99), COALESCE(e.start_minute, 99), e.name`, visFilter)

	startVal := startMonth*100 + startDay
	endVal := endMonth*100 + endDay

	rows, err := r.db.QueryContext(ctx, query, calendarID, year, startVal, endVal)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return scanEvents(rows)
}

// ListEventsForEntity returns all events linked to a specific entity.
// Used for the reverse entity-event lookup on entity pages.
func (r *calendarRepo) ListEventsForEntity(ctx context.Context, entityID string, role int) ([]Event, error) {
	visFilter := "AND e.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}

	query := fmt.Sprintf(`
		SELECT `+eventCols+`
		FROM calendar_events e `+eventJoins+`
		WHERE e.entity_id = ?
		  %s
		ORDER BY e.year, e.month, e.day`, visFilter)

	rows, err := r.db.QueryContext(ctx, query, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEvents(rows)
}

// ListUpcomingEvents returns events on or after the given date, ordered
// chronologically.
//
// Recurring events appear once at their stored (year, month, day); they
// surface here only if the stored date is on/after the supplied cursor.
// V3 will ship unified recurring expansion; see Q-V2-6 resolution at
// decisions/2026-05-28-cal-timeline-v2-design.md.
func (r *calendarRepo) ListUpcomingEvents(ctx context.Context, calendarID string, year, month, day int, role int, limit int) ([]Event, error) {
	visFilter := "AND e.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}

	query := fmt.Sprintf(`
		SELECT `+eventCols+`
		FROM calendar_events e `+eventJoins+`
		WHERE e.calendar_id = ?
		  AND (
		    e.year > ? OR
		    (e.year = ? AND e.month > ?) OR
		    (e.year = ? AND e.month = ? AND e.day >= ?)
		  )
		  %s
		ORDER BY e.year, e.month, e.day, e.name
		LIMIT ?`, visFilter)

	rows, err := r.db.QueryContext(ctx, query,
		calendarID,
		year, year, month, year, month, day,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEvents(rows)
}

// EventDatesForCalendars batch-reads (year,month,day,name) for every event in
// the given calendars in ONE query (no N+1 across calendars), filtered to
// dm_only base visibility for non-DM viewers. Ordered by date so the caller can
// pick each calendar's soonest upcoming event using that calendar's own current
// date (dates aren't comparable across calendars). W5d — powers the next-event
// sort + the adaptive widget's agenda.
func (r *calendarRepo) EventDatesForCalendars(ctx context.Context, calIDs []string, role int) (map[string][]CalendarEventDate, error) {
	out := map[string][]CalendarEventDate{}
	if len(calIDs) == 0 {
		return out, nil
	}
	ph := make([]string, len(calIDs))
	args := make([]any, len(calIDs))
	for i, id := range calIDs {
		ph[i] = "?"
		args[i] = id
	}
	visFilter := "AND e.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}
	q := fmt.Sprintf(`SELECT e.calendar_id, e.year, e.month, e.day, e.name
		FROM calendar_events e
		WHERE e.calendar_id IN (%s) %s
		ORDER BY e.calendar_id, e.year, e.month, e.day, e.name`,
		strings.Join(ph, ","), visFilter)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var d CalendarEventDate
		if err := rows.Scan(&d.CalendarID, &d.Year, &d.Month, &d.Day, &d.Name); err != nil {
			return nil, err
		}
		out[d.CalendarID] = append(out[d.CalendarID], d)
	}
	return out, rows.Err()
}

// scanEvents reads event rows into a slice.
//
// PRE-EXISTING BUG FIXED HERE (C-CAL-RSVP-P1 Step-0 finding, out-of-dispatch but
// blocking): this list was missing &evt.RecurrenceDayOfWeek, so it supplied 36
// destinations for eventCols' 37 columns. Every LIST query routed through here
// (month/week/day/range/upcoming/search/ledger) therefore failed against MariaDB
// with "sql: expected 37 destination arguments in Scan, not 36" — only the
// single-row GetEvent, which has its own correct inline scan, worked. It could
// not be caught by any existing test because nothing in the repo executes real
// SQL (no sqlmock/testify/dockertest in go.mod), and it was introduced when
// migration 011 added recurrence_day_of_week to eventCols + GetEvent + the
// INSERT/UPDATE but not to this function. Adding collect_rsvps on top of the
// existing drift would have kept every list query broken, so the fix ships with
// this lane rather than after it. TestEventColsMatchScanDestinations now pins
// both lists so the next column can't repeat it.
func scanEvents(rows *sql.Rows) ([]Event, error) {
	var events []Event
	for rows.Next() {
		var evt Event
		if err := rows.Scan(
			&evt.ID, &evt.CalendarID, &evt.EntityID, &evt.Name, &evt.Description, &evt.DescriptionHTML,
			&evt.Year, &evt.Month, &evt.Day, &evt.StartHour, &evt.StartMinute,
			&evt.EndYear, &evt.EndMonth, &evt.EndDay, &evt.EndHour, &evt.EndMinute,
			&evt.IsRecurring, &evt.RecurrenceType,
			&evt.RecurrenceInterval, &evt.RecurrenceEndYear, &evt.RecurrenceEndMonth,
			&evt.RecurrenceEndDay, &evt.RecurrenceMaxOccurrences, &evt.RecurrenceDayOfWeek,
			&evt.Visibility, &evt.VisibilityRules, &evt.Category, &evt.Tier,
			&evt.Color, &evt.Icon, &evt.AllDay, &evt.CollectRSVPs,
			&evt.CreatedBy, &evt.CreatedAt, &evt.UpdatedAt,
			&evt.EntityName, &evt.EntityIcon, &evt.EntityColor,
		); err != nil {
			return nil, err
		}
		events = append(events, evt)
	}
	return events, rows.Err()
}

// UpdateEventVisibility sets the visibility and per-user rules on an event.
func (r *calendarRepo) UpdateEventVisibility(ctx context.Context, eventID string, visibility string, visRules *string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE calendar_events SET visibility = ?, visibility_rules = ? WHERE id = ?`,
		visibility, visRules, eventID,
	)
	return err
}

// UpdateCalendarVisibility sets the per-calendar visibility + rules (W5b).
// Bulk-replace: visibility_rules is written wholesale (the editor sends the
// complete allow/deny set), mirroring UpdateEventVisibility.
func (r *calendarRepo) UpdateCalendarVisibility(ctx context.Context, calendarID string, visibility string, visRules *string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE calendars SET visibility = ?, visibility_rules = ? WHERE id = ?`,
		visibility, visRules, calendarID,
	)
	return err
}

// SearchEvents returns events matching a name query, filtered by role-based visibility.
func (r *calendarRepo) SearchEvents(ctx context.Context, calendarID, query string, role int) ([]Event, error) {
	visFilter := "AND e.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}

	q := fmt.Sprintf(`
		SELECT `+eventCols+`
		FROM calendar_events e `+eventJoins+`
		WHERE e.calendar_id = ? AND e.name LIKE ? %s
		ORDER BY e.name
		LIMIT 10`, visFilter)

	escaped := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(query)
	rows, err := r.db.QueryContext(ctx, q, calendarID, "%"+escaped+"%")
	if err != nil {
		return nil, fmt.Errorf("search events: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}

// --- Weather ---

// GetWeather returns the current weather state for a calendar, or nil if none set.
func (r *calendarRepo) GetWeather(ctx context.Context, calendarID string) (*Weather, error) {
	w := &Weather{}
	var windSpeedKPHf sql.NullFloat64
	var windSpeedTier, windDir sql.NullString
	var windDirDegi sql.NullInt32
	var precipType sql.NullString
	var precipIntensity sql.NullFloat64

	err := r.db.QueryRowContext(ctx,
		`SELECT id, calendar_id, preset_id, preset_label, icon, color,
		        temperature_celsius, wind_speed_kph, wind_speed_tier,
		        wind_direction, wind_direction_degrees,
		        precipitation_type, precipitation_intensity,
		        zone_id, zone_name, description, updated_at
		 FROM calendar_weather WHERE calendar_id = ?`, calendarID,
	).Scan(&w.ID, &w.CalendarID, &w.PresetID, &w.PresetLabel, &w.Icon, &w.Color,
		&w.TemperatureCelsius, &windSpeedKPHf, &windSpeedTier,
		&windDir, &windDirDegi,
		&precipType, &precipIntensity,
		&w.ZoneID, &w.ZoneName, &w.Description, &w.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Build Wind struct if any wind data is present.
	if windSpeedKPHf.Valid || windDir.Valid {
		wind := &Wind{}
		if windSpeedKPHf.Valid {
			v := windSpeedKPHf.Float64
			wind.SpeedKPH = &v
		}
		if windSpeedTier.Valid {
			wind.SpeedTier = &windSpeedTier.String
		}
		if windDir.Valid {
			wind.Direction = &windDir.String
		}
		if windDirDegi.Valid {
			v := int(windDirDegi.Int32)
			wind.DirectionDegrees = &v
		}
		w.Wind = wind
	}

	// Build Precipitation struct if any precipitation data is present.
	if precipType.Valid {
		p := &Precipitation{Type: &precipType.String}
		if precipIntensity.Valid {
			p.Intensity = &precipIntensity.Float64
		}
		w.Precipitation = p
	}

	return w, nil
}

// SetWeather upserts the current weather state for a calendar.
func (r *calendarRepo) SetWeather(ctx context.Context, calendarID string, input WeatherInput) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_weather (calendar_id, preset_id, preset_label, icon, color,
		        temperature_celsius, wind_speed_kph, wind_speed_tier,
		        wind_direction, wind_direction_degrees,
		        precipitation_type, precipitation_intensity,
		        zone_id, zone_name, description)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		        preset_id = VALUES(preset_id), preset_label = VALUES(preset_label),
		        icon = VALUES(icon), color = VALUES(color),
		        temperature_celsius = VALUES(temperature_celsius),
		        wind_speed_kph = VALUES(wind_speed_kph), wind_speed_tier = VALUES(wind_speed_tier),
		        wind_direction = VALUES(wind_direction), wind_direction_degrees = VALUES(wind_direction_degrees),
		        precipitation_type = VALUES(precipitation_type), precipitation_intensity = VALUES(precipitation_intensity),
		        zone_id = VALUES(zone_id), zone_name = VALUES(zone_name),
		        description = VALUES(description)`,
		calendarID, input.PresetID, input.PresetLabel, input.Icon, input.Color,
		input.TemperatureCelsius, input.WindSpeedKPH, input.WindSpeedTier,
		input.WindDirection, input.WindDirectionDeg,
		input.PrecipitationType, input.PrecipitationIntensity,
		input.ZoneID, input.ZoneName, input.Description,
	)
	return err
}

// --- Weather zones (V2 Wave 0 PR 3 / C-CAL-WEATHER-ZONES) ---

// GetWeatherZones returns all zone definitions for a calendar ordered
// by zone_id (stable for the wire-contract response).
func (r *calendarRepo) GetWeatherZones(ctx context.Context, calendarID string) ([]WeatherZone, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT calendar_id, zone_id, name, payload, created_at, updated_at
		 FROM calendar_weather_zones WHERE calendar_id = ? ORDER BY zone_id`,
		calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var zones []WeatherZone
	for rows.Next() {
		var z WeatherZone
		var payloadRaw []byte
		if err := rows.Scan(&z.CalendarID, &z.ZoneID, &z.Name, &payloadRaw, &z.CreatedAt, &z.UpdatedAt); err != nil {
			return nil, err
		}
		if len(payloadRaw) > 0 {
			if err := json.Unmarshal(payloadRaw, &z.Payload); err != nil {
				return nil, fmt.Errorf("unmarshal zone %q payload: %w", z.ZoneID, err)
			}
		}
		zones = append(zones, z)
	}
	return zones, rows.Err()
}

// ApplyWeatherZones replaces the full zone set for a calendar in one
// transaction (delete-then-insert pattern mirroring SetMonths /
// SetSeasons / etc.). Validation runs at the service layer before
// this method is called.
func (r *calendarRepo) ApplyWeatherZones(ctx context.Context, calendarID string, zones []WeatherZone) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM calendar_weather_zones WHERE calendar_id = ?`, calendarID); err != nil {
		return fmt.Errorf("delete zones: %w", err)
	}
	for _, z := range zones {
		payloadJSON, err := json.Marshal(z.Payload)
		if err != nil {
			return fmt.Errorf("marshal zone %q payload: %w", z.ZoneID, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_weather_zones (calendar_id, zone_id, name, payload)
			 VALUES (?, ?, ?, ?)`,
			calendarID, z.ZoneID, z.Name, string(payloadJSON),
		); err != nil {
			return fmt.Errorf("insert zone %q: %w", z.ZoneID, err)
		}
	}
	return tx.Commit()
}

// SetActiveWeatherZone updates the active-zone reference on the
// existing calendar_weather row. zone_id + zone_name columns were
// added in migration 003; passing "" for both clears the active zone.
// Upserts so a calendar with no prior weather row still gets the
// active-zone reference recorded.
func (r *calendarRepo) SetActiveWeatherZone(ctx context.Context, calendarID, zoneID, zoneName string) error {
	var zoneIDPtr, zoneNamePtr any
	if zoneID != "" {
		zoneIDPtr = zoneID
	}
	if zoneName != "" {
		zoneNamePtr = zoneName
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_weather (calendar_id, zone_id, zone_name)
		 VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		        zone_id = VALUES(zone_id),
		        zone_name = VALUES(zone_name)`,
		calendarID, zoneIDPtr, zoneNamePtr)
	return err
}

// --- Cycles ---

// SetCycles replaces all cycles and their entries for a calendar.
func (r *calendarRepo) SetCycles(ctx context.Context, calendarID string, cycles []CycleInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete existing cycles (entries cascade via FK).
	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_cycles WHERE calendar_id = ?`, calendarID); err != nil {
		return err
	}
	for _, c := range cycles {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_cycles (calendar_id, name, cycle_length, type, sort_order)
			 VALUES (?, ?, ?, ?, ?)`,
			calendarID, c.Name, c.CycleLength, c.Type, c.SortOrder)
		if err != nil {
			return err
		}
		cycleID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for _, e := range c.Entries {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO calendar_cycle_entries (cycle_id, name, icon, year_offset, sort_order)
				 VALUES (?, ?, ?, ?, ?)`,
				cycleID, e.Name, e.Icon, e.YearOffset, e.SortOrder,
			); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// GetCycles returns all cycles with their entries for a calendar.
func (r *calendarRepo) GetCycles(ctx context.Context, calendarID string) ([]Cycle, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, calendar_id, name, cycle_length, type, sort_order
		 FROM calendar_cycles WHERE calendar_id = ? ORDER BY sort_order`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cycles []Cycle
	for rows.Next() {
		var c Cycle
		if err := rows.Scan(&c.ID, &c.CalendarID, &c.Name, &c.CycleLength, &c.Type, &c.SortOrder); err != nil {
			return nil, err
		}
		cycles = append(cycles, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load entries for each cycle.
	for i := range cycles {
		entryRows, err := r.db.QueryContext(ctx,
			`SELECT id, cycle_id, name, icon, year_offset, sort_order
			 FROM calendar_cycle_entries WHERE cycle_id = ? ORDER BY sort_order`, cycles[i].ID)
		if err != nil {
			return nil, err
		}
		for entryRows.Next() {
			var e CycleEntry
			if err := entryRows.Scan(&e.ID, &e.CycleID, &e.Name, &e.Icon, &e.YearOffset, &e.SortOrder); err != nil {
				entryRows.Close()
				return nil, err
			}
			cycles[i].Entries = append(cycles[i].Entries, e)
		}
		entryRows.Close()
		if err := entryRows.Err(); err != nil {
			return nil, err
		}
	}
	return cycles, nil
}

// --- Festivals ---

// SetFestivals replaces all festivals for a calendar.
func (r *calendarRepo) SetFestivals(ctx context.Context, calendarID string, festivals []FestivalInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_festivals WHERE calendar_id = ?`, calendarID); err != nil {
		return err
	}
	for _, f := range festivals {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_festivals (calendar_id, name, month, day, after_month, description, color, icon, sort_order)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			calendarID, f.Name, f.Month, f.Day, f.AfterMonth, f.Description, f.Color, f.Icon, f.SortOrder,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetFestivals returns all festivals for a calendar.
func (r *calendarRepo) GetFestivals(ctx context.Context, calendarID string) ([]Festival, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, calendar_id, name, month, day, after_month, description, color, icon, sort_order
		 FROM calendar_festivals WHERE calendar_id = ? ORDER BY sort_order`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var festivals []Festival
	for rows.Next() {
		var f Festival
		if err := rows.Scan(&f.ID, &f.CalendarID, &f.Name, &f.Month, &f.Day, &f.AfterMonth,
			&f.Description, &f.Color, &f.Icon, &f.SortOrder); err != nil {
			return nil, err
		}
		festivals = append(festivals, f)
	}
	return festivals, rows.Err()
}
