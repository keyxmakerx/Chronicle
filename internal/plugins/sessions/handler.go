package sessions

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// MemberLister provides campaign membership data for RSVP.
type MemberLister interface {
	ListMembers(ctx context.Context, campaignID string) ([]campaigns.CampaignMember, error)
}

// Handler processes HTTP requests for the sessions plugin.
type Handler struct {
	svc          SessionService
	memberLister MemberLister
}

// NewHandler creates a new sessions Handler.
func NewHandler(svc SessionService) *Handler {
	return &Handler{svc: svc}
}

// SetMemberLister wires a campaign member lister for RSVP invite-all.
func (h *Handler) SetMemberLister(ml MemberLister) {
	h.memberLister = ml
}

// ListSessions renders the session list page.
// GET /campaigns/:id/sessions
func (h *Handler) ListSessions(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	sessionList, err := h.svc.ListSessions(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}

	csrfToken := middleware.GetCSRFToken(c)
	isOwner := cc.MemberRole >= campaigns.RoleOwner
	isScribe := cc.MemberRole >= campaigns.RoleScribe

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK,
			SessionListFragment(cc, sessionList, csrfToken, isOwner, isScribe))
	}
	return middleware.Render(c, http.StatusOK,
		SessionListPage(cc, sessionList, csrfToken, isOwner, isScribe))
}

// ShowSession renders a session detail page.
// GET /campaigns/:id/sessions/:sid
func (h *Handler) ShowSession(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	sessionID := c.Param("sid")

	session, err := h.requireSessionInCampaign(c, sessionID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	csrfToken := middleware.GetCSRFToken(c)
	isOwner := cc.MemberRole >= campaigns.RoleOwner
	isScribe := cc.MemberRole >= campaigns.RoleScribe
	userID := auth.GetUserID(c)

	return middleware.Render(c, http.StatusOK,
		SessionDetailPage(cc, session, csrfToken, isOwner, isScribe, userID))
}

// CreateSession creates a new session.
// POST /campaigns/:id/sessions
func (h *Handler) CreateSession(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)

	name := c.FormValue("name")
	summary := c.FormValue("summary")
	scheduledDate := c.FormValue("scheduled_date")

	var summaryPtr *string
	if summary != "" {
		summaryPtr = &summary
	}
	var datePtr *string
	if scheduledDate != "" {
		datePtr = &scheduledDate
	}

	// Parse optional calendar date fields.
	var calYear, calMonth, calDay *int
	if y := c.FormValue("calendar_year"); y != "" {
		v, _ := strconv.Atoi(y)
		calYear = &v
	}
	if m := c.FormValue("calendar_month"); m != "" {
		v, _ := strconv.Atoi(m)
		calMonth = &v
	}
	if d := c.FormValue("calendar_day"); d != "" {
		v, _ := strconv.Atoi(d)
		calDay = &v
	}

	session, err := h.svc.CreateSession(c.Request().Context(), cc.Campaign.ID, CreateSessionInput{
		Name:          name,
		Summary:       summaryPtr,
		ScheduledDate: datePtr,
		CalendarYear:  calYear,
		CalendarMonth: calMonth,
		CalendarDay:   calDay,
		CreatedBy:     userID,
	})
	if err != nil {
		if appErr, ok := err.(*apperror.AppError); ok {
			return c.JSON(appErr.Code, map[string]string{"error": appErr.Message})
		}
		return err
	}

	// Auto-invite all campaign members.
	if h.memberLister != nil {
		members, err := h.memberLister.ListMembers(c.Request().Context(), cc.Campaign.ID)
		if err == nil {
			var userIDs []string
			for _, m := range members {
				userIDs = append(userIDs, m.UserID)
			}
			_ = h.svc.InviteAll(c.Request().Context(), session.ID, userIDs)
		}
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect",
			"/campaigns/"+cc.Campaign.ID+"/sessions/"+session.ID)
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther,
		"/campaigns/"+cc.Campaign.ID+"/sessions/"+session.ID)
}

