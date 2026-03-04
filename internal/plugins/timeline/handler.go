package timeline

import (
	"context"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// MemberLister provides campaign membership data for the visibility user selector.
type MemberLister interface {
	ListMembers(ctx context.Context, campaignID string) ([]campaigns.CampaignMember, error)
}

// Handler processes HTTP requests for the timeline plugin.
type Handler struct {
	svc          TimelineService
	memberLister MemberLister
}

// NewHandler creates a new timeline Handler.
func NewHandler(svc TimelineService) *Handler {
	return &Handler{svc: svc}
}

// SetMemberLister injects the campaign member lister for visibility settings.
func (h *Handler) SetMemberLister(ml MemberLister) {
	h.memberLister = ml
}

// requireTimelineInCampaign fetches a timeline by ID and verifies it belongs
// to the given campaign. Returns 404 if not found or mismatched, preventing
// cross-campaign IDOR attacks.
func (h *Handler) requireTimelineInCampaign(c echo.Context, timelineID, campaignID string) (*Timeline, error) {
	return middleware.RequireInCampaign(c.Request().Context(), h.svc.GetTimeline, timelineID, campaignID, "timeline")
}

// effectiveRole returns the role to use for content filtering. When
// "view as player" mode is active, owners see content as a player would.
func effectiveRole(c echo.Context, cc *campaigns.CampaignContext) int {
	ctx := c.Request().Context()
	if layouts.IsViewingAsPlayer(ctx) {
		return int(campaigns.RolePlayer)
	}
	return int(cc.MemberRole)
}

// Index lists all timelines for a campaign.
// GET /campaigns/:id/timelines
func (h *Handler) Index(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	role := effectiveRole(c, cc)
	userID := auth.GetUserID(c)

	timelines, err := h.svc.ListTimelines(ctx, cc.Campaign.ID, role, userID)
	if err != nil {
		return err
	}

	data := TimelineListData{
		CampaignID: cc.Campaign.ID,
		Timelines:  timelines,
		IsOwner:    cc.MemberRole >= campaigns.RoleOwner,
		IsScribe:   cc.MemberRole >= campaigns.RoleScribe,
		CSRFToken:  middleware.GetCSRFToken(c),
	}

	if c.Request().Header.Get("HX-Request") != "" {
		return middleware.Render(c, http.StatusOK, TimelineListFragment(cc, data))
	}
	return middleware.Render(c, http.StatusOK, TimelineListPage(cc, data))
}

// Show renders a single timeline with its events.
// GET /campaigns/:id/timelines/:tid
func (h *Handler) Show(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	timelineID := c.Param("tid")
	role := effectiveRole(c, cc)
	userID := auth.GetUserID(c)

	t, err := h.requireTimelineInCampaign(c, timelineID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	events, err := h.svc.ListTimelineEvents(ctx, timelineID, role, userID)
	if err != nil {
		return err
	}

	groups, err := h.svc.ListEntityGroups(ctx, timelineID)
	if err != nil {
		return err
	}

	// Load campaign members for visibility settings (Owner only).
	var members []MemberRef
	if cc.MemberRole >= campaigns.RoleOwner && h.memberLister != nil {
		if ms, err := h.memberLister.ListMembers(ctx, cc.Campaign.ID); err == nil {
			for _, m := range ms {
				members = append(members, MemberRef{
					UserID:   m.UserID,
					Username: m.DisplayName,
					Role:     m.Role.String(),
				})
			}
		}
	}

	data := TimelineViewData{
		CampaignID:   cc.Campaign.ID,
		Timeline:     t,
		Events:       events,
		EntityGroups: groups,
		IsOwner:      cc.MemberRole >= campaigns.RoleOwner,
		IsScribe:     cc.MemberRole >= campaigns.RoleScribe,
		CSRFToken:    middleware.GetCSRFToken(c),
		Members:      members,
	}

	if c.Request().Header.Get("HX-Request") != "" {
		return middleware.Render(c, http.StatusOK, TimelineShowFragment(cc, data))
	}
	return middleware.Render(c, http.StatusOK, TimelineShowPage(cc, data))
}

// CreateForm handles timeline creation from a form POST.
// POST /campaigns/:id/timelines
func (h *Handler) CreateForm(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	userID := auth.GetUserID(c)

	name := c.FormValue("name")

	// Validate field lengths.
	if err := apperror.ValidateRequired("name", name); err != nil {
		return err
	}
	if err := apperror.ValidateStringLength("name", name, apperror.MaxNameLength); err != nil {
		return err
	}

	calendarID := c.FormValue("calendar_id")
	var calPtr *string
	if calendarID != "" {
		calPtr = &calendarID
	}
	desc := c.FormValue("description")
	var descPtr *string
	if desc != "" {
		descPtr = &desc
	}

	color := c.FormValue("color")
	icon := c.FormValue("icon")
	visibility := c.FormValue("visibility")

	t, err := h.svc.CreateTimeline(ctx, cc.Campaign.ID, CreateTimelineInput{
		CampaignID:  cc.Campaign.ID,
		CalendarID:  calPtr,
		Name:        name,
		Description: descPtr,
		Color:       color,
		Icon:        icon,
		Visibility:  visibility,
		ZoomDefault: ZoomYear,
		CreatedBy:   userID,
	})
	if err != nil {
		return err
	}

	return c.Redirect(http.StatusSeeOther,
		fmt.Sprintf("/campaigns/%s/timelines/%s", cc.Campaign.ID, t.ID))
}

// UpdateAPI updates timeline settings.
// PUT /campaigns/:id/timelines/:tid
func (h *Handler) UpdateAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	timelineID := c.Param("tid")

	if _, err := h.requireTimelineInCampaign(c, timelineID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
		Color       string  `json:"color"`
		Icon        string  `json:"icon"`
		Visibility  string  `json:"visibility"`
		ZoomDefault string  `json:"zoom_default"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	return h.svc.UpdateTimeline(ctx, timelineID, UpdateTimelineInput{
		Name:        req.Name,
		Description: req.Description,
		Color:       req.Color,
		Icon:        req.Icon,
		Visibility:  req.Visibility,
		ZoomDefault: req.ZoomDefault,
	})
}

// DeleteAPI deletes a timeline and all associated data.
// DELETE /campaigns/:id/timelines/:tid
func (h *Handler) DeleteAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	timelineID := c.Param("tid")

	if _, err := h.requireTimelineInCampaign(c, timelineID, cc.Campaign.ID); err != nil {
		return err
	}

	if err := h.svc.DeleteTimeline(ctx, timelineID); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// LinkEventAPI links a calendar event to a timeline.
// POST /campaigns/:id/timelines/:tid/events
func (h *Handler) LinkEventAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	timelineID := c.Param("tid")

	if _, err := h.requireTimelineInCampaign(c, timelineID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
		EventID       string  `json:"event_id"`
		Label         *string `json:"label"`
		ColorOverride *string `json:"color_override"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	link, err := h.svc.LinkEvent(ctx, timelineID, req.EventID, LinkEventInput{
		Label:         req.Label,
		ColorOverride: req.ColorOverride,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, link)
}

// UnlinkEventAPI removes a calendar event from a timeline.
// DELETE /campaigns/:id/timelines/:tid/events/:eid
func (h *Handler) UnlinkEventAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	timelineID := c.Param("tid")
	eventID := c.Param("eid")

	if _, err := h.requireTimelineInCampaign(c, timelineID, cc.Campaign.ID); err != nil {
		return err
	}

	if err := h.svc.UnlinkEvent(ctx, timelineID, eventID); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// --- Standalone Event Handlers ---

// CreateStandaloneEventAPI creates a new standalone event on a timeline.
// POST /campaigns/:id/timelines/:tid/standalone-events
func (h *Handler) CreateStandaloneEventAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	timelineID := c.Param("tid")
	userID := auth.GetUserID(c)

	if _, err := h.requireTimelineInCampaign(c, timelineID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
		Name            string  `json:"name"`
		Description     *string `json:"description"`
		DescriptionHTML *string `json:"description_html"`
		EntityID        *string `json:"entity_id"`
		Year            int     `json:"year"`
		Month           int     `json:"month"`
		Day             int     `json:"day"`
		StartHour       *int    `json:"start_hour"`
		StartMinute     *int    `json:"start_minute"`
		EndYear         *int    `json:"end_year"`
		EndMonth        *int    `json:"end_month"`
		EndDay          *int    `json:"end_day"`
		EndHour         *int    `json:"end_hour"`
		EndMinute       *int    `json:"end_minute"`
		IsRecurring     bool    `json:"is_recurring"`
		RecurrenceType  *string `json:"recurrence_type"`
		Category        *string `json:"category"`
		Visibility      string  `json:"visibility"`
		Label           *string `json:"label"`
		Color           *string `json:"color"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	e, err := h.svc.CreateStandaloneEvent(ctx, timelineID, CreateTimelineEventInput{
		Name:            req.Name,
		Description:     req.Description,
		DescriptionHTML: req.DescriptionHTML,
		EntityID:        req.EntityID,
		Year:            req.Year,
		Month:           req.Month,
		Day:             req.Day,
		StartHour:       req.StartHour,
		StartMinute:     req.StartMinute,
		EndYear:         req.EndYear,
		EndMonth:        req.EndMonth,
		EndDay:          req.EndDay,
		EndHour:         req.EndHour,
		EndMinute:       req.EndMinute,
		IsRecurring:     req.IsRecurring,
		RecurrenceType:  req.RecurrenceType,
		Category:        req.Category,
		Visibility:      req.Visibility,
		Label:           req.Label,
		Color:           req.Color,
		CreatedBy:       userID,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, e)
}

// UpdateStandaloneEventAPI modifies a standalone event.
// PUT /campaigns/:id/timelines/:tid/standalone-events/:eid
func (h *Handler) UpdateStandaloneEventAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	timelineID := c.Param("tid")
	eventID := c.Param("eid")

	if _, err := h.requireTimelineInCampaign(c, timelineID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
		Name            string  `json:"name"`
		Description     *string `json:"description"`
		DescriptionHTML *string `json:"description_html"`
		EntityID        *string `json:"entity_id"`
		Year            int     `json:"year"`
		Month           int     `json:"month"`
		Day             int     `json:"day"`
		StartHour       *int    `json:"start_hour"`
		StartMinute     *int    `json:"start_minute"`
		EndYear         *int    `json:"end_year"`
		EndMonth        *int    `json:"end_month"`
		EndDay          *int    `json:"end_day"`
		EndHour         *int    `json:"end_hour"`
		EndMinute       *int    `json:"end_minute"`
		IsRecurring     bool    `json:"is_recurring"`
		RecurrenceType  *string `json:"recurrence_type"`
		Category        *string `json:"category"`
		Visibility      string  `json:"visibility"`
		Label           *string `json:"label"`
		Color           *string `json:"color"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	if err := h.svc.UpdateStandaloneEvent(ctx, timelineID, eventID, UpdateTimelineEventInput{
		Name:            req.Name,
		Description:     req.Description,
		DescriptionHTML: req.DescriptionHTML,
		EntityID:        req.EntityID,
		Year:            req.Year,
		Month:           req.Month,
		Day:             req.Day,
		StartHour:       req.StartHour,
		StartMinute:     req.StartMinute,
		EndYear:         req.EndYear,
		EndMonth:        req.EndMonth,
		EndDay:          req.EndDay,
		EndHour:         req.EndHour,
		EndMinute:       req.EndMinute,
		IsRecurring:     req.IsRecurring,
		RecurrenceType:  req.RecurrenceType,
		Category:        req.Category,
		Visibility:      req.Visibility,
		Label:           req.Label,
		Color:           req.Color,
	}); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// DeleteStandaloneEventAPI removes a standalone event from a timeline.
// DELETE /campaigns/:id/timelines/:tid/standalone-events/:eid
func (h *Handler) DeleteStandaloneEventAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	timelineID := c.Param("tid")
	eventID := c.Param("eid")

	if _, err := h.requireTimelineInCampaign(c, timelineID, cc.Campaign.ID); err != nil {
		return err
	}

	if err := h.svc.DeleteStandaloneEvent(ctx, timelineID, eventID); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// TimelineDataAPI returns JSON data for the D3.js timeline visualization.
// GET /campaigns/:id/timelines/:tid/data
func (h *Handler) TimelineDataAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	timelineID := c.Param("tid")
	role := effectiveRole(c, cc)
	userID := auth.GetUserID(c)

	t, err := h.requireTimelineInCampaign(c, timelineID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	events, err := h.svc.ListTimelineEvents(ctx, timelineID, role, userID)
	if err != nil {
		return err
	}

	groups, err := h.svc.ListEntityGroups(ctx, timelineID)
	if err != nil {
		return err
	}

	// Fetch calendar eras for visualization background bands.
	var eras []CalendarEra
	if t.HasCalendar() && t.CalendarID != nil {
		eras, _ = h.svc.ListCalendarEras(ctx, *t.CalendarID)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"timeline": t,
		"events":   events,
		"groups":   groups,
		"eras":     eras,
	})
}

// ListAvailableEventsAPI returns calendar events not yet linked to a timeline.
// GET /campaigns/:id/timelines/:tid/available-events
func (h *Handler) ListAvailableEventsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	timelineID := c.Param("tid")
	role := int(cc.MemberRole)

	if _, err := h.requireTimelineInCampaign(c, timelineID, cc.Campaign.ID); err != nil {
		return err
	}

	events, err := h.svc.ListAvailableEvents(ctx, timelineID, role)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, events)
}

// LinkAllEventsAPI links all available calendar events to a timeline at once.
// POST /campaigns/:id/timelines/:tid/events/all
func (h *Handler) LinkAllEventsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	timelineID := c.Param("tid")
	role := int(cc.MemberRole)

	if _, err := h.requireTimelineInCampaign(c, timelineID, cc.Campaign.ID); err != nil {
		return err
	}

	count, err := h.svc.LinkAllEvents(ctx, timelineID, role)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]int{"linked": count})
}

