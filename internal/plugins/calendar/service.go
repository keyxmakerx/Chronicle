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
// authorized entirely at the route — listing event kinds Owner only, every
// other write CanAuthorDmOnly (Owner or a granted co-Director) — so they
// carry no viewer parameter at all.
package calendar

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"time"
	"unicode/utf8"

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

// GameNightsAffectedByAnchorMove is satisfied by the sessions plugin and
// injected from internal/app/routes.go (plugins reach each other only
// through service interfaces, never direct repo access) — the same
// cross-plugin seam EntityVisibilityGate above uses. It lets
// PreviewAnchorMove name the sessions/game-nights an anchor move would
// re-date without this plugin importing internal/plugins/sessions directly.
//
// Wired optionally, like EntityVisibilityGate: nil-safe, so a build that
// hasn't wired the sessions side yet still returns a preview — just with no
// affected sessions listed, rather than failing the whole preview.
type GameNightsAffectedByAnchorMove interface {
	// SessionsInWorldDateRange returns up to `limit` sessions/game-nights in
	// campaignID whose in-world date falls within [fromYMD, toYMD]
	// (inclusive, by the OLD anchor mapping), for previewing an anchor
	// move's blast radius. Ordered soonest-first. limit<=0 means no cap.
	SessionsInWorldDateRange(ctx context.Context, campaignID string, fromYear, fromMonth, fromDay, toYear, toMonth, toDay, limit int) ([]AffectedSession, error)
}

// AffectedSession is one session/game-night SessionsInWorldDateRange returns:
// its display name and its in-world date under the OLD anchor mapping.
type AffectedSession struct {
	Name                                     string
	OldWorldYear, OldWorldMonth, OldWorldDay int
}

// maxAnchorMovePreviewAffected caps how many affected sessions
// PreviewAnchorMove reports — a future warning box names a few examples, not
// every session in the campaign.
const maxAnchorMovePreviewAffected = 3

// AnchorMovePreview is PreviewAnchorMove's read-only result: how many days
// later/earlier the proposed anchor move shifts every in-world date, plus up
// to maxAnchorMovePreviewAffected sessions/game-nights it would re-date, so a
// future UI can render the operator's warning box before the real anchor
// write is submitted.
type AnchorMovePreview struct {
	// DeltaDays is positive when the move is later, negative when earlier.
	// Zero for a calendar's first-time anchor set (see PreviewAnchorMove's
	// doc comment) — there is no prior mapping to shift away from.
	DeltaDays int `json:"delta_days"`
	// FirstTimeSet is true when the calendar has no anchor yet: there is
	// nothing to warn about moving, since nothing was ever mapped before.
	// Affected is always empty in this case.
	FirstTimeSet bool                     `json:"first_time_set"`
	Affected     []AffectedSessionPreview `json:"affected,omitempty"`
}

// AffectedSessionPreview is one session's date under the current anchor and
// what it would become under the proposed one.
type AffectedSessionPreview struct {
	Name     string `json:"name"`
	OldYear  int    `json:"old_year"`
	OldMonth int    `json:"old_month"`
	OldDay   int    `json:"old_day"`
	NewYear  int    `json:"new_year"`
	NewMonth int    `json:"new_month"`
	NewDay   int    `json:"new_day"`
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
	// SetCurrentDate moves calendarID's current date and time, for a caller
	// that only knows the date (the Foundry sync). A real-time calendar's date
	// follows the wall clock, so it refuses with a validation error (422); a
	// date or time outside the calendar's own months, days, hours or minutes
	// is a bad request (400), so a caller can tell the two apart.
	SetCurrentDate(ctx context.Context, calendarID, campaignID string, year, month, day, hour, minute int) error
	DeleteCalendar(ctx context.Context, calendarID, campaignID string) error
	SetDefaultCalendar(ctx context.Context, campaignID, calendarID string) error

	// Import / presets (the calendar-creation wizard, #741). PreviewImport
	// and PreviewPreset are pure parses — no DB write — so the wizard's
	// Review step can show what a file/preset contains before anything is
	// created. CreateCalendarFromImport does the actual write: see its own
	// doc comment for the current-date and event-kind-slug rules.
	PreviewImport(ctx context.Context, data []byte) (*ImportResult, error)
	PreviewPreset(ctx context.Context, name string) (*ImportResult, error)
	// PreviewRealWorld is the real-world calendar's fixed structure for the
	// wizard; it is built here, never taken from the browser.
	PreviewRealWorld(ctx context.Context) (*ImportResult, error)
	// TodayInZone is today's date on the wall clock of an IANA zone, for a
	// real-world calendar whose date follows the real one.
	TodayInZone(zone string) (year, month, day int, err error)
	CreateCalendarFromImport(ctx context.Context, campaignID string, ir *ImportResult, opts CreateCalendarFromImportOptions) (*Calendar, error)

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
	// event is visible to v. Like ListEventsForMonth, a future event not yet
	// announced (dropUnannouncedFutureEvents) is not found for a player.
	GetEventForViewer(ctx context.Context, eventID, calendarID, campaignID string, v permissions.Viewer) (*Event, error)
	// ListEventsForMonth applies the same player filtering as
	// ListUpcomingEvents, including dropping future events not yet announced.
	ListEventsForMonth(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) ([]Event, error)
	// ListUpcomingEvents returns up to limit events on or after the
	// calendar's current date, chronological, viewer-filtered exactly like
	// ListEventsForMonth (SQL role filter + per-user visibility_rules +
	// hidden-entity-link redaction). Powers the "what's coming up" dashboard
	// and category-dashboard preview blocks (campaigns.dashCalendarPreview/
	// dashCalendarFull, entities.catCalendarPreview) and the calendar
	// preview's "Coming up" list — none of them are scoped to a single
	// month, unlike ListEventsForMonth. Does not expand
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
	// campaign, see EventKind's doc comment) — calendar structure, not
	// content a Player ever reads directly. Listing is Owner only end to
	// end; creating, editing and deleting are CanAuthorDmOnly (Owner or a
	// granted co-Director), gated at the route (routes.go).
	ListEventKinds(ctx context.Context, campaignID string) ([]EventKind, error)
	CreateEventKind(ctx context.Context, campaignID string, input EventKindInput) (*EventKind, error)
	UpdateEventKind(ctx context.Context, kindID int, campaignID string, input UpdateEventKindInput) error
	DeleteEventKind(ctx context.Context, kindID int, campaignID string) error

	// Eras. Calendar structure, gated CanAuthorDmOnly (Owner or a granted
	// co-Director) at the route (routes.go), resolved through the calendar
	// the caller reached (GetEraByID itself is not calendar-scoped; this
	// service never calls it with an untrusted id without also checking the
	// result's CalendarID — see eraInCalendar).
	CreateEra(ctx context.Context, calendarID, campaignID string, input EraInput) (*Era, error)
	UpdateEra(ctx context.Context, eraID int, calendarID, campaignID string, input UpdateEraInput) error
	DeleteEra(ctx context.Context, eraID int, calendarID, campaignID string) error

	// Moon. Calendar structure; the hidden flag is gated CanAuthorDmOnly
	// (Owner or a granted co-Director) at the route (routes.go).
	SetMoonHidden(ctx context.Context, moonID int, calendarID, campaignID string, hidden bool) error

	// PreviewAnchorMove is a READ-ONLY preview of moving a real-anchored
	// calendar's anchor: it computes the day shift and up to three affected
	// sessions but writes nothing. Owner-only (see routes.go); the real
	// anchor write this previews is a separate, not-yet-built endpoint.
	PreviewAnchorMove(ctx context.Context, calendarID, campaignID string, newAnchorYear, newAnchorMonth, newAnchorDay int, newRealDate time.Time) (*AnchorMovePreview, error)

	// --- Bulk structure writers ---
	//
	// SetMonths/SetWeekdays/SetMoons/SetSeasons replace a calendar's whole
	// list for that sub-resource in one call. They exist for the campaign
	// import path (calendarImportAdapter, internal/app/export_adapters.go)
	// and the calendar plugin's own native/Simple-Calendar/Calendaria import
	// (internal/plugins/calendar/import.go), neither of which has a stored
	// row per item to update incrementally the way CreateEra/CreateEventKind
	// do. Not wired to any HTTP route yet — calendar-settings editing UI is
	// a later slice — so today's only callers are trusted in-process ones; a
	// caller reaching these through a future HTTP handler must still be
	// gated there. Whether that gate is Owner-only or CanAuthorDmOnly (like
	// eras/event kinds/the moon hidden flag above) is this future slice's
	// own decision, not assumed here.
	SetMonths(ctx context.Context, calendarID, campaignID string, months []MonthInput) error
	SetWeekdays(ctx context.Context, calendarID, campaignID string, weekdays []WeekdayInput) error
	SetMoons(ctx context.Context, calendarID, campaignID string, moons []MoonInput) error
	SetSeasons(ctx context.Context, calendarID, campaignID string, seasons []Season) error
	// SetCycles/SetFestivals/SetWeather are the same bulk-replace shape as
	// the four above, giving the campaign-backup importer a service-level
	// caller for the repository side (CalendarRepository.SetCycles/
	// SetFestivals, WeatherRepository.Set) from outside this package.
	SetCycles(ctx context.Context, calendarID, campaignID string, cycles []CycleInput) error
	SetFestivals(ctx context.Context, calendarID, campaignID string, festivals []FestivalInput) error
	SetWeather(ctx context.Context, calendarID, campaignID string, input WeatherInput) error

	// Day weather: one reading per calendar day. ListDayWeather gives a
	// Player only days up to the calendar's today (future weather is the
	// Director's alone); an Owner or co-Director sees every day. The writes
	// are gated CanAuthorDmOnly at the route (routes.go).
	ListDayWeather(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) ([]DayWeather, error)
	SetDayWeather(ctx context.Context, calendarID, campaignID string, days []DayWeatherInput) error
	ClearDayWeather(ctx context.Context, calendarID, campaignID string, dates []DayDate) error

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

	// SearchCalendarEvents implements entities.CalendarSearcher (wired from
	// internal/app/routes.go): campaign-wide calendar-event search results
	// for the global quick-search popup. Role-only picker convention,
	// matching CalendarEventLister's shape in
	// internal/plugins/timeline/service.go: the interface carries no
	// per-request user id, so a calendar's own visibility_rules allow/deny
	// list is not evaluated — only role-level dm_only gating, at both the
	// calendar (ListCalendars already filters those out) and each event
	// (EventRepository.SearchEvents' own SQL role filter).
	SearchCalendarEvents(ctx context.Context, campaignID, query string, role int) ([]map[string]string, error)

	// ListEventsForCalendar and ListErasForCalendar back timeline's calendar
	// selector, event picker and era bands (internal/app/routes.go's
	// calendarEventListerAdapter/calendarEraListerAdapter). Both are
	// campaign-scoped: a calendar id from a different campaign, or one the
	// role cannot see, answers with an empty slice, never an error.
	// ListEventsForCalendar filters dm_only events like every other
	// role-only read here; ListErasForCalendar additionally gates on role
	// alone, Owner/co-DM only, because eras are calendar structure
	// (GetCalendarForViewer strips them the same way).
	ListEventsForCalendar(ctx context.Context, campaignID, calendarID string, role int) ([]Event, error)
	ListErasForCalendar(ctx context.Context, campaignID, calendarID string, role int) ([]Era, error)
}