// UpdateSessionAPI updates a session.
// PUT /campaigns/:id/sessions/:sid
func (h *Handler) UpdateSessionAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	sessionID := c.Param("sid")

	if _, err := h.requireSessionInCampaign(c, sessionID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
		Name          string  `json:"name"`
		Summary       *string `json:"summary"`
		ScheduledDate *string `json:"scheduled_date"`
		CalendarYear  *int    `json:"calendar_year"`
		CalendarMonth *int    `json:"calendar_month"`
		CalendarDay   *int    `json:"calendar_day"`
		Status        string  `json:"status"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if err := h.svc.UpdateSession(c.Request().Context(), sessionID, UpdateSessionInput{
		Name:          req.Name,
		Summary:       req.Summary,
		ScheduledDate: req.ScheduledDate,
		CalendarYear:  req.CalendarYear,
		CalendarMonth: req.CalendarMonth,
		CalendarDay:   req.CalendarDay,
		Status:        req.Status,
	}); err != nil {
		if appErr, ok := err.(*apperror.AppError); ok {
			return c.JSON(appErr.Code, map[string]string{"error": appErr.Message})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "update failed"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteSessionAPI deletes a session.
// DELETE /campaigns/:id/sessions/:sid
func (h *Handler) DeleteSessionAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	sessionID := c.Param("sid")

	if _, err := h.requireSessionInCampaign(c, sessionID, cc.Campaign.ID); err != nil {
		return err
	}

	if err := h.svc.DeleteSession(c.Request().Context(), sessionID); err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect",
			"/campaigns/"+cc.Campaign.ID+"/sessions")
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- RSVP ---

// RSVPSession updates the current user's attendance status.
// POST /campaigns/:id/sessions/:sid/rsvp
func (h *Handler) RSVPSession(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	sessionID := c.Param("sid")
	userID := auth.GetUserID(c)

	if _, err := h.requireSessionInCampaign(c, sessionID, cc.Campaign.ID); err != nil {
		return err
	}

	status := c.FormValue("status")
	if status == "" {
		var req struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err == nil {
			status = req.Status
		}
	}

	if err := h.svc.UpdateRSVP(c.Request().Context(), sessionID, userID, status); err != nil {
		if appErr, ok := err.(*apperror.AppError); ok {
			return c.JSON(appErr.Code, map[string]string{"error": appErr.Message})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "RSVP failed"})
	}

	if middleware.IsHTMX(c) {
		// Re-render the attendee list.
		attendees, _ := h.svc.ListAttendees(c.Request().Context(), sessionID)
		csrfToken := middleware.GetCSRFToken(c)
		return middleware.Render(c, http.StatusOK,
			AttendeeList(cc, sessionID, attendees, csrfToken, userID))
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- Entity Linking ---

// LinkEntityAPI links an entity to a session.
// POST /campaigns/:id/sessions/:sid/entities
func (h *Handler) LinkEntityAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	sessionID := c.Param("sid")

	if _, err := h.requireSessionInCampaign(c, sessionID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
		EntityID string `json:"entity_id"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if err := h.svc.LinkEntity(c.Request().Context(), sessionID, req.EntityID, req.Role); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "link failed"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UnlinkEntityAPI removes an entity link from a session.
// DELETE /campaigns/:id/sessions/:sid/entities/:eid
func (h *Handler) UnlinkEntityAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	sessionID := c.Param("sid")
	entityID := c.Param("eid")

	if _, err := h.requireSessionInCampaign(c, sessionID, cc.Campaign.ID); err != nil {
		return err
	}

	if err := h.svc.UnlinkEntity(c.Request().Context(), sessionID, entityID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "unlink failed"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- Helpers ---

// requireSessionInCampaign fetches a session and verifies it belongs to the campaign.
func (h *Handler) requireSessionInCampaign(c echo.Context, sessionID, campaignID string) (*Session, error) {
	session, err := h.svc.GetSession(c.Request().Context(), sessionID)
	if err != nil {
		return nil, err
	}
	if session.CampaignID != campaignID {
		return nil, echo.NewHTTPError(http.StatusNotFound, "session not found")
	}
	return session, nil
}
