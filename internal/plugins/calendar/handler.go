// Package calendar - handler.go binds HTTP requests to explicit input
// structs, calls the service, and renders the response. No business logic
// lives here — see routes.go for the route table and service.go for the
// rules each route ultimately enforces.
package calendar

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler processes HTTP requests for the calendar plugin.
type Handler struct {
	svc CalendarService
}

// NewHandler creates a new calendar Handler.
func NewHandler(svc CalendarService) *Handler {
	return &Handler{svc: svc}
}

// viewerFrom builds the request's permissions.Viewer (ADR-049): the
// campaign-membership-aware role (VisibilityRole promotes a co-DM grant to
// Owner-equivalent for content visibility) plus the authenticated user id,
// empty for an anonymous public-campaign visitor. Every read handler below
// builds its viewer this exact way so a viewer can never be constructed by
// hand with an accidentally-empty-means-trusted user id.
func viewerFrom(c echo.Context, cc *campaigns.CampaignContext) permissions.Viewer {
	return permissions.RequestViewer(cc.VisibilityRole(), auth.GetUserID(c))
}

// --- Calendars ---

// ListCalendarsAPI lists a campaign's calendars.
// GET /campaigns/:id/calendars
func (h *Handler) ListCalendarsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	cals, err := h.svc.ListCalendars(c.Request().Context(), cc.Campaign.ID, viewerFrom(c, cc))
	if err != nil {
		return err
	}
	if cals == nil {
		cals = []Calendar{}
	}
	return c.JSON(http.StatusOK, cals)
}

// GetCalendarAPI returns one calendar with its sub-resources.
// GET /campaigns/:id/calendars/:calid
func (h *Handler) GetCalendarAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	cal, err := h.svc.GetCalendarForViewer(c.Request().Context(), c.Param("calid"), cc.Campaign.ID, viewerFrom(c, cc))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, cal)
}

// CreateCalendarAPI creates a calendar. Owner only.
// POST /campaigns/:id/calendars
func (h *Handler) CreateCalendarAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	var req struct {
		Mode             string  `json:"mode"`
		Name             string  `json:"name"`
		Description      *string `json:"description"`
		EpochName        *string `json:"epoch_name"`
		CurrentYear      int     `json:"current_year"`
		HoursPerDay      int     `json:"hours_per_day"`
		MinutesPerHour   int     `json:"minutes_per_hour"`
		SecondsPerMinute int     `json:"seconds_per_minute"`
		LeapYearEvery    int     `json:"leap_year_every"`
		LeapYearOffset   int     `json:"leap_year_offset"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	cal, err := h.svc.CreateCalendar(c.Request().Context(), cc.Campaign.ID, CreateCalendarInput{
		Mode:             req.Mode,
		Name:             req.Name,
		Description:      req.Description,
		EpochName:        req.EpochName,
		CurrentYear:      req.CurrentYear,
		HoursPerDay:      req.HoursPerDay,
		MinutesPerHour:   req.MinutesPerHour,
		SecondsPerMinute: req.SecondsPerMinute,
		LeapYearEvery:    req.LeapYearEvery,
		LeapYearOffset:   req.LeapYearOffset,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, cal)
}

// UpdateCalendarAPI applies a partial settings update. Owner only.
// PUT /campaigns/:id/calendars/:calid
func (h *Handler) UpdateCalendarAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	var req struct {
		Name             string              `json:"name"`
		Description      patch.Field[string] `json:"description"`
		EpochName        patch.Field[string] `json:"epoch_name"`
		Mode             patch.Field[string] `json:"mode"`
		CurrentYear      patch.Field[int]    `json:"current_year"`
		CurrentMonth     patch.Field[int]    `json:"current_month"`
		CurrentDay       patch.Field[int]    `json:"current_day"`
		CurrentHour      patch.Field[int]    `json:"current_hour"`
		CurrentMinute    patch.Field[int]    `json:"current_minute"`
		HoursPerDay      patch.Field[int]    `json:"hours_per_day"`
		MinutesPerHour   patch.Field[int]    `json:"minutes_per_hour"`
		SecondsPerMinute patch.Field[int]    `json:"seconds_per_minute"`
		LeapYearEvery    patch.Field[int]    `json:"leap_year_every"`
		LeapYearOffset   patch.Field[int]    `json:"leap_year_offset"`
		SetRealTime      *bool               `json:"set_real_time"`
		RealTimeZone     *string             `json:"real_time_zone"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	return h.svc.UpdateCalendar(c.Request().Context(), c.Param("calid"), cc.Campaign.ID, UpdateCalendarInput{
		Name:             req.Name,
		Description:      req.Description,
		EpochName:        req.EpochName,
		Mode:             req.Mode,
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
		SetRealTime:      req.SetRealTime,
		RealTimeZone:     req.RealTimeZone,
	})
}