// calendarService is the concrete CalendarService.
type calendarService struct {
	calRepo     CalendarRepository
	eventRepo   EventRepository
	kindRepo    EventKindRepository
	weatherRepo WeatherRepository
	entityGate  EntityVisibilityGate
	gameNights  GameNightsAffectedByAnchorMove
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

// SetGameNightsAffectedByAnchorMove injects the sessions-plugin lookup
// PreviewAnchorMove uses to name affected game nights. Same optional,
// nil-safe wiring pattern as SetEntityVisibilityGate above: unset, the
// preview still works, just with an empty Affected list.
//
// TODO(#806): has no caller — needs a sessions-plugin adapter wired from
// internal/app/routes.go (mirroring the SetEntityVisibilityGate call beside
// it) backed by a real SessionsInWorldDateRange query, which the sessions
// plugin doesn't have yet either; until both exist, PreviewAnchorMove always
// reports zero affected sessions, so this must land before the anchor-move
// WRITE endpoint ships.
func (s *calendarService) SetGameNightsAffectedByAnchorMove(g GameNightsAffectedByAnchorMove) {
	s.gameNights = g
}

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
	// Captured before Hemisphere is merged below, so the hemisphere-seeding
	// check afterward can tell "first choice / a changed choice" from
	// "resending the value already stored" (a no-op re-save must not reseed).
	previousHemisphere := cal.Hemisphere

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
	hemisphere := input.Hemisphere.Ptr(cal.Hemisphere)
	if hemisphere != nil && *hemisphere != HemisphereNorth && *hemisphere != HemisphereSouth {
		return apperror.NewValidation("hemisphere must be \"" + HemisphereNorth + "\" or \"" + HemisphereSouth + "\"")
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
	cal.Hemisphere = hemisphere
	cal.ForecastsEnabled = input.ForecastsEnabled.Val(cal.ForecastsEnabled)
	cal.MonthStartsNewWeek = input.MonthStartsNewWeek.Val(cal.MonthStartsNewWeek)

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

	// Auto-seed the four default seasons on a real-life calendar's first (or
	// changed) hemisphere choice — the operator's signed answer to "what a
	// new real-world calendar starts with" (see defaultRealLifeSeasons).
	// Gated on the hemisphere actually changing, so resending the value
	// already stored (e.g. a plain settings re-save) never reseeds.
	if input.Hemisphere.Present() && hemisphere != nil && cal.Mode == ModeRealLife &&
		(previousHemisphere == nil || *previousHemisphere != *hemisphere) {
		if err := s.seedHemisphereSeasonsIfEmpty(ctx, cal.ID, *hemisphere); err != nil {
			return err
		}
	}
	return nil
}

// SetCurrentDate validates the date against the calendar's own geometry and
// writes it through UpdateCalendar, so the write path stays the one the
// settings page uses. See the interface doc comment for the error split.
func (s *calendarService) SetCurrentDate(ctx context.Context, calendarID, campaignID string, year, month, day, hour, minute int) error {
	cal, err := s.calendarInCampaign(ctx, calendarID, campaignID)
	if err != nil {
		return err
	}
	if cal.UsesRealTime() {
		return apperror.NewValidation("this calendar tracks real-world time; its date cannot be set by hand")
	}
	if err := s.loadCalendarGeometry(ctx, cal); err != nil {
		return err
	}
	if month < 1 || month > len(cal.Months) {
		return apperror.NewBadRequest(fmt.Sprintf("month must be between 1 and %d", len(cal.Months)))
	}
	if days := cal.MonthDays(month-1, year); day < 1 || day > days {
		return apperror.NewBadRequest(fmt.Sprintf("day must be between 1 and %d for that month", days))
	}
	if hour < 0 || hour >= cal.HoursPerDay || minute < 0 || minute >= cal.MinutesPerHour {
		return apperror.NewBadRequest("hour or minute is outside this calendar's day")
	}
	return s.UpdateCalendar(ctx, calendarID, campaignID, UpdateCalendarInput{
		Name:          cal.Name,
		CurrentYear:   patch.Of(year),
		CurrentMonth:  patch.Of(month),
		CurrentDay:    patch.Of(day),
		CurrentHour:   patch.Of(hour),
		CurrentMinute: patch.Of(minute),
	})
}

// seedHemisphereSeasonsIfEmpty seeds the four default real-life seasons for
// hemisphere onto calendarID, but ONLY when it currently has none — a
// calendar with any authored or previously-seeded season is never
// overwritten. Re-labeling seasons on a later hemisphere flip for a calendar
// that already has some is a deliberate scope limit, not built here.
func (s *calendarService) seedHemisphereSeasonsIfEmpty(ctx context.Context, calendarID, hemisphere string) error {
	existing, err := s.calRepo.GetSeasons(ctx, calendarID)
	if err != nil {
		return fmt.Errorf("check existing seasons before hemisphere seeding: %w", err)
	}
	if len(existing) > 0 {
		return nil
	}
	if err := s.calRepo.SetSeasons(ctx, calendarID, defaultRealLifeSeasons(hemisphere)); err != nil {
		return fmt.Errorf("seed hemisphere seasons: %w", err)
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

// --- Import / presets (the calendar-creation wizard, #741) ---

// PreviewImport parses raw uploaded calendar data through the shared,
// auto-detecting importer without writing anything, so the wizard's
// Import/Review step can show what a file contains before the owner
// confirms creating a calendar from it. A parse failure is reported as a
// BadRequest (the file's problem, not the server's).
func (s *calendarService) PreviewImport(ctx context.Context, data []byte) (*ImportResult, error) {
	ir, err := DetectAndParse(data)
	if err != nil {
		return nil, apperror.NewBadRequest(err.Error())
	}
	return ir, nil
}

// PreviewPreset is PreviewImport's sibling for a shipped preset by name. A
// preset IS an export (presets.go's own doc comment) — LoadPreset goes
// through the exact same DetectAndParse an upload does — so this returns
// the identical *ImportResult shape and a caller cannot (and need not) tell
// the two apart.
func (s *calendarService) PreviewPreset(ctx context.Context, name string) (*ImportResult, error) {
	ir, err := LoadPreset(name)
	if err != nil {
		return nil, apperror.NewNotFound("unknown preset")
	}
	return ir, nil
}

// PreviewRealWorld returns the Gregorian calendar the wizard's real-world
// path creates. It is PreviewPreset's sibling for a calendar with no preset
// file: the same *ImportResult shape, built fresh on every call.
func (s *calendarService) PreviewRealWorld(_ context.Context) (*ImportResult, error) {
	ir, err := GregorianImportResult()
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return ir, nil
}

// TodayInZone returns today's date in zone. The instant is taken first and
// then converted, so the date is the zone's own, not the server's.
func (s *calendarService) TodayInZone(zone string) (int, int, int, error) {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return 0, 0, 0, apperror.NewValidation("unknown time zone")
	}
	y, m, d := time.Now().In(loc).Date()
	return y, int(m), d, nil
}

// CreateCalendarFromImport creates a new calendar from a previously-parsed
// ImportResult (an uploaded file or a shipped preset — see PreviewImport/
// PreviewPreset). It creates the bare calendar row through the same
// validation CreateCalendar applies, writes the import's full structure
// (months/weekdays/moons/seasons/eras) in one transaction via
// CalendarRepository.ApplyImport, then recreates any events the import
// carried (#779) via applyImportedEvents.
//
// The created calendar's current date is either what the import file
// itself specified (ir.Today) or an explicit override in opts — never a
// silent, unexplained default (#741): when the import left Month or Day
// unspecified and opts doesn't supply it either, this returns a validation
// error instead of guessing day 1.
//
// ir is mutated in place: applyImportedEvents appends to ir.Warnings for
// anything discovered only at apply time (an event's kind slug not
// existing in this campaign), so the caller should read ir.Warnings AFTER
// this returns for the complete set — parse-time and apply-time combined —
// not just what PreviewImport/PreviewPreset returned earlier.
func (s *calendarService) CreateCalendarFromImport(ctx context.Context, campaignID string, ir *ImportResult, opts CreateCalendarFromImportOptions) (*Calendar, error) {
	if ir == nil {
		return nil, apperror.NewValidation("import result is required")
	}

	name := ir.CalendarName
	if name == "" {
		name = "Imported Calendar"
	}

	year := ir.Today.Year
	if opts.CurrentYear != nil {
		year = *opts.CurrentYear
	}
	month := ir.Today.Month
	if opts.CurrentMonth != nil {
		month = opts.CurrentMonth
	}
	if month == nil {
		return nil, apperror.NewValidation("current_month is required: this import did not specify a current month; confirm one")
	}
	day := ir.Today.Day
	if opts.CurrentDay != nil {
		day = opts.CurrentDay
	}
	if day == nil {
		return nil, apperror.NewValidation("current_day is required: this import did not specify a current day; confirm one")
	}
	// The month/day above came from either the import file or an explicit
	// caller override — either way, nothing upstream has checked it against
	// THIS calendar's own month structure yet. Without this, a bad value
	// (an out-of-range month, or a day beyond that month's length) would
	// land in current_month/current_day unchecked, and every date
	// computation downstream (MonthDays, DayOfYear, the preview grid) reads
	// those columns as already-valid.
	if err := validateImportCurrentDate(ir, year, *month, *day); err != nil {
		return nil, err
	}

	cal, err := s.CreateCalendar(ctx, campaignID, CreateCalendarInput{
		Mode:             ir.Settings.Mode,
		Name:             name,
		EpochName:        ir.Settings.EpochName,
		CurrentYear:      year,
		HoursPerDay:      ir.Settings.HoursPerDay,
		MinutesPerHour:   ir.Settings.MinutesPerHour,
		SecondsPerMinute: ir.Settings.SecondsPerMinute,
		LeapYearEvery:    ir.Settings.LeapYearEvery,
		LeapYearOffset:   ir.Settings.LeapYearOffset,
	})
	if err != nil {
		return nil, err
	}

	// From here on cal.ID is a real, committed row. Every step below can
	// still fail (a bad real_time_zone, a malformed payload on an imported
	// event, ApplyImport's own transaction), and none of them roll the bare
	// calendar back on their own — so a failure here used to leave an
	// orphaned, structure-less calendar behind, and a retried Create would
	// pile up another one alongside it. cleanupOnFailure deletes that row
	// (cascading its as-yet-empty sub-resources) before propagating the
	// original error, so the caller sees exactly one failed attempt and no
	// residue. The delete is best-effort: if IT fails too, that's logged,
	// not swallowed, but the original cause is still what the caller gets.
	cleanupOnFailure := func(cause error) error {
		if delErr := s.calRepo.Delete(ctx, cal.ID); delErr != nil {
			slog.Error("calendar: failed to clean up partially-created calendar after import failure",
				"calendar_id", cal.ID, "campaign_id", campaignID, "cause", cause, "cleanup_error", delErr)
		}
		return cause
	}

	cal.CurrentMonth = *month
	cal.CurrentDay = *day
	// Chronicle-only settings CreateCalendarInput has no field for: carried
	// onto the row here (rather than lost) so re-importing a Chronicle
	// export restores them — the same fields UpdateCalendar lets an owner
	// manage after the fact.
	cal.Hemisphere = ir.Settings.Hemisphere
	cal.ForecastsEnabled = ir.Settings.ForecastsEnabled
	cal.MonthStartsNewWeek = ir.Settings.MonthStartsNewWeek
	if ir.Settings.TracksRealTime {
		if ir.Settings.RealTimeZone == nil || *ir.Settings.RealTimeZone == "" {
			return nil, cleanupOnFailure(apperror.NewValidation("real_time_zone is required when tracks_real_time is set"))
		}
		if _, err := time.LoadLocation(*ir.Settings.RealTimeZone); err != nil {
			return nil, cleanupOnFailure(apperror.NewBadRequest("real_time_zone must be a valid IANA time zone name"))
		}
		cal.TracksRealTime = true
		cal.RealTimeZone = ir.Settings.RealTimeZone
	}

	if err := s.calRepo.ApplyImport(ctx, cal, ir); err != nil {
		return nil, cleanupOnFailure(fmt.Errorf("apply import: %w", err))
	}

	if err := s.applyImportedEvents(ctx, cal.ID, campaignID, ir); err != nil {
		return nil, cleanupOnFailure(fmt.Errorf("apply imported events: %w", err))
	}

	// A failure past this point is a read of data already fully and validly
	// committed (structure + events both succeeded above) — not a partial
	// write — so it is NOT run through cleanupOnFailure: the calendar is
	// genuinely complete, and deleting a good calendar because reading it
	// back once failed would be the wrong tradeoff.
	if err := s.loadSubresources(ctx, cal); err != nil {
		return nil, err
	}
	return cal, nil
}

// applyImportedEvents recreates ir.Events (Chronicle-only, #779) on the
// freshly-created calendarID. An event's Kind slug is resolved against
// THIS campaign's event kinds (never the source campaign's numeric ids,
// which mean nothing here — see ExportEvent.Kind's doc comment); a slug
// that doesn't resolve gets the event created WITHOUT a kind rather than
// dropped or failing the whole import (#741: warn, never refuse), and
// ir.Warnings records which ones so the caller can tell the owner.
//
// Every recreated event is forced to visibility="dm_only", ignoring
// ee.Visibility entirely — fail closed, not a round-trip of the source's
// own value. ExportEvent.Visibility DOES carry the source event's base
// visibility, but VisibilityRules (a per-event allow-list, e.g. "only
// these three players") is deliberately NOT part of the export format
// (export/import stays out of scope for that — the fix here is sized to
// the risk, not to adding visibility_rules portability): an event that was
// "everyone" at the base level but restricted to a handful of players via
// visibility_rules would export as plain "everyone" with the allow-list
// silently dropped, and reimporting it verbatim would hand it to every
// player. Since there's no way to tell, from the export alone, whether a
// given event's true audience was narrower than its base Visibility says,
// every imported event defaults to the most restrictive setting instead —
// the owner can loosen individual events afterward once they've reviewed
// them. One summary warning covers this for the whole import, not one per
// event.
//
// Building and inserting goes through buildValidatedEvent — the same
// validation and description_html sanitization CreateEvent applies to a
// hand-created event — so an imported description_html can't carry
// unsanitized markup into storage, and the same field-length/format checks
// apply. An event that FAILS validation is skipped with its own warning
// (#741: warn, don't abort); a genuine DB error (a malformed payload, a
// color that still doesn't fit after normalization) aborts the whole
// import instead, since that signals something CreateCalendarFromImport's
// caller should see rather than silently limp past.
func (s *calendarService) applyImportedEvents(ctx context.Context, calendarID, campaignID string, ir *ImportResult) error {
	if len(ir.Events) == 0 {
		return nil
	}
	kinds, err := s.kindRepo.List(ctx, campaignID)
	if err != nil {
		return fmt.Errorf("list event kinds: %w", err)
	}
	slugToID := make(map[string]int, len(kinds))
	for _, k := range kinds {
		slugToID[k.Slug] = k.ID
	}

	created := 0
	for _, ee := range ir.Events {
		var kindID *int
		if ee.Kind != nil && *ee.Kind != "" {
			if id, ok := slugToID[*ee.Kind]; ok {
				kindID = &id
			} else {
				ir.Warnings = append(ir.Warnings, fmt.Sprintf(
					"event %q referenced kind %q, which does not exist in this campaign; imported without a kind",
					ee.Name, *ee.Kind))
			}
		}

		// normalizeColor: the same defensive clamp import.go applies to
		// month/season/moon/era colors, so a color value that doesn't
		// survive the round-trip fails validation cleanly below rather than
		// crashing the DB write under strict SQL mode. An absent or blank
		// color is left alone (nil/"" means "inherit from kind").
		color := ee.Color
		if color != nil && *color != "" {
			normalized := normalizeColor(*color)
			color = &normalized
		}

		input := CreateEventInput{
			Name:                     ee.Name,
			Description:              ee.Description,
			DescriptionHTML:          ee.DescriptionHTML,
			Year:                     ee.Year,
			Month:                    ee.Month,
			Day:                      ee.Day,
			StartHour:                ee.StartHour,
			StartMinute:              ee.StartMinute,
			EndYear:                  ee.EndYear,
			EndMonth:                 ee.EndMonth,
			EndDay:                   ee.EndDay,
			EndHour:                  ee.EndHour,
			EndMinute:                ee.EndMinute,
			IsRecurring:              ee.IsRecurring,
			RecurrenceType:           ee.RecurrenceType,
			RecurrenceInterval:       ee.RecurrenceInterval,
			RecurrenceEndYear:        ee.RecurrenceEndYear,
			RecurrenceEndMonth:       ee.RecurrenceEndMonth,
			RecurrenceEndDay:         ee.RecurrenceEndDay,
			RecurrenceMaxOccurrences: ee.RecurrenceMaxOccurrences,
			Visibility:               "dm_only", // fail closed — see doc comment above
			CanAuthorDmOnly:          true,      // the fixed import default, not a per-event author escalation
			KindID:                   kindID,
			Announced:                ee.Announced,
			Color:                    color,
			Icon:                     ee.Icon,
			AllDay:                   ee.AllDay,
			Payload:                  ee.Payload,
		}

		evt, err := buildValidatedEvent(calendarID, input)
		if err != nil {
			ir.Warnings = append(ir.Warnings, fmt.Sprintf(
				"event %q failed validation and was skipped: %v", ee.Name, err))
			continue
		}
		if err := s.eventRepo.CreateEvent(ctx, evt); err != nil {
			return fmt.Errorf("create imported event %q: %w", ee.Name, err)
		}
		created++
	}

	if created > 0 {
		ir.Warnings = append(ir.Warnings,
			"Imported events default to Director-only visibility; share individually if players should see them.")
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
	evt, err := buildValidatedEvent(calendarID, input)
	if err != nil {
		return nil, err
	}
	if err := s.eventRepo.CreateEvent(ctx, evt); err != nil {
		return nil, fmt.Errorf("create event: %w", err)
	}
	return evt, nil
}

// buildValidatedEvent runs every check CreateEvent applies to a new event
// (name, visibility, recurrence type, announced, payload, color/icon/tier
// lengths, description sizes) and sanitizes description_html, returning a
// ready-to-insert *Event — or the same validation error CreateEvent would
// return. It touches no DB.
//
// This is the ONE path both CreateEvent and applyImportedEvents (recreating
// events from an import, #779) go through, so an imported event gets
// exactly the same sanitization and validation a hand-created one does —
// see applyImportedEvents' doc comment for why that matters (an
// unsanitized description_html from an untrusted upload is a stored-XSS
// risk, not just a data-quality one).
func buildValidatedEvent(calendarID string, input CreateEventInput) (*Event, error) {
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

	// CreatedBy is nil, not a pointer to "", for a caller that has no
	// Chronicle user to attribute (applyImportedEvents: an imported event
	// wasn't authored by anyone in this campaign) — the handler path always
	// passes a real user id from auth.GetUserID.
	var createdBy *string
	if input.CreatedBy != "" {
		v := input.CreatedBy
		createdBy = &v
	}

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
		CreatedBy:                createdBy,
	}
	return evt, nil
}

// GetEventForViewer returns an event only if it belongs to calendarID,
// calendarID belongs to campaignID AND is itself visible to v, and the
// event is visible to v. Every failure mode collapses to the same NotFound.
// The linked entity's name/icon/color are blanked (and the id nilled) for
// any viewer not permitted to see that entity separately.
func (s *calendarService) GetEventForViewer(ctx context.Context, eventID, calendarID, campaignID string, v permissions.Viewer) (*Event, error) {
	cal, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v)
	if err != nil {
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
	if !v.SkipsPerUserRules() {
		if events, err = s.dropUnannouncedFutureEvents(ctx, cal, campaignID, events); err != nil {
			return nil, err
		}
		if len(events) == 0 {
			return nil, apperror.NewNotFound("event not found")
		}
	}
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

// upcomingEventsOverfetchFactor/upcomingEventsMaxFetch: see
// ListUpcomingEvents for why it over-fetches before filtering rather than
// trusting the repository's own LIMIT.
const (
	upcomingEventsOverfetchFactor = 5
	upcomingEventsMaxFetch        = 200
)

// ListUpcomingEvents is the "what's coming up" preview read — see the
// interface doc comment.
//
// The repository's SQL LIMIT applies only the role-level (dm_only) filter;
// the per-user visibility_rules and the unannounced-future-event rule run
// afterward, in Go. Passing limit straight through would truncate before
// those filters, so a viewer could see "nothing upcoming" while visible
// events exist further down. It fetches a wider page, filters, then trims.
func (s *calendarService) ListUpcomingEvents(ctx context.Context, calendarID, campaignID string, limit int, v permissions.Viewer) ([]Event, error) {
	cal, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v)
	if err != nil {
		return nil, err
	}
	fetchLimit := limit
	if limit > 0 {
		fetchLimit = limit * upcomingEventsOverfetchFactor
		if fetchLimit <= 0 || fetchLimit > upcomingEventsMaxFetch {
			fetchLimit = upcomingEventsMaxFetch
		}
	}
	events, err := s.eventRepo.ListUpcomingEvents(ctx, calendarID, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay, v.Role(), fetchLimit)
	if err != nil {
		return nil, fmt.Errorf("list upcoming events: %w", err)
	}
	events = filterEventsByUser(events, v)
	if !v.SkipsPerUserRules() {
		if events, err = s.dropUnannouncedFutureEvents(ctx, cal, campaignID, events); err != nil {
			return nil, err
		}
	}
	if limit > 0 && len(events) > limit {
		events = events[:limit]
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

// --- Event kinds (campaign-scoped; list Owner only, writes CanAuthorDmOnly) ---

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

// --- Eras (per-calendar, CanAuthorDmOnly — Owner or a granted co-Director) ---

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

// SetMoonHidden toggles a moon's visibility to players. CanAuthorDmOnly —
// Owner or a granted co-Director — end to end (the route gates it);
// CalendarRepository.SetMoonHidden is itself scoped to calendarID, so a
// moon belonging to a sibling calendar in the SAME campaign is rejected
// exactly like one from another campaign.
func (s *calendarService) SetMoonHidden(ctx context.Context, moonID int, calendarID, campaignID string, hidden bool) error {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	if err := s.calRepo.SetMoonHidden(ctx, calendarID, moonID, hidden); err != nil {
		return err
	}
	return nil
}

// --- Real-date anchor (preview only — the real anchor WRITE this previews
// is a separate, not-yet-built endpoint) ---

// anchorPreviewWindowYears bounds how far into a calendar's future
// PreviewAnchorMove asks the sessions plugin about: enough to surface "the
// next several sessions" as examples without querying the calendar's entire
// unbounded future.
const anchorPreviewWindowYears = 5

// daysBetween returns the whole-day difference b-a for two DATE-granular
// (no time-of-day) real dates, normalizing away any time-of-day or zone on
// the inputs first — anchor_real_date is a DATE column, so there is never a
// genuine partial-day component to round, but a *time.Time value can still
// carry a non-UTC Location whose wall-clock day would otherwise disagree
// with a plain Sub.
func daysBetween(a, b time.Time) int {
	da := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	db := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	return int(db.Sub(da).Hours() / 24)
}

// anchorPreviewWindowEnd is the far end of the world-date range
// PreviewAnchorMove queries: the calendar's current in-world date out
// anchorPreviewWindowYears years.
func anchorPreviewWindowEnd(cal *Calendar) (year, month, day int) {
	year = cal.CurrentYear + anchorPreviewWindowYears
	month = len(cal.Months)
	if month == 0 {
		return year, 12, 31
	}
	day = cal.MonthDays(month-1, year)
	if day == 0 {
		day = 28
	}
	return year, month, day
}

// PreviewAnchorMove computes, WITHOUT WRITING ANYTHING, what moving
// calendarID's real-date anchor to (newAnchorYear, newAnchorMonth,
// newAnchorDay) <-> newRealDate would do: the day shift every session
// scheduled at a fixed in-world date would see in the REAL (Gregorian) date
// it now lands on, plus up to maxAnchorMovePreviewAffected named examples —
// see AnchorMovePreview's doc comment for the exact shape a future warning
// UI renders from this.
//
// A calendar with no anchor yet needs no warning: nothing was ever mapped,
// so there is nothing to move away from. That case returns
// AnchorMovePreview{FirstTimeSet: true} rather than a delta.
//
// The day-shift math: for a calendar day D, its mapped real date is
// AnchorRealDate + (AbsoluteDay(D) - AbsoluteDay(anchor)) days. Holding a
// session's own in-world date fixed and solving for how its real date
// changes between the old and new anchor collapses to one constant —
// DeltaDays — independent of which in-world day the session falls on: move
// both ends of the anchor by the same amount and nothing downstream moves
// at all, which is exactly what falls out when realDeltaDays equals the
// world-date shift.
func (s *calendarService) PreviewAnchorMove(ctx context.Context, calendarID, campaignID string, newAnchorYear, newAnchorMonth, newAnchorDay int, newRealDate time.Time) (*AnchorMovePreview, error) {
	cal, err := s.calendarInCampaign(ctx, calendarID, campaignID)
	if err != nil {
		return nil, err
	}
	if !cal.HasRealAnchor() {
		return &AnchorMovePreview{FirstTimeSet: true}, nil
	}
	if err := s.loadCalendarGeometry(ctx, cal); err != nil {
		return nil, err
	}

	oldWorldAbs := cal.AbsoluteDay(*cal.AnchorYear, *cal.AnchorMonth, *cal.AnchorDay)
	newWorldAbs := cal.AbsoluteDay(newAnchorYear, newAnchorMonth, newAnchorDay)
	realDeltaDays := daysBetween(*cal.AnchorRealDate, newRealDate)
	deltaDays := realDeltaDays - (newWorldAbs - oldWorldAbs)

	preview := &AnchorMovePreview{DeltaDays: deltaDays}
	if s.gameNights == nil || deltaDays == 0 {
		// No configured lookup, or a no-op move (both ends shifted by the
		// same amount): either way there is nothing to list.
		return preview, nil
	}

	fromYear, fromMonth, fromDay := cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay
	toYear, toMonth, toDay := anchorPreviewWindowEnd(cal)
	sessions, err := s.gameNights.SessionsInWorldDateRange(ctx, campaignID,
		fromYear, fromMonth, fromDay, toYear, toMonth, toDay, maxAnchorMovePreviewAffected)
	if err != nil {
		return nil, fmt.Errorf("list sessions affected by anchor move: %w", err)
	}

	anchorReal := time.Date(cal.AnchorRealDate.Year(), cal.AnchorRealDate.Month(), cal.AnchorRealDate.Day(), 0, 0, 0, 0, time.UTC)
	for _, sess := range sessions {
		// The session's own in-world date never moves — what moves is the
		// real date it maps to. oldReal is that mapping under the anchor as
		// it stands today; newReal is the same session shifted by the one
		// constant every session shifts by (see the method doc comment).
		oldReal := anchorReal.AddDate(0, 0, cal.AbsoluteDay(sess.OldWorldYear, sess.OldWorldMonth, sess.OldWorldDay)-oldWorldAbs)
		newReal := oldReal.AddDate(0, 0, deltaDays)
		preview.Affected = append(preview.Affected, AffectedSessionPreview{
			Name:     sess.Name,
			OldYear:  oldReal.Year(),
			OldMonth: int(oldReal.Month()),
			OldDay:   oldReal.Day(),
			NewYear:  newReal.Year(),
			NewMonth: int(newReal.Month()),
			NewDay:   newReal.Day(),
		})
	}
	return preview, nil
}

// SetMonths replaces calendarID's month list. See the interface doc comment
// for who calls this today and why it takes no MonthEditImpact preview the
// way V4's did: every current caller is importing into a calendar that has
// no events yet, so there is nothing for a month-position edit to re-date.
func (s *calendarService) SetMonths(ctx context.Context, calendarID, campaignID string, months []MonthInput) error {
	if err := validateMonthInputs(months); err != nil {
		return err
	}
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	return s.calRepo.SetMonths(ctx, calendarID, months)
}

// SetWeekdays replaces calendarID's weekday list.
func (s *calendarService) SetWeekdays(ctx context.Context, calendarID, campaignID string, weekdays []WeekdayInput) error {
	if len(weekdays) > maxCalendarWeekdays {
		return apperror.NewBadRequest(fmt.Sprintf("a calendar can have at most %d weekdays", maxCalendarWeekdays))
	}
	for _, w := range weekdays {
		if err := validateStructureName("weekday", w.Name, maxCalendarShortNameLength); err != nil {
			return err
		}
	}
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	return s.calRepo.SetWeekdays(ctx, calendarID, weekdays)
}

// SetMoons replaces (upserts by ID, see MoonInput's doc comment)
// calendarID's moon list.
func (s *calendarService) SetMoons(ctx context.Context, calendarID, campaignID string, moons []MoonInput) error {
	if len(moons) > maxCalendarMoons {
		return apperror.NewBadRequest(fmt.Sprintf("a calendar can have at most %d moons", maxCalendarMoons))
	}
	for i := range moons {
		if err := validateStructureName("moon", moons[i].Name, maxCalendarShortNameLength); err != nil {
			return err
		}
		moons[i].Color = normalizeColor(moons[i].Color)
	}
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	return s.calRepo.SetMoons(ctx, calendarID, moons)
}

// SetSeasons replaces calendarID's season list.
func (s *calendarService) SetSeasons(ctx context.Context, calendarID, campaignID string, seasons []Season) error {
	if len(seasons) > maxCalendarSeasons {
		return apperror.NewBadRequest(fmt.Sprintf("a calendar can have at most %d seasons", maxCalendarSeasons))
	}
	for i := range seasons {
		se := &seasons[i]
		if err := validateStructureName("season", se.Name, maxCalendarShortNameLength); err != nil {
			return err
		}
		if se.WeatherEffect != nil && utf8.RuneCountInString(*se.WeatherEffect) > maxCalendarWeatherEffectLength {
			return apperror.NewBadRequest(fmt.Sprintf("a season's weather effect can be at most %d characters", maxCalendarWeatherEffectLength))
		}
		if se.Description != nil && utf8.RuneCountInString(*se.Description) > apperror.MaxDescriptionLength {
			return apperror.NewBadRequest(fmt.Sprintf("a season's description can be at most %d characters", apperror.MaxDescriptionLength))
		}
		se.Color = normalizeColor(se.Color)
	}
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	return s.calRepo.SetSeasons(ctx, calendarID, seasons)
}

// SetCycles replaces calendarID's cycle list, each with its own entries.
func (s *calendarService) SetCycles(ctx context.Context, calendarID, campaignID string, cycles []CycleInput) error {
	if len(cycles) > maxCalendarCycles {
		return apperror.NewBadRequest(fmt.Sprintf("a calendar can have at most %d cycles", maxCalendarCycles))
	}
	for i := range cycles {
		c := &cycles[i]
		if err := validateStructureName("cycle", c.Name, maxCalendarShortNameLength); err != nil {
			return err
		}
		if len(c.Entries) > maxCalendarCycleEntries {
			return apperror.NewBadRequest(fmt.Sprintf("a cycle can have at most %d entries", maxCalendarCycleEntries))
		}
		for _, e := range c.Entries {
			if err := validateStructureName("cycle entry", e.Name, maxCalendarShortNameLength); err != nil {
				return err
			}
		}
	}
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	return s.calRepo.SetCycles(ctx, calendarID, cycles)
}

// SetFestivals replaces calendarID's festival list.
func (s *calendarService) SetFestivals(ctx context.Context, calendarID, campaignID string, festivals []FestivalInput) error {
	if len(festivals) > maxCalendarFestivals {
		return apperror.NewBadRequest(fmt.Sprintf("a calendar can have at most %d festivals", maxCalendarFestivals))
	}
	for i := range festivals {
		f := &festivals[i]
		if err := validateStructureName("festival", f.Name, apperror.MaxNameLength); err != nil {
			return err
		}
		if f.Description != nil && utf8.RuneCountInString(*f.Description) > apperror.MaxDescriptionLength {
			return apperror.NewBadRequest(fmt.Sprintf("a festival's description can be at most %d characters", apperror.MaxDescriptionLength))
		}
		f.Color = normalizeOptionalColor(f.Color)
	}
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	return s.calRepo.SetFestivals(ctx, calendarID, festivals)
}

// SetWeather replaces calendarID's current weather reading. Reading it goes
// through weatherRepo.Get inside loadSubresources; SetWeather is only the
// plumbing the campaign-backup importer needs to restore a reading a GM had
// already set before the backup. Per-day readings are SetDayWeather's.
func (s *calendarService) SetWeather(ctx context.Context, calendarID, campaignID string, input WeatherInput) error {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	return s.weatherRepo.Set(ctx, calendarID, input)
}

// --- Day weather ---

// maxDayWeatherBatch caps one day-weather write: a long fantasy year fits,
// an unbounded loop of upserts in one request does not.
const maxDayWeatherBatch = 1000

// Column widths of calendar_weather_days (migration 021).
const (
	maxWeatherPresetIDLength    = 50
	maxWeatherPresetLabelLength = 100
	maxWeatherIconLength        = 50
	maxWeatherColorLength       = 20
	maxWeatherTierLength        = 20
	maxWeatherDirectionLength   = 5
	maxWeatherPrecipTypeLength  = 20
	maxWeatherZoneIDLength      = 50
	maxWeatherZoneNameLength    = 100
	maxWeatherDescriptionLength = 2000
)

// dayOnOrBefore reports whether (y, m, d) is on or before (cy, cm, cd).
// Month numbers are positions in the calendar's month order, so a plain
// tuple comparison is calendar order without loading the months.
func dayOnOrBefore(y, m, d, cy, cm, cd int) bool {
	if y != cy {
		return y < cy
	}
	if m != cm {
		return m < cm
	}
	return d <= cd
}

// ListDayWeather returns a year's (or one month's) day readings for v. A
// viewer who cannot see dm_only content gets only days up to today: the
// forecast switch does not open future days yet (TODO(#917)).
func (s *calendarService) ListDayWeather(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) ([]DayWeather, error) {
	cal, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v)
	if err != nil {
		return nil, err
	}
	director := v.SkipsPerUserRules()
	if !director && year > cal.CurrentYear {
		return []DayWeather{}, nil
	}
	days, err := s.weatherRepo.ListDays(ctx, calendarID, year, month)
	if err != nil {
		return nil, fmt.Errorf("list day weather: %w", err)
	}
	out := make([]DayWeather, 0, len(days))
	for _, d := range days {
		if director || dayOnOrBefore(d.Year, d.Month, d.Day, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay) {
			out = append(out, d)
		}
	}
	return out, nil
}

// SetDayWeather validates and stores day readings. Every date must be a real
// day of this calendar; a generated reading never replaces a manual one
// (WeatherRepository.SetDays).
func (s *calendarService) SetDayWeather(ctx context.Context, calendarID, campaignID string, days []DayWeatherInput) error {
	if len(days) > maxDayWeatherBatch {
		return apperror.NewBadRequest(fmt.Sprintf("at most %d days can be set at once", maxDayWeatherBatch))
	}
	cal, err := s.calendarInCampaign(ctx, calendarID, campaignID)
	if err != nil {
		return err
	}
	if err := s.loadCalendarGeometry(ctx, cal); err != nil {
		return err
	}
	seen := make(map[DayDate]bool, len(days))
	for i := range days {
		d := &days[i]
		if err := validateDayDate(cal, d.Year, d.Month, d.Day); err != nil {
			return err
		}
		key := DayDate{d.Year, d.Month, d.Day}
		if seen[key] {
			return apperror.NewBadRequest("a day can appear only once")
		}
		seen[key] = true
		switch d.Source {
		case "":
			d.Source = WeatherSourceManual
		case WeatherSourceManual, WeatherSourceGenerated:
		default:
			return apperror.NewBadRequest("source must be manual or generated")
		}
		if err := validateWeatherInput(d.WeatherInput); err != nil {
			return err
		}
	}
	return s.weatherRepo.SetDays(ctx, calendarID, days)
}

// ClearDayWeather removes the readings on the given days.
func (s *calendarService) ClearDayWeather(ctx context.Context, calendarID, campaignID string, dates []DayDate) error {
	if len(dates) > maxDayWeatherBatch {
		return apperror.NewBadRequest(fmt.Sprintf("at most %d days can be cleared at once", maxDayWeatherBatch))
	}
	cal, err := s.calendarInCampaign(ctx, calendarID, campaignID)
	if err != nil {
		return err
	}
	if err := s.loadCalendarGeometry(ctx, cal); err != nil {
		return err
	}
	for _, d := range dates {
		if err := validateDayDate(cal, d.Year, d.Month, d.Day); err != nil {
			return err
		}
	}
	return s.weatherRepo.ClearDays(ctx, calendarID, dates)
}

// maxDayWeatherYear bounds a day reading's year well inside the INT column,
// so an absurd year fails as a 400 rather than a driver error.
const maxDayWeatherYear = 1_000_000

// validateDayDate rejects a date that is not a day of cal (geometry loaded).
func validateDayDate(cal *Calendar, year, month, day int) error {
	if year < -maxDayWeatherYear || year > maxDayWeatherYear {
		return apperror.NewBadRequest(fmt.Sprintf("year must be between %d and %d", -maxDayWeatherYear, maxDayWeatherYear))
	}
	if month < 1 || month > len(cal.Months) {
		return apperror.NewBadRequest(fmt.Sprintf("month %d is not a month of this calendar", month))
	}
	if day < 1 || day > cal.MonthDays(month-1, year) {
		return apperror.NewBadRequest(fmt.Sprintf("day %d is not a day of month %d in year %d", day, month, year))
	}
	return nil
}

// validateWeatherInput holds a reading to its columns' widths and to
// physically sensible numbers, so bad input fails as a clean 400.
func validateWeatherInput(in WeatherInput) error {
	for _, f := range []struct {
		name string
		v    *string
		max  int
	}{
		{"preset_id", in.PresetID, maxWeatherPresetIDLength},
		{"preset_label", in.PresetLabel, maxWeatherPresetLabelLength},
		{"icon", in.Icon, maxWeatherIconLength},
		{"color", in.Color, maxWeatherColorLength},
		{"wind_speed_tier", in.WindSpeedTier, maxWeatherTierLength},
		{"wind_direction", in.WindDirection, maxWeatherDirectionLength},
		{"precipitation_type", in.PrecipitationType, maxWeatherPrecipTypeLength},
		{"zone_id", in.ZoneID, maxWeatherZoneIDLength},
		{"zone_name", in.ZoneName, maxWeatherZoneNameLength},
		{"description", in.Description, maxWeatherDescriptionLength},
	} {
		if err := validateOptionalText(f.name, f.v, f.max); err != nil {
			return err
		}
	}
	if in.Color != nil && *in.Color != "" && !hexColorPattern.MatchString(*in.Color) {
		return apperror.NewBadRequest("color must be a hex color (#rgb or #rrggbb)")
	}
	if t := in.TemperatureCelsius; t != nil && (math.IsNaN(*t) || *t < -100 || *t > 100) {
		return apperror.NewBadRequest("temperature_celsius must be between -100 and 100")
	}
	if w := in.WindSpeedKPH; w != nil && (math.IsNaN(*w) || *w < 0 || *w > 1000) {
		return apperror.NewBadRequest("wind_speed_kph must be between 0 and 1000")
	}
	if d := in.WindDirectionDeg; d != nil && (*d < 0 || *d > 359) {
		return apperror.NewBadRequest("wind_direction_degrees must be between 0 and 359")
	}
	if p := in.PrecipitationIntensity; p != nil && (math.IsNaN(*p) || *p < 0 || *p > 1) {
		return apperror.NewBadRequest("precipitation_intensity must be between 0 and 1")
	}
	return nil
}

// validateMonthInputs holds a whole-list month write to the limits an
// uploaded calendar file gets. The campaign backup import writes through
// SetMonths without going through the import parsers.
func validateMonthInputs(months []MonthInput) error {
	if len(months) > maxCalendarMonths {
		return apperror.NewBadRequest(fmt.Sprintf("a calendar can have at most %d months", maxCalendarMonths))
	}
	for _, m := range months {
		if m.Days < 1 || m.Days > maxCalendarMonthDays || m.LeapYearDays > maxCalendarMonthDays {
			return apperror.NewBadRequest(fmt.Sprintf("month %q must have between 1 and %d days", m.Name, maxCalendarMonthDays))
		}
		if err := validateStructureName("month", m.Name, maxCalendarShortNameLength); err != nil {
			return err
		}
	}
	return nil
}

// validateStructureName refuses a month, weekday, moon or season name longer
// than its column, with a clear message rather than a database error.
func validateStructureName(kind, name string, maxLen int) error {
	if utf8.RuneCountInString(name) > maxLen {
		return apperror.NewBadRequest(fmt.Sprintf("a %s name can be at most %d characters", kind, maxLen))
	}
	return nil
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

// SearchCalendarEvents implements entities.CalendarSearcher — see the
// interface doc comment for the role-only shape and what it does and does
// not filter. Every candidate calendar comes from ListCalendars (role +
// per-user visibility filtered), so a dm_only or otherwise-hidden
// calendar's events are never even searched; within a visible calendar,
// EventRepository.SearchEvents applies the same SQL dm_only filter
// ListEventsForMonth's own read does, and the per-user layer runs after it
// as it does for the pages. The searcher is told only a role, not who is
// asking, so an event with its own allow or deny list stays out of a
// non-GM search; the calendar page, which knows the viewer, still shows it
// to whoever may see it. Capped at 10 events per calendar by that same
// query (a search result list, not a full export).
func (s *calendarService) SearchCalendarEvents(ctx context.Context, campaignID, query string, role int) ([]map[string]string, error) {
	v := permissions.RequestViewer(role, "")
	cals, err := s.ListCalendars(ctx, campaignID, v)
	if err != nil {
		return nil, fmt.Errorf("search calendar events: list calendars: %w", err)
	}

	var results []map[string]string
	for _, cal := range cals {
		events, err := s.eventRepo.SearchEvents(ctx, cal.ID, query, role)
		if err != nil {
			return nil, fmt.Errorf("search calendar events: search calendar %s: %w", cal.ID, err)
		}
		for _, evt := range filterEventsByUser(events, v) {
			icon := evt.KindIcon
			if icon == "" {
				icon = "fa-calendar-day"
			}
			results = append(results, map[string]string{
				"id":         evt.ID,
				"name":       evt.Name,
				"type_name":  "Calendar Event",
				"type_icon":  icon,
				"type_color": evt.KindColor,
				"url":        fmt.Sprintf("/campaigns/%s/calendars/%s/view", campaignID, cal.ID),
			})
		}
	}
	return results, nil
}

// ListEventsForCalendar implements timeline.CalendarEventLister — see the
// interface doc comment. calendarInCampaignForViewer folds "wrong campaign"
// and "role can't see it" into the same NotFound, which becomes an empty
// slice here rather than an error. Linked pages the role can't see are
// blanked the same way every other event read here blanks them.
func (s *calendarService) ListEventsForCalendar(ctx context.Context, campaignID, calendarID string, role int) ([]Event, error) {
	v := permissions.RequestViewer(role, "")
	if _, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v); err != nil {
		if isCalendarNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	events, err := s.eventRepo.ListAllEvents(ctx, calendarID)
	if err != nil {
		return nil, fmt.Errorf("list events for calendar: %w", err)
	}
	events = filterEventsByUser(events, v)
	if err := s.redactHiddenEntityLinks(ctx, campaignID, events, v); err != nil {
		return nil, err
	}
	return events, nil
}

// ListErasForCalendar implements timeline.CalendarEraLister — see the
// interface doc comment for why this gates on role alone rather than the
// calendar's own visibility.
func (s *calendarService) ListErasForCalendar(ctx context.Context, campaignID, calendarID string, role int) ([]Era, error) {
	if !permissions.CanSeeDmOnly(role) {
		return nil, nil
	}
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		if isCalendarNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return s.calRepo.GetEras(ctx, calendarID)
}

// --- Shared validation helpers ---

// validateImportCurrentDate checks month/day (already resolved from either
// the import file or a caller override — see CreateCalendarFromImport)
// against the import's OWN month structure (ir.Months), the same shape
// CalendarRepository.ApplyImport is about to write. It duplicates the shape
// of Calendar.MonthDays' leap-year arithmetic rather than calling it,
// because at this point in CreateCalendarFromImport no *Calendar with
// Months loaded exists yet — the bare row isn't even created until after
// this check passes.
//
// A month/day pair that looks fine to the human confirming it in the wizard
// but doesn't fit the structure that same import describes (an off-by-one
// in a hand-edited file, or a malicious upload) is rejected here rather
// than silently landing in current_month/current_day, where every later
// date computation trusts it unchecked.
func validateImportCurrentDate(ir *ImportResult, year, month, day int) error {
	n := len(ir.Months)
	if n == 0 {
		// No month structure to check against yet — CalendarRepository.
		// ApplyImport / the wizard surface a months-less import as its own,
		// separate problem; nothing for this check to compare against.
		return nil
	}
	if month < 1 || month > n {
		return apperror.NewValidation(fmt.Sprintf(
			"current_month must be between 1 and %d for this calendar's %d months", n, n))
	}
	m := ir.Months[month-1]
	days := m.Days
	if ir.Settings.Mode == ModeRealLife && ir.Settings.TracksRealTime {
		// A TracksRealTime calendar's real month lengths follow the true
		// Gregorian 4/100/400 leap rule (MonthDays' own UsesRealTime branch),
		// which the naive leap_year_every/offset arithmetic below cannot
		// express — it would accept Feb 29 in a century year like 2100 that
		// isn't actually a leap year. Mirror daysInGregorianMonth here since
		// no *Calendar exists yet to call MonthDays on.
		days = daysInGregorianMonth(year, month)
	} else if ir.Settings.LeapYearEvery > 0 && (year-ir.Settings.LeapYearOffset)%ir.Settings.LeapYearEvery == 0 {
		days += m.LeapYearDays
	}
	if days < 1 {
		days = 1
	}
	if day < 1 || day > days {
		return apperror.NewValidation(fmt.Sprintf(
			"current_day must be between 1 and %d for month %q", days, m.Name))
	}
	return nil
}

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
