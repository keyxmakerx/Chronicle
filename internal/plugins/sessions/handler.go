package sessions

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/patch"
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
	baseURL      string        // Application base URL for RSVP links (e.g. "https://chronicle.example.com").
	userDir      UserDirectory // Resolves a user's stored IANA timezone for the availability overlay.
	// campaignReader is the one-read source of the co-DM grant set the overlay
	// roster's role column needs. Nil-safe.
	campaignReader CampaignReader
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

	// ADR-055 rule 3: a linked entity's name must be ABSENT for a viewer who
	// could not see that entity directly — never rendered, not merely hidden
	// by CSS or replaced with a placeholder. The repository join that built
	// session.Entities has no privacy predicate of its own (it can't, without
	// importing the entities plugin's repository — rule 8), so this is the
	// gate.
	ctx := c.Request().Context()
	visibleEntities, err := h.svc.FilterEntitiesForViewer(ctx, cc.Campaign.ID, session.Entities, int(cc.VisibilityRole()), userID)
	if err != nil {
		return err
	}
	session.Entities = visibleEntities

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
	scheduledTime := c.FormValue("scheduled_time") // "HH:MM" from the modal's time input.

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

// updateSessionRequest is the wire form of PUT /campaigns/:id/sessions/:sid.
//
// It is a PARTIAL update: absent preserves, explicit null clears, a present
// value replaces (see UpdateSessionInput and API-CONTRACT.md). patch.Field is
// what makes "absent" and "null" different here — a plain pointer collapses
// them, which would let a {status}-only "Mark Complete" body wipe every other
// field.
//
// It is a named type so the regression test can decode the real templ
// clients' literal request bodies through the real binder.
type updateSessionRequest struct {
	Name                patch.Field[string] `json:"name"`
	Summary             patch.Field[string] `json:"summary"`
	ScheduledDate       patch.Field[string] `json:"scheduled_date"`
	ScheduledTime       patch.Field[string] `json:"scheduled_time"`
	CalendarYear        patch.Field[int]    `json:"calendar_year"`
	CalendarMonth       patch.Field[int]    `json:"calendar_month"`
	CalendarDay         patch.Field[int]    `json:"calendar_day"`
	Status              patch.Field[string] `json:"status"`
	IsRecurring         patch.Field[bool]   `json:"is_recurring"`
	RecurrenceType      patch.Field[string] `json:"recurrence_type"`
	RecurrenceInterval  patch.Field[int]    `json:"recurrence_interval"`
	RecurrenceDayOfWeek patch.Field[int]    `json:"recurrence_day_of_week"`
	RecurrenceEndDate   patch.Field[string] `json:"recurrence_end_date"`
}

// toInput maps the wire request onto the service input. Pure field carriage —
// the presence state travels with the value, so no logic belongs here. The
// direct conversion is deliberate: it compiles only while the two structs
// stay field-for-field identical, so a field added to one and not the other
// is a build error rather than a silently-dropped key.
func (r updateSessionRequest) toInput() UpdateSessionInput {
	return UpdateSessionInput(r)
}

// UpdateSessionAPI updates a session.
// PUT /campaigns/:id/sessions/:sid
func (h *Handler) UpdateSessionAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	sessionID := c.Param("sid")

	existing, err := h.requireSessionInCampaign(c, sessionID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	var req updateSessionRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	// MoveSession wraps UpdateSession's exact partial update and additionally
	// flags-and-reports responders ONLY when the stored schedule actually
	// changed on a session that already has RSVP responses (a recap save or
	// a Mark Complete leaves nextSession's own meaning — the auto-generated
	// next occurrence, if any — untouched).
	nextSession, responded, err := h.svc.MoveSession(c.Request().Context(), sessionID, req.toInput())
	if err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}
	if len(responded) > 0 {
		msg := "\"" + existing.Name + "\" was moved. Please check your answer."
		link := "/campaigns/" + cc.Campaign.ID + "/sessions/" + sessionID
		if nErr := h.svc.NotifyUsers(c.Request().Context(), responded, cc.Campaign.ID, NotifSessionMoved, msg, link); nErr != nil {
			slog.Error("notifying responded members of a moved session", slog.String("error", nErr.Error()))
		}
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
			"status":          "ok",
			"next_session_id": nextSession.ID,
		})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteSessionAPI soft-deletes a session (co-Director/Owner — see routes.go)