// DeleteCalendarAPI deletes a calendar. Owner only.
// DELETE /campaigns/:id/calendars/:calid
func (h *Handler) DeleteCalendarAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if err := h.svc.DeleteCalendar(c.Request().Context(), c.Param("calid"), cc.Campaign.ID); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// SetDefaultCalendarAPI marks a calendar as the campaign's default. Owner only.
// PUT /campaigns/:id/calendars/:calid/default
func (h *Handler) SetDefaultCalendarAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if err := h.svc.SetDefaultCalendar(c.Request().Context(), cc.Campaign.ID, c.Param("calid")); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// --- Events ---

// ListEventsAPI lists a calendar's events for one (year, month).
// GET /campaigns/:id/calendars/:calid/events?year=YYYY&month=M
func (h *Handler) ListEventsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	year, err := strconv.Atoi(c.QueryParam("year"))
	if err != nil {
		return apperror.NewBadRequest("year is required")
	}
	month, err := strconv.Atoi(c.QueryParam("month"))
	if err != nil {
		return apperror.NewBadRequest("month is required")
	}
	events, err := h.svc.ListEventsForMonth(c.Request().Context(), c.Param("calid"), cc.Campaign.ID, year, month, viewerFrom(c, cc))
	if err != nil {
		return err
	}
	if events == nil {
		events = []Event{}
	}
	return c.JSON(http.StatusOK, events)
}

// GetEventAPI returns one event.
// GET /campaigns/:id/calendars/:calid/events/:eid
func (h *Handler) GetEventAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	evt, err := h.svc.GetEventForViewer(c.Request().Context(), c.Param("eid"), c.Param("calid"), cc.Campaign.ID, viewerFrom(c, cc))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, evt)
}

