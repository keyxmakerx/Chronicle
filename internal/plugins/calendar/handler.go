package calendar

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/audit"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler processes HTTP requests for the calendar plugin.
type Handler struct {
	svc            CalendarService
	addonSvc       addons.AddonService
	auditSvc       audit.AuditService
	tierLister     TierDefinitionsLister
	timelineLister TimelineLister // cross-plugin read for the Calendars dashboard (W1).
	entityCreator  EntityCreator  // cross-plugin write for "create entity from event" (C-CAL-EDITOR-EXPANSION PR1).
	// schedule is the sessions read the Bench RSVP panel needs (C-CALV4-RSVP-P8);
	// rsvpRead is the event-RSVP read for the same panel's answer column. Both
	// nil-safe: without them the panel renders its unfilled state and the rest
	// of the Bench is untouched.
	schedule BenchScheduleReader
	rsvpRead RSVPService
	// mailStatus answers the ONE email question the Bench asks: is a mail
	// server configured (C-CALV4-RSVP-P8B §8). Nil-safe like the two above.
	mailStatus MailStatusReader
	// ownWeek is the /schedule Painter's read: the VIEWER'S OWN composed week
	// (C-CALV4-RSVP-P8 Part B). Separate from `schedule` because it answers a
	// different permission question — "may I read back what I saved", whose
	// answer is always yes — and nil-safe for the same reason as the rest.
	ownWeek ScheduleOwnWeekReader
}

// TierDefinitionsLister surfaces the campaign-aware tier vocabulary
// to the V2 calendar handler without forcing a broad campaigns
// service dependency. Implemented by `campaigns.CampaignService`
// (single-method match against existing `GetEventTierDefinitions`).
// Narrow interface keeps the calendar plugin's test surface
// tractable + matches the existing TimelineLister pattern.
type TierDefinitionsLister interface {
	GetEventTierDefinitions(ctx context.Context, campaignID string) ([]campaigns.TierDefinition, error)
}

// NewHandler creates a new calendar Handler.
func NewHandler(svc CalendarService) *Handler {
	return &Handler{svc: svc}
}

// SetAddonService sets the addon service for auto-enabling the calendar addon
// when a calendar is created. Called after all plugins are wired.
func (h *Handler) SetAddonService(svc addons.AddonService) {
	h.addonSvc = svc
}

// SetAuditService wires the audit-log emitter for calendar mutations.
// Called after all plugins are constructed (avoids init-order cycles).
// Matches the entities plugin pattern.
func (h *Handler) SetAuditService(svc audit.AuditService) {
	h.auditSvc = svc
}

// SetTierDefinitionsLister wires the tier-definition surface used by
// the V2 calendar shell (Wave 1.6.5). Called after both calendar
// and campaigns plugins are constructed. Nil-safe: V2 handler falls
// back to platform-default tier rendering when the lister is unset
// or returns an error (graceful degradation per PR #370 Phase 2).
func (h *Handler) SetTierDefinitionsLister(lister TierDefinitionsLister) {
	h.tierLister = lister
}

// logCalendarAudit fires a fire-and-forget audit entry for a calendar-
// scoped mutation. EntityType is the audit-log resource label (e.g.
// "calendar", "calendar_event", "calendar_era"); entityID is the
// stringified resource id; details carries the action-specific payload.
// Errors are slog-logged and never block the primary operation per
// dispatch §"Failure handling". User-id comes from echo.Context — the
// established pattern from internal/plugins/entities/handler.go.
func (h *Handler) logCalendarAudit(c echo.Context, campaignID, action, entityType, entityID, entityName string, details map[string]any) {
	if h.auditSvc == nil {
		return
	}
	userID := auth.GetUserID(c)
	if err := h.auditSvc.Log(c.Request().Context(), &audit.AuditEntry{
		CampaignID: campaignID,
		UserID:     userID,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		EntityName: entityName,
		Details:    details,
	}); err != nil {
		slog.Warn("calendar audit log failed",
			slog.String("action", action),
			slog.String("entity_id", entityID),
			slog.Any("error", err),
		)
	}
}

// requireCalendarInCampaign fetches a calendar by ID and verifies it belongs
// to the given campaign. Returns 404 if not found or mismatched, preventing
// cross-campaign IDOR attacks.
func (h *Handler) requireCalendarInCampaign(c echo.Context, calendarID, campaignID string) (*Calendar, error) {
	return middleware.RequireInCampaign(c.Request().Context(), h.svc.GetCalendarByID, calendarID, campaignID, "calendar")
}

// requireVisibleCalendar is requireCalendarInCampaign plus a per-calendar
// visibility check (C-CAL-DASHBOARD-W5a): a viewer who may not see the calendar
// gets NotFound — identical to a missing calendar, so a hidden calendar's
// existence never leaks to a player who guesses its ID. Owner/co-DM always
// pass. Use on player-reachable calendar-by-ID surfaces; owner-gated management
// routes keep requireCalendarInCampaign.
func (h *Handler) requireVisibleCalendar(c echo.Context, calendarID, campaignID string) (*Calendar, error) {
	cal, err := h.requireCalendarInCampaign(c, calendarID, campaignID)
	if err != nil {
		return nil, err
	}
	cc := campaigns.GetCampaignContext(c)
	if !calendarVisibleTo(cal, cc.VisibilityRole(), auth.GetUserID(c)) {
		return nil, apperror.NewNotFound("calendar not found")
	}
	return cal, nil
}

// Index is the V1 calendar entry point. If no calendars exist it shows the
// BUILDER WIZARD (the create flow); otherwise it 301s to the V2 shell, which
// owns the active-calendar + multi-cal switcher views that replaced the V1
// single/list pages (C-CAL-V1-V2-CUTOVER).
//
// The zero-calendar branch used to render the three-card V1 setup chooser.
// C-CALV4-WIZARD-P13 [WZ-13] SIGNED retired it: a campaign's FIRST calendar is
// exactly the case the wizard was designed for, and landing a first-run on the
// un-designed surface is the one outcome that would have made the whole wave
// pointless.
// GET /campaigns/:id/calendars
func (h *Handler) Index(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	cals, err := h.svc.ListCalendars(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}

	// No calendars: show the builder wizard (the create flow).
	if len(cals) == 0 {
		return h.ShowBuilder(c)
	}

	// C-CAL-V1-V2-CUTOVER: any campaign WITH calendars goes to V2 — the active
	// calendar + the multi-cal switcher replace the V1 single/list views. Only
	// the 0-calendar setup chooser above stays on V1 (no V2 create flow yet; the
	// V2 empty state links back here). The V1 list templates are retired in a
	// follow-on.
	return c.Redirect(http.StatusMovedPermanently,
		"/campaigns/"+cc.Campaign.ID+"/calendar/v2")
}