// and, per the operator's "moving or deleting an event that collects RSVPs
// notifies the people who answered" rule, tells everyone who had already
// responded. Their answer is kept, not cleared — RestoreSessionAPI below puts
// it back exactly as it was.
// DELETE /campaigns/:id/sessions/:sid
func (h *Handler) DeleteSessionAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	sessionID := c.Param("sid")

	sess, err := h.requireSessionInCampaign(c, sessionID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	responded, err := h.svc.DeleteSession(c.Request().Context(), sessionID)
	if err != nil {
		return err
	}
	if len(responded) > 0 {
		msg := "\"" + sess.Name + "\" was cancelled."
		link := "/campaigns/" + cc.Campaign.ID + "/sessions"
		if nErr := h.svc.NotifyUsers(c.Request().Context(), responded, cc.Campaign.ID, NotifSessionCancelled, msg, link); nErr != nil {
			slog.Error("notifying responded members of a cancelled session", slog.String("error", nErr.Error()))
		}
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect",
			"/campaigns/"+cc.Campaign.ID+"/sessions")
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// RestoreSessionAPI undoes a soft delete (co-Director/Owner — see routes.go)
// and tells everyone who had answered before it was cancelled that it's back
// on, so a stale "cancelled" notification doesn't stay the last word they saw.
// POST /campaigns/:id/sessions/:sid/restore
func (h *Handler) RestoreSessionAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	sessionID := c.Param("sid")

	// requireSessionInCampaign goes through GetSession -> FindByID, which
	// filters deleted_at IS NULL — a session to be RESTORED is by definition
	// still soft-deleted at this point, so that check would always 404 here.
	// GetSessionIncludingDeleted is the one read path built to see it; the
	// campaign-id comparison below is the same IDOR scoping
	// requireSessionInCampaign would otherwise have done.
	existing, err := h.svc.GetSessionIncludingDeleted(c.Request().Context(), sessionID)
	if err != nil {
		return err
	}
	if existing.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("session not found")
	}

	sess, responded, err := h.svc.RestoreSession(c.Request().Context(), sessionID)
	if err != nil {
		return err
	}
	if len(responded) > 0 {
		msg := "\"" + sess.Name + "\" is back on."
		link := "/campaigns/" + cc.Campaign.ID + "/sessions/" + sess.ID
		if nErr := h.svc.NotifyUsers(c.Request().Context(), responded, cc.Campaign.ID, NotifSessionRestored, msg, link); nErr != nil {
			slog.Error("notifying responded members of a restored session", slog.String("error", nErr.Error()))
		}
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect",
			"/campaigns/"+cc.Campaign.ID+"/sessions/"+sess.ID)
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
	// note/occurrenceDate are an OPT-IN extension (Part C: game-night RSVP):
	// a caller that never sends them keeps hitting the exact old path
	// (UpdateRSVP, session_attendees, unchanged for every existing session,
	// recurring or not). Only a caller that explicitly sends one of them
	// engages UpdateRSVPDetailed, so no shipped caller's behavior changes.
	var note patch.Field[string]
	var occurrenceDate *string
	if status == "" {
		var req struct {
			Status         string              `json:"status"`
			Note           patch.Field[string] `json:"note"`
			OccurrenceDate *string             `json:"occurrenceDate"`
		}
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err == nil {
			status = req.Status
			note = req.Note
			occurrenceDate = req.OccurrenceDate
		}
	}

	var err error
	if note.Present() || occurrenceDate != nil {
		err = h.svc.UpdateRSVPDetailed(c.Request().Context(), sessionID, userID, status, occurrenceDate, note)
	} else {
		err = h.svc.UpdateRSVP(c.Request().Context(), sessionID, userID, status)
	}
	if err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}

	// A per-occurrence answer has no session_attendees row to re-render from
	// (see UpdateRSVPDetailed's doc comment) — the occurrence roster fragment
	// is a future UI's job, not built here (documented gap).
	if occurrenceDate == nil && middleware.IsHTMX(c) {
		// Re-render the attendee list.
		attendees, _ := h.svc.ListAttendees(c.Request().Context(), sessionID)
		csrfToken := middleware.GetCSRFToken(c)
		return middleware.Render(c, http.StatusOK,
			AttendeeList(cc, sessionID, attendees, csrfToken, userID))
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// SetRSVPExcludedAPI is the Director's own "leave myself out of the tally"
// switch (operator answer #3: the Director counts like any member, with a
// switch to leave themselves out). Self-only and organizer/owner-only — the
// service enforces that, this handler just passes the caller's own id and
// campaign role through.
// PUT /campaigns/:id/sessions/:sid/rsvp-exclude
func (h *Handler) SetRSVPExcludedAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	sessionID := c.Param("sid")
	userID := auth.GetUserID(c)

	if _, err := h.requireSessionInCampaign(c, sessionID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
		Excluded       bool    `json:"excluded"`
		OccurrenceDate *string `json:"occurrenceDate"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	isOwner := cc.MemberRole >= campaigns.RoleOwner
	if err := h.svc.SetExcludedFromCount(c.Request().Context(), sessionID, userID, isOwner, req.Excluded, req.OccurrenceDate); err != nil {
		return err
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
			// can't inject markup into the email. dateStr is our own formatted
			// label; URLs are hex tokens — both safe.
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
// read: it validates the token and shows a POST form; ApplyRSVPToken records
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
	if !h.tokenUserStillBelongs(c.Request().Context(), token) {
		return c.HTML(http.StatusOK, rsvpResultHTML("RSVP Failed",
			"You're no longer a member of this campaign, so this link can't be used.", false))
	}
	label := rsvpActionLabel(token.Action)
	return c.HTML(http.StatusOK, tokenConfirmHTML("Confirm Your RSVP",
		fmt.Sprintf("You're responding %q. Tap below to confirm.", label),
		fmt.Sprintf("/rsvp/%s", tokenStr), "Confirm — "+label, middleware.GetCSRFToken(c)))
}

// ApplyRSVPToken applies a one-click RSVP and consumes the token.
// POST /rsvp/:token — the state-changing half of the token flow.
func (h *Handler) ApplyRSVPToken(c echo.Context) error {
	tokenStr := c.Param("token")
	if tokenStr == "" {
		return c.HTML(http.StatusBadRequest, rsvpResultHTML("Invalid Link", "This RSVP link is invalid.", false))
	}
	// A link cannot outlive the access that justified it: the token is
	// resolved and the roster re-checked BEFORE anything is applied, exactly
	// as /proposals/respond/:token and /calendar-rsvp/:token do, so a player
	// removed from the campaign cannot still apply a stale RSVP link.
	preToken, err := h.svc.ValidateRSVPToken(c.Request().Context(), tokenStr)
	if err != nil {
		msg := apperror.UserMessage(err, "This RSVP link is invalid or has expired.")
		return c.HTML(http.StatusOK, rsvpResultHTML("RSVP Failed", msg, false))
	}
	if !h.tokenUserStillBelongs(c.Request().Context(), preToken) {
		return c.HTML(http.StatusOK, rsvpResultHTML("RSVP Failed",
			"You're no longer a member of this campaign, so this link can't be used.", false))
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

// tokenUserStillBelongs reports whether an RSVP token's user is STILL a member
// of the campaign the token's session belongs to.
//
// FAIL-CLOSED throughout: a missing session, a lookup error, a nil member lister
// — all deny. The campaign is derived from the token's own session, never from a
// URL, because the token is the credential and there is no campaign in the path
// on the public /rsvp/:token route.
func (h *Handler) tokenUserStillBelongs(ctx context.Context, token *RSVPToken) bool {
	if token == nil {
		return false
	}
	session, err := h.svc.GetSession(ctx, token.SessionID)
	if err != nil || session == nil {
		return false
	}
	return h.isCampaignMember(ctx, session.CampaignID, token.UserID)
}

// isCampaignMember reports whether userID is currently a member of campaignID.
// FAIL-CLOSED: a nil lister or a lookup error denies. Used by the public
// token routes to enforce current membership before applying.
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
// title + message are escaped so any interpolated user/data value (e.g. a
// session name in a confirm message) is inert.
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
// form the user must submit to apply. Because a mail scanner / link
// prefetcher issues a GET, not a POST, this page defeats the "state-changing
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

// --- Calendar feed (operator answer #1: a private, replaceable per-member
// link into the member's own calendar app, on for every campaign by default,
// owner can switch off) ---

// GetFeedSettingsAPI reports whether this campaign's calendar feed is on.
// GET /campaigns/:id/sessions/feed-settings
func (h *Handler) GetFeedSettingsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	enabled, err := h.svc.IsCalendarFeedEnabled(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]bool{"enabled": enabled})
}

// SetFeedSettingsAPI is the owner's campaign-wide kill switch.
// PUT /campaigns/:id/sessions/feed-settings
func (h *Handler) SetFeedSettingsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if err := h.svc.SetCalendarFeedEnabled(c.Request().Context(), cc.Campaign.ID, req.Enabled); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// GetFeedTokenAPI returns (creating on first call) the caller's own private
// feed URL.
// POST /campaigns/:id/sessions/feed/token
func (h *Handler) GetFeedTokenAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)
	tok, err := h.svc.GetOrCreateFeedToken(c.Request().Context(), cc.Campaign.ID, userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]string{"url": "/sessions/feed/" + tok.Token + ".ics"})
}

// ReplaceFeedTokenAPI mints a new token and drops the old one at once — "if
// it gets out, replace it: the old link stops working immediately."
// POST /campaigns/:id/sessions/feed/token/replace
func (h *Handler) ReplaceFeedTokenAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)
	tok, err := h.svc.ReplaceFeedToken(c.Request().Context(), cc.Campaign.ID, userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]string{"url": "/sessions/feed/" + tok.Token + ".ics"})
}

// GameNightFeedICS serves one member's private game-night feed as an ICS
// subscription. Public — the token IS the credential, mirroring /rsvp/:token
// — so an unknown token and a campaign-disabled feed both answer a plain 404
// rather than distinguishing "wrong token" from "turned off" to a stranger.
// GET /sessions/feed/:token(.ics)
func (h *Handler) GameNightFeedICS(c echo.Context) error {
	tokenStr := strings.TrimSuffix(c.Param("token"), ".ics")
	campaignID, userID, err := h.svc.ResolveFeedToken(c.Request().Context(), tokenStr)
	if err != nil {
		return c.String(http.StatusNotFound, "not found")
	}
	// This token has no expiry (unlike a session RSVP token), so a member
	// removed from the campaign after subscribing must be re-checked on
	// every fetch — otherwise their calendar app keeps pulling every planned
	// session indefinitely. Checked BEFORE any session data is read (see
	// ResolveFeedToken/ListFeedSessions' split), fails closed like every
	// other token route.
	if !h.isCampaignMember(c.Request().Context(), campaignID, userID) {
		return c.String(http.StatusNotFound, "not found")
	}
	sessions, err := h.svc.ListFeedSessions(c.Request().Context(), campaignID)
	if err != nil {
		return c.String(http.StatusNotFound, "not found")
	}
	c.Response().Header().Set("Content-Type", "text/calendar; charset=utf-8")
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.String(http.StatusOK, h.svc.BuildFeedICS(sessions))
}

// --- "Suggest another time" (the third RSVP email-link fix: the token is
// consumed only AFTER the suggestion validates, never before) ---

// RedeemSuggestToken shows the suggest-a-time form for an emailed
// "Suggest another time" link. Read-only — nothing is written until the form
// is submitted, matching every other token route's GET-confirm/POST-apply
// split (a mail scanner's background GET must never act).
// GET /rsvp/:token/suggest
func (h *Handler) RedeemSuggestToken(c echo.Context) error {
	tokenStr := c.Param("token")
	token, err := h.svc.ValidateRSVPToken(c.Request().Context(), tokenStr)
	if err != nil || token.Action != RSVPActionSuggest {
		return c.HTML(http.StatusOK, rsvpResultHTML("Invalid Link",
			"This link is invalid, has expired, or has already been used.", false))
	}
	// Same re-check every other token route in this file makes: a link
	// cannot outlive the access that justified it. Without this, a member
	// removed from the campaign (or a link for a since-deleted session)
	// could still submit a suggestion and notify the organizer.
	if !h.tokenUserStillBelongs(c.Request().Context(), token) {
		return c.HTML(http.StatusOK, rsvpResultHTML("Invalid Link",
			"You're no longer a member of this campaign, so this link can't be used.", false))
	}
	return c.HTML(http.StatusOK, suggestFormHTML(
		fmt.Sprintf("/rsvp/%s/suggest", tokenStr), middleware.GetCSRFToken(c)))
}

// ApplySuggestToken records the suggestion — see
// SessionService.ValidateAndRecordSuggestion for the validate-before-consume
// ordering — and tells the organizer.
// POST /rsvp/:token/suggest
func (h *Handler) ApplySuggestToken(c echo.Context) error {
	tokenStr := c.Param("token")

	preToken, err := h.svc.ValidateRSVPToken(c.Request().Context(), tokenStr)
	if err != nil || preToken.Action != RSVPActionSuggest {
		return c.HTML(http.StatusOK, rsvpResultHTML("Suggestion Failed",
			"This link is invalid, has expired, or has already been used.", false))
	}
	if !h.tokenUserStillBelongs(c.Request().Context(), preToken) {
		return c.HTML(http.StatusOK, rsvpResultHTML("Suggestion Failed",
			"You're no longer a member of this campaign, so this link can't be used.", false))
	}

	date := c.FormValue("date")
	var timeVal, note *string
	if v := c.FormValue("time"); v != "" {
		timeVal = &v
	}
	if v := c.FormValue("note"); v != "" {
		note = &v
	}

	suggestion, err := h.svc.ValidateAndRecordSuggestion(c.Request().Context(), tokenStr, date, timeVal, note)
	if err != nil {
		msg := apperror.UserMessage(err, "This link is invalid or has expired.")
		return c.HTML(http.StatusOK, rsvpResultHTML("Suggestion Failed", msg, false))
	}

	if sess, sErr := h.svc.GetSession(c.Request().Context(), suggestion.SessionID); sErr == nil && sess != nil {
		msg := fmt.Sprintf("A player suggested a different time for %q.", sess.Name)
		link := "/campaigns/" + sess.CampaignID + "/sessions/" + sess.ID
		if nErr := h.svc.NotifyUsers(c.Request().Context(), []string{sess.CreatedBy}, sess.CampaignID,
			NotifSessionRescheduleSuggested, msg, link); nErr != nil {
			slog.Error("notifying organizer of a suggested time", slog.String("error", nErr.Error()))
		}
	}

	return c.HTML(http.StatusOK, rsvpResultHTML("Suggestion Sent",
		"Thanks — the organizer has been told. You can close this page.", true))
}

// suggestFormHTML renders the "suggest another time" form, matching
// tokenConfirmHTML/rsvpResultHTML's minimal inline-styled page.
func suggestFormHTML(actionURL, csrfToken string) string {
	return `<!DOCTYPE html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Suggest another time - Chronicle</title>
<style>body{font-family:system-ui;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;background:#f8f9fa}
.card{text-align:center;padding:2.5rem;border-radius:12px;background:#fff;box-shadow:0 2px 12px rgba(0,0,0,.08);max-width:360px;width:100%}
h1{font-size:1.25rem;margin:0 0 .5rem}p{color:#666;margin:0 0 1.25rem;font-size:.9rem}
label{display:block;text-align:left;font-size:.8rem;font-weight:600;margin:.75rem 0 .25rem}
input,textarea{width:100%;box-sizing:border-box;padding:.5rem;border:1px solid #ddd;border-radius:6px;font:inherit}
button{margin-top:1.25rem;font:inherit;font-weight:600;padding:.65rem 1.6rem;border:0;border-radius:8px;background:#6366f1;color:#fff;cursor:pointer;width:100%}</style></head><body>
<div class="card"><h1>Suggest another time</h1><p>Pick a date and, if you know it, a time. The organizer will be told — this does not change your own answer.</p>
<form method="POST" action="` + html.EscapeString(actionURL) + `">
<input type="hidden" name="csrf_token" value="` + html.EscapeString(csrfToken) + `">
<label for="sug-date">Date</label><input id="sug-date" type="date" name="date" required>
<label for="sug-time">Time (optional)</label><input id="sug-time" type="time" name="time">
<label for="sug-note">Note (optional)</label><textarea id="sug-note" name="note" maxlength="140" rows="2"></textarea>
<button type="submit">Send suggestion</button>
</form></div></body></html>`
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