// CreateEventAPI creates an event. Scribe+; the service refuses (403) a
// visibility=dm_only or a visibility_rules from a caller who may not author
// dm_only content (CalendarService.CreateEvent's doc comment).
// POST /campaigns/:id/calendars/:calid/events
func (h *Handler) CreateEventAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	var req struct {
		Name                     string  `json:"name"`
		Description              *string `json:"description"`
		DescriptionHTML          *string `json:"description_html"`
		EntityID                 *string `json:"entity_id"`
		Year                     int     `json:"year"`
		Month                    int     `json:"month"`
		Day                      int     `json:"day"`
		StartHour                *int    `json:"start_hour"`
		StartMinute              *int    `json:"start_minute"`
		EndYear                  *int    `json:"end_year"`
		EndMonth                 *int    `json:"end_month"`
		EndDay                   *int    `json:"end_day"`
		EndHour                  *int    `json:"end_hour"`
		EndMinute                *int    `json:"end_minute"`
		IsRecurring              bool    `json:"is_recurring"`
		RecurrenceType           *string `json:"recurrence_type"`
		RecurrenceInterval       *int    `json:"recurrence_interval"`
		RecurrenceEndYear        *int    `json:"recurrence_end_year"`
		RecurrenceEndMonth       *int    `json:"recurrence_end_month"`
		RecurrenceEndDay         *int    `json:"recurrence_end_day"`
		RecurrenceMaxOccurrences *int    `json:"recurrence_max_occurrences"`
		Visibility               string  `json:"visibility"`
		VisibilityRules          *string `json:"visibility_rules"`
		KindID                   *int    `json:"kind_id"`
		Announced                *string `json:"announced"`
		Tier                     *string `json:"tier"`
		Color                    *string `json:"color"`
		Icon                     *string `json:"icon"`
		AllDay                   bool    `json:"all_day"`
		Payload                  *string `json:"payload"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	evt, err := h.svc.CreateEvent(c.Request().Context(), c.Param("calid"), cc.Campaign.ID, CreateEventInput{
		CanAuthorDmOnly:          viewerFrom(c, cc).SkipsPerUserRules(),
		Name:                     req.Name,
		Description:              req.Description,
		DescriptionHTML:          req.DescriptionHTML,
		EntityID:                 req.EntityID,
		Year:                     req.Year,
		Month:                    req.Month,
		Day:                      req.Day,
		StartHour:                req.StartHour,
		StartMinute:              req.StartMinute,
		EndYear:                  req.EndYear,
		EndMonth:                 req.EndMonth,
		EndDay:                   req.EndDay,
		EndHour:                  req.EndHour,
		EndMinute:                req.EndMinute,
		IsRecurring:              req.IsRecurring,
		RecurrenceType:           req.RecurrenceType,
		RecurrenceInterval:       req.RecurrenceInterval,
		RecurrenceEndYear:        req.RecurrenceEndYear,
		RecurrenceEndMonth:       req.RecurrenceEndMonth,
		RecurrenceEndDay:         req.RecurrenceEndDay,
		RecurrenceMaxOccurrences: req.RecurrenceMaxOccurrences,
		Visibility:               req.Visibility,
		VisibilityRules:          req.VisibilityRules,
		KindID:                   req.KindID,
		Announced:                req.Announced,
		Tier:                     req.Tier,
		Color:                    req.Color,
		Icon:                     req.Icon,
		AllDay:                   req.AllDay,
		Payload:                  req.Payload,
		CreatedBy:                auth.GetUserID(c),
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, evt)
}

// UpdateEventAPI applies a partial update to an event. Scribe+; the service
// (CalendarService.UpdateEvent) refuses (403) a caller who may not author
// dm_only content changing visibility/visibility_rules to anything other
// than what is already stored, and answers NotFound for an event the caller
// cannot see at all.
// PUT /campaigns/:id/calendars/:calid/events/:eid
func (h *Handler) UpdateEventAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	var req struct {
		Name                     patch.Field[string] `json:"name"`
		Description              patch.Field[string] `json:"description"`
		DescriptionHTML          patch.Field[string] `json:"description_html"`
		EntityID                 patch.Field[string] `json:"entity_id"`
		Year                     patch.Field[int]    `json:"year"`
		Month                    patch.Field[int]    `json:"month"`
		Day                      patch.Field[int]    `json:"day"`
		StartHour                patch.Field[int]    `json:"start_hour"`
		StartMinute              patch.Field[int]    `json:"start_minute"`
		EndYear                  patch.Field[int]    `json:"end_year"`
		EndMonth                 patch.Field[int]    `json:"end_month"`
		EndDay                   patch.Field[int]    `json:"end_day"`
		EndHour                  patch.Field[int]    `json:"end_hour"`
		EndMinute                patch.Field[int]    `json:"end_minute"`
		IsRecurring              patch.Field[bool]   `json:"is_recurring"`
		RecurrenceType           patch.Field[string] `json:"recurrence_type"`
		RecurrenceInterval       patch.Field[int]    `json:"recurrence_interval"`
		RecurrenceEndYear        patch.Field[int]    `json:"recurrence_end_year"`
		RecurrenceEndMonth       patch.Field[int]    `json:"recurrence_end_month"`
		RecurrenceEndDay         patch.Field[int]    `json:"recurrence_end_day"`
		RecurrenceMaxOccurrences patch.Field[int]    `json:"recurrence_max_occurrences"`
		Visibility               patch.Field[string] `json:"visibility"`
		VisibilityRules          patch.Field[string] `json:"visibility_rules"`
		KindID                   patch.Field[int]    `json:"kind_id"`
		Announced                patch.Field[string] `json:"announced"`
		Tier                     patch.Field[string] `json:"tier"`
		Color                    patch.Field[string] `json:"color"`
		Icon                     patch.Field[string] `json:"icon"`
		AllDay                   patch.Field[bool]   `json:"all_day"`
		Payload                  patch.Field[string] `json:"payload"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	return h.svc.UpdateEvent(c.Request().Context(), c.Param("eid"), c.Param("calid"), cc.Campaign.ID, UpdateEventInput{
		Name:                     req.Name,
		Description:              req.Description,
		DescriptionHTML:          req.DescriptionHTML,
		EntityID:                 req.EntityID,
		Year:                     req.Year,
		Month:                    req.Month,
		Day:                      req.Day,
		StartHour:                req.StartHour,
		StartMinute:              req.StartMinute,
		EndYear:                  req.EndYear,
		EndMonth:                 req.EndMonth,
		EndDay:                   req.EndDay,
		EndHour:                  req.EndHour,
		EndMinute:                req.EndMinute,
		IsRecurring:              req.IsRecurring,
		RecurrenceType:           req.RecurrenceType,
		RecurrenceInterval:       req.RecurrenceInterval,
		RecurrenceEndYear:        req.RecurrenceEndYear,
		RecurrenceEndMonth:       req.RecurrenceEndMonth,
		RecurrenceEndDay:         req.RecurrenceEndDay,
		RecurrenceMaxOccurrences: req.RecurrenceMaxOccurrences,
		Visibility:               req.Visibility,
		VisibilityRules:          req.VisibilityRules,
		KindID:                   req.KindID,
		Announced:                req.Announced,
		Tier:                     req.Tier,
		Color:                    req.Color,
		Icon:                     req.Icon,
		AllDay:                   req.AllDay,
		Payload:                  req.Payload,
	}, viewerFrom(c, cc))
}