// EmbedCalendar returns a compact calendar grid fragment for dashboard embedding.
// Accepts calId as a route param or query param for dashboard block config.
// GET /campaigns/:id/calendars/:calId/embed
func (h *Handler) EmbedCalendar(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	// Support both route param and query param for backward compat with dashboard blocks.
	calID := c.Param("calId")
	if calID == "" {
		calID = c.QueryParam("calendarId")
	}

	var cal *Calendar
	var err error
	if calID != "" {
		cal, err = h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
		if err != nil {
			return middleware.Render(c, http.StatusOK, CalendarEmbedEmpty(cc))
		}
	} else {
		// No calendar specified: use the default/first calendar.
		cal, err = h.svc.GetCalendar(ctx, cc.Campaign.ID)
		if err != nil {
			return err
		}
	}

	// No calendar exists — return a setup prompt.
	if cal == nil {
		return middleware.Render(c, http.StatusOK, CalendarEmbedEmpty(cc))
	}

	year := cal.CurrentYear
	month := cal.CurrentMonth
	if q := c.QueryParam("year"); q != "" {
		if v, err := strconv.Atoi(q); err == nil {
			year = v
		}
	}
	if q := c.QueryParam("month"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v >= 1 && v <= len(cal.Months) {
			month = v
		}
	}

	role := cc.VisibilityRole()
	userID := auth.GetUserID(c)
	events, err := h.svc.ListEventsForMonth(ctx, cal.ID, year, month, role, userID)
	if err != nil {
		return err
	}

	data := CalendarViewData{
		Calendar:        cal,
		Year:            year,
		MonthIndex:      month,
		Events:          events,
		CampaignID:      cc.Campaign.ID,
		UserID:          userID,
		IsOwner:         cc.MemberRole >= campaigns.RoleOwner,
		IsScribe:        cc.MemberRole >= campaigns.RoleScribe,
		CanAuthorDmOnly: cc.CanAuthorDmOnly(),
		CSRFToken:       middleware.GetCSRFToken(c),
	}

	return middleware.Render(c, http.StatusOK, CalendarEmbedFragment(cc, data))
}

// CreateCalendar handles calendar creation from the setup form.
// POST /campaigns/:id/calendars
func (h *Handler) CreateCalendar(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	mode := c.FormValue("mode")
	name := c.FormValue("name")
	if name == "" {
		name = "Campaign Calendar"
	}
	epochName := c.FormValue("epoch_name")
	startYear, _ := strconv.Atoi(c.FormValue("start_year"))
	if startYear == 0 {
		startYear = 1
	}

	var epoch *string
	if epochName != "" {
		epoch = &epochName
	}

	// Service handles mode-specific defaults and seeds months/weekdays.
	cal, err := h.svc.CreateCalendar(ctx, cc.Campaign.ID, CreateCalendarInput{
		Mode:        mode,
		Name:        name,
		EpochName:   epoch,
		CurrentYear: startYear,
	})
	if err != nil {
		return err
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarCreated, "calendar", cal.ID, cal.Name,
		map[string]any{"mode": string(mode), "epoch_name": epochName, "current_year": startYear})

	// Auto-enable the calendar addon for this campaign so dashboard/entity
	// blocks render immediately without manual extension toggling.
	if h.addonSvc != nil {
		addon, aErr := h.addonSvc.GetBySlug(ctx, "calendar")
		if aErr == nil && addon != nil {
			userID := auth.GetUserID(c)
			if eErr := h.addonSvc.EnableForCampaign(ctx, cc.Campaign.ID, addon.ID, userID); eErr != nil {
				slog.Warn("auto-enable calendar addon failed", slog.Any("error", eErr))
			}
		}
	}

	// Redirect to settings for fantasy mode so users can immediately customize
	// months, weekdays, etc. Real-life mode goes straight to the calendar — the
	// V2 shell (C-WIDGET-BINDING-QA1 Bug 1: was landing on the old V1 view, which
	// is missing the V2 worldstate features/animations). The fantasy →settings
	// step is mode-agnostic (the settings editor, not the V1 view). The full
	// V1→V2 cutover (C-CAL-V1-V2-CUTOVER) since 301'd every V1 view route to V2,
	// so the settings landing below is itself a preserved route, not a V1 view.
	if mode == ModeRealLife {
		return c.Redirect(http.StatusSeeOther,
			fmt.Sprintf("/campaigns/%s/calendar/v2/%s", cc.Campaign.ID, cal.ID))
	}
	return c.Redirect(http.StatusSeeOther,
		fmt.Sprintf("/campaigns/%s/calendars/%s/settings", cc.Campaign.ID, cal.ID))
}

