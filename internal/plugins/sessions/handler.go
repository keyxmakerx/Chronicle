package sessions

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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

// MailSender sends email notifications. Wraps the SMTP service interface.
type MailSender interface {
	SendHTMLMail(ctx context.Context, to []string, subject, plainBody, htmlBody string) error
	IsConfigured(ctx context.Context) bool
}

// Handler processes HTTP requests for the sessions plugin.
type Handler struct {
	svc          SessionService
	memberLister MemberLister
	mailer       MailSender
	baseURL      string // Application base URL for RSVP links (e.g. "https://chronicle.example.com").
}

// NewHandler creates a new sessions Handler.
func NewHandler(svc SessionService) *Handler {
	return &Handler{svc: svc}
}

// SetMemberLister wires a campaign member lister for RSVP invite-all.
func (h *Handler) SetMemberLister(ml MemberLister) {
	h.memberLister = ml
}

// SetMailSender wires the SMTP mail sender for RSVP email notifications.
func (h *Handler) SetMailSender(ms MailSender, baseURL string) {
	h.mailer = ms
	h.baseURL = baseURL
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
	userID := auth.GetUserID(c)

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK,
			SessionListFragment(cc, sessionList, csrfToken, isOwner, isScribe, userID))
	}
	return middleware.Render(c, http.StatusOK,
		SessionListPage(cc, sessionList, csrfToken, isOwner, isScribe, userID))
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

	// Parse recurrence fields.
	isRecurring := c.FormValue("is_recurring") == "1"
	var recType *string
	if rt := c.FormValue("recurrence_type"); rt != "" && isRecurring {
		recType = &rt
	}
	recInterval := 1
	if ri := c.FormValue("recurrence_interval"); ri != "" {
		if v, err2 := strconv.Atoi(ri); err2 == nil && v > 0 {
			recInterval = v
		}
	}
	var recEndDate *string
	if red := c.FormValue("recurrence_end_date"); red != "" {
		recEndDate = &red
	}

	session, err := h.svc.CreateSession(c.Request().Context(), cc.Campaign.ID, CreateSessionInput{
		Name:               name,
		Summary:            summaryPtr,
		ScheduledDate:      datePtr,
		CalendarYear:       calYear,
		CalendarMonth:      calMonth,
		CalendarDay:        calDay,
		IsRecurring:        isRecurring,
		RecurrenceType:     recType,
		RecurrenceInterval: recInterval,
		RecurrenceEndDate:  recEndDate,
		CreatedBy:          userID,
	})
	if err != nil {
		if appErr, ok := err.(*apperror.AppError); ok {
			return c.JSON(appErr.Code, map[string]string{"error": appErr.Message})
		}
		return err
	}

	// Auto-invite all campaign members and send RSVP emails.
	if h.memberLister != nil {
		members, err := h.memberLister.ListMembers(c.Request().Context(), cc.Campaign.ID)
		if err == nil {
			var userIDs []string
			for _, m := range members {
				userIDs = append(userIDs, m.UserID)
			}
			_ = h.svc.InviteAll(c.Request().Context(), session.ID, userIDs)

			// Send RSVP emails if SMTP is configured.
			if h.mailer != nil && h.mailer.IsConfigured(c.Request().Context()) {
				go h.sendRSVPEmails(context.Background(), session, cc.Campaign.Name, members)
			}
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
		Name                string  `json:"name"`
		Summary             *string `json:"summary"`
		ScheduledDate       *string `json:"scheduled_date"`
		CalendarYear        *int    `json:"calendar_year"`
		CalendarMonth       *int    `json:"calendar_month"`
		CalendarDay         *int    `json:"calendar_day"`
		Status              string  `json:"status"`
		IsRecurring         bool    `json:"is_recurring"`
		RecurrenceType      *string `json:"recurrence_type"`
		RecurrenceInterval  int     `json:"recurrence_interval"`
		RecurrenceDayOfWeek *int    `json:"recurrence_day_of_week"`
		RecurrenceEndDate   *string `json:"recurrence_end_date"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if err := h.svc.UpdateSession(c.Request().Context(), sessionID, UpdateSessionInput{
		Name:                req.Name,
		Summary:             req.Summary,
		ScheduledDate:       req.ScheduledDate,
		CalendarYear:        req.CalendarYear,
		CalendarMonth:       req.CalendarMonth,
		CalendarDay:         req.CalendarDay,
		Status:              req.Status,
		IsRecurring:         req.IsRecurring,
		RecurrenceType:      req.RecurrenceType,
		RecurrenceInterval:  req.RecurrenceInterval,
		RecurrenceDayOfWeek: req.RecurrenceDayOfWeek,
		RecurrenceEndDate:   req.RecurrenceEndDate,
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

	if err := h.svc.LinkEntity(c.Request().Context(), sessionID, req.EntityID, req.Role, cc.Campaign.ID); err != nil {
		if appErr, ok := err.(*apperror.AppError); ok {
			return c.JSON(appErr.Code, map[string]string{"error": appErr.Message})
		}
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

// --- RSVP Email Notifications ---

// sendRSVPEmails sends RSVP invitation emails to all campaign members.
// Runs in a goroutine to avoid blocking the HTTP response.
func (h *Handler) sendRSVPEmails(ctx context.Context, session *Session, campaignName string, members []campaigns.CampaignMember) {
	for _, m := range members {
		if m.Email == "" {
			continue
		}

		// Generate one-click accept/decline tokens.
		acceptToken, declineToken, err := h.svc.CreateRSVPTokens(ctx, session.ID, m.UserID)
		if err != nil {
			slog.Warn("failed to create rsvp tokens", slog.Any("error", err), slog.String("user_id", m.UserID))
			continue
		}

		dateStr := "TBD"
		if session.ScheduledDate != nil {
			dateStr = session.FormatScheduledDate()
		}

		subject := fmt.Sprintf("Session Invite: %s — %s", session.Name, campaignName)
		acceptURL := fmt.Sprintf("%s/rsvp/%s", h.baseURL, acceptToken)
		declineURL := fmt.Sprintf("%s/rsvp/%s", h.baseURL, declineToken)

		plainBody := fmt.Sprintf(`You've been invited to a game session!

Session: %s
Campaign: %s
Date: %s

Accept: %s
Decline: %s

These links expire in 7 days.
`, session.Name, campaignName, dateStr, acceptURL, declineURL)

		htmlBody := fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="utf-8"></head><body style="font-family:system-ui,-apple-system,sans-serif;max-width:480px;margin:0 auto;padding:20px;color:#333">
<div style="text-align:center;margin-bottom:24px">
  <div style="font-size:32px;margin-bottom:8px">🎲</div>
  <h1 style="font-size:20px;margin:0">Session Invite</h1>
</div>
<div style="background:#f8f9fa;border-radius:8px;padding:20px;margin-bottom:24px">
  <h2 style="font-size:16px;margin:0 0 8px">%s</h2>
  <p style="margin:4px 0;color:#666;font-size:14px"><strong>Campaign:</strong> %s</p>
  <p style="margin:4px 0;color:#666;font-size:14px"><strong>Date:</strong> %s</p>
</div>
<div style="text-align:center;margin-bottom:24px">
  <p style="margin:0 0 16px;color:#666;font-size:14px">Can you make it?</p>
  <a href="%s" style="display:inline-block;padding:10px 24px;background:#22c55e;color:#fff;text-decoration:none;border-radius:6px;font-weight:600;margin:0 8px">✓ Going</a>
  <a href="%s" style="display:inline-block;padding:10px 24px;background:#ef4444;color:#fff;text-decoration:none;border-radius:6px;font-weight:600;margin:0 8px">✗ Can't Make It</a>
</div>
<p style="text-align:center;color:#999;font-size:12px">These links expire in 7 days.</p>
</body></html>`,
			session.Name, campaignName, dateStr, acceptURL, declineURL)

		if err := h.mailer.SendHTMLMail(ctx, []string{m.Email}, subject, plainBody, htmlBody); err != nil {
			slog.Warn("failed to send rsvp email",
				slog.Any("error", err),
				slog.String("to", m.Email),
				slog.String("session_id", session.ID),
			)
		}
	}
}

