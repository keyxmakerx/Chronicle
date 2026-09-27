// Package calendar - service.go holds the calendar plugin's business logic
// over the repositories (CalendarRepository, EventRepository,
// EventKindRepository, WeatherRepository). It never imports Echo — the
// handler binds requests and the service decides what happens.
//
// Every read that a Player, Scribe or public visitor can reach goes through
// a permissions.Viewer (ADR-049): an anonymous request can never be mistaken
// for a trusted system caller, and dm_only content / hidden moons / per-user
// visibility_rules are resolved the same way for every viewer weaker than
// Owner. Structural resources (event kinds, eras, the moon hidden flag) are
// Owner-only end to end, so they carry no viewer parameter at all.
package calendar

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// slugPattern validates an event kind's slug: lowercase, digits and hyphens,
// no leading/trailing hyphen. Mirrors the shape other plugins use for
// user-authored slugs referenced by other rows.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// faIconPattern and hexColorPattern restrict an icon or color value to a
// closed character class: icons and colors are interpolated into class and
// style attributes by the clients that render them, so a quote or an angle
// bracket in either must be structurally unrepresentable rather than merely
// discouraged. faIconPattern accepts only a Font Awesome class name (fa- +
// lowercase letters/digits/hyphens); hexColorPattern is the same #rgb/
// #rrggbb pattern internal/plugins/entities/service.go and
// internal/plugins/timeline/service.go already use for their own color
// fields. Kept as its own local copy per this plugin's isolation (cross-
// plugin reuse goes through a service interface, never a shared regex
// imported across the boundary).
var (
	faIconPattern   = regexp.MustCompile(`^fa-[a-z0-9-]+$`)
	hexColorPattern = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
)

// Column-width limits: a value that doesn't fit its column must fail as a
// clean apperror, never reach the driver and come back as a raw "data too
// long" error. These match the LIVE schema (calendar_events.color/icon were
// widened to VARCHAR(20)/VARCHAR(50) in migration 002, superseding their
// original 001 definition) rather than any one migration file in isolation.
const (
	maxEventColorLength    = 20    // calendar_events.color VARCHAR(20)
	maxEventIconLength     = 50    // calendar_events.icon VARCHAR(50)
	maxEventTierLength     = 64    // calendar_events.tier VARCHAR(64)
	maxEpochNameLength     = 100   // calendars.epoch_name VARCHAR(100)
	maxRealTimeZoneLength  = 64    // calendars.real_time_zone VARCHAR(64)
	maxEventKindSlugLength = 50    // calendar_event_kinds.slug VARCHAR(50)
	maxEventKindNameLength = 100   // calendar_event_kinds.name VARCHAR(100)
	maxTextColumnBytes     = 65535 // a TEXT column's byte capacity (description, description_html)
)

// generateID creates a random UUID v4 string. Duplicated per-package by
// convention (see timeline.generateID) rather than shared across the plugin
// boundary.
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// EntityVisibilityGate resolves which of a set of entity IDs a viewer (role
// + user id) may actually see — the same seam maps.MapService uses
// (maps/service.go's EntityVisibilityGate). An event's entity_id join reads
// straight from the entities table with no visibility filter of its own
// (unlike the entity_event_links ties in entity_ties_repository.go, which
// already replicate entities' own filter), so this is the one place that
// closes it for the single linked-entity fields on Event.
type EntityVisibilityGate interface {
	FilterViewableEntityIDs(ctx context.Context, campaignID string, entityIDs []string, role int, userID string) (map[string]bool, error)
}

// CalendarService defines the calendar plugin's business logic. Handlers
// call it; it never imports Echo.
type CalendarService interface {
	// Calendars.
	CreateCalendar(ctx context.Context, campaignID string, input CreateCalendarInput) (*Calendar, error)
	// GetCalendarForViewer returns a calendar only if it belongs to
	// campaignID and v may see it (ADR-049); both failure modes collapse to
	// the same NotFound. Eager-loads every sub-resource; Eras and
	// EventKinds are stripped and Moons are filtered for a viewer that does
	// not skip the per-user layer.
	GetCalendarForViewer(ctx context.Context, calendarID, campaignID string, v permissions.Viewer) (*Calendar, error)
	// GetDefaultCalendarForViewer returns the campaign's default calendar
	// (Calendar.IsDefault), gated exactly like GetCalendarForViewer, for
	// callers that want "the campaign's calendar" without already knowing its
	// id — e.g. the skybox dashboard/template block, which is campaign-level
	// and binds to no specific calendar. Returns apperror.NotFound both when
	// the campaign has no default calendar and when it has one the viewer may
	// not see, the same collapse GetCalendarForViewer uses.
	GetDefaultCalendarForViewer(ctx context.Context, campaignID string, v permissions.Viewer) (*Calendar, error)
	// ListCalendars returns a campaign's calendars, role- and per-user
	// visibility-filtered. No sub-resources are eager-loaded (list view).
	ListCalendars(ctx context.Context, campaignID string, v permissions.Viewer) ([]Calendar, error)
	UpdateCalendar(ctx context.Context, calendarID, campaignID string, input UpdateCalendarInput) error
	DeleteCalendar(ctx context.Context, calendarID, campaignID string) error
	SetDefaultCalendar(ctx context.Context, campaignID, calendarID string) error

	// Events. CreateEvent has no stored row to weigh a viewer against, so it
	// takes no Viewer: authorization for its one viewer-dependent decision
	// (may this caller author dm_only content) travels pre-resolved on
	// CreateEventInput.CanAuthorDmOnly, set by the handler from the request
	// viewer — see that field's doc comment. Every method below THIS point
	// takes a permissions.Viewer: an event on a calendar the viewer cannot
	// see, or an event the viewer cannot see itself, must answer identically
	// to one that doesn't exist — a write is not an exception to that (a
	// caller who cannot GET an event must not be able to PUT it either).
	// Visibility writes are further gated by who may AUTHOR dm_only content:
	// v.SkipsPerUserRules() is exactly campaigns.CanAuthorDmOnly()'s
	// condition when the caller's Viewer was built from cc.VisibilityRole()
	// (see handler.go's viewerFrom) — an Owner or a granted co-DM, never a
	// plain Scribe. visibility_rules carries no such gate (canChangeVisibility's
	// doc comment says why).
	CreateEvent(ctx context.Context, calendarID, campaignID string, input CreateEventInput) (*Event, error)
	// GetEventForViewer is GetEvent's viewer-aware sibling: it returns
	// NotFound (never the event) unless the event belongs to calendarID,
	// calendarID belongs to campaignID AND is itself visible to v, AND the
	// event is visible to v.
	GetEventForViewer(ctx context.Context, eventID, calendarID, campaignID string, v permissions.Viewer) (*Event, error)
	ListEventsForMonth(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) ([]Event, error)
	// ListUpcomingEvents returns up to limit events on or after the
	// calendar's current date, chronological, viewer-filtered exactly like
	// ListEventsForMonth (SQL role filter + per-user visibility_rules +
	// hidden-entity-link redaction). Powers the "what's coming up" dashboard
	// and category-dashboard preview blocks (campaigns.dashCalendarPreview/
	// dashCalendarFull, entities.catCalendarPreview) — none of them are
	// scoped to a single month, unlike ListEventsForMonth. Does not expand
	// recurrence beyond each event's own stored date (the repository read
	// this wraps is a literal date-range scan); a recurring event only
	// appears here on its next literal occurrence row, not a virtual one.
	ListUpcomingEvents(ctx context.Context, calendarID, campaignID string, limit int, v permissions.Viewer) ([]Event, error)
	UpdateEvent(ctx context.Context, eventID, calendarID, campaignID string, input UpdateEventInput, v permissions.Viewer) error
	DeleteEvent(ctx context.Context, eventID, calendarID, campaignID string, v permissions.Viewer) error
	// SetEventVisibility is the dm_only toggle's dedicated action endpoint;
	// its route already requires campaigns.CanAuthorDmOnly, but the check
	// is repeated here (v.SkipsPerUserRules()) rather than trusted blindly.
	SetEventVisibility(ctx context.Context, eventID, calendarID, campaignID string, input UpdateEventVisibilityInput, v permissions.Viewer) error

	// Event kinds. Campaign-scoped (shared by every calendar in the
	// campaign, see EventKind's doc comment) and Owner-only end to end —
	// calendar structure, not content a Player ever reads directly.
	ListEventKinds(ctx context.Context, campaignID string) ([]EventKind, error)
	CreateEventKind(ctx context.Context, campaignID string, input EventKindInput) (*EventKind, error)
	UpdateEventKind(ctx context.Context, kindID int, campaignID string, input UpdateEventKindInput) error
	DeleteEventKind(ctx context.Context, kindID int, campaignID string) error

	// Eras. Owner-only end to end (calendar structure), resolved through the
	// calendar the caller reached (GetEraByID itself is not calendar-scoped;
	// this service never calls it with an untrusted id without also
	// checking the result's CalendarID — see eraInCalendar).
	CreateEra(ctx context.Context, calendarID, campaignID string, input EraInput) (*Era, error)
	UpdateEra(ctx context.Context, eraID int, calendarID, campaignID string, input UpdateEraInput) error
	DeleteEra(ctx context.Context, eraID int, calendarID, campaignID string) error

	// Moon. Owner-only end to end (the hidden flag is calendar structure).
	SetMoonHidden(ctx context.Context, moonID int, calendarID, campaignID string, hidden bool) error

	// --- Bulk structure writers ---
	//
	// SetMonths/SetWeekdays/SetMoons/SetSeasons replace a calendar's whole
	// list for that sub-resource in one call. They exist for the campaign
	// import path (calendarImportAdapter, internal/app/export_adapters.go)
	// and the calendar plugin's own native/Simple-Calendar/Calendaria import
	// (internal/plugins/calendar/import.go), neither of which has a stored
	// row per item to update incrementally the way CreateEra/CreateEventKind
	// do. Not wired to any HTTP route yet (calendar-v5 slice 2, #741) — Owner-
	// only calendar-settings editing UI is a later slice — so today's only
	// callers are trusted in-process ones; a caller reaching these through a
	// future HTTP handler must still be gated Owner-only there, the same as
	// every other calendar-structure write in this file.
	SetMonths(ctx context.Context, calendarID, campaignID string, months []MonthInput) error
	SetWeekdays(ctx context.Context, calendarID, campaignID string, weekdays []WeekdayInput) error
	SetMoons(ctx context.Context, calendarID, campaignID string, moons []MoonInput) error
	SetSeasons(ctx context.Context, calendarID, campaignID string, seasons []Season) error

	// ListAllEventsForCalendar returns every event for a calendar with no
	// role or per-user visibility filter — a bulk, unredacted read for
	// SYSTEM-ONLY callers that apply their own gating afterward: the
	// campaign export walk (calendarExportAdapter) and the Director-only AI
	// export (aiexport.CalendarLister). v must be a permissions.SystemViewer
	// (ADR-049); any other viewer is refused, since nothing here re-checks
	// per-event visibility the way ListEventsForMonth's SQL role filter +
	// filterEventsByUser do — a caller that isn't declaring system trust
	// must not get the unfiltered list.
	ListAllEventsForCalendar(ctx context.Context, calendarID, campaignID string, v permissions.Viewer) ([]Event, error)
}

