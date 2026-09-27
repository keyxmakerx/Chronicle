// Package calendar - service.go holds the calendar plugin's business logic
// over the slice-1 repositories (CalendarRepository, EventRepository,
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
	"regexp"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// slugPattern validates an event kind's slug: lowercase, digits and hyphens,
// no leading/trailing hyphen. Mirrors the shape other plugins use for
// user-authored slugs referenced by other rows.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// faIconPattern and hexColorPattern are a security control, not a shape
// nicety: an icon or color value that reaches an unescaped HTML template
// verbatim (a confirmed weakness elsewhere in the product) can carry a
// quote, an angle bracket or arbitrary markup. Restricting both fields to a
// closed character class makes that unrepresentable, independent of
// whatever templates end up rendering them. faIconPattern accepts only a
// Font Awesome class name (fa- + lowercase letters/digits/hyphens);
// hexColorPattern is the exact pattern internal/plugins/entities/service.go
// and internal/plugins/timeline/service.go already use for the same
// reason — kept as its own local copy here (a shared validator is being
// built on another branch; this one joins it later) rather than importing
// across the plugin boundary.
var (
	faIconPattern   = regexp.MustCompile(`^fa-[a-z0-9-]+$`)
	hexColorPattern = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
)

// Column-width limits narrower than apperror's generic Max*Length constants
// (rule 6: a value that doesn't fit its column must fail as a clean
// apperror, never reach the driver and come back as a raw "data too long"
// error). calendar_events.color/icon are intentionally tighter than
// calendar_event_kinds' — an event's color/icon is a short override, not a
// rich identifier — see migrations/001_calendar_tables.up.sql and
// db/migrations. calendars.epoch_name and calendar_eras.color match their
// own column widths too.
const (
	maxEventColorLength = 7   // calendar_events.color VARCHAR(7)
	maxEventIconLength  = 10  // calendar_events.icon VARCHAR(10)
	maxEventTierLength  = 64  // calendar_events.tier VARCHAR(64)
	maxEpochNameLength  = 100 // calendars.epoch_name VARCHAR(100)
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

// CalendarService defines the calendar plugin's business logic. Handlers
// call it; it never imports Echo.
type CalendarService interface {
	// Calendars.
	CreateCalendar(ctx context.Context, campaignID string, input CreateCalendarInput) (*Calendar, error)
	// GetCalendarForViewer returns a calendar only if it belongs to
	// campaignID and v may see it (ADR-049); both failure modes collapse to
	// the same NotFound (ADR-058-style, see GetEventForViewer). Eager-loads
	// every sub-resource; Eras and EventKinds are stripped and Moons are
	// filtered for a viewer that does not skip the per-user layer.
	GetCalendarForViewer(ctx context.Context, calendarID, campaignID string, v permissions.Viewer) (*Calendar, error)
	// ListCalendars returns a campaign's calendars, role- and per-user
	// visibility-filtered. No sub-resources are eager-loaded (list view).
	ListCalendars(ctx context.Context, campaignID string, v permissions.Viewer) ([]Calendar, error)
	UpdateCalendar(ctx context.Context, calendarID, campaignID string, input UpdateCalendarInput) error
	DeleteCalendar(ctx context.Context, calendarID, campaignID string) error
	SetDefaultCalendar(ctx context.Context, campaignID, calendarID string) error

	// Events.
	CreateEvent(ctx context.Context, calendarID, campaignID string, input CreateEventInput) (*Event, error)
	// GetEventForViewer is GetEvent's viewer-aware sibling (ADR-049/058): it
	// returns NotFound (never the event) unless the event belongs to
	// calendarID, calendarID belongs to campaignID, AND v may see it.
	GetEventForViewer(ctx context.Context, eventID, calendarID, campaignID string, v permissions.Viewer) (*Event, error)
	ListEventsForMonth(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) ([]Event, error)
	UpdateEvent(ctx context.Context, eventID, calendarID, campaignID string, input UpdateEventInput) error
	DeleteEvent(ctx context.Context, eventID, calendarID, campaignID string) error
	// SetEventVisibility is the dm_only toggle: Owner-only end to end, so it
	// takes no viewer parameter (the route itself is Owner-gated).
	SetEventVisibility(ctx context.Context, eventID, calendarID, campaignID string, input UpdateEventVisibilityInput) error

	// Event kinds. Campaign-scoped (shared by every calendar in the
	// campaign, see EventKind's doc comment) and Owner-only end to end —
	// calendar structure, not content a Player ever reads directly.
	ListEventKinds(ctx context.Context, campaignID string) ([]EventKind, error)
	CreateEventKind(ctx context.Context, campaignID string, input EventKindInput) (*EventKind, error)
	UpdateEventKind(ctx context.Context, kindID int, campaignID string, input EventKindInput) error
	DeleteEventKind(ctx context.Context, kindID int, campaignID string) error

	// Eras. Owner-only end to end (calendar structure), resolved through the
	// calendar the caller reached (GetEraByID itself is not calendar-scoped;
	// this service never calls it with an untrusted id — see repository.go).
	CreateEra(ctx context.Context, calendarID, campaignID string, input EraInput) (*Era, error)
	UpdateEra(ctx context.Context, eraID int, calendarID, campaignID string, input EraInput) error
	DeleteEra(ctx context.Context, eraID int, calendarID, campaignID string) error

	// Moon. Owner-only end to end (the hidden flag is calendar structure).
	SetMoonHidden(ctx context.Context, moonID int, calendarID, campaignID string, hidden bool) error
}

// calendarService is the concrete CalendarService.
type calendarService struct {
	calRepo     CalendarRepository
	eventRepo   EventRepository
	kindRepo    EventKindRepository
	weatherRepo WeatherRepository
}

// NewCalendarService constructs a CalendarService over the four slice-1
// repositories.
func NewCalendarService(calRepo CalendarRepository, eventRepo EventRepository, kindRepo EventKindRepository, weatherRepo WeatherRepository) CalendarService {
	return &calendarService{calRepo: calRepo, eventRepo: eventRepo, kindRepo: kindRepo, weatherRepo: weatherRepo}
}

// --- Cross-campaign / cross-calendar scoping (rule: no cross-tenant reach) ---

// calendarInCampaign loads a calendar and confirms it belongs to campaignID,
// collapsing "doesn't exist" and "exists in a different campaign" into the
// exact same NotFound message — a wrong-campaign id must look identical to a
// missing one, or its existence becomes an oracle for a prober (ADR-058).
// This is the one gate every calendar-, event-, era- and moon-scoped method
// below calls first.
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

// --- Visibility (ADR-049): the one predicate behind every calendar/event
// decision in this package, mirroring timeline's canUserView/
// timelineVisibleToViewer (ADR-058) so list and single-item reads can't
// drift apart. ---

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
// visibility decision. filterCalendarsByUser's per-row filter and
// GetCalendarForViewer's single-item lookup both call it.
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

// --- Calendars ---

// CreateCalendar creates a bare calendar (no months/weekdays/moons — later
// slices own seeding a preset or an editing UI for those). Mode defaults to
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
	if input.Description != nil {
		if err := apperror.ValidateStringLength("description", *input.Description, apperror.MaxDescriptionLength); err != nil {
			return nil, err
		}
	}
	if input.EpochName != nil {
		if err := apperror.ValidateStringLength("epoch_name", *input.EpochName, maxEpochNameLength); err != nil {
			return nil, err
		}
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
		Visibility:       "everyone",
	}
	if err := s.calRepo.Create(ctx, cal); err != nil {
		return nil, fmt.Errorf("create calendar: %w", err)
	}
	return cal, nil
}