// DeleteEventAPI deletes an event. Owner only.
// DELETE /campaigns/:id/calendars/:calid/events/:eid
func (h *Handler) DeleteEventAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if err := h.svc.DeleteEvent(c.Request().Context(), c.Param("eid"), c.Param("calid"), cc.Campaign.ID, viewerFrom(c, cc)); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// SetEventVisibilityAPI is the dm_only toggle. Gated on
// campaigns.CanAuthorDmOnly (the Owner, or a co-DM with a grant), not
// RequireRole(Owner) — see routes.go.
// PUT /campaigns/:id/calendars/:calid/events/:eid/visibility
func (h *Handler) SetEventVisibilityAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	var req struct {
		Visibility      string              `json:"visibility"`
		VisibilityRules patch.Field[string] `json:"visibility_rules"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if err := h.svc.SetEventVisibility(c.Request().Context(), c.Param("eid"), c.Param("calid"), cc.Campaign.ID, UpdateEventVisibilityInput{
		Visibility:      req.Visibility,
		VisibilityRules: req.VisibilityRules,
	}, viewerFrom(c, cc)); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// --- Event kinds (campaign-scoped, Owner only end to end) ---

// ListEventKindsAPI lists a campaign's event kinds.
// GET /campaigns/:id/calendars/event-kinds
func (h *Handler) ListEventKindsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	kinds, err := h.svc.ListEventKinds(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}
	if kinds == nil {
		kinds = []EventKind{}
	}
	return c.JSON(http.StatusOK, kinds)
}

func bindEventKindInput(c echo.Context) (EventKindInput, error) {
	var req struct {
		Slug             string `json:"slug"`
		Name             string `json:"name"`
		Icon             string `json:"icon"`
		Color            string `json:"color"`
		SortOrder        int    `json:"sort_order"`
		DefaultAnnounced string `json:"default_announced"`
	}
	if err := c.Bind(&req); err != nil {
		return EventKindInput{}, apperror.NewBadRequest("invalid request")
	}
	return EventKindInput{
		Slug:             req.Slug,
		Name:             req.Name,
		Icon:             req.Icon,
		Color:            req.Color,
		SortOrder:        req.SortOrder,
		DefaultAnnounced: req.DefaultAnnounced,
	}, nil
}

// CreateEventKindAPI creates an event kind. Owner only.
// POST /campaigns/:id/calendars/event-kinds
func (h *Handler) CreateEventKindAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	input, err := bindEventKindInput(c)
	if err != nil {
		return err
	}
	kind, err := h.svc.CreateEventKind(c.Request().Context(), cc.Campaign.ID, input)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, kind)
}

// UpdateEventKindAPI applies a partial update to an event kind. Owner only.
// PUT /campaigns/:id/calendars/event-kinds/:kindID
func (h *Handler) UpdateEventKindAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	kindID, err := strconv.Atoi(c.Param("kindID"))
	if err != nil {
		return apperror.NewBadRequest("invalid event kind id")
	}
	var req struct {
		Slug             patch.Field[string] `json:"slug"`
		Name             string              `json:"name"`
		Icon             patch.Field[string] `json:"icon"`
		Color            patch.Field[string] `json:"color"`
		SortOrder        patch.Field[int]    `json:"sort_order"`
		DefaultAnnounced patch.Field[string] `json:"default_announced"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if err := h.svc.UpdateEventKind(c.Request().Context(), kindID, cc.Campaign.ID, UpdateEventKindInput{
		Slug:             req.Slug,
		Name:             req.Name,
		Icon:             req.Icon,
		Color:            req.Color,
		SortOrder:        req.SortOrder,
		DefaultAnnounced: req.DefaultAnnounced,
	}); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// DeleteEventKindAPI deletes an event kind. Owner only.
// DELETE /campaigns/:id/calendars/event-kinds/:kindID
func (h *Handler) DeleteEventKindAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	kindID, err := strconv.Atoi(c.Param("kindID"))
	if err != nil {
		return apperror.NewBadRequest("invalid event kind id")
	}
	if err := h.svc.DeleteEventKind(c.Request().Context(), kindID, cc.Campaign.ID); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// --- Eras (per-calendar, Owner only end to end) ---

func bindEraInput(c echo.Context) (EraInput, error) {
	var req struct {
		Name        string  `json:"name"`
		StartYear   int     `json:"start_year"`
		StartMonth  int     `json:"start_month"`
		StartDay    int     `json:"start_day"`
		EndYear     *int    `json:"end_year"`
		EndMonth    *int    `json:"end_month"`
		EndDay      *int    `json:"end_day"`
		Description *string `json:"description"`
		Color       string  `json:"color"`
		SortOrder   int     `json:"sort_order"`
	}
	if err := c.Bind(&req); err != nil {
		return EraInput{}, apperror.NewBadRequest("invalid request")
	}
	return EraInput{
		Name:        req.Name,
		StartYear:   req.StartYear,
		StartMonth:  req.StartMonth,
		StartDay:    req.StartDay,
		EndYear:     req.EndYear,
		EndMonth:    req.EndMonth,
		EndDay:      req.EndDay,
		Description: req.Description,
		Color:       req.Color,
		SortOrder:   req.SortOrder,
	}, nil
}

// CreateEraAPI creates an era. Owner only.
// POST /campaigns/:id/calendars/:calid/eras
func (h *Handler) CreateEraAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	input, err := bindEraInput(c)
	if err != nil {
		return err
	}
	era, err := h.svc.CreateEra(c.Request().Context(), c.Param("calid"), cc.Campaign.ID, input)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, era)
}