// calendarService is the concrete CalendarService.
type calendarService struct {
	calRepo     CalendarRepository
	eventRepo   EventRepository
	kindRepo    EventKindRepository
	weatherRepo WeatherRepository
	entityGate  EntityVisibilityGate
}

// NewCalendarService constructs a CalendarService over the four
// repositories.
func NewCalendarService(calRepo CalendarRepository, eventRepo EventRepository, kindRepo EventKindRepository, weatherRepo WeatherRepository) CalendarService {
	return &calendarService{calRepo: calRepo, eventRepo: eventRepo, kindRepo: kindRepo, weatherRepo: weatherRepo}
}

// SetEntityVisibilityGate injects the entity-visibility check used to blank
// an event's linked-entity fields for a viewer who cannot separately see
// that entity (ADR-055 rule 3). Reached via a type assertion at wiring time
// (see app/routes.go), the same pattern maps.MapService's
// SetEntityVisibilityGate uses, so the CalendarService interface itself
// stays unchanged for callers that don't need it (tests, mocks).
func (s *calendarService) SetEntityVisibilityGate(g EntityVisibilityGate) { s.entityGate = g }

// --- Cross-campaign / cross-calendar scoping (no cross-tenant reach) ---

// calendarInCampaign loads a calendar and confirms it belongs to campaignID,
// collapsing "doesn't exist" and "exists in a different campaign" into the
// exact same NotFound message — a wrong-campaign id must look identical to a
// missing one, or its existence becomes an oracle for a prober. This is the
// one gate every calendar-, event-, era- and moon-scoped method below calls
// first. It does NOT check visibility — see calendarVisibleToViewer for
// that; callers that reach content through a Viewer must check both.
func (s *calendarService) calendarInCampaign(ctx context.Context, calendarID, campaignID string) (*Calendar, error) {
	cal, err := s.calRepo.GetByID(ctx, calendarID)
	if err != nil {
		// CalendarRepository.GetByID already returns apperror.NewNotFound
		// when the row is missing; anything else is a real infra error.
		return nil, err
	}
	if cal.CampaignID != campaignID {
		return nil, apperror.NewNotFound("calendar not found")
	}
	return cal, nil
}

// calendarInCampaignForViewer is calendarInCampaign plus the visibility
// check: a calendar the viewer cannot see (dm_only, or excluded by its own
// visibility_rules) answers exactly like a missing one, for every event/era
// read or write reached through it — a hidden calendar's events must never
// be reachable just because the calendar id and campaign id are otherwise
// valid.
func (s *calendarService) calendarInCampaignForViewer(ctx context.Context, calendarID, campaignID string, v permissions.Viewer) (*Calendar, error) {
	cal, err := s.calendarInCampaign(ctx, calendarID, campaignID)
	if err != nil {
		return nil, err
	}
	if !calendarVisibleToViewer(*cal, v) {
		return nil, apperror.NewNotFound("calendar not found")
	}
	return cal, nil
}

// --- Visibility (ADR-049): the one predicate behind every calendar/event
// decision in this package, mirroring timeline's canUserView/
// timelineVisibleToViewer so list and single-item reads can't drift apart. ---

// canUserView resolves a visibility ("everyone"/"dm_only") + optional
// per-user rules JSON against v. Callers that already know v skips the
// per-user layer (SkipsPerUserRules) should short-circuit before calling
// this; it re-derives the dm_only half itself too, since some callers (a
// direct by-id fetch with no SQL role filter) reach it without that
// short-circuit.
func canUserView(visibility string, visRulesJSON *string, v permissions.Viewer) bool {
	if visibility == "dm_only" && !permissions.CanSeeDmOnly(v.Role()) {
		return false
	}
	return ParseVisibilityRules(visRulesJSON).Allows(v.UserID())
}

// calendarVisibleToViewer is the one predicate behind every calendar
// visibility decision. filterCalendarsByUser's per-row filter and every
// single-item lookup both call it.
func calendarVisibleToViewer(cal Calendar, v permissions.Viewer) bool {
	if v.SkipsPerUserRules() {
		return true
	}
	return canUserView(cal.Visibility, cal.VisibilityRules, v)
}

