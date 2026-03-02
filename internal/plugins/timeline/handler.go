package timeline

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler processes HTTP requests for the timeline plugin.
type Handler struct {
	svc TimelineService
}

// NewHandler creates a new timeline Handler.
func NewHandler(svc TimelineService) *Handler {
	return &Handler{svc: svc}
}

// requireTimelineInCampaign fetches a timeline by ID and verifies it belongs
// to the given campaign. Returns 404 if not found or mismatched, preventing
// cross-campaign IDOR attacks.
func (h *Handler) requireTimelineInCampaign(c echo.Context, timelineID, campaignID string) (*Timeline, error) {
	t, err := h.svc.GetTimeline(c.Request().Context(), timelineID)
	if err != nil {
		return nil, err
	}
	if t.CampaignID != campaignID {
		return nil, echo.NewHTTPError(http.StatusNotFound, "timeline not found")
	}
	return t, nil
}

// Index lists all timelines for a campaign.
// GET /campaigns/:id/timelines
func (h *Handler) Index(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	role := int(cc.MemberRole)
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
	role := int(cc.MemberRole)
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

	data := TimelineViewData{
		CampaignID:   cc.Campaign.ID,
		Timeline:     t,
		Events:       events,
		EntityGroups: groups,
		IsOwner:      cc.MemberRole >= campaigns.RoleOwner,
		IsScribe:     cc.MemberRole >= campaigns.RoleScribe,
		CSRFToken:    middleware.GetCSRFToken(c),
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
	calendarID := c.FormValue("calendar_id")
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
		CalendarID:  calendarID,
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
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
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
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
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

// TimelineDataAPI returns JSON data for the D3.js timeline visualization.
// GET /campaigns/:id/timelines/:tid/data
func (h *Handler) TimelineDataAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	timelineID := c.Param("tid")
	role := int(cc.MemberRole)
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

	return c.JSON(http.StatusOK, map[string]any{
		"timeline": t,
		"events":   events,
		"groups":   groups,
	})
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