// UpdateCalendarAPI updates calendar settings.
// PUT /campaigns/:id/calendars/:calId/settings
func (h *Handler) UpdateCalendarAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var req struct {
		Name             string  `json:"name"`
		Description      *string `json:"description"`
		EpochName        *string `json:"epoch_name"`
		CurrentYear      int     `json:"current_year"`
		CurrentMonth     int     `json:"current_month"`
		CurrentDay       int     `json:"current_day"`
		CurrentHour      int     `json:"current_hour"`
		CurrentMinute    int     `json:"current_minute"`
		HoursPerDay      int     `json:"hours_per_day"`
		MinutesPerHour   int     `json:"minutes_per_hour"`
		SecondsPerMinute int     `json:"seconds_per_minute"`
		LeapYearEvery    int     `json:"leap_year_every"`
		LeapYearOffset   int     `json:"leap_year_offset"`
		// Real-time settings (C-REAL-CALENDAR-P2). TracksRealTime is a *bool so an
		// omitted field leaves the stored flag unchanged (a client that never sends
		// it can't accidentally disable real-time); the settings form always sends
		// it. RealTimeZone is the IANA anchor, required+validated by the service on
		// enable (RC-2).
		TracksRealTime *bool   `json:"tracks_real_time"`
		RealTimeZone   *string `json:"real_time_zone"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	if err := h.svc.UpdateCalendar(ctx, cal.ID, UpdateCalendarInput{
		Name:             req.Name,
		Description:      req.Description,
		EpochName:        req.EpochName,
		CurrentYear:      req.CurrentYear,
		CurrentMonth:     req.CurrentMonth,
		CurrentDay:       req.CurrentDay,
		CurrentHour:      req.CurrentHour,
		CurrentMinute:    req.CurrentMinute,
		HoursPerDay:      req.HoursPerDay,
		MinutesPerHour:   req.MinutesPerHour,
		SecondsPerMinute: req.SecondsPerMinute,
		LeapYearEvery:    req.LeapYearEvery,
		LeapYearOffset:   req.LeapYearOffset,
		SetRealTime:      req.TracksRealTime,
		RealTimeZone:     req.RealTimeZone,
	}); err != nil {
		return err
	}
	auditMeta := map[string]any{
		"current_year": req.CurrentYear, "current_month": req.CurrentMonth, "current_day": req.CurrentDay,
		"hours_per_day": req.HoursPerDay, "minutes_per_hour": req.MinutesPerHour,
	}
	// Enabling/disabling real-time tracking changes the calendar's date authority,
	// so record the toggle (and the anchor zone when enabling) in the audit trail.
	if req.TracksRealTime != nil {
		auditMeta["tracks_real_time"] = *req.TracksRealTime
		if *req.TracksRealTime && req.RealTimeZone != nil {
			auditMeta["real_time_zone"] = *req.RealTimeZone
		}
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarUpdated, "calendar", cal.ID, req.Name, auditMeta)
	return nil
}

// UpdateMonthsAPI replaces all months.
// PUT /campaigns/:id/calendars/:calId/months
func (h *Handler) UpdateMonthsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var months []MonthInput
	if err := c.Bind(&months); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	if err := h.svc.SetMonths(ctx, cal.ID, months); err != nil {
		return err
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarMonthsSet, "calendar", cal.ID, cal.Name,
		map[string]any{"count": len(months)})
	return nil
}

// UpdateWeekdaysAPI replaces all weekdays.
// PUT /campaigns/:id/calendars/:calId/weekdays
func (h *Handler) UpdateWeekdaysAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var weekdays []WeekdayInput
	if err := c.Bind(&weekdays); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	if err := h.svc.SetWeekdays(ctx, cal.ID, weekdays); err != nil {
		return err
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarWeekdaysSet, "calendar", cal.ID, cal.Name,
		map[string]any{"count": len(weekdays)})
	return nil
}

// UpdateMoonsAPI replaces all moons.
// PUT /campaigns/:id/calendars/:calId/moons
func (h *Handler) UpdateMoonsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var moons []MoonInput
	if err := c.Bind(&moons); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	if err := h.svc.SetMoons(ctx, cal.ID, moons); err != nil {
		return err
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarMoonsSet, "calendar", cal.ID, cal.Name,
		map[string]any{"count": len(moons)})
	return nil
}

// CreateEventAPI creates a new event.
// POST /campaigns/:id/calendars/:calId/events
func (h *Handler) CreateEventAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	var cal *Calendar
	var err error
	if calID != "" {
		cal, err = h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
		if err != nil {
			return err
		}
	} else {
		// No calendar specified: use the default calendar (for dashboard widget quick-add).
		cal, err = h.svc.GetCalendar(ctx, cc.Campaign.ID)
		if err != nil {
			return err
		}
		if cal == nil {
			return apperror.NewNotFound("no default calendar found")
		}
	}

	var req struct {
		Name               string  `json:"name"`
		Description        *string `json:"description"`
		DescriptionHTML    *string `json:"description_html"`
		EntityID           *string `json:"entity_id"`
		Year               int     `json:"year"`
		Month              int     `json:"month"`
		Day                int     `json:"day"`
		StartHour          *int    `json:"start_hour"`
		StartMinute        *int    `json:"start_minute"`
		EndYear            *int    `json:"end_year"`
		EndMonth           *int    `json:"end_month"`
		EndDay             *int    `json:"end_day"`
		EndHour            *int    `json:"end_hour"`
		EndMinute          *int    `json:"end_minute"`
		IsRecurring        bool    `json:"is_recurring"`
		RecurrenceType     *string `json:"recurrence_type"`
		RecurrenceInterval *int    `json:"recurrence_interval"`
		Visibility         string  `json:"visibility"`
		VisibilityRules    *string `json:"visibility_rules"`
		Category           *string `json:"category"`
		// Tier + AllDay: internal-UI-only binding completion (C-CAL-LARGE-EDITOR).
		// The columns (calendar_events.tier / .all_day) and the service inputs
		// already existed; only this handler's request struct was dropping them,
		// so the editor drawer's tier segment + all-day toggle had no wire. No
		// schema, no new endpoint, no external-module-API change (APIHandler is
		// untouched). Tier is a campaign tier-definition slug ("" = platform
		// default); AllDay pairs with the drawer's nil-times all-day model.
		Tier   *string `json:"tier"`
		AllDay bool    `json:"all_day"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	// Get user ID from session context.
	userID := ""
	if session := c.Get("session"); session != nil {
		if s, ok := session.(interface{ GetUserID() string }); ok {
			userID = s.GetUserID()
		}
	}

	// Only co-DMs (Owner or DM-grantee) can author dm_only events; everyone
	// else is downgraded to 'everyone'. Co-DM capability (C-CAL-COGM-CAPABILITY).
	visibility := req.Visibility
	if visibility == "dm_only" && !cc.CanAuthorDmOnly() && !cc.IsSiteAdmin {
		visibility = "everyone"
	}

	evt, err := h.svc.CreateEvent(ctx, cal.ID, CreateEventInput{
		Name:               req.Name,
		Description:        req.Description,
		DescriptionHTML:    req.DescriptionHTML,
		EntityID:           req.EntityID,
		Year:               req.Year,
		Month:              req.Month,
		Day:                req.Day,
		StartHour:          req.StartHour,
		StartMinute:        req.StartMinute,
		EndYear:            req.EndYear,
		EndMonth:           req.EndMonth,
		EndDay:             req.EndDay,
		EndHour:            req.EndHour,
		EndMinute:          req.EndMinute,
		IsRecurring:        req.IsRecurring,
		RecurrenceType:     req.RecurrenceType,
		RecurrenceInterval: req.RecurrenceInterval,
		Visibility:         visibility,
		VisibilityRules:    req.VisibilityRules,
		Category:           req.Category,
		Tier:               req.Tier,
		AllDay:             req.AllDay,
		CreatedBy:          userID,
	})
	if err != nil {
		return err
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarEventCreated, "calendar_event", evt.ID, evt.Name,
		map[string]any{"calendar_id": cal.ID, "year": req.Year, "month": req.Month, "day": req.Day,
			"is_recurring": req.IsRecurring, "visibility": visibility})

	return c.JSON(http.StatusCreated, evt)
}

// requireEventInCampaign fetches an event and verifies its calendar belongs to
// the given campaign. Returns 404 for cross-campaign IDOR attempts.
func (h *Handler) requireEventInCampaign(c echo.Context, eventID, campaignID string) (*Event, error) {
	ctx := c.Request().Context()
	evt, err := h.svc.GetEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	// Verify event's calendar belongs to this campaign.
	cal, err := h.svc.GetCalendarByID(ctx, evt.CalendarID)
	if err != nil || cal == nil || cal.CampaignID != campaignID {
		return nil, apperror.NewNotFound("event not found")
	}
	return evt, nil
}