// UpdateEraAPI applies a partial update to an era. Owner only.
// PUT /campaigns/:id/calendars/:calid/eras/:eraID
func (h *Handler) UpdateEraAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	eraID, err := strconv.Atoi(c.Param("eraID"))
	if err != nil {
		return apperror.NewBadRequest("invalid era id")
	}
	var req struct {
		Name        string              `json:"name"`
		StartYear   patch.Field[int]    `json:"start_year"`
		StartMonth  patch.Field[int]    `json:"start_month"`
		StartDay    patch.Field[int]    `json:"start_day"`
		EndYear     patch.Field[int]    `json:"end_year"`
		EndMonth    patch.Field[int]    `json:"end_month"`
		EndDay      patch.Field[int]    `json:"end_day"`
		Description patch.Field[string] `json:"description"`
		Color       patch.Field[string] `json:"color"`
		SortOrder   patch.Field[int]    `json:"sort_order"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if err := h.svc.UpdateEra(c.Request().Context(), eraID, c.Param("calid"), cc.Campaign.ID, UpdateEraInput{
		Name:        req.Name,
		StartYear:   req.StartYear,
		StartMonth:  req.StartMonth,
		StartDay:    req.StartDay,
		EndYear:     req.EndYear,
		EndMonth:    req.EndMonth,
		EndDay:      req.EndDay,
		Description: req.Description,
		Color:       req.Color,
		SortOrder:   req.SortOrder,
	}); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// DeleteEraAPI deletes an era. Owner only.
// DELETE /campaigns/:id/calendars/:calid/eras/:eraID
func (h *Handler) DeleteEraAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	eraID, err := strconv.Atoi(c.Param("eraID"))
	if err != nil {
		return apperror.NewBadRequest("invalid era id")
	}
	if err := h.svc.DeleteEra(c.Request().Context(), eraID, c.Param("calid"), cc.Campaign.ID); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// --- Moon ---

// SetMoonHiddenAPI toggles a moon's visibility to players. Owner only.
// PUT /campaigns/:id/calendars/:calid/moons/:moonID/hidden
func (h *Handler) SetMoonHiddenAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	moonID, err := strconv.Atoi(c.Param("moonID"))
	if err != nil {
		return apperror.NewBadRequest("invalid moon id")
	}
	var req struct {
		Hidden bool `json:"hidden"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if err := h.svc.SetMoonHidden(c.Request().Context(), moonID, c.Param("calid"), cc.Campaign.ID, req.Hidden); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// --- Part B: calendar creation wizard (presets, import) ---
//
// Every route below is Owner only (routes.go), matching the rest of this
// file's calendar-structure mutation routes: creating a calendar's initial
// structure is calendar structure, the same category eras/event kinds/the
// moon hidden flag already fall in.

// maxCalendarImportSize caps an uploaded calendar file. Calendar exports are
// JSON only (never a zip, unlike campaigns' own import/export — see
// internal/plugins/campaigns/export_handler.go's maxImportZipSize), so a
// much smaller ceiling is appropriate; 10MB comfortably fits even a large
// calendar's full event history.
const maxCalendarImportSize = 10 * 1024 * 1024

// readCalendarImportFile reads and size-caps the "file" field of a
// multipart upload, shared by PreviewImportAPI and CreateFromImportAPI —
// mirrors campaigns.ExportHandler.ImportCampaign's own FormFile handling.
func readCalendarImportFile(c echo.Context) ([]byte, error) {
	fh, err := c.FormFile("file")
	if err != nil {
		return nil, apperror.NewBadRequest("file upload required")
	}
	if fh.Size > maxCalendarImportSize {
		return nil, apperror.NewBadRequest(fmt.Sprintf("file too large, maximum %d MB", maxCalendarImportSize/(1024*1024)))
	}
	src, err := fh.Open()
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("open uploaded file: %w", err))
	}
	defer func() { _ = src.Close() }()
	data, err := io.ReadAll(io.LimitReader(src, maxCalendarImportSize+1))
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("read uploaded file: %w", err))
	}
	if int64(len(data)) > maxCalendarImportSize {
		return nil, apperror.NewBadRequest(fmt.Sprintf("file too large, maximum %d MB", maxCalendarImportSize/(1024*1024)))
	}
	return data, nil
}

