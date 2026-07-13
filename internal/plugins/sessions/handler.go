package sessions

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// MailSender sends email notifications. Wraps the SMTP service interface.
type MailSender interface {
	SendHTMLMail(ctx context.Context, to []string, subject, plainBody, htmlBody string) error
	IsConfigured(ctx context.Context) bool
}

// Handler processes HTTP requests for the sessions plugin.
type Handler struct {
	svc          SessionService
	memberLister campaigns.MemberLister
	mailer       MailSender
	baseURL      string // Application base URL for RSVP links (e.g. "https://chronicle.example.com").
	userDir      UserDirectory // Resolves a user's stored IANA timezone for the availability overlay.
}

// NewHandler creates a new sessions Handler.
func NewHandler(svc SessionService) *Handler {
	return &Handler{svc: svc}
}

// SetMemberLister wires a campaign member lister for RSVP invite-all.
func (h *Handler) SetMemberLister(ml campaigns.MemberLister) {
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
	scheduledTime := c.FormValue("scheduled_time") // "HH:MM" from the modal's time input (C-SCHED-P3).

	// Validate field lengths.
	if err := apperror.ValidateRequired("name", name); err != nil {
		return err
	}
	if err := apperror.ValidateStringLength("name", name, apperror.MaxNameLength); err != nil {
		return err
	}

	var summaryPtr *string
	if summary != "" {
		summaryPtr = &summary
	}
	var datePtr *string
	if scheduledDate != "" {
		datePtr = &scheduledDate
	}
	var timePtr *string
	if scheduledTime != "" {
		timePtr = &scheduledTime
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
		ScheduledTime:      timePtr,
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

	// If created from the calendar context, trigger a refresh instead of redirect.
	if middleware.IsHTMX(c) && c.FormValue("from") == "calendar" {
		c.Response().Header().Set("HX-Trigger", "sessions-refresh")
		c.Response().Header().Set("HX-Retarget", "#sessions-modal-content")
		c.Response().Header().Set("HX-Reswap", "innerHTML")
		return c.NoContent(http.StatusNoContent)
	}

	return middleware.HTMXRedirect(c, "/campaigns/"+cc.Campaign.ID+"/sessions/"+session.ID)
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
		ScheduledTime       *string `json:"scheduled_time"`
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

	nextSession, err := h.svc.UpdateSession(c.Request().Context(), sessionID, UpdateSessionInput{
		Name:                req.Name,
		Summary:             req.Summary,
		ScheduledDate:       req.ScheduledDate,
		ScheduledTime:       req.ScheduledTime,
		CalendarYear:        req.CalendarYear,
		CalendarMonth:       req.CalendarMonth,
		CalendarDay:         req.CalendarDay,
		Status:              req.Status,
		IsRecurring:         req.IsRecurring,
		RecurrenceType:      req.RecurrenceType,
		RecurrenceInterval:  req.RecurrenceInterval,
		RecurrenceDayOfWeek: req.RecurrenceDayOfWeek,
		RecurrenceEndDate:   req.RecurrenceEndDate,
	})
	if err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}

	// If a recurring session was completed, a new session was auto-generated.
	// Send RSVP emails for the new session.
	if nextSession != nil {
		slog.Info("auto-generated next recurring session",
			slog.String("session_id", nextSession.ID),
			slog.String("campaign_id", cc.Campaign.ID),
		)

		if h.mailer != nil && h.mailer.IsConfigured(c.Request().Context()) && h.memberLister != nil {
			members, mErr := h.memberLister.ListMembers(c.Request().Context(), cc.Campaign.ID)
			if mErr == nil {
				go h.sendRSVPEmails(context.Background(), nextSession, cc.Campaign.Name, members)
			}
		}

		return c.JSON(http.StatusOK, map[string]string{
			"status":         "ok",
			"next_session_id": nextSession.ID,
		})
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

// UpdateRecapAPI saves the session recap (post-session write-up visible to all members).
// PUT /campaigns/:id/sessions/:sid/recap
func (h *Handler) UpdateRecapAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	sessionID := c.Param("sid")

	if _, err := h.requireSessionInCampaign(c, sessionID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
		Recap     *string `json:"recap"`
		RecapHTML *string `json:"recap_html"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if err := h.svc.UpdateSessionRecap(c.Request().Context(), sessionID, req.Recap, req.RecapHTML); err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
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
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
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
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
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
			// Escape the operator-authored session name + campaign name so they
			// can't inject markup into the email (C-SCHED-P3 0c, same sweep as the
			// proposal invite). dateStr is our own formatted label; URLs are hex
			// tokens — both safe.
			html.EscapeString(session.Name), html.EscapeString(campaignName), dateStr, acceptURL, declineURL)

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

// RedeemRSVPToken renders the confirm interstitial for a one-click RSVP token.
// GET /rsvp/:token — no auth required, token is the credential. GET is a pure
// read (0b): it validates the token and shows a POST form; ApplyRSVPToken records
// the RSVP, so a mail scanner prefetching the link can't auto-RSVP.
func (h *Handler) RedeemRSVPToken(c echo.Context) error {
	tokenStr := c.Param("token")
	if tokenStr == "" {
		return c.HTML(http.StatusBadRequest, rsvpResultHTML("Invalid Link", "This RSVP link is invalid.", false))
	}
	token, err := h.svc.ValidateRSVPToken(c.Request().Context(), tokenStr)
	if err != nil {
		msg := apperror.UserMessage(err, "This RSVP link is invalid or has expired.")
		return c.HTML(http.StatusOK, rsvpResultHTML("RSVP Failed", msg, false))
	}
	label := rsvpActionLabel(token.Action)
	return c.HTML(http.StatusOK, tokenConfirmHTML("Confirm Your RSVP",
		fmt.Sprintf("You're responding %q. Tap below to confirm.", label),
		fmt.Sprintf("/rsvp/%s", tokenStr), "Confirm — "+label, middleware.GetCSRFToken(c)))
}

// ApplyRSVPToken applies a one-click RSVP and consumes the token.
// POST /rsvp/:token — the state-changing half of the token flow (0b).
func (h *Handler) ApplyRSVPToken(c echo.Context) error {
	tokenStr := c.Param("token")
	if tokenStr == "" {
		return c.HTML(http.StatusBadRequest, rsvpResultHTML("Invalid Link", "This RSVP link is invalid.", false))
	}
	token, err := h.svc.ApplyRSVPToken(c.Request().Context(), tokenStr)
	if err != nil {
		msg := apperror.UserMessage(err, "This RSVP link is invalid or has expired.")
		return c.HTML(http.StatusOK, rsvpResultHTML("RSVP Failed", msg, false))
	}
	var action string
	switch token.Action {
	case RSVPDeclined:
		action = "declined"
	case RSVPTentative:
		action = "marked as maybe"
	default:
		action = "accepted"
	}
	return c.HTML(http.StatusOK, rsvpResultHTML("RSVP Recorded",
		"Your response has been "+action+". You can close this page.", true))
}

// rsvpActionLabel renders an RSVP action for the confirm interstitial.
func rsvpActionLabel(action string) string {
	switch action {
	case RSVPDeclined:
		return "Can't Make It"
	case RSVPTentative:
		return "Maybe"
	default:
		return "Going"
	}
}

// isCampaignMember reports whether userID is currently a member of campaignID.
// FAIL-CLOSED: a nil lister or a lookup error denies (used by the public token
// routes to enforce current membership before applying — C-SCHED-P3 0a).
func (h *Handler) isCampaignMember(ctx context.Context, campaignID, userID string) bool {
	if h.memberLister == nil {
		return false
	}
	members, err := h.memberLister.ListMembers(ctx, campaignID)
	if err != nil {
		return false
	}
	for _, m := range members {
		if m.UserID == userID {
			return true
		}
	}
	return false
}

// rsvpResultHTML returns a simple standalone HTML page for RSVP token results.
// title + message are escaped (C-SCHED-P3 0c sweep) so any interpolated
// user/data value (e.g. a session name in a confirm message) is inert.
func rsvpResultHTML(title, message string, success bool) string {
	icon := "fa-circle-xmark"
	color := "red"
	if success {
		icon = "fa-circle-check"
		color = "green"
	}
	return `<!DOCTYPE html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + html.EscapeString(title) + ` - Chronicle</title>
<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.1/css/all.min.css">
<style>body{font-family:system-ui;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;background:#f8f9fa}
.card{text-align:center;padding:3rem;border-radius:12px;background:#fff;box-shadow:0 2px 12px rgba(0,0,0,.08);max-width:400px}
.icon{font-size:3rem;color:` + color + `;margin-bottom:1rem}h1{font-size:1.25rem;margin:0 0 .5rem}
p{color:#666;margin:0;font-size:.9rem}</style></head><body>
<div class="card"><div class="icon"><i class="fa-solid ` + icon + `"></i></div>
<h1>` + html.EscapeString(title) + `</h1><p>` + html.EscapeString(message) + `</p></div></body></html>`
}

// tokenConfirmHTML renders the GET interstitial for a one-click token: a POST
// form the user must submit to apply (C-SCHED-P3 0b). Because a mail scanner /
// link prefetcher issues a GET, not a POST, this page defeats the "state-changing
// GET" hazard for both the RSVP and proposal token routes. All interpolated
// values are escaped; actionURL is a same-origin token path.
//
// csrfToken is threaded into a hidden field because these POST routes ride the
// global CSRF middleware (they are not under the exempt /api/ or /ws prefixes):
// the GET already ran through the middleware and minted the cookie, so
// double-submit matches on the POST. Without it the confirm click would 403 and
// the apply would never run.
func tokenConfirmHTML(title, message, actionURL, confirmLabel, csrfToken string) string {
	return `<!DOCTYPE html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + html.EscapeString(title) + ` - Chronicle</title>
<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.1/css/all.min.css">
<style>body{font-family:system-ui;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;background:#f8f9fa}
.card{text-align:center;padding:3rem;border-radius:12px;background:#fff;box-shadow:0 2px 12px rgba(0,0,0,.08);max-width:400px}
.icon{font-size:3rem;color:#6366f1;margin-bottom:1rem}h1{font-size:1.25rem;margin:0 0 .5rem}
p{color:#666;margin:0 0 1.5rem;font-size:.9rem}
button{font:inherit;font-weight:600;padding:.65rem 1.6rem;border:0;border-radius:8px;background:#6366f1;color:#fff;cursor:pointer}</style></head><body>
<div class="card"><div class="icon"><i class="fa-solid fa-circle-question"></i></div>
<h1>` + html.EscapeString(title) + `</h1><p>` + html.EscapeString(message) + `</p>
<form method="POST" action="` + html.EscapeString(actionURL) + `"><input type="hidden" name="csrf_token" value="` + html.EscapeString(csrfToken) + `"><button type="submit">` + html.EscapeString(confirmLabel) + `</button></form>
</div></body></html>`
}

// SidebarRSVP returns an HTMX fragment showing planned sessions with RSVP statuses.
// GET /campaigns/:id/sidebar/sessions-rsvp
func (h *Handler) SidebarRSVP(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	userID := auth.GetUserID(c)

	planned, err := h.svc.ListPlannedSessions(ctx, cc.Campaign.ID)
	if err != nil {
		slog.Warn("sidebar RSVP: list planned sessions failed", slog.Any("error", err))
		return c.HTML(http.StatusOK, "") // Graceful degradation: empty sidebar section.
	}

	if len(planned) == 0 {
		return c.HTML(http.StatusOK, "") // Nothing to show.
	}

	// Fetch attendees for each planned session.
	for i := range planned {
		attendees, err := h.svc.ListAttendees(ctx, planned[i].ID)
		if err == nil {
			planned[i].Attendees = attendees
		}
	}

	return middleware.Render(c, http.StatusOK,
		SidebarSessionsRSVP(cc.Campaign.ID, planned, userID))
}

// EmbedSessions returns an HTMX fragment for the dashboard session tracker block.
// Shows upcoming planned sessions with RSVP counts in a compact format.
// GET /campaigns/:id/sessions/embed
func (h *Handler) EmbedSessions(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	userID := auth.GetUserID(c)

	planned, err := h.svc.ListPlannedSessions(ctx, cc.Campaign.ID)
	if err != nil {
		slog.Warn("embed sessions: list planned sessions failed", slog.Any("error", err))
		return c.HTML(http.StatusOK, "")
	}

	// Apply limit from query param (default 5, max 20).
	limit := 5
	if l := c.QueryParam("limit"); l != "" {
		if v, parseErr := strconv.Atoi(l); parseErr == nil && v >= 1 {
			limit = v
		}
		if limit > 20 {
			limit = 20
		}
	}
	if limit > len(planned) {
		limit = len(planned)
	}
	planned = planned[:limit]

	// Fetch attendees for each session.
	for i := range planned {
		attendees, err := h.svc.ListAttendees(ctx, planned[i].ID)
		if err == nil {
			planned[i].Attendees = attendees
		}
	}

	return middleware.Render(c, http.StatusOK,
		SessionsEmbedFragment(cc.Campaign.ID, planned, userID))
}

// --- Helpers ---

// requireSessionInCampaign fetches a session and verifies it belongs to the campaign.
func (h *Handler) requireSessionInCampaign(c echo.Context, sessionID, campaignID string) (*Session, error) {
	return middleware.RequireInCampaign(c.Request().Context(), h.svc.GetSession, sessionID, campaignID, "session")
}