// eventEditorRecord is what GET .../events/:eid returns, and it is a DELIBERATE
// SUBSET of Event rather than the aggregate (C-CALV4-DAYCARD, R2-2a, [DC-8]).
//
// ONLY FIELDS THE EDITOR WRITES BACK. No campaign data, no member list, no
// roster, no audience resolution, no calendar structure, no created_by, no
// timestamps, no collect_rsvps (that flag is deliberately OFF the shared update
// path so a lossless quick-save cannot clobber it, and handing it to the editor
// is the first step towards it being sent back). This is not a general-purpose
// event API and it must not become one — returning Event directly is exactly
// how it would.
//
// THE TWO AUDIENCE FIELDS ARE GATED A SECOND TIME, INSIDE THE BODY. The route's
// floor is RolePlayer because a player may read an event they can see, but
// `visibility_rules` NAMES OTHER USERS. Both ride the floor that WRITES the
// event — Scribe, which is the shipped PUT …/events/:eid floor and which
// already accepts both fields in its request body. A PLAYER RECEIVES NEITHER
// KEY AT ALL: permission is absence, applied to a route body rather than to
// markup.
//
// THEY ARE GATED TOGETHER RATHER THAN SPLIT, and that is a deliberate choice
// with a stated reason. A Scribe may edit an event, and the shipped PUT
// re-writes the whole record — so withholding `visibility_rules` from the
// person who is about to overwrite it does not protect the audience, it
// DESTROYS it, silently, on the first save of a restricted event. The editor
// round-trips what it does not offer, and it can only do that with what it was
// given.
type eventEditorRecord struct {
	ID              string  `json:"id"`
	CalendarID      string  `json:"calendar_id"`
	Name            string  `json:"name"`
	Description     *string `json:"description,omitempty"`
	DescriptionHTML *string `json:"description_html,omitempty"`
	EntityID        *string `json:"entity_id,omitempty"`
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
	AllDay          bool    `json:"all_day"`
	Category        *string `json:"category,omitempty"`
	Visibility      string  `json:"visibility,omitempty"`
	VisibilityRules *string `json:"visibility_rules,omitempty"`
	// RECURRENCE RIDES BECAUSE THE EDITOR MUST SEND IT BACK, and this is the
	// R2 fix-forward (DC2-RECUR-DATALOSS): the record shipped without it and
	// the editor therefore could not round-trip it even in principle.
	//
	// `is_recurring` is the field that makes this load-bearing rather than
	// tidy. It is a VALUE-typed bool on the shipped PUT's request struct and
	// service.UpdateEvent writes it UNGUARDED, deliberately — "IsRecurring —
	// bool: false IS the value, not 'absent'" (service.go, C-CAL-NULL-PRESERVE).
	// So a body that OMITS the key sends false, and a title-only save through
	// this editor would un-repeat a recurring event AND leave the row in the
	// half-state C-CAL-RECURRING-PARTIAL-STATE-CLEANUP already had to clean up
	// once: is_recurring=false with recurrence_type/interval/end_* still
	// populated. The nil-guarded pointer siblings around it cannot save it,
	// because they preserve exactly the fields that make the half-state.
	//
	// This is the SAME discipline the two audience fields above are gated
	// under, stated once more in the direction that bites: the editor
	// round-trips what it does not offer, and it can only do that with what it
	// was given. It authors none of these three in this stage (§5's table marks
	// recurrence PARTIAL, and the exotic units are chipped GM-side) — carrying
	// them is what makes NOT authoring them lossless instead of destructive.
	//
	// The three END/MAX fields are NOT here on purpose: the shipped PUT does
	// not bind them at all, so the service's nil-guard preserves them without
	// the client's help, and a field the editor cannot write back has no
	// business on a record whose law is "only fields the editor writes back".
	//
	// UNGATED, unlike the audience pair: a recurrence rule names no user and
	// resolves no audience. It is the same class as `description`, which is
	// also ungated, and a player has no editor to feed it to in any case.
	IsRecurring        bool    `json:"is_recurring"`
	RecurrenceType     *string `json:"recurrence_type,omitempty"`
	RecurrenceInterval *int    `json:"recurrence_interval,omitempty"`
}

// newEventEditorRecord projects one event for one viewer's authoring floor.
func newEventEditorRecord(e Event, canAuthor bool) eventEditorRecord {
	rec := eventEditorRecord{
		ID: e.ID, CalendarID: e.CalendarID, Name: e.Name,
		Description: e.Description, DescriptionHTML: e.DescriptionHTML,
		EntityID: e.EntityID,
		Year:     e.Year, Month: e.Month, Day: e.Day,
		StartHour: e.StartHour, StartMinute: e.StartMinute,
		EndYear: e.EndYear, EndMonth: e.EndMonth, EndDay: e.EndDay,
		EndHour: e.EndHour, EndMinute: e.EndMinute,
		AllDay:  e.AllDay, Category: e.Category,
		IsRecurring:    e.IsRecurring,
		RecurrenceType: e.RecurrenceType, RecurrenceInterval: e.RecurrenceInterval,
	}
	if canAuthor {
		rec.Visibility = e.Visibility
		rec.VisibilityRules = e.VisibilityRules
	}
	return rec
}

// GetEventAPI returns ONE event record for the v4 event editor.
// GET /campaigns/:id/calendars/:calId/events/:eid
//
// HIDDEN, FILTERED AND MISSING ARE THE SAME ANSWER, FROM ONE BRANCH AND ONE
// BODY. The W5a split (app_dashboard.go:96 — "UNIFYING THE TWO REOPENS THE W5a
// LEAK") applies here in its strictest form: an event on a calendar this viewer
// cannot see, an event their own filter removes, an event whose :calId does not
// own it, and an event id that does not exist must be INDISTINGUISHABLE. Not
// 403-vs-404, not a different message, not a different shape. Every refusal
// below returns this one value, deliberately constructed once.
//
// IDOR IS CLOSED TWICE, exactly as UpdateEventAPI closes it once:
// requireVisibleCalendar resolves :calId inside this campaign AND applies the
// per-calendar visibility gate, and requireEventInCampaign resolves :eid inside
// this campaign. The route must not trust a :calId that does not own :eid, so
// the ownership is then checked directly rather than inferred.
func (h *Handler) GetEventAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil || cc.Campaign == nil {
		return apperror.NewMissingContext()
	}
	// The ONE refusal. Built here so every path below returns the same value.
	notFound := apperror.NewNotFound("event not found")

	cal, err := h.requireVisibleCalendar(c, c.Param("calId"), cc.Campaign.ID)
	if err != nil {
		return notFound
	}
	evt, err := h.requireEventInCampaign(c, c.Param("eid"), cc.Campaign.ID)
	if err != nil {
		return notFound
	}
	if evt.CalendarID != cal.ID {
		return notFound
	}
	// THE SAME VIEWER FILTER THE GRID USES. filterEventsByUser compacts IN
	// PLACE, so it is handed a fresh one-element slice and the result is read
	// rather than the input (the COMMON §7 slice trap, in miniature).
	if len(filterEventsByUser([]Event{*evt}, cc.VisibilityRole(), auth.GetUserID(c))) == 0 {
		return notFound
	}
	return c.JSON(http.StatusOK, newEventEditorRecord(*evt, cc.MemberRole >= campaigns.RoleScribe))
}