// GetCalendarForViewer returns a calendar with its sub-resources, gated by
// campaign membership and visibility (see the interface doc comment).
func (s *calendarService) GetCalendarForViewer(ctx context.Context, calendarID, campaignID string, v permissions.Viewer) (*Calendar, error) {
	cal, err := s.calendarInCampaign(ctx, calendarID, campaignID)
	if err != nil {
		return nil, err
	}
	if !calendarVisibleToViewer(*cal, v) {
		return nil, apperror.NewNotFound("calendar not found")
	}
	if err := s.loadSubresources(ctx, cal); err != nil {
		return nil, err
	}
	if !v.SkipsPerUserRules() {
		// Event kinds and eras are calendar STRUCTURE (Owner-only end to
		// end, .ai/conventions.md); a Player never learns of their
		// existence through the calendar read either. Moons are content —
		// only the hidden ones are stripped.
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
	if description != nil {
		if err := apperror.ValidateStringLength("description", *description, apperror.MaxDescriptionLength); err != nil {
			return err
		}
	}
	epochName := input.EpochName.Ptr(cal.EpochName)
	if epochName != nil {
		if err := apperror.ValidateStringLength("epoch_name", *epochName, maxEpochNameLength); err != nil {
			return err
		}
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

// CreateEvent creates an event on calendarID. Visibility and per-user
// visibility_rules must already reflect the caller's permission (the
// handler downgrades a non-Owner's attempted dm_only/visibility_rules
// before this is ever called, mirroring maps.CreateMarkerAPI); this method
// still validates the shape.
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
// calendarID belongs to campaignID, and v may see it. Every failure mode
// collapses to the same NotFound (see the interface doc comment).
func (s *calendarService) GetEventForViewer(ctx context.Context, eventID, calendarID, campaignID string, v permissions.Viewer) (*Event, error) {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return nil, err
	}
	evt, err := s.eventRepo.GetEvent(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("get event: %w", err)
	}
	if evt == nil || evt.CalendarID != calendarID || !eventVisibleToViewer(*evt, v) {
		return nil, apperror.NewNotFound("event not found")
	}
	return evt, nil
}

// ListEventsForMonth returns a calendar's events for (year, month),
// role-filtered by SQL (dm_only) and per-user-filtered in Go
// (visibility_rules) on top.
func (s *calendarService) ListEventsForMonth(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) ([]Event, error) {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return nil, err
	}
	events, err := s.eventRepo.ListEventsForMonth(ctx, calendarID, year, month, v.Role())
	if err != nil {
		return nil, fmt.Errorf("list events for month: %w", err)
	}
	return filterEventsByUser(events, v), nil
}

// eventInCalendar loads an event and confirms it belongs to calendarID
// (which the caller has already confirmed belongs to campaignID), collapsing
// "doesn't exist" and "belongs to a different calendar" into one NotFound.
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

// UpdateEvent applies a partial update to an event (load-merge-write; see
// UpdateEventInput's doc comment). Visibility/visibility_rules are expected
// to already reflect the caller's permission — see CreateEvent's doc.
func (s *calendarService) UpdateEvent(ctx context.Context, eventID, calendarID, campaignID string, input UpdateEventInput) error {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	evt, err := s.eventInCalendar(ctx, eventID, calendarID)
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

	// An ABSENT description_html preserves the stored (already-sanitized)
	// HTML; a present value (including empty) is re-sanitized on write, the
	// same rule entities/timeline apply to rich text.
	descHTML := evt.DescriptionHTML
	if input.DescriptionHTML.Present() {
		descHTML = sanitizeDescriptionHTMLField(input.DescriptionHTML)
	}

	evt.Name = name
	evt.Description = input.Description.Ptr(evt.Description)
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

// DeleteEvent removes an event, scoped to calendarID/campaignID.
func (s *calendarService) DeleteEvent(ctx context.Context, eventID, calendarID, campaignID string) error {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	if _, err := s.eventInCalendar(ctx, eventID, calendarID); err != nil {
		return err
	}
	if err := s.eventRepo.DeleteEvent(ctx, eventID); err != nil {
		return fmt.Errorf("delete event: %w", err)
	}
	return nil
}

// SetEventVisibility is the dm_only toggle: Owner-only end to end (the
// route gates it), so the visibility value itself needs no downgrade here.
func (s *calendarService) SetEventVisibility(ctx context.Context, eventID, calendarID, campaignID string, input UpdateEventVisibilityInput) error {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	evt, err := s.eventInCalendar(ctx, eventID, calendarID)
	if err != nil {
		return err
	}
	if input.Visibility != "everyone" && input.Visibility != "dm_only" {
		return apperror.NewValidation("visibility must be \"everyone\" or \"dm_only\"")
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
// validateEventKindInput does not (Slug/Name shape + length limits); the
// repository still re-validates DefaultAnnounced and the slug uniqueness
// constraint, so this is defense in depth, not the only check.
func validateEventKindShape(input EventKindInput) error {
	if err := apperror.ValidateRequired("name", input.Name); err != nil {
		return err
	}
	if err := apperror.ValidateStringLength("name", input.Name, apperror.MaxNameLength); err != nil {
		return err
	}
	if err := apperror.ValidateRequired("slug", input.Slug); err != nil {
		return err
	}
	if !slugPattern.MatchString(input.Slug) {
		return apperror.NewValidation("slug must be lowercase letters, digits and hyphens")
	}
	// Icon/color are validated by closed character class, not just length:
	// an event kind's icon has previously reached an unescaped HTML template
	// verbatim elsewhere in the product, so free text here is refused
	// outright rather than merely length-capped.
	if err := apperror.ValidateStringLength("icon", input.Icon, apperror.MaxIconLength); err != nil {
		return err
	}
	if !faIconPattern.MatchString(input.Icon) {
		return apperror.NewBadRequest("icon must be a Font Awesome class name (fa-lowercase-with-hyphens)")
	}
	if err := apperror.ValidateStringLength("color", input.Color, apperror.MaxColorLength); err != nil {
		return err
	}
	if !hexColorPattern.MatchString(input.Color) {
		return apperror.NewBadRequest("color must be a hex color (#rgb or #rrggbb)")
	}
	if input.DefaultAnnounced != "" && !IsSupportedAnnounced(input.DefaultAnnounced) {
		return apperror.NewValidation("default_announced must be \"" + AnnouncedAhead + "\" or \"" + AnnouncedOnDay + "\"")
	}
	return nil
}

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

func (s *calendarService) UpdateEventKind(ctx context.Context, kindID int, campaignID string, input EventKindInput) error {
	if err := validateEventKindShape(input); err != nil {
		return err
	}
	if err := s.kindRepo.Update(ctx, kindID, campaignID, input); err != nil {
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
	if err := validateEraShape(input); err != nil {
		return nil, err
	}
	era, err := s.calRepo.CreateEra(ctx, calendarID, input)
	if err != nil {
		return nil, fmt.Errorf("create era: %w", err)
	}
	return era, nil
}

func (s *calendarService) UpdateEra(ctx context.Context, eraID int, calendarID, campaignID string, input EraInput) error {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	if err := validateEraShape(input); err != nil {
		return err
	}
	// UpdateEra is itself scoped to calendarID (WHERE id = ? AND
	// calendar_id = ?) and returns apperror.NewNotFound on no match.
	if err := s.calRepo.UpdateEra(ctx, calendarID, eraID, input); err != nil {
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

// validateEraShape checks the era fields a handler binds directly (EraInput
// carries no ID from an untrusted caller path here — CreateEra/UpdateEra
// build it fresh per call, see handler.go).
func validateEraShape(input EraInput) error {
	if err := apperror.ValidateRequired("name", input.Name); err != nil {
		return err
	}
	if err := apperror.ValidateStringLength("name", input.Name, apperror.MaxNameLength); err != nil {
		return err
	}
	if input.Description != nil {
		if err := apperror.ValidateStringLength("description", *input.Description, apperror.MaxDescriptionLength); err != nil {
			return err
		}
	}
	if input.Color != "" {
		if err := apperror.ValidateStringLength("color", input.Color, apperror.MaxColorLength); err != nil {
			return err
		}
	}
	if input.EndYear != nil {
		endMonth, endDay := 1, 1
		if input.EndMonth != nil {
			endMonth = *input.EndMonth
		}
		if input.EndDay != nil {
			endDay = *input.EndDay
		}
		if dateLess(*input.EndYear, endMonth, endDay, input.StartYear, input.StartMonth, input.StartDay) {
			return apperror.NewValidation("era end date must not be before its start date")
		}
	}
	return nil
}

// --- Moon ---

// SetMoonHidden toggles a moon's visibility to players. Owner-only end to
// end (the route gates it); CalendarRepository.SetMoonHidden is itself
// scoped to calendarID.
func (s *calendarService) SetMoonHidden(ctx context.Context, moonID int, calendarID, campaignID string, hidden bool) error {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	if err := s.calRepo.SetMoonHidden(ctx, calendarID, moonID, hidden); err != nil {
		return err
	}
	return nil
}

// --- Shared validation helpers ---

// validateEventCosmeticLengths enforces calendar_events' own (narrower)
// column widths for color/icon/tier — see the maxEvent*Length doc comment.
// All three are optional; a nil pointer is simply not checked.
//
// Icon and color are each optional overrides of the event's kind (nil/empty
// means "inherit"), so an absent or blank value is not checked — but ANY
// non-empty value must match the same closed character class
// validateEventKindShape enforces, for the same reason: an icon/color value
// has previously reached an unescaped HTML template verbatim elsewhere in
// the product.
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