// formIntPtr parses a multipart/form value as *int, or nil when absent or
// unparseable. An unparseable override is treated as "not supplied" (the
// value the import itself determined, or a validation error if it
// specified none, still governs) rather than a hard 400 on a field the
// wizard's own review step already validated client-side.
func formIntPtr(c echo.Context, key string) *int {
	v := c.FormValue(key)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return nil
	}
	return &n
}

// presetSummary is one entry in ListPresetsAPI's response: enough to show a
// preset card without a second request per preset.
type presetSummary struct {
	Name         string       `json:"name"`
	CalendarName string       `json:"calendar_name"`
	Format       ImportFormat `json:"format"`
	MonthCount   int          `json:"month_count"`
	WeekdayCount int          `json:"weekday_count"`
}

// calendarImportResponse is CreateFromPresetAPI/CreateFromImportAPI's
// response shape: the created calendar plus any warnings the import
// produced (parse-time clamps, apply-time event-kind-slug fallbacks — see
// ImportResult.Warnings), so the wizard's confirmation step can show
// whatever didn't come through exactly as the source described it.
type calendarImportResponse struct {
	Calendar *Calendar `json:"calendar"`
	Warnings []string  `json:"warnings,omitempty"`
}

// ListPresetsAPI lists the shipped calendar presets. Owner only.
// GET /campaigns/:id/calendars/presets
func (h *Handler) ListPresetsAPI(c echo.Context) error {
	names, err := PresetNames()
	if err != nil {
		return apperror.NewInternal(err)
	}
	summaries := make([]presetSummary, 0, len(names))
	for _, name := range names {
		ir, err := h.svc.PreviewPreset(c.Request().Context(), name)
		if err != nil {
			return err
		}
		summaries = append(summaries, presetSummary{
			Name:         name,
			CalendarName: ir.CalendarName,
			Format:       ir.Format,
			MonthCount:   len(ir.Months),
			WeekdayCount: len(ir.Weekdays),
		})
	}
	return c.JSON(http.StatusOK, summaries)
}