// UpdateEventAPI updates an existing event.
// PUT /campaigns/:id/calendar/events/:eid
func (h *Handler) UpdateEventAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	eventID := c.Param("eid")

	// IDOR protection: verify event belongs to this campaign's calendar.
	if _, err := h.requireEventInCampaign(c, eventID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
		Name               string  `json:"name"`
		Description        *string `json:"description"`
		DescriptionHTML    *string `json:"description_html"`
		EntityID           *string `json:"entity_id"`
		Year               int     `json:"year"`
		Month              int     `json:"month"`
		Day                int     `json:"day"`
		StartHour          *int    `json:"start_hour"`
		StartMinute        *int    `json:"start_minute"`
		EndYear            *int    `json:"end_year"`
		EndMonth           *int    `json:"end_month"`
		EndDay             *int    `json:"end_day"`
		EndHour            *int    `json:"end_hour"`
		EndMinute          *int    `json:"end_minute"`
		IsRecurring        bool    `json:"is_recurring"`
		RecurrenceType     *string `json:"recurrence_type"`
		RecurrenceInterval *int    `json:"recurrence_interval"`
		Visibility         string  `json:"visibility"`
		VisibilityRules    *string `json:"visibility_rules"`
		Category           *string `json:"category"`
		// Tier + AllDay: internal-UI-only binding completion (C-CAL-LARGE-EDITOR).
		// See CreateEventAPI for the rationale — existing columns/inputs, no
		// schema, no new endpoint, external module API untouched.
		Tier   *string `json:"tier"`
		AllDay bool    `json:"all_day"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	// Only co-DMs (Owner or DM-grantee) can set dm_only visibility; everyone
	// else is downgraded to 'everyone'. Co-DM capability (C-CAL-COGM-CAPABILITY).
	visibility := req.Visibility
	if visibility == "dm_only" && !cc.CanAuthorDmOnly() && !cc.IsSiteAdmin {
		visibility = "everyone"
	}

	if err := h.svc.UpdateEvent(ctx, eventID, UpdateEventInput{
		Name:               req.Name,
		Description:        req.Description,
		DescriptionHTML:    req.DescriptionHTML,
		EntityID:           req.EntityID,
		Year:               req.Year,
		Month:              req.Month,
		Day:                req.Day,
		StartHour:          req.StartHour,
		StartMinute:        req.StartMinute,
		EndYear:            req.EndYear,
		EndMonth:           req.EndMonth,
		EndDay:             req.EndDay,
		EndHour:            req.EndHour,
		EndMinute:          req.EndMinute,
		IsRecurring:        req.IsRecurring,
		RecurrenceType:     req.RecurrenceType,
		RecurrenceInterval: req.RecurrenceInterval,
		Visibility:         visibility,
		VisibilityRules:    req.VisibilityRules,
		Category:           req.Category,
		Tier:               req.Tier,
		AllDay:             req.AllDay,
	}); err != nil {
		return err
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarEventUpdated, "calendar_event", eventID, req.Name,
		map[string]any{"year": req.Year, "month": req.Month, "day": req.Day, "visibility": visibility})
	return nil
}

// DeleteEventAPI deletes an event.
// DELETE /campaigns/:id/calendar/events/:eid
func (h *Handler) DeleteEventAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	eventID := c.Param("eid")

	// IDOR protection: verify event belongs to this campaign's calendar.
	evt, err := h.requireEventInCampaign(c, eventID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	if err := h.svc.DeleteEvent(ctx, eventID); err != nil {
		return err
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarEventDeleted, "calendar_event", eventID, evt.Name,
		map[string]any{"calendar_id": evt.CalendarID, "year": evt.Year, "month": evt.Month, "day": evt.Day})
	return c.NoContent(http.StatusOK)
}

// UpdateEventVisibilityAPI updates the visibility and per-user rules for a calendar event.
// PUT /campaigns/:id/calendar/events/:eid/visibility
func (h *Handler) UpdateEventVisibilityAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	eventID := c.Param("eid")

	// IDOR protection: verify event belongs to this campaign's calendar.
	evt, err := h.requireEventInCampaign(c, eventID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var input UpdateEventVisibilityInput
	if err := c.Bind(&input); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	if err := h.svc.UpdateEventVisibility(ctx, eventID, input); err != nil {
		return err
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarEventVisibilityChanged, "calendar_event", eventID, evt.Name,
		map[string]any{"old_visibility": evt.Visibility, "new_visibility": input.Visibility})
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateCalendarVisibilityAPI updates a calendar's per-calendar visibility +
// allow/deny rules (C-CAL-DASHBOARD-W5b). PUT /campaigns/:id/calendars/:calId/visibility.
// Route-gated to Owner/co-DM (CanControlWorldState — same population the
// worldstate PUT uses); IDOR-guarded via requireCalendarInCampaign.
func (h *Handler) UpdateCalendarVisibilityAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var input UpdateCalendarVisibilityInput
	if err := c.Bind(&input); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	if err := h.svc.UpdateCalendarVisibility(ctx, calID, input); err != nil {
		return err
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarVisibilityChanged, "calendar", calID, cal.Name,
		map[string]any{"old_visibility": cal.Visibility, "new_visibility": input.Visibility})
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateSeasonsAPI replaces all seasons.
// PUT /campaigns/:id/calendars/:calId/seasons
func (h *Handler) UpdateSeasonsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var seasons []Season
	if err := c.Bind(&seasons); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	if err := h.svc.SetSeasons(ctx, cal.ID, seasons); err != nil {
		return err
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarSeasonsSet, "calendar", cal.ID, cal.Name,
		map[string]any{"count": len(seasons)})
	return nil
}

// UpdateErasAPI replaces all eras.
// PUT /campaigns/:id/calendars/:calId/eras
func (h *Handler) UpdateErasAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var eras []EraInput
	if err := c.Bind(&eras); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	if err := h.svc.SetEras(ctx, cal.ID, eras); err != nil {
		return err
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarErasSet, "calendar", cal.ID, cal.Name,
		map[string]any{"count": len(eras)})
	return nil
}

// --- C-CAL-WCF-UI: weather / cycles / festivals settings handlers ---
//
// Internal UI bindings for sub-resources that previously only had
// syncapi (Foundry-facing) endpoints. The data layer + service + repo
// were already shipped; this layer is the HTTP wiring the Chronicle
// web UI calls.
//
// All three responders route validation errors through
// respondSettingsError so the inline error region in each form gets
// the structured `{ error, message, category }` body it expects.
// Untyped errors fall through to Echo's framework handler (the
// existing safety net).

// UpdateWeatherAPI sets the current weather state for a calendar.
// PUT /campaigns/:id/calendars/:calId/weather
//
// PUT-only on purpose: GetWeather is implicit in ShowSettings, which
// eager-loads cal.Weather. The form re-renders from that state.
func (h *Handler) UpdateWeatherAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return respondSettingsError(c, err)
	}

	var input WeatherInput
	if err := c.Bind(&input); err != nil {
		return respondSettingsError(c, apperror.NewBadRequest("invalid weather payload"))
	}

	if err := h.svc.SetWeather(ctx, cal.ID, input); err != nil {
		return respondSettingsError(c, err)
	}
	details := map[string]any{}
	if input.PresetID != nil {
		details["preset_id"] = *input.PresetID
	}
	if input.TemperatureCelsius != nil {
		details["temperature_celsius"] = *input.TemperatureCelsius
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarWeatherSet, "calendar", cal.ID, cal.Name, details)
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// GetWeatherZonesAPI returns the per-calendar zone catalog plus the
// active-zone reference.
// GET /campaigns/:id/calendars/:calId/weather/zones
func (h *Handler) GetWeatherZonesAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return respondSettingsError(c, err)
	}

	state, err := h.svc.GetWeatherZones(ctx, cal.ID)
	if err != nil {
		return respondSettingsError(c, err)
	}
	if state.Zones == nil {
		state.Zones = []WeatherZone{}
	}
	return c.JSON(http.StatusOK, state)
}

// UpdateWeatherZonesAPI replaces the catalog and optionally updates
// the active-zone reference (REPLACE semantics matching the other
// settings PUTs). A non-empty active_zone must point at one of the
// supplied zones.
// PUT /campaigns/:id/calendars/:calId/weather/zones
func (h *Handler) UpdateWeatherZonesAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return respondSettingsError(c, err)
	}

	var state WeatherZonesState
	if err := c.Bind(&state); err != nil {
		return respondSettingsError(c, apperror.NewBadRequest("invalid weather zones payload"))
	}

	if err := h.svc.SetWeatherZones(ctx, cal.ID, state); err != nil {
		return respondSettingsError(c, err)
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarWeatherZonesSet, "calendar", cal.ID, cal.Name,
		map[string]any{"zone_count": len(state.Zones), "active_zone": state.ActiveZone})
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateCyclesAPI replaces all cycles for a calendar.
// PUT /campaigns/:id/calendars/:calId/cycles
func (h *Handler) UpdateCyclesAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return respondSettingsError(c, err)
	}

	var cycles []CycleInput
	if err := c.Bind(&cycles); err != nil {
		return respondSettingsError(c, apperror.NewBadRequest("invalid cycles payload"))
	}

	if err := h.svc.SetCycles(ctx, cal.ID, cycles); err != nil {
		return respondSettingsError(c, err)
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarCyclesSet, "calendar", cal.ID, cal.Name,
		map[string]any{"count": len(cycles)})
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateFestivalsAPI replaces all festivals for a calendar.
// PUT /campaigns/:id/calendars/:calId/festivals
//
// Operator-confirmed (2026-05-19): festivals are first-class entries
// on the calendar structure, distinct from the orphaned festival-as-
// Character entities surfaced in Issue #19 (operator-side cleanup).
func (h *Handler) UpdateFestivalsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return respondSettingsError(c, err)
	}

	var festivals []FestivalInput
	if err := c.Bind(&festivals); err != nil {
		return respondSettingsError(c, apperror.NewBadRequest("invalid festivals payload"))
	}

	if err := h.svc.SetFestivals(ctx, cal.ID, festivals); err != nil {
		return respondSettingsError(c, err)
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarFestivalsSet, "calendar", cal.ID, cal.Name,
		map[string]any{"count": len(festivals)})
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateEventCategoriesAPI replaces all event categories.
// PUT /campaigns/:id/calendars/:calId/event-categories
func (h *Handler) UpdateEventCategoriesAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var cats []EventCategoryInput
	if err := c.Bind(&cats); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	if err := h.svc.SetEventCategories(ctx, cal.ID, cats); err != nil {
		return err
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarCategoriesSet, "calendar", cal.ID, cal.Name,
		map[string]any{"count": len(cats)})
	return nil
}

// GetEventCategoriesAPI returns all event categories for a calendar.
// GET /campaigns/:id/calendars/:calId/event-categories
func (h *Handler) GetEventCategoriesAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	cats, err := h.svc.GetEventCategories(ctx, cal.ID)
	if err != nil {
		return err
	}
	if cats == nil {
		cats = []EventCategory{}
	}
	return c.JSON(http.StatusOK, cats)
}

// DeleteCalendarAPI removes the calendar and all its data.
// DELETE /campaigns/:id/calendars/:calId
func (h *Handler) DeleteCalendarAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	if err := h.svc.DeleteCalendar(ctx, calID); err != nil {
		return err
	}
	epochName := ""
	if cal.EpochName != nil {
		epochName = *cal.EpochName
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarDeleted, "calendar", calID, cal.Name,
		map[string]any{"mode": string(cal.Mode), "epoch_name": epochName, "current_year": cal.CurrentYear})
	return c.NoContent(http.StatusOK)
}

// ShowSettings renders the calendar settings page.
// GET /campaigns/:id/calendars/:calId/settings
func (h *Handler) ShowSettings(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	csrfToken := middleware.GetCSRFToken(c)
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, CalendarSettingsFragment(cc, cal, csrfToken))
	}
	return middleware.Render(c, http.StatusOK, CalendarSettingsPage(cc, cal, csrfToken))
}

// AdvanceDateAPI moves the current date forward by N days.
// POST /campaigns/:id/calendars/:calId/advance
func (h *Handler) AdvanceDateAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var req struct {
		Days int `json:"days"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if req.Days < 1 || req.Days > 3650 {
		return apperror.NewBadRequest("days must be between 1 and 3650")
	}

	if err := h.svc.AdvanceDate(ctx, cal.ID, req.Days); err != nil {
		return err
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarDateAdvanced, "calendar", cal.ID, cal.Name,
		map[string]any{"days": req.Days})
	return nil
}