// --- RSVP Token Redemption ---

// RedeemRSVPToken handles one-click email RSVP via token.
// GET /rsvp/:token — no auth required, token is the credential.
func (h *Handler) RedeemRSVPToken(c echo.Context) error {
	tokenStr := c.Param("token")
	if tokenStr == "" {
		return c.HTML(http.StatusBadRequest, rsvpResultHTML("Invalid Link", "This RSVP link is invalid.", false))
	}

	token, err := h.svc.RedeemRSVPToken(c.Request().Context(), tokenStr)
	if err != nil {
		msg := "This RSVP link is invalid or has expired."
		if appErr, ok := err.(*apperror.AppError); ok {
			msg = appErr.Message
		}
		return c.HTML(http.StatusOK, rsvpResultHTML("RSVP Failed", msg, false))
	}

	action := "accepted"
	if token.Action == RSVPDeclined {
		action = "declined"
	} else if token.Action == RSVPTentative {
		action = "marked as maybe"
	}

	return c.HTML(http.StatusOK, rsvpResultHTML("RSVP Recorded",
		"Your response has been "+action+". You can close this page.", true))
}

// rsvpResultHTML returns a simple standalone HTML page for RSVP token results.
func rsvpResultHTML(title, message string, success bool) string {
	icon := "fa-circle-xmark"
	color := "red"
	if success {
		icon = "fa-circle-check"
		color = "green"
	}
	return `<!DOCTYPE html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + title + ` - Chronicle</title>
<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.1/css/all.min.css">
<style>body{font-family:system-ui;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;background:#f8f9fa}
.card{text-align:center;padding:3rem;border-radius:12px;background:#fff;box-shadow:0 2px 12px rgba(0,0,0,.08);max-width:400px}
.icon{font-size:3rem;color:` + color + `;margin-bottom:1rem}h1{font-size:1.25rem;margin:0 0 .5rem}
p{color:#666;margin:0;font-size:.9rem}</style></head><body>
<div class="card"><div class="icon"><i class="fa-solid ` + icon + `"></i></div>
<h1>` + title + `</h1><p>` + message + `</p></div></body></html>`
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