// PreviewPresetAPI parses one shipped preset without creating anything.
// Owner only.
// GET /campaigns/:id/calendars/presets/:name
func (h *Handler) PreviewPresetAPI(c echo.Context) error {
	ir, err := h.svc.PreviewPreset(c.Request().Context(), c.Param("name"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, ir)
}

// CreateFromPresetAPI creates a new calendar from a shipped preset by name.
// Owner only. The wizard's Review step is expected to have already called
// PreviewPresetAPI and shown ir.Today for confirmation; current_month/
// current_day in the body are only REQUIRED when the preset itself left
// them unspecified — the Calendaria-format "elven" preset does (Calendaria
// has no day-level current-date concept at all, only a year — see
// parseCalendaria's own comment), so this is a real, exercised path, not a
// hypothetical one; the other three shipped presets are Chronicle-native
// exports that always specify a full current date. Same contract
// CreateFromImportAPI enforces for an upload that might.
// POST /campaigns/:id/calendars/presets/:name
func (h *Handler) CreateFromPresetAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	var req struct {
		CurrentYear  *int `json:"current_year"`
		CurrentMonth *int `json:"current_month"`
		CurrentDay   *int `json:"current_day"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	ir, err := h.svc.PreviewPreset(c.Request().Context(), c.Param("name"))
	if err != nil {
		return err
	}
	cal, err := h.svc.CreateCalendarFromImport(c.Request().Context(), cc.Campaign.ID, ir, CreateCalendarFromImportOptions{
		CurrentYear:  req.CurrentYear,
		CurrentMonth: req.CurrentMonth,
		CurrentDay:   req.CurrentDay,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, calendarImportResponse{Calendar: cal, Warnings: ir.Warnings})
}

// PreviewImportAPI parses an uploaded calendar file without creating
// anything. Owner only.
// POST /campaigns/:id/calendars/import/preview (multipart, field "file")
func (h *Handler) PreviewImportAPI(c echo.Context) error {
	data, err := readCalendarImportFile(c)
	if err != nil {
		return err
	}
	ir, err := h.svc.PreviewImport(c.Request().Context(), data)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, ir)
}

// CreateFromImportAPI creates a new calendar from an uploaded file. Owner
// only. current_year/current_month/current_day are ordinary multipart form
// fields alongside "file" — see CreateCalendarFromImportOptions's doc
// comment on when current_month/current_day are required.
// POST /campaigns/:id/calendars/import (multipart, field "file")
func (h *Handler) CreateFromImportAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	data, err := readCalendarImportFile(c)
	if err != nil {
		return err
	}
	ir, err := h.svc.PreviewImport(c.Request().Context(), data)
	if err != nil {
		return err
	}
	cal, err := h.svc.CreateCalendarFromImport(c.Request().Context(), cc.Campaign.ID, ir, CreateCalendarFromImportOptions{
		CurrentYear:  formIntPtr(c, "current_year"),
		CurrentMonth: formIntPtr(c, "current_month"),
		CurrentDay:   formIntPtr(c, "current_day"),
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, calendarImportResponse{Calendar: cal, Warnings: ir.Warnings})
}