// AdvanceTimeAPI moves the current time forward by hours and/or minutes,
// rolling over into days as needed.
// POST /campaigns/:id/calendars/:calId/advance-time
func (h *Handler) AdvanceTimeAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var req struct {
		Hours   int `json:"hours"`
		Minutes int `json:"minutes"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if req.Hours < 0 || req.Minutes < 0 {
		return apperror.NewBadRequest("hours and minutes must be non-negative")
	}
	if req.Hours == 0 && req.Minutes == 0 {
		return apperror.NewBadRequest("must advance by at least 1 minute or 1 hour")
	}
	if req.Hours > 87600 { // ~10 years of 24-hour days
		return apperror.NewBadRequest("hours must be at most 87600")
	}

	if err := h.svc.AdvanceTime(ctx, cal.ID, req.Hours, req.Minutes); err != nil {
		return err
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarTimeAdvanced, "calendar", cal.ID, cal.Name,
		map[string]any{"hours": req.Hours, "minutes": req.Minutes})
	return nil
}

// (EntityEventsFragment — the per-entity calendar-events HTMX fragment — was
// retired in C-CAL-EMBED-CONVERGE-POLISH. The per-entity calendar is now the
// `entity_calendar` registry block (worldstate band + #402 linked events);
// the old auto-appended events list on every entity page is gone with it.)

// UpcomingEventsFragment returns an HTMX fragment with upcoming calendar events.
// Used by the calendar_preview dashboard block via lazy-loading.
// Supports both /calendars/:calId/upcoming and /calendars/upcoming (default calendar).
// GET /campaigns/:id/calendars/:calId/upcoming
func (h *Handler) UpcomingEventsFragment(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	calID := c.Param("calId")
	var cal *Calendar
	var err error
	if calID != "" {
		cal, err = h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
		if err != nil {
			return middleware.Render(c, http.StatusOK, UpcomingEventsEmpty())
		}
	} else {
		cal, err = h.svc.GetCalendar(ctx, cc.Campaign.ID)
		if err != nil {
			return err
		}
	}
	if cal == nil {
		return middleware.Render(c, http.StatusOK, UpcomingEventsEmpty())
	}

	limit := 5
	if q := c.QueryParam("limit"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v >= 1 && v <= 20 {
			limit = v
		}
	}

	role := cc.VisibilityRole()
	userID := auth.GetUserID(c)
	events, err := h.svc.ListUpcomingEvents(ctx, cal.ID, limit, role, userID)
	if err != nil {
		return err
	}

	return middleware.Render(c, http.StatusOK, UpcomingEventsBlock(cc, cal, events))
}

// ShowTimeline renders the timeline (list) view of calendar events.
// GET /campaigns/:id/calendars/:calId/timeline
func (h *Handler) ShowTimeline(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	// Default to current year, allow override via query param.
	year := cal.CurrentYear
	if q := c.QueryParam("year"); q != "" {
		if v, err := strconv.Atoi(q); err == nil {
			year = v
		}
	}

	role := cc.VisibilityRole()
	userID := auth.GetUserID(c)
	events, err := h.svc.ListEventsForYear(ctx, cal.ID, year, role, userID)
	if err != nil {
		return err
	}

	data := TimelineViewData{
		Calendar:        cal,
		Year:            year,
		Events:          events,
		CampaignID:      cc.Campaign.ID,
		IsOwner:         cc.MemberRole >= campaigns.RoleOwner,
		IsScribe:        cc.MemberRole >= campaigns.RoleScribe,
		CanAuthorDmOnly: cc.CanAuthorDmOnly(),
		CSRFToken:       middleware.GetCSRFToken(c),
	}

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, TimelineFragment(cc, data))
	}
	return middleware.Render(c, http.StatusOK, TimelinePage(cc, data))
}

// ExportCalendarAPI returns the calendar as a downloadable JSON file.
// GET /campaigns/:id/calendars/:calId/export
func (h *Handler) ExportCalendarAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	// Optionally include events.
	var events []Event
	includeEvents := c.QueryParam("events") == "true"
	if includeEvents {
		events, err = h.svc.ListAllEvents(ctx, cal.ID)
		if err != nil {
			slog.Error("export: failed to list events", slog.Any("error", err))
		}
	}

	export := BuildExport(cal, events, includeEvents)
	c.Response().Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s-calendar.json"`, cc.Campaign.Slug))
	return c.JSON(http.StatusOK, export)
}