// filterCalendarsByUser applies the per-user visibility layer to a calendar
// slice. Compacts in place (cals[:0]) — the caller's backing array is
// mutated and must not be read again.
func filterCalendarsByUser(cals []Calendar, v permissions.Viewer) []Calendar {
	filtered := cals[:0]
	for _, c := range cals {
		if calendarVisibleToViewer(c, v) {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// eventVisibleToViewer is the one predicate behind every event visibility
// decision, mirroring calendarVisibleToViewer.
func eventVisibleToViewer(evt Event, v permissions.Viewer) bool {
	if v.SkipsPerUserRules() {
		return true
	}
	return canUserView(evt.Visibility, evt.VisibilityRules, v)
}

// filterEventsByUser applies the per-user visibility layer to an event
// slice already SQL-role-filtered for dm_only (see ListEventsForMonth):
// this only needs to catch the per-user visibility_rules layer SQL can't
// see, but re-checks dm_only too since it costs nothing and keeps the
// predicate identical to the single-item path.
func filterEventsByUser(events []Event, v permissions.Viewer) []Event {
	if v.SkipsPerUserRules() {
		return events
	}
	filtered := events[:0]
	for _, e := range events {
		if eventVisibleToViewer(e, v) {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// filterMoonsForViewer strips moons HiddenFromPlayers for a viewer that does
// not skip the per-user layer. GetMoons returns every moon regardless of
// caller (see its doc comment); this is the one place that narrows it for
// players.
func filterMoonsForViewer(moons []Moon, v permissions.Viewer) []Moon {
	if v.SkipsPerUserRules() {
		return moons
	}
	filtered := moons[:0]
	for _, m := range moons {
		if !m.HiddenFromPlayers {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// redactHiddenEntityLinks blanks EntityID/EntityName/EntityIcon/EntityColor
// on any event whose linked entity the viewer isn't separately permitted to
// see, mirroring maps.ListMarkers's identical treatment of a marker's linked
// entity. Owners/co-DMs/system callers are unfiltered. If the gate was never
// wired (a construction bug, not a policy outcome), this fails CLOSED —
// every linked entity is blanked — rather than leaking a name because
// nothing was configured to check it.
func (s *calendarService) redactHiddenEntityLinks(ctx context.Context, campaignID string, events []Event, v permissions.Viewer) error {
	if v.SkipsPerUserRules() {
		return nil
	}
	ids := make([]string, 0, len(events))
	seen := make(map[string]bool, len(events))
	for _, e := range events {
		if e.EntityID == nil || *e.EntityID == "" || seen[*e.EntityID] {
			continue
		}
		seen[*e.EntityID] = true
		ids = append(ids, *e.EntityID)
	}
	if len(ids) == 0 {
		return nil
	}

	var viewable map[string]bool
	if s.entityGate == nil {
		slog.Error("calendar: entity visibility gate not configured; blanking all linked entity names",
			slog.String("campaign_id", campaignID))
		viewable = map[string]bool{}
	} else {
		var err error
		viewable, err = s.entityGate.FilterViewableEntityIDs(ctx, campaignID, ids, v.Role(), v.UserID())
		if err != nil {
			return fmt.Errorf("filter viewable event entities: %w", err)
		}
	}

	for i := range events {
		e := &events[i]
		if e.EntityID == nil || *e.EntityID == "" {
			continue
		}
		if !viewable[*e.EntityID] {
			// Blank the ID too, not just the name/icon: a bare id still
			// tells the viewer a specific hidden entity exists.
			e.EntityID = nil
			e.EntityName = ""
			e.EntityIcon = ""
			e.EntityColor = ""
		}
	}
	return nil
}

// canChangeVisibility decides whether a present visibility write may
// proceed: an author (v.SkipsPerUserRules(), see the CalendarService doc
// comment on why that's the correct test) may set it to anything; anyone
// else may only "change" it to the value already stored — which is not a
// change at all, so it is silently treated as a no-op rather than either
// rewritten or refused. Anything else — a genuine attempted change by a
// non-author — is a 403, never a silent rewrite.
//
// visibility_rules (the per-user allow/deny list) is NOT gated the same
// way: it can only ever narrow who among the players sees an "everyone"
// event, never hide anything from an Owner/co-DM (eventVisibleToViewer
// short-circuits to visible for any v.SkipsPerUserRules() caller before
// visibility_rules is even consulted), so a Scribe setting it is a content
// decision within their existing Create/edit authority, not a dm_only-shaped
// privilege — the field is scoped by the ordinary Scribe route gate, not by
// CanAuthorDmOnly.
func canChangeVisibility(v permissions.Viewer, newVisibility, storedVisibility string) error {
	if v.SkipsPerUserRules() || newVisibility == storedVisibility {
		return nil
	}
	return apperror.NewForbidden("only the campaign owner or a granted co-DM may change this event's visibility")
}

// --- Calendars ---

// CreateCalendar creates a bare calendar (no months/weekdays/moons — later
// work owns seeding a preset or an editing UI for those). Mode defaults to
// fantasy; the day/hour/minute/second geometry defaults to a standard
// 24/60/60 day when the caller leaves it zero.
func (s *calendarService) CreateCalendar(ctx context.Context, campaignID string, input CreateCalendarInput) (*Calendar, error) {
	if err := apperror.ValidateRequired("name", input.Name); err != nil {
		return nil, err
	}
	if err := apperror.ValidateStringLength("name", input.Name, apperror.MaxNameLength); err != nil {
		return nil, err
	}
	mode := input.Mode
	if mode == "" {
		mode = ModeFantasy
	}
	if mode != ModeFantasy && mode != ModeRealLife {
		return nil, apperror.NewValidation("mode must be \"" + ModeFantasy + "\" or \"" + ModeRealLife + "\"")
	}
	hoursPerDay := input.HoursPerDay
	if hoursPerDay == 0 {
		hoursPerDay = 24
	}
	minutesPerHour := input.MinutesPerHour
	if minutesPerHour == 0 {
		minutesPerHour = 60
	}
	secondsPerMinute := input.SecondsPerMinute
	if secondsPerMinute == 0 {
		secondsPerMinute = 60
	}
	if hoursPerDay < 0 || minutesPerHour < 0 || secondsPerMinute < 0 {
		return nil, apperror.NewValidation("hours_per_day, minutes_per_hour and seconds_per_minute must not be negative")
	}
	if err := validateOptionalText("description", input.Description, apperror.MaxDescriptionLength); err != nil {
		return nil, err
	}
	if err := validateOptionalText("epoch_name", input.EpochName, maxEpochNameLength); err != nil {
		return nil, err
	}
	visibility := input.Visibility
	if visibility == "" {
		visibility = "everyone"
	}
	if visibility != "everyone" && visibility != "dm_only" {
		return nil, apperror.NewValidation("visibility must be \"everyone\" or \"dm_only\"")
	}
	if err := validateVisibilityRulesJSON(input.VisibilityRules); err != nil {
		return nil, err
	}

	cal := &Calendar{
		ID:               generateID(),
		CampaignID:       campaignID,
		Mode:             mode,
		Name:             input.Name,
		Description:      input.Description,
		EpochName:        input.EpochName,
		CurrentYear:      input.CurrentYear,
		HoursPerDay:      hoursPerDay,
		MinutesPerHour:   minutesPerHour,
		SecondsPerMinute: secondsPerMinute,
		LeapYearEvery:    input.LeapYearEvery,
		LeapYearOffset:   input.LeapYearOffset,
		Visibility:       visibility,
		VisibilityRules:  input.VisibilityRules,
	}
	if err := s.calRepo.Create(ctx, cal); err != nil {
		return nil, fmt.Errorf("create calendar: %w", err)
	}
	return cal, nil
}

// GetCalendarForViewer returns a calendar with its sub-resources, gated by
// campaign membership and visibility (see the interface doc comment).
func (s *calendarService) GetCalendarForViewer(ctx context.Context, calendarID, campaignID string, v permissions.Viewer) (*Calendar, error) {
	cal, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v)
	if err != nil {
		return nil, err
	}
	return s.finishCalendarForViewer(ctx, cal, v)
}

// GetDefaultCalendarForViewer returns the campaign's default calendar with
// its sub-resources, gated the same way as GetCalendarForViewer (see the
// interface doc comment). Shares finishCalendarForViewer with it so the two
// entry points — "this calendar id" and "whichever calendar is default" —
// can never load or filter sub-resources differently.
func (s *calendarService) GetDefaultCalendarForViewer(ctx context.Context, campaignID string, v permissions.Viewer) (*Calendar, error) {
	cal, err := s.calRepo.GetDefaultByCampaignID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if cal == nil || cal.CampaignID != campaignID || !calendarVisibleToViewer(*cal, v) {
		return nil, apperror.NewNotFound("calendar not found")
	}
	return s.finishCalendarForViewer(ctx, cal, v)
}

// finishCalendarForViewer eager-loads cal's sub-resources and applies the
// viewer-gated stripping (event kinds/eras hidden entirely, hidden moons
// filtered) that every "one calendar, for this viewer" read needs. Callers
// have already resolved cal and checked visibility.
func (s *calendarService) finishCalendarForViewer(ctx context.Context, cal *Calendar, v permissions.Viewer) (*Calendar, error) {
	if err := s.loadSubresources(ctx, cal); err != nil {
		return nil, err
	}
	if !v.SkipsPerUserRules() {
		// Event kinds and eras are calendar STRUCTURE (Owner-only end to
		// end, see .ai/conventions.md's permission table); a Player never
		// learns of their existence through the calendar read either.
		// Moons are content — only the hidden ones are stripped.
		cal.EventKinds = nil
		cal.Eras = nil
		cal.Moons = filterMoonsForViewer(cal.Moons, v)
	}
	return cal, nil
}

// loadSubresources eager-loads every sub-resource onto cal, unfiltered.
// Viewer-gating (stripping Eras/EventKinds, filtering hidden Moons) is the
// caller's job — see GetCalendarForViewer.
func (s *calendarService) loadSubresources(ctx context.Context, cal *Calendar) error {
	var err error
	if cal.Months, err = s.calRepo.GetMonths(ctx, cal.ID); err != nil {
		return fmt.Errorf("load months: %w", err)
	}
	if cal.Weekdays, err = s.calRepo.GetWeekdays(ctx, cal.ID); err != nil {
		return fmt.Errorf("load weekdays: %w", err)
	}
	if cal.Moons, err = s.calRepo.GetMoons(ctx, cal.ID); err != nil {
		return fmt.Errorf("load moons: %w", err)
	}
	if cal.Seasons, err = s.calRepo.GetSeasons(ctx, cal.ID); err != nil {
		return fmt.Errorf("load seasons: %w", err)
	}
	if cal.Eras, err = s.calRepo.GetEras(ctx, cal.ID); err != nil {
		return fmt.Errorf("load eras: %w", err)
	}
	if cal.EventKinds, err = s.kindRepo.List(ctx, cal.CampaignID); err != nil {
		return fmt.Errorf("load event kinds: %w", err)
	}
	if cal.Cycles, err = s.calRepo.GetCycles(ctx, cal.ID); err != nil {
		return fmt.Errorf("load cycles: %w", err)
	}
	if cal.Festivals, err = s.calRepo.GetFestivals(ctx, cal.ID); err != nil {
		return fmt.Errorf("load festivals: %w", err)
	}
	weather, err := s.weatherRepo.Get(ctx, cal.ID)
	if err != nil {
		return fmt.Errorf("load weather: %w", err)
	}
	cal.Weather = weather
	return nil
}

// loadCalendarGeometry loads just enough of a calendar to drive
// Event.OccursOn / MonthDays / WeekLength (Months + Weekdays), for
// ListEventsForMonth's recurrence placement — cheaper than
// loadSubresources's full eager-load, which this doesn't need.
func (s *calendarService) loadCalendarGeometry(ctx context.Context, cal *Calendar) error {
	var err error
	if cal.Months, err = s.calRepo.GetMonths(ctx, cal.ID); err != nil {
		return fmt.Errorf("load months: %w", err)
	}
	if cal.Weekdays, err = s.calRepo.GetWeekdays(ctx, cal.ID); err != nil {
		return fmt.Errorf("load weekdays: %w", err)
	}
	return nil
}

// ListCalendars returns a campaign's calendars, role- and per-user
// visibility-filtered. No sub-resources are loaded (see the interface doc).
func (s *calendarService) ListCalendars(ctx context.Context, campaignID string, v permissions.Viewer) ([]Calendar, error) {
	cals, err := s.calRepo.ListByCampaignID(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list calendars: %w", err)
	}
	return filterCalendarsByUser(cals, v), nil
}

// UpdateCalendar applies a partial settings update (load-merge-write, see
// UpdateCalendarInput's doc comment).
func (s *calendarService) UpdateCalendar(ctx context.Context, calendarID, campaignID string, input UpdateCalendarInput) error {
	cal, err := s.calendarInCampaign(ctx, calendarID, campaignID)
	if err != nil {
		return err
	}

	name := input.Name
	if name == "" {
		return apperror.NewValidation("calendar name is required")
	}
	if err := apperror.ValidateStringLength("name", name, apperror.MaxNameLength); err != nil {
		return err
	}
	mode := input.Mode.Val(cal.Mode)
	if mode != ModeFantasy && mode != ModeRealLife {
		return apperror.NewValidation("mode must be \"" + ModeFantasy + "\" or \"" + ModeRealLife + "\"")
	}
	hoursPerDay := input.HoursPerDay.Val(cal.HoursPerDay)
	minutesPerHour := input.MinutesPerHour.Val(cal.MinutesPerHour)
	secondsPerMinute := input.SecondsPerMinute.Val(cal.SecondsPerMinute)
	if hoursPerDay <= 0 || minutesPerHour <= 0 || secondsPerMinute <= 0 {
		return apperror.NewValidation("hours_per_day, minutes_per_hour and seconds_per_minute must be positive")
	}
	description := input.Description.Ptr(cal.Description)
	if err := validateOptionalText("description", description, apperror.MaxDescriptionLength); err != nil {
		return err
	}
	epochName := input.EpochName.Ptr(cal.EpochName)
	if err := validateOptionalText("epoch_name", epochName, maxEpochNameLength); err != nil {
		return err
	}

	cal.Name = name
	cal.Description = description
	cal.EpochName = epochName
	cal.Mode = mode
	cal.CurrentYear = input.CurrentYear.Val(cal.CurrentYear)
	cal.CurrentMonth = input.CurrentMonth.Val(cal.CurrentMonth)
	cal.CurrentDay = input.CurrentDay.Val(cal.CurrentDay)
	cal.CurrentHour = input.CurrentHour.Val(cal.CurrentHour)
	cal.CurrentMinute = input.CurrentMinute.Val(cal.CurrentMinute)
	cal.HoursPerDay = hoursPerDay
	cal.MinutesPerHour = minutesPerHour
	cal.SecondsPerMinute = secondsPerMinute
	cal.LeapYearEvery = input.LeapYearEvery.Val(cal.LeapYearEvery)
	cal.LeapYearOffset = input.LeapYearOffset.Val(cal.LeapYearOffset)

	// SetRealTime is nil for every caller that does not manage the flag —
	// see UpdateCalendarInput's doc comment.
	if input.SetRealTime != nil {
		if *input.SetRealTime {
			if input.RealTimeZone == nil || *input.RealTimeZone == "" {
				return apperror.NewValidation("real_time_zone is required to enable real-time tracking")
			}
			if err := apperror.ValidateStringLength("real_time_zone", *input.RealTimeZone, maxRealTimeZoneLength); err != nil {
				return err
			}
			if _, err := time.LoadLocation(*input.RealTimeZone); err != nil {
				return apperror.NewBadRequest("real_time_zone must be a valid IANA time zone name")
			}
			cal.TracksRealTime = true
			cal.RealTimeZone = input.RealTimeZone
		} else {
			cal.TracksRealTime = false
			cal.RealTimeZone = nil
		}
	}

	if err := s.calRepo.Update(ctx, cal); err != nil {
		return fmt.Errorf("update calendar: %w", err)
	}
	return nil
}

// DeleteCalendar removes a calendar and its structural sub-resources
// (cascaded by FK, see CalendarRepository.Delete's doc comment).
func (s *calendarService) DeleteCalendar(ctx context.Context, calendarID, campaignID string) error {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	if err := s.calRepo.Delete(ctx, calendarID); err != nil {
		return fmt.Errorf("delete calendar: %w", err)
	}
	return nil
}

// SetDefaultCalendar marks one calendar as the campaign's default.
// CalendarRepository.SetDefault is itself campaign-scoped in SQL (WHERE
// id = ? AND campaign_id = ?), so this is a thin pass-through.
func (s *calendarService) SetDefaultCalendar(ctx context.Context, campaignID, calendarID string) error {
	if err := s.calRepo.SetDefault(ctx, campaignID, calendarID); err != nil {
		return fmt.Errorf("set default calendar: %w", err)
	}
	return nil
}

// --- Events ---

// CreateEvent creates an event on calendarID. input.CanAuthorDmOnly decides
// whether the request may set visibility=dm_only (there is no stored value
// to compare against on create, so any attempt by a non-author is refused
// outright rather than silently downgraded — see canChangeVisibility's doc
// comment for why a downgrade is the wrong fix). visibility_rules carries no
// such gate — see canChangeVisibility's doc comment on why that field is
// scoped by the ordinary Scribe route gate instead.
func (s *calendarService) CreateEvent(ctx context.Context, calendarID, campaignID string, input CreateEventInput) (*Event, error) {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return nil, err
	}
	if err := apperror.ValidateRequired("name", input.Name); err != nil {
		return nil, err
	}
	if err := apperror.ValidateStringLength("name", input.Name, apperror.MaxNameLength); err != nil {
		return nil, err
	}
	visibility := input.Visibility
	if visibility == "" {
		visibility = "everyone"
	}
	if visibility != "everyone" && visibility != "dm_only" {
		return nil, apperror.NewValidation("visibility must be \"everyone\" or \"dm_only\"")
	}
	if visibility == "dm_only" && !input.CanAuthorDmOnly {
		return nil, apperror.NewForbidden("only the campaign owner or a granted co-DM may create a dm_only event")
	}
	if err := validateVisibilityRulesJSON(input.VisibilityRules); err != nil {
		return nil, err
	}
	if !IsSupportedRecurrenceType(derefString(input.RecurrenceType)) {
		return nil, apperror.NewValidation("unsupported recurrence_type")
	}
	announced := derefString(input.Announced)
	if announced != "" && !IsSupportedAnnounced(announced) {
		return nil, apperror.NewValidation("announced must be \"" + AnnouncedAhead + "\" or \"" + AnnouncedOnDay + "\"")
	}
	if err := validEventPayload(input.Payload); err != nil {
		return nil, err
	}
	if err := validateEventCosmeticLengths(input.Color, input.Icon, input.Tier); err != nil {
		return nil, err
	}
	if err := validateOptionalText("description", input.Description, maxTextColumnBytes); err != nil {
		return nil, err
	}
	if err := validateOptionalText("description_html", input.DescriptionHTML, maxTextColumnBytes); err != nil {
		return nil, err
	}

	descHTML := sanitizeDescriptionHTML(input.DescriptionHTML)

	evt := &Event{
		ID:                       generateID(),
		CalendarID:               calendarID,
		EntityID:                 input.EntityID,
		Name:                     input.Name,
		Description:              input.Description,
		DescriptionHTML:          descHTML,
		Year:                     input.Year,
		Month:                    input.Month,
		Day:                      input.Day,
		StartHour:                input.StartHour,
		StartMinute:              input.StartMinute,
		EndYear:                  input.EndYear,
		EndMonth:                 input.EndMonth,
		EndDay:                   input.EndDay,
		EndHour:                  input.EndHour,
		EndMinute:                input.EndMinute,
		IsRecurring:              input.IsRecurring,
		RecurrenceType:           input.RecurrenceType,
		RecurrenceInterval:       input.RecurrenceInterval,
		RecurrenceEndYear:        input.RecurrenceEndYear,
		RecurrenceEndMonth:       input.RecurrenceEndMonth,
		RecurrenceEndDay:         input.RecurrenceEndDay,
		RecurrenceMaxOccurrences: input.RecurrenceMaxOccurrences,
		Visibility:               visibility,
		VisibilityRules:          input.VisibilityRules,
		KindID:                   input.KindID,
		Announced:                input.Announced,
		Tier:                     input.Tier,
		Color:                    input.Color,
		Icon:                     input.Icon,
		AllDay:                   input.AllDay,
		Payload:                  input.Payload,
		CreatedBy:                &input.CreatedBy,
	}
	if err := s.eventRepo.CreateEvent(ctx, evt); err != nil {
		return nil, fmt.Errorf("create event: %w", err)
	}
	return evt, nil
}

// GetEventForViewer returns an event only if it belongs to calendarID,
// calendarID belongs to campaignID AND is itself visible to v, and the
// event is visible to v. Every failure mode collapses to the same NotFound.
// The linked entity's name/icon/color are blanked (and the id nilled) for
// any viewer not permitted to see that entity separately.
func (s *calendarService) GetEventForViewer(ctx context.Context, eventID, calendarID, campaignID string, v permissions.Viewer) (*Event, error) {
	if _, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v); err != nil {
		return nil, err
	}
	evt, err := s.eventRepo.GetEvent(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("get event: %w", err)
	}
	if evt == nil || evt.CalendarID != calendarID || !eventVisibleToViewer(*evt, v) {
		return nil, apperror.NewNotFound("event not found")
	}
	events := []Event{*evt}
	if err := s.redactHiddenEntityLinks(ctx, campaignID, events, v); err != nil {
		return nil, err
	}
	result := events[0]
	return &result, nil
}

// ListEventsForMonth returns a calendar's events for (year, month),
// role-filtered by SQL (dm_only) and per-user-filtered in Go
// (visibility_rules) on top, then narrowed to the events that actually
// OCCUR in this month: the repository widens in every recurring candidate
// from anywhere in the calendar (recurringCandidateClause) so Event.OccursOn
// can decide exact placement in Go — this is that placement step, so a
// weekly event recurring into a DIFFERENT month never appears here as a
// false positive.
func (s *calendarService) ListEventsForMonth(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) ([]Event, error) {
	cal, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v)
	if err != nil {
		return nil, err
	}
	events, err := s.eventRepo.ListEventsForMonth(ctx, calendarID, year, month, v.Role())
	if err != nil {
		return nil, fmt.Errorf("list events for month: %w", err)
	}
	events = filterEventsByUser(events, v)
	if err := s.loadCalendarGeometry(ctx, cal); err != nil {
		return nil, err
	}
	events = filterRecurringToMonth(events, cal, year, month)
	if err := s.redactHiddenEntityLinks(ctx, campaignID, events, v); err != nil {
		return nil, err
	}
	return events, nil
}

// filterRecurringToMonth drops a recurring candidate that does not actually
// land on any day of (year, month) — see ListEventsForMonth's doc comment.
// Non-recurring (including multi-day spanning) events pass through
// unchanged: OccursOn only answers a single-day placement question for a
// recurring rule, not "does this stored [start,end] window overlap this
// month", which the repository's spanningCandidateClause already answers in
// SQL (see event_repository.go).
func filterRecurringToMonth(events []Event, cal *Calendar, year, month int) []Event {
	filtered := events[:0]
	for _, e := range events {
		if e.IsRecurring && !occursSomewhereInMonth(e, cal, year, month) {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}

// occursSomewhereInMonth reports whether e has at least one occurrence on
// some day of (year, month) per Event.OccursOn.
func occursSomewhereInMonth(e Event, cal *Calendar, year, month int) bool {
	days := cal.MonthDays(month-1, year)
	for day := 1; day <= days; day++ {
		if e.OccursOn(cal, year, month, day) {
			return true
		}
	}
	return false
}

// ListUpcomingEvents is the "what's coming up" preview read — see the
// interface doc comment.
func (s *calendarService) ListUpcomingEvents(ctx context.Context, calendarID, campaignID string, limit int, v permissions.Viewer) ([]Event, error) {
	cal, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v)
	if err != nil {
		return nil, err
	}
	events, err := s.eventRepo.ListUpcomingEvents(ctx, calendarID, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay, v.Role(), limit)
	if err != nil {
		return nil, fmt.Errorf("list upcoming events: %w", err)
	}
	events = filterEventsByUser(events, v)
	if !v.SkipsPerUserRules() {
		if events, err = s.dropUnannouncedFutureEvents(ctx, cal, campaignID, events); err != nil {
			return nil, err
		}
	}
	if err := s.redactHiddenEntityLinks(ctx, campaignID, events, v); err != nil {
		return nil, err
	}
	return events, nil
}

// dropUnannouncedFutureEvents removes events a non-author viewer cannot yet
// know about, mirroring aiexport's RenderCalendarEvents Safe-mode filter
// (ai_workspace/aiexport/renderer.go): a future event (strictly after the
// calendar's current date) whose EffectiveAnnounced resolves to
// AnnouncedOnDay ("only knowable on the day itself") stays secret until that
// day arrives. An AnnouncedAhead event (a yearly festival, or one explicitly
// marked ahead) is common knowledge regardless of date and is never dropped.
// Callers that skip per-user rules (Owner/co-DM/system) never call this —
// content authors already know their own unannounced events.
func (s *calendarService) dropUnannouncedFutureEvents(ctx context.Context, cal *Calendar, campaignID string, events []Event) ([]Event, error) {
	if len(events) == 0 {
		return events, nil
	}
	// AbsoluteDay needs the calendar's month lengths; calendarInCampaignForViewer
	// (this method's caller) returns a bare calendar row with no sub-resources
	// loaded, so they're fetched here rather than assumed present.
	months, err := s.calRepo.GetMonths(ctx, cal.ID)
	if err != nil {
		return nil, fmt.Errorf("load months: %w", err)
	}
	cal.Months = months
	kinds, err := s.kindRepo.List(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list event kinds: %w", err)
	}
	kindByID := make(map[int]*EventKind, len(kinds))
	for i := range kinds {
		kindByID[kinds[i].ID] = &kinds[i]
	}
	currentAbsDay := cal.CurrentAbsoluteDay()

	visible := events[:0]
	for _, e := range events {
		var kind *EventKind
		if e.KindID != nil {
			kind = kindByID[*e.KindID]
		}
		announced := e.EffectiveAnnounced(kind)
		eventAbsDay := cal.AbsoluteDay(e.Year, e.Month, e.Day)
		if announced == AnnouncedOnDay && eventAbsDay > currentAbsDay {
			continue // hasn't happened yet, and this viewer has no advance word
		}
		visible = append(visible, e)
	}
	return visible, nil
}

// eventInCalendar loads an event and confirms it belongs to calendarID
// (which the caller has already confirmed belongs to campaignID), collapsing
// "doesn't exist" and "belongs to a different calendar" into one NotFound.
// It does not check visibility — see eventInCalendarForViewer.
func (s *calendarService) eventInCalendar(ctx context.Context, eventID, calendarID string) (*Event, error) {
	evt, err := s.eventRepo.GetEvent(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("get event: %w", err)
	}
	if evt == nil || evt.CalendarID != calendarID {
		return nil, apperror.NewNotFound("event not found")
	}
	return evt, nil
}

// eventInCalendarForViewer is eventInCalendar plus the visibility check: an
// event the viewer cannot see must answer NotFound on a write just as it
// does on a read — a caller who cannot GET an event must not be able to
// PUT/DELETE it either by knowing (or guessing) its id.
func (s *calendarService) eventInCalendarForViewer(ctx context.Context, eventID, calendarID string, v permissions.Viewer) (*Event, error) {
	evt, err := s.eventInCalendar(ctx, eventID, calendarID)
	if err != nil {
		return nil, err
	}
	if !eventVisibleToViewer(*evt, v) {
		return nil, apperror.NewNotFound("event not found")
	}
	return evt, nil
}

// UpdateEvent applies a partial update to an event (load-merge-write; see
// UpdateEventInput's doc comment). The event (and its calendar) must be
// visible to v or this answers NotFound, closing the blind-write hole a
// caller who cannot GET the event would otherwise have through PUT.
// Visibility/visibility_rules are only ever changed for an author
// (v.SkipsPerUserRules()); anyone else re-sending the stored value is a
// no-op, and any other attempted value is a 403 — never a silent rewrite.
func (s *calendarService) UpdateEvent(ctx context.Context, eventID, calendarID, campaignID string, input UpdateEventInput, v permissions.Viewer) error {
	if _, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v); err != nil {
		return err
	}
	evt, err := s.eventInCalendarForViewer(ctx, eventID, calendarID, v)
	if err != nil {
		return err
	}

	name := input.Name.Val(evt.Name)
	if name == "" {
		return apperror.NewValidation("event name is required")
	}
	if err := apperror.ValidateStringLength("name", name, apperror.MaxNameLength); err != nil {
		return err
	}
	visibility := input.Visibility.Val(evt.Visibility)
	if visibility != "everyone" && visibility != "dm_only" {
		return apperror.NewValidation("visibility must be \"everyone\" or \"dm_only\"")
	}
	if err := canChangeVisibility(v, visibility, evt.Visibility); err != nil {
		return err
	}
	visRules := input.VisibilityRules.Ptr(evt.VisibilityRules)
	if err := validateVisibilityRulesJSON(visRules); err != nil {
		return err
	}
	recurrenceType := input.RecurrenceType.Ptr(evt.RecurrenceType)
	if !IsSupportedRecurrenceType(derefString(recurrenceType)) {
		return apperror.NewValidation("unsupported recurrence_type")
	}
	announced := input.Announced.Ptr(evt.Announced)
	if a := derefString(announced); a != "" && !IsSupportedAnnounced(a) {
		return apperror.NewValidation("announced must be \"" + AnnouncedAhead + "\" or \"" + AnnouncedOnDay + "\"")
	}
	payload := input.Payload.Ptr(evt.Payload)
	if err := validEventPayload(payload); err != nil {
		return err
	}
	color := input.Color.Ptr(evt.Color)
	icon := input.Icon.Ptr(evt.Icon)
	tier := input.Tier.Ptr(evt.Tier)
	if err := validateEventCosmeticLengths(color, icon, tier); err != nil {
		return err
	}
	description := input.Description.Ptr(evt.Description)
	if err := validateOptionalText("description", description, maxTextColumnBytes); err != nil {
		return err
	}

	// An ABSENT description_html preserves the stored (already-sanitized)
	// HTML; a present value (including empty) is re-sanitized on write, the
	// same rule entities/timeline apply to rich text.
	descHTML := evt.DescriptionHTML
	if input.DescriptionHTML.Present() {
		if htmlVal, ok := input.DescriptionHTML.Get(); ok {
			if err := apperror.ValidateStringLength("description_html", htmlVal, maxTextColumnBytes); err != nil {
				return err
			}
		}
		descHTML = sanitizeDescriptionHTMLField(input.DescriptionHTML)
	}

	evt.Name = name
	evt.Description = description
	evt.DescriptionHTML = descHTML
	evt.EntityID = input.EntityID.Ptr(evt.EntityID)
	evt.Year = input.Year.Val(evt.Year)
	evt.Month = input.Month.Val(evt.Month)
	evt.Day = input.Day.Val(evt.Day)
	evt.StartHour = input.StartHour.Ptr(evt.StartHour)
	evt.StartMinute = input.StartMinute.Ptr(evt.StartMinute)
	evt.EndYear = input.EndYear.Ptr(evt.EndYear)
	evt.EndMonth = input.EndMonth.Ptr(evt.EndMonth)
	evt.EndDay = input.EndDay.Ptr(evt.EndDay)
	evt.EndHour = input.EndHour.Ptr(evt.EndHour)
	evt.EndMinute = input.EndMinute.Ptr(evt.EndMinute)
	evt.IsRecurring = input.IsRecurring.Val(evt.IsRecurring)
	evt.RecurrenceType = recurrenceType
	evt.RecurrenceInterval = input.RecurrenceInterval.Ptr(evt.RecurrenceInterval)
	evt.RecurrenceEndYear = input.RecurrenceEndYear.Ptr(evt.RecurrenceEndYear)
	evt.RecurrenceEndMonth = input.RecurrenceEndMonth.Ptr(evt.RecurrenceEndMonth)
	evt.RecurrenceEndDay = input.RecurrenceEndDay.Ptr(evt.RecurrenceEndDay)
	evt.RecurrenceMaxOccurrences = input.RecurrenceMaxOccurrences.Ptr(evt.RecurrenceMaxOccurrences)
	evt.Visibility = visibility
	evt.VisibilityRules = visRules
	evt.KindID = input.KindID.Ptr(evt.KindID)
	evt.Announced = announced
	evt.Tier = tier
	evt.Color = color
	evt.Icon = icon
	evt.AllDay = input.AllDay.Val(evt.AllDay)
	evt.Payload = payload

	if err := s.eventRepo.UpdateEvent(ctx, evt); err != nil {
		return fmt.Errorf("update event: %w", err)
	}
	return nil
}

// DeleteEvent removes an event, scoped to calendarID/campaignID and visible
// to v (see eventInCalendarForViewer's doc comment on why a write checks
// visibility the same way a read does).
func (s *calendarService) DeleteEvent(ctx context.Context, eventID, calendarID, campaignID string, v permissions.Viewer) error {
	if _, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v); err != nil {
		return err
	}
	if _, err := s.eventInCalendarForViewer(ctx, eventID, calendarID, v); err != nil {
		return err
	}
	if err := s.eventRepo.DeleteEvent(ctx, eventID); err != nil {
		return fmt.Errorf("delete event: %w", err)
	}
	return nil
}

// SetEventVisibility is the dm_only toggle's dedicated action endpoint. Its
// route already requires campaigns.CanAuthorDmOnly (v.SkipsPerUserRules()
// here), but that is re-checked rather than trusted blindly, matching every
// other defense-in-depth check in this file.
func (s *calendarService) SetEventVisibility(ctx context.Context, eventID, calendarID, campaignID string, input UpdateEventVisibilityInput, v permissions.Viewer) error {
	if _, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v); err != nil {
		return err
	}
	evt, err := s.eventInCalendarForViewer(ctx, eventID, calendarID, v)
	if err != nil {
		return err
	}
	if input.Visibility != "everyone" && input.Visibility != "dm_only" {
		return apperror.NewValidation("visibility must be \"everyone\" or \"dm_only\"")
	}
	if err := canChangeVisibility(v, input.Visibility, evt.Visibility); err != nil {
		return err
	}
	visRules := input.VisibilityRules.Ptr(evt.VisibilityRules)
	if err := validateVisibilityRulesJSON(visRules); err != nil {
		return err
	}
	if err := s.eventRepo.UpdateEventVisibility(ctx, eventID, input.Visibility, visRules); err != nil {
		return fmt.Errorf("set event visibility: %w", err)
	}
	return nil
}

// --- Event kinds (campaign-scoped, Owner-only end to end) ---

// validateEventKindShape checks the fields the repository's own
// validateEventKindInput does not (Slug/Name shape + length limits, plus
// Icon/Color against a closed character class); the repository still
// re-validates DefaultAnnounced and the slug uniqueness constraint, so this
// is defense in depth, not the only check.
func validateEventKindShape(input EventKindInput) error {
	if err := apperror.ValidateRequired("name", input.Name); err != nil {
		return err
	}
	if err := apperror.ValidateStringLength("name", input.Name, maxEventKindNameLength); err != nil {
		return err
	}
	if err := apperror.ValidateRequired("slug", input.Slug); err != nil {
		return err
	}
	if err := apperror.ValidateStringLength("slug", input.Slug, maxEventKindSlugLength); err != nil {
		return err
	}
	if !slugPattern.MatchString(input.Slug) {
		return apperror.NewValidation("slug must be lowercase letters, digits and hyphens")
	}
	if err := validateIconAndColor(input.Icon, input.Color); err != nil {
		return err
	}
	if input.DefaultAnnounced != "" && !IsSupportedAnnounced(input.DefaultAnnounced) {
		return apperror.NewValidation("default_announced must be \"" + AnnouncedAhead + "\" or \"" + AnnouncedOnDay + "\"")
	}
	return nil
}

// validateIconAndColor checks icon/color length THEN pattern, as two
// independent rules: a value can fail either one without the other (a
// too-long-but-well-formed icon, or a short-but-invalid one).
func validateIconAndColor(icon, color string) error {
	if err := apperror.ValidateStringLength("icon", icon, maxEventKindIconLength); err != nil {
		return err
	}
	if !faIconPattern.MatchString(icon) {
		return apperror.NewBadRequest("icon must be a Font Awesome class name (fa-lowercase-with-hyphens)")
	}
	if err := apperror.ValidateStringLength("color", color, apperror.MaxColorLength); err != nil {
		return err
	}
	if !hexColorPattern.MatchString(color) {
		return apperror.NewBadRequest("color must be a hex color (#rgb or #rrggbb)")
	}
	return nil
}

// maxEventKindIconLength is calendar_event_kinds.icon's own width — matches
// apperror.MaxIconLength today, named separately so the two can diverge
// without a caller silently picking up an unrelated column's limit.
const maxEventKindIconLength = apperror.MaxIconLength

func (s *calendarService) ListEventKinds(ctx context.Context, campaignID string) ([]EventKind, error) {
	kinds, err := s.kindRepo.List(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list event kinds: %w", err)
	}
	return kinds, nil
}

func (s *calendarService) CreateEventKind(ctx context.Context, campaignID string, input EventKindInput) (*EventKind, error) {
	if input.DefaultAnnounced == "" {
		input.DefaultAnnounced = AnnouncedOnDay
	}
	if err := validateEventKindShape(input); err != nil {
		return nil, err
	}
	kind, err := s.kindRepo.Create(ctx, campaignID, input)
	if err != nil {
		return nil, err // apperror.NewConflict on duplicate slug, propagated as-is
	}
	return kind, nil
}

// UpdateEventKind applies a partial update to an event kind (load-merge-
// write, see UpdateEventKindInput's doc comment). GetByID is already
// campaign-scoped (returns nil for a kind belonging to another campaign),
// so this can't reach across tenants by guessing a numeric id.
func (s *calendarService) UpdateEventKind(ctx context.Context, kindID int, campaignID string, input UpdateEventKindInput) error {
	kind, err := s.kindRepo.GetByID(ctx, kindID, campaignID)
	if err != nil {
		return fmt.Errorf("get event kind for update: %w", err)
	}
	if kind == nil {
		return apperror.NewNotFound("event kind not found")
	}

	name := input.Name
	if name == "" {
		return apperror.NewValidation("event kind name is required")
	}
	if err := apperror.ValidateStringLength("name", name, maxEventKindNameLength); err != nil {
		return err
	}
	slug := input.Slug.Val(kind.Slug)
	if err := apperror.ValidateRequired("slug", slug); err != nil {
		return err
	}
	if err := apperror.ValidateStringLength("slug", slug, maxEventKindSlugLength); err != nil {
		return err
	}
	if !slugPattern.MatchString(slug) {
		return apperror.NewValidation("slug must be lowercase letters, digits and hyphens")
	}
	icon := input.Icon.Val(kind.Icon)
	color := input.Color.Val(kind.Color)
	if err := validateIconAndColor(icon, color); err != nil {
		return err
	}
	defaultAnnounced := input.DefaultAnnounced.Val(kind.DefaultAnnounced)
	if defaultAnnounced != "" && !IsSupportedAnnounced(defaultAnnounced) {
		return apperror.NewValidation("default_announced must be \"" + AnnouncedAhead + "\" or \"" + AnnouncedOnDay + "\"")
	}

	merged := EventKindInput{
		Slug:             slug,
		Name:             name,
		Icon:             icon,
		Color:            color,
		SortOrder:        input.SortOrder.Val(kind.SortOrder),
		DefaultAnnounced: defaultAnnounced,
	}
	if err := s.kindRepo.Update(ctx, kindID, campaignID, merged); err != nil {
		return err
	}
	return nil
}

func (s *calendarService) DeleteEventKind(ctx context.Context, kindID int, campaignID string) error {
	return s.kindRepo.Delete(ctx, kindID, campaignID)
}

// --- Eras (per-calendar, Owner-only end to end) ---

func (s *calendarService) CreateEra(ctx context.Context, calendarID, campaignID string, input EraInput) (*Era, error) {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return nil, err
	}
	if err := validateEraShape(input.Name, input.Description, input.Color, input.StartYear, input.StartMonth, input.StartDay, input.EndYear, input.EndMonth, input.EndDay); err != nil {
		return nil, err
	}
	era, err := s.calRepo.CreateEra(ctx, calendarID, input)
	if err != nil {
		return nil, fmt.Errorf("create era: %w", err)
	}
	return era, nil
}

// eraInCalendar loads an era via the UNSCOPED GetEraByID and confirms it
// belongs to calendarID (which the caller has already confirmed belongs to
// campaignID) — GetEraByID's own doc comment warns it must never be called
// with an untrusted id without this check; this is that check.
func (s *calendarService) eraInCalendar(ctx context.Context, eraID int, calendarID string) (*Era, error) {
	era, err := s.calRepo.GetEraByID(ctx, eraID)
	if err != nil {
		return nil, fmt.Errorf("get era: %w", err)
	}
	if era == nil || era.CalendarID != calendarID {
		return nil, apperror.NewNotFound("era not found in calendar")
	}
	return era, nil
}

// UpdateEra applies a partial update to an era (load-merge-write, see
// UpdateEraInput's doc comment).
func (s *calendarService) UpdateEra(ctx context.Context, eraID int, calendarID, campaignID string, input UpdateEraInput) error {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	era, err := s.eraInCalendar(ctx, eraID, calendarID)
	if err != nil {
		return err
	}

	name := input.Name
	if name == "" {
		return apperror.NewValidation("era name is required")
	}
	startYear := input.StartYear.Val(era.StartYear)
	startMonth := input.StartMonth.Val(era.StartMonth)
	startDay := input.StartDay.Val(era.StartDay)
	endYear := input.EndYear.Ptr(era.EndYear)
	endMonth := input.EndMonth.Ptr(era.EndMonth)
	endDay := input.EndDay.Ptr(era.EndDay)
	description := input.Description.Ptr(era.Description)
	color := input.Color.Val(era.Color)
	if err := validateEraShape(name, description, color, startYear, startMonth, startDay, endYear, endMonth, endDay); err != nil {
		return err
	}

	merged := EraInput{
		Name:        name,
		StartYear:   startYear,
		StartMonth:  startMonth,
		StartDay:    startDay,
		EndYear:     endYear,
		EndMonth:    endMonth,
		EndDay:      endDay,
		Description: description,
		Color:       color,
		SortOrder:   input.SortOrder.Val(era.SortOrder),
	}
	// UpdateEra is itself scoped to calendarID (WHERE id = ? AND
	// calendar_id = ?) and returns apperror.NewNotFound on no match.
	if err := s.calRepo.UpdateEra(ctx, calendarID, eraID, merged); err != nil {
		return err
	}
	return nil
}

func (s *calendarService) DeleteEra(ctx context.Context, eraID int, calendarID, campaignID string) error {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	if err := s.calRepo.DeleteEra(ctx, calendarID, eraID); err != nil {
		return err
	}
	return nil
}

// validateEraShape checks the era fields shared by create and (merged)
// update. color is checked against the same closed character class as an
// event kind's — an era's color renders the same way.
func validateEraShape(name string, description *string, color string, startYear, startMonth, startDay int, endYear, endMonth, endDay *int) error {
	if err := apperror.ValidateRequired("name", name); err != nil {
		return err
	}
	if err := apperror.ValidateStringLength("name", name, apperror.MaxNameLength); err != nil {
		return err
	}
	if err := validateOptionalText("description", description, apperror.MaxDescriptionLength); err != nil {
		return err
	}
	if color != "" {
		if err := apperror.ValidateStringLength("color", color, apperror.MaxColorLength); err != nil {
			return err
		}
		if !hexColorPattern.MatchString(color) {
			return apperror.NewBadRequest("color must be a hex color (#rgb or #rrggbb)")
		}
	}
	if endYear != nil {
		em, ed := 1, 1
		if endMonth != nil {
			em = *endMonth
		}
		if endDay != nil {
			ed = *endDay
		}
		if dateLess(*endYear, em, ed, startYear, startMonth, startDay) {
			return apperror.NewValidation("era end date must not be before its start date")
		}
	}
	return nil
}

// --- Moon ---

// SetMoonHidden toggles a moon's visibility to players. Owner-only end to
// end (the route gates it); CalendarRepository.SetMoonHidden is itself
// scoped to calendarID, so a moon belonging to a sibling calendar in the
// SAME campaign is rejected exactly like one from another campaign.
func (s *calendarService) SetMoonHidden(ctx context.Context, moonID int, calendarID, campaignID string, hidden bool) error {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	if err := s.calRepo.SetMoonHidden(ctx, calendarID, moonID, hidden); err != nil {
		return err
	}
	return nil
}

// SetMonths replaces calendarID's month list. See the interface doc comment
// for who calls this today and why it takes no MonthEditImpact preview the
// way V4's did: every current caller is importing into a calendar that has
// no events yet, so there is nothing for a month-position edit to re-date.
func (s *calendarService) SetMonths(ctx context.Context, calendarID, campaignID string, months []MonthInput) error {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	return s.calRepo.SetMonths(ctx, calendarID, months)
}

// SetWeekdays replaces calendarID's weekday list.
func (s *calendarService) SetWeekdays(ctx context.Context, calendarID, campaignID string, weekdays []WeekdayInput) error {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	return s.calRepo.SetWeekdays(ctx, calendarID, weekdays)
}

// SetMoons replaces (upserts by ID, see MoonInput's doc comment)
// calendarID's moon list.
func (s *calendarService) SetMoons(ctx context.Context, calendarID, campaignID string, moons []MoonInput) error {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	return s.calRepo.SetMoons(ctx, calendarID, moons)
}

// SetSeasons replaces calendarID's season list.
func (s *calendarService) SetSeasons(ctx context.Context, calendarID, campaignID string, seasons []Season) error {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	return s.calRepo.SetSeasons(ctx, calendarID, seasons)
}

// ListAllEventsForCalendar is the unfiltered bulk event read for system
// callers — see the interface doc comment for the trust boundary.
func (s *calendarService) ListAllEventsForCalendar(ctx context.Context, calendarID, campaignID string, v permissions.Viewer) ([]Event, error) {
	if !v.IsSystem() {
		return nil, apperror.NewForbidden("ListAllEventsForCalendar is system-only")
	}
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return nil, err
	}
	return s.eventRepo.ListAllEvents(ctx, calendarID)
}

// --- Shared validation helpers ---

// validateEventCosmeticLengths enforces calendar_events' own color/icon/tier
// column widths and, for color/icon, the same closed character class an
// event kind's own values go through. All three are optional overrides of
// the event's kind (nil/empty means "inherit"), so an absent or blank value
// is not checked.
func validateEventCosmeticLengths(color, icon, tier *string) error {
	if color != nil && *color != "" {
		if err := apperror.ValidateStringLength("color", *color, maxEventColorLength); err != nil {
			return err
		}
		if !hexColorPattern.MatchString(*color) {
			return apperror.NewBadRequest("color must be a hex color (#rgb or #rrggbb)")
		}
	}
	if icon != nil && *icon != "" {
		if err := apperror.ValidateStringLength("icon", *icon, maxEventIconLength); err != nil {
			return err
		}
		if !faIconPattern.MatchString(*icon) {
			return apperror.NewBadRequest("icon must be a Font Awesome class name (fa-lowercase-with-hyphens)")
		}
	}
	if tier != nil {
		if err := apperror.ValidateStringLength("tier", *tier, maxEventTierLength); err != nil {
			return err
		}
	}
	return nil
}

// validateOptionalText checks an optional field against maxLen when it is
// present, so a value that doesn't fit its column fails as a clean
// apperror rather than reaching the driver as a raw "data too long" error.
func validateOptionalText(field string, value *string, maxLen int) error {
	if value == nil {
		return nil
	}
	return apperror.ValidateStringLength(field, *value, maxLen)
}

// validateVisibilityRulesJSON rejects a malformed visibility_rules blob
// before it reaches storage, mirroring timeline's validateVisibilityRules.
func validateVisibilityRulesJSON(rulesJSON *string) error {
	if rulesJSON == nil || *rulesJSON == "" {
		return nil
	}
	var rules VisibilityRules
	if err := json.Unmarshal([]byte(*rulesJSON), &rules); err != nil {
		return apperror.NewValidation("visibility_rules must be valid JSON: " + err.Error())
	}
	return nil
}

// derefString returns "" for a nil pointer, the pointee otherwise — used for
// validating an optional *string against a fixed accepted set.
func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// sanitizeDescriptionHTML sanitizes an optional incoming description_html on
// CREATE, mirroring timeline.CreateStandaloneEvent's rich-text handling: nil
// or empty input carries no HTML forward.
func sanitizeDescriptionHTML(html *string) *string {
	if html == nil || *html == "" {
		return nil
	}
	sanitized := sanitize.HTML(*html)
	return &sanitized
}

// sanitizeDescriptionHTMLField sanitizes a PRESENT patch.Field[string] on
// UPDATE: a present-but-empty or explicit-null field clears the stored HTML,
// a present non-empty value is sanitized. Callers must check
// field.Present() themselves first (an absent field must preserve the
// stored HTML untouched, never reach this function).
func sanitizeDescriptionHTMLField(field patch.Field[string]) *string {
	v, ok := field.Get()
	if !ok || v == "" {
		return nil
	}
	sanitized := sanitize.HTML(v)
	return &sanitized
}