// ListCalendarsAPI returns available calendars for the create timeline form.
// GET /campaigns/:id/timelines/calendars
func (h *Handler) ListCalendarsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	calendars, err := h.svc.ListCalendars(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, calendars)
}

// CreateEntityGroupAPI creates a new entity group for swim-lane organization.
// POST /campaigns/:id/timelines/:tid/groups
func (h *Handler) CreateEntityGroupAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	timelineID := c.Param("tid")

	if _, err := h.requireTimelineInCampaign(c, timelineID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	g, err := h.svc.CreateEntityGroup(c.Request().Context(), timelineID, CreateEntityGroupInput{
		Name:  req.Name,
		Color: req.Color,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, g)
}

// UpdateEntityGroupAPI modifies an existing entity group.
// PUT /campaigns/:id/timelines/:tid/groups/:gid
func (h *Handler) UpdateEntityGroupAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	timelineID := c.Param("tid")
	groupID, err := parseIntParam(c, "gid")
	if err != nil {
		return err
	}

	if _, err := h.requireTimelineInCampaign(c, timelineID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	return h.svc.UpdateEntityGroup(c.Request().Context(), timelineID, groupID, UpdateEntityGroupInput{
		Name:  req.Name,
		Color: req.Color,
	})
}

// DeleteEntityGroupAPI removes an entity group and all its members.
// DELETE /campaigns/:id/timelines/:tid/groups/:gid
func (h *Handler) DeleteEntityGroupAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	timelineID := c.Param("tid")
	groupID, err := parseIntParam(c, "gid")
	if err != nil {
		return err
	}

	if _, err := h.requireTimelineInCampaign(c, timelineID, cc.Campaign.ID); err != nil {
		return err
	}

	if err := h.svc.DeleteEntityGroup(c.Request().Context(), timelineID, groupID); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// AddGroupMemberAPI adds an entity to an entity group.
// POST /campaigns/:id/timelines/:tid/groups/:gid/members
func (h *Handler) AddGroupMemberAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	timelineID := c.Param("tid")
	groupID, err := parseIntParam(c, "gid")
	if err != nil {
		return err
	}

	if _, err := h.requireTimelineInCampaign(c, timelineID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
		EntityID string `json:"entity_id"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if req.EntityID == "" {
		return apperror.NewBadRequest("entity_id is required")
	}

	if err := h.svc.AddGroupMember(c.Request().Context(), timelineID, groupID, req.EntityID); err != nil {
		return err
	}
	return c.NoContent(http.StatusCreated)
}

// RemoveGroupMemberAPI removes an entity from an entity group.
// DELETE /campaigns/:id/timelines/:tid/groups/:gid/members/:eid
func (h *Handler) RemoveGroupMemberAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	timelineID := c.Param("tid")
	groupID, err := parseIntParam(c, "gid")
	if err != nil {
		return err
	}
	entityID := c.Param("eid")

	if _, err := h.requireTimelineInCampaign(c, timelineID, cc.Campaign.ID); err != nil {
		return err
	}

	if err := h.svc.RemoveGroupMember(c.Request().Context(), timelineID, groupID, entityID); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// ListEntityGroupsAPI returns all entity groups for a timeline.
// GET /campaigns/:id/timelines/:tid/groups
func (h *Handler) ListEntityGroupsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	timelineID := c.Param("tid")

	if _, err := h.requireTimelineInCampaign(c, timelineID, cc.Campaign.ID); err != nil {
		return err
	}

	groups, err := h.svc.ListEntityGroups(c.Request().Context(), timelineID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, groups)
}

// UpdateTimelineVisibilityAPI updates timeline visibility and per-user rules.
// PUT /campaigns/:id/timelines/:tid/visibility
func (h *Handler) UpdateTimelineVisibilityAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	timelineID := c.Param("tid")

	t, err := h.requireTimelineInCampaign(c, timelineID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var req struct {
		Visibility      string  `json:"visibility"`
		VisibilityRules *string `json:"visibility_rules"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	// Build a full update preserving existing settings.
	return h.svc.UpdateTimeline(ctx, timelineID, UpdateTimelineInput{
		Name:            t.Name,
		Description:     t.Description,
		DescriptionHTML: t.DescriptionHTML,
		Color:           t.Color,
		Icon:            t.Icon,
		Visibility:      req.Visibility,
		VisibilityRules: req.VisibilityRules,
		ZoomDefault:     t.ZoomDefault,
	})
}

// UpdateEventVisibilityAPI updates per-event visibility on a timeline event link.
// PUT /campaigns/:id/timelines/:tid/events/:eid/visibility
func (h *Handler) UpdateEventVisibilityAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	timelineID := c.Param("tid")
	eventID := c.Param("eid")

	if _, err := h.requireTimelineInCampaign(c, timelineID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
		VisibilityOverride *string `json:"visibility_override"`
		VisibilityRules    *string `json:"visibility_rules"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	if err := h.svc.UpdateEventLinkVisibility(ctx, timelineID, eventID, UpdateEventVisibilityInput{
		VisibilityOverride: req.VisibilityOverride,
		VisibilityRules:    req.VisibilityRules,
	}); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// UpdateStandaloneEventVisibilityAPI updates visibility and per-user rules on a standalone event.
// PUT /campaigns/:id/timelines/:tid/standalone-events/:eid/visibility
func (h *Handler) UpdateStandaloneEventVisibilityAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	timelineID := c.Param("tid")
	eventID := c.Param("eid")

	if _, err := h.requireTimelineInCampaign(c, timelineID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
		Visibility      string  `json:"visibility"`
		VisibilityRules *string `json:"visibility_rules"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	// Get the standalone event and verify it belongs to this timeline.
	e, err := h.svc.GetStandaloneEvent(ctx, eventID)
	if err != nil {
		return err
	}
	if e.TimelineID != timelineID {
		return apperror.NewNotFound("event not found")
	}

	// Validate visibility.
	if req.Visibility == "" {
		req.Visibility = e.Visibility
	}
	if req.Visibility != "everyone" && req.Visibility != "dm_only" {
		return apperror.NewBadRequest("visibility must be 'everyone' or 'dm_only'")
	}

	e.Visibility = req.Visibility
	e.VisibilityRules = req.VisibilityRules

	if err := h.svc.UpdateStandaloneEvent(ctx, timelineID, eventID, UpdateTimelineEventInput{
		Name:            e.Name,
		Description:     e.Description,
		DescriptionHTML: e.DescriptionHTML,
		EntityID:        e.EntityID,
		Year:            e.Year,
		Month:           e.Month,
		Day:             e.Day,
		StartHour:       e.StartHour,
		StartMinute:     e.StartMinute,
		EndYear:         e.EndYear,
		EndMonth:        e.EndMonth,
		EndDay:          e.EndDay,
		EndHour:         e.EndHour,
		EndMinute:       e.EndMinute,
		IsRecurring:     e.IsRecurring,
		RecurrenceType:  e.RecurrenceType,
		Category:        e.Category,
		Visibility:      req.Visibility,
		VisibilityRules: req.VisibilityRules,
		Label:           e.Label,
		Color:           e.Color,
	}); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// ListCampaignMembersAPI returns campaign members for the visibility user selector.
// GET /campaigns/:id/timelines/members
func (h *Handler) ListCampaignMembersAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)

	if h.memberLister == nil {
		return c.JSON(http.StatusOK, []MemberRef{})
	}

	ms, err := h.memberLister.ListMembers(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	refs := make([]MemberRef, 0, len(ms))
	for _, m := range ms {
		refs = append(refs, MemberRef{
			UserID:   m.UserID,
			Username: m.DisplayName,
			Role:     m.Role.String(),
		})
	}
	return c.JSON(http.StatusOK, refs)
}

// PreviewAPI returns an HTMX fragment for the dashboard timeline preview block.
// GET /campaigns/:id/timelines/preview
func (h *Handler) PreviewAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	role := effectiveRole(c, cc)
	userID := auth.GetUserID(c)

	timelines, err := h.svc.ListTimelines(ctx, cc.Campaign.ID, role, userID)
	if err != nil {
		return err
	}

	// Apply limit from query param (default 5, max 20).
	limit := 5
	if l := c.QueryParam("limit"); l != "" {
		if _, err := fmt.Sscanf(l, "%d", &limit); err != nil || limit < 1 {
			limit = 5
		}
		if limit > 20 {
			limit = 20
		}
	}
	if limit > len(timelines) {
		limit = len(timelines)
	}
	timelines = timelines[:limit]

	return middleware.Render(c, http.StatusOK, timelinePreviewFragment(cc.Campaign.ID, timelines))
}

// parseIntParam extracts an integer path parameter, returning 400 on failure.
func parseIntParam(c echo.Context, name string) (int, error) {
	s := c.Param(name)
	var v int
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil {
		return 0, apperror.NewBadRequest(name + " must be a number")
	}
	return v, nil
}