// ImportCalendarAPI handles calendar import from an uploaded JSON file.
// Accepts Simple Calendar, Calendaria, Fantasy-Calendar, and Chronicle formats.
// POST /campaigns/:id/calendars/:calId/import
func (h *Handler) ImportCalendarAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calId")

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	// Read uploaded file (multipart form or raw JSON body).
	var data []byte
	file, fileErr := c.FormFile("file")
	if fileErr == nil {
		// Multipart upload.
		src, openErr := file.Open()
		if openErr != nil {
			return apperror.NewBadRequest("could not read uploaded file")
		}
		defer func() { _ = src.Close() }()
		data, err = io.ReadAll(io.LimitReader(src, 10*1024*1024)) // 10MB limit
		if err != nil {
			return apperror.NewBadRequest("could not read uploaded file")
		}
	} else {
		// Try raw JSON body.
		data, err = io.ReadAll(io.LimitReader(c.Request().Body, 10*1024*1024))
		if err != nil || len(data) == 0 {
			return apperror.NewBadRequest("no file uploaded and no JSON body")
		}
	}

	// Parse and detect format.
	result, parseErr := DetectAndParse(data)
	if parseErr != nil {
		return apperror.NewBadRequest(parseErr.Error())
	}

	// Check for preview mode — return what would be imported without applying.
	if c.QueryParam("preview") == "true" {
		return c.JSON(http.StatusOK, result)
	}

	// Apply the import to the existing calendar.
	if err := h.svc.ApplyImport(ctx, cal.ID, result); err != nil {
		slog.Error("import: failed to apply", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to apply import"))
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarImported, "calendar", cal.ID, cal.Name,
		map[string]any{
			"format":   result.Format,
			"months":   len(result.Months),
			"weekdays": len(result.Weekdays),
			"moons":    len(result.Moons),
			"seasons":  len(result.Seasons),
			"eras":     len(result.Eras),
		})

	// Return JSON response with summary.
	return c.JSON(http.StatusOK, map[string]any{
		"status":   "ok",
		"format":   result.Format,
		"name":     result.CalendarName,
		"months":   len(result.Months),
		"weekdays": len(result.Weekdays),
		"moons":    len(result.Moons),
		"seasons":  len(result.Seasons),
		"eras":     len(result.Eras),
	})
}

// ImportPreviewAPI returns a preview of what would be imported from a JSON file
// without actually applying the changes. Used by the import UI for confirmation.
// POST /campaigns/:id/calendar/import/preview
func (h *Handler) ImportPreviewAPI(c echo.Context) error {
	// Read uploaded file.
	var data []byte
	var err error
	file, fileErr := c.FormFile("file")
	if fileErr == nil {
		src, openErr := file.Open()
		if openErr != nil {
			return apperror.NewBadRequest("could not read uploaded file")
		}
		defer func() { _ = src.Close() }()
		data, err = io.ReadAll(io.LimitReader(src, 10*1024*1024))
		if err != nil {
			return apperror.NewBadRequest("could not read uploaded file")
		}
	} else {
		data, err = io.ReadAll(io.LimitReader(c.Request().Body, 10*1024*1024))
		if err != nil || len(data) == 0 {
			return apperror.NewBadRequest("no file uploaded")
		}
	}

	result, parseErr := DetectAndParse(data)
	if parseErr != nil {
		return apperror.NewBadRequest(parseErr.Error())
	}

	// Return the parsed preview as JSON.
	return c.JSON(http.StatusOK, result)
}

// ImportFromSetupAPI handles import during calendar setup (no existing calendar).
// Creates a new calendar and applies the imported configuration.
// POST /campaigns/:id/calendars/import-setup
func (h *Handler) ImportFromSetupAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	// Read uploaded file.
	file, fileErr := c.FormFile("file")
	if fileErr != nil {
		return apperror.NewBadRequest("no file uploaded")
	}
	src, err := file.Open()
	if err != nil {
		return apperror.NewBadRequest("could not read uploaded file")
	}
	defer func() { _ = src.Close() }()
	data, err := io.ReadAll(io.LimitReader(src, 10*1024*1024))
	if err != nil {
		return apperror.NewBadRequest("could not read uploaded file")
	}

	// Parse the import.
	result, parseErr := DetectAndParse(data)
	if parseErr != nil {
		return apperror.NewBadRequest(parseErr.Error())
	}

	// Create a new fantasy calendar with the imported name.
	calName := result.CalendarName
	if calName == "" {
		calName = "Imported Calendar"
	}
	input := CreateCalendarInput{
		Mode:             ModeFantasy,
		Name:             calName,
		EpochName:        result.Settings.EpochName,
		CurrentYear:      result.Settings.CurrentYear,
		HoursPerDay:      result.Settings.HoursPerDay,
		MinutesPerHour:   result.Settings.MinutesPerHour,
		SecondsPerMinute: result.Settings.SecondsPerMinute,
		LeapYearEvery:    result.Settings.LeapYearEvery,
		LeapYearOffset:   result.Settings.LeapYearOffset,
	}

	cal, err := h.svc.CreateCalendar(ctx, cc.Campaign.ID, input)
	if err != nil {
		return err
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarCreated, "calendar", cal.ID, cal.Name,
		map[string]any{"mode": string(ModeFantasy), "via": "import_setup"})

	// Apply imported sub-resources.
	if err := h.svc.ApplyImport(ctx, cal.ID, result); err != nil {
		slog.Error("import-setup: failed to apply", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to apply import"))
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarImported, "calendar", cal.ID, cal.Name,
		map[string]any{
			"format":   result.Format,
			"months":   len(result.Months),
			"weekdays": len(result.Weekdays),
			"moons":    len(result.Moons),
			"seasons":  len(result.Seasons),
			"eras":     len(result.Eras),
		})

	// Auto-enable the calendar addon.
	if h.addonSvc != nil {
		addon, aErr := h.addonSvc.GetBySlug(ctx, "calendar")
		if aErr == nil && addon != nil {
			userID := auth.GetUserID(c)
			if eErr := h.addonSvc.EnableForCampaign(ctx, cc.Campaign.ID, addon.ID, userID); eErr != nil {
				slog.Warn("auto-enable calendar addon failed", slog.Any("error", eErr))
			}
		}
	}

	// Redirect to settings page so user can review the import.
	return c.Redirect(http.StatusSeeOther,
		fmt.Sprintf("/campaigns/%s/calendars/%s/settings", cc.Campaign.ID, cal.ID))
}

// Silence unused import warning for io package (used in request body reading).
var _ = io.ReadAll

// CalendarViewData holds all data needed to render the calendar grid.
type CalendarViewData struct {
	Calendar   *Calendar
	Year       int
	MonthIndex int // 1-based month index
	Events     []Event
	CampaignID string
	UserID     string
	IsOwner    bool
	IsScribe   bool
	// CanAuthorDmOnly: may the viewer create/mark dm_only content? Co-DM
	// capability (Owner OR DM-grantee) — C-CAL-COGM-CAPABILITY. Gates the
	// "DM Only" visibility option so a Scribe isn't offered an action the
	// server downgrades (the UI-lie fix). NOT the same as IsOwner: a co-DM
	// is not an Owner but can author secrets.
	CanAuthorDmOnly bool
	CSRFToken       string
}

// CurrentMonthDef returns the month definition for the current view month.
func (d CalendarViewData) CurrentMonthDef() *Month {
	idx := d.MonthIndex - 1
	if idx >= 0 && idx < len(d.Calendar.Months) {
		return &d.Calendar.Months[idx]
	}
	return nil
}

// CurrentMonthDays returns the number of days in the current month,
// accounting for leap years.
func (d CalendarViewData) CurrentMonthDays() int {
	return d.Calendar.MonthDays(d.MonthIndex-1, d.Year)
}

// CurrentSeason returns the season for a given day in the current month, or nil.
func (d CalendarViewData) CurrentSeason(day int) *Season {
	return d.Calendar.SeasonForDate(d.MonthIndex, day)
}

// PrevMonth returns year, month for the previous month (wrapping at year boundary).
func (d CalendarViewData) PrevMonth() (int, int) {
	m := d.MonthIndex - 1
	y := d.Year
	if m < 1 {
		m = len(d.Calendar.Months)
		y--
	}
	return y, m
}

// NextMonth returns year, month for the next month (wrapping at year boundary).
func (d CalendarViewData) NextMonth() (int, int) {
	m := d.MonthIndex + 1
	y := d.Year
	if m > len(d.Calendar.Months) {
		m = 1
		y++
	}
	return y, m
}

// EventsForDay returns events that fall on the given day.
func (d CalendarViewData) EventsForDay(day int) []Event {
	var result []Event
	for _, e := range d.Events {
		if e.Day == day {
			result = append(result, e)
		}
	}
	return result
}

// IsToday returns true if the given day/month/year matches the calendar's current date.
func (d CalendarViewData) IsToday(day int) bool {
	return d.Year == d.Calendar.CurrentYear &&
		d.MonthIndex == d.Calendar.CurrentMonth &&
		day == d.Calendar.CurrentDay
}

// AbsoluteDay calculates the total days from year 0 for moon phase computation.
func (d CalendarViewData) AbsoluteDay(day int) int {
	yearLength := d.Calendar.YearLength()
	total := d.Year * yearLength
	// Add days from months before current month.
	for i := 0; i < d.MonthIndex-1 && i < len(d.Calendar.Months); i++ {
		total += d.Calendar.Months[i].Days
	}
	total += day
	return total
}

// WeekdayIndex returns the weekday index (0-based) for a given day in the current month/year.
func (d CalendarViewData) WeekdayIndex(day int) int {
	wl := d.Calendar.WeekLength()
	if wl == 0 {
		return 0
	}
	absDay := d.AbsoluteDay(day)
	idx := absDay % wl
	if idx < 0 {
		idx += wl
	}
	return idx
}

// StartWeekdayOffset returns how many blank cells to render before day 1
// of the current month in the grid.
func (d CalendarViewData) StartWeekdayOffset() int {
	return d.WeekdayIndex(1)
}

// TimelineViewData holds data for the chronological timeline view.
type TimelineViewData struct {
	Calendar   *Calendar
	Year       int
	Events     []Event
	CampaignID string
	IsOwner    bool
	IsScribe   bool
	// CanAuthorDmOnly: co-DM capability (Owner or DM-grantee) —
	// C-CAL-COGM-CAPABILITY. Gates the dm_only authoring affordances.
	CanAuthorDmOnly bool
	CSRFToken       string
}

// MonthName returns the month name for a 1-based month index.
func (d TimelineViewData) MonthName(month int) string {
	if month >= 1 && month <= len(d.Calendar.Months) {
		return d.Calendar.Months[month-1].Name
	}
	return fmt.Sprintf("Month %d", month)
}

// EventsByMonth groups events by their month index for timeline rendering.
func (d TimelineViewData) EventsByMonth() []TimelineMonth {
	monthMap := make(map[int][]Event)
	for _, evt := range d.Events {
		monthMap[evt.Month] = append(monthMap[evt.Month], evt)
	}
	// Produce ordered slice.
	var result []TimelineMonth
	for m := 1; m <= len(d.Calendar.Months); m++ {
		if events, ok := monthMap[m]; ok {
			result = append(result, TimelineMonth{
				Index:  m,
				Name:   d.Calendar.Months[m-1].Name,
				Events: events,
			})
		}
	}
	return result
}

// TimelineMonth groups events under a month header for timeline display.
type TimelineMonth struct {
	Index  int
	Name   string
	Events []Event
}

// daysInGregorianMonth returns the number of days in a Gregorian calendar month.
func daysInGregorianMonth(year, month int) int {
	return time.Date(year, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
}
