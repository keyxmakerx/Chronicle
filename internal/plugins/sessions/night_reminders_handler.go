package sessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/notifyprefs"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RemindNightAPI asks players to answer one game night, on the site and by
// email. Only the night's organiser, the campaign owner or a co-Director may.
// POST /campaigns/:id/sessions/:sid/remind {"date": "YYYY-MM-DD", "userId": optional}
func (h *Handler) RemindNightAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	session, err := h.requireSessionInCampaign(c, c.Param("sid"), cc.Campaign.ID)
	if err != nil {
		return err
	}
	userID := auth.GetUserID(c)
	if session.CreatedBy != userID && cc.MemberRole < campaigns.RoleOwner && !cc.IsDmGranted {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "only the person running this game night can send reminders"})
	}
	var req struct {
		Date   string `json:"date"`
		UserID string `json:"userId"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	if h.memberLister == nil {
		return apperror.NewInternal(errors.New("sessions: member lister not wired"))
	}
	list, err := h.memberLister.ListMembers(ctx, cc.Campaign.ID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	members := make([]NightMember, 0, len(list))
	for _, m := range list {
		members = append(members, NightMember{UserID: m.UserID, Name: m.DisplayName})
	}
	asker := h.resolveDisplayName(ctx, userID)
	rem, err := h.svc.RemindNight(ctx, cc.Campaign.ID, session.ID, req.Date, userID, asker, req.UserID, gameNightsToday(), members)
	if err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}
	if len(rem.UserIDs) > 0 && h.mailer != nil {
		want := map[string]bool{}
		for _, id := range rem.UserIDs {
			want[id] = true
		}
		var to []campaigns.CampaignMember
		for _, m := range list {
			if want[m.UserID] {
				to = append(to, m)
			}
		}
		go h.sendNightReminderEmails(context.Background(), session, rem.Night, cc.Campaign.Name, askerLabel(asker), to)
	}
	return c.JSON(http.StatusOK, rem.Result)
}

// sendNightReminderEmails emails each reminded player. A one-off night's
// email carries the same one-click Going / Can't make it buttons as the
// invite; a repeating night's answers are kept per night, which those links
// can't name, so its email opens the night on the calendar instead.
// Best-effort and off the request path: the bell already carries it.
func (h *Handler) sendNightReminderEmails(ctx context.Context, session *Session, night GameNight, campaignName, asker string, members []campaigns.CampaignMember) {
	if h.mailer == nil || !h.mailer.IsConfigured(ctx) {
		return
	}
	footPlain, footHTML := h.prefsFooter()
	when := nightWhenWords(night.Date, night.Time)
	if night.TZ != "" {
		when += " (" + night.TZ + ")"
	}
	openURL := h.baseURL + nightsLink(session.CampaignID, session.ID, night.Date)
	subject := fmt.Sprintf("Are you coming? %s — %s", session.Name, campaignName)
	for _, m := range members {
		if m.Email == "" || !h.emailWanted(ctx, m.UserID, notifyprefs.GameNightInvites) {
			continue
		}
		plain := fmt.Sprintf("%s asked you to say whether you're coming.\n\nGame night: %s\nCampaign: %s\nWhen: %s\n\n", asker, session.Name, campaignName, when)
		buttons := ""
		if !night.Recurring {
			accept, decline, _, err := h.svc.CreateRSVPTokens(ctx, session.ID, m.UserID)
			if err != nil {
				slog.Warn("night reminder: creating rsvp tokens failed", slog.Any("error", err), slog.String("user_id", m.UserID))
				continue
			}
			acceptURL := fmt.Sprintf("%s/rsvp/%s", h.baseURL, accept)
			declineURL := fmt.Sprintf("%s/rsvp/%s", h.baseURL, decline)
			plain += fmt.Sprintf("Going: %s\nCan't make it: %s\n\nThese links expire in 7 days. ", acceptURL, declineURL)
			buttons = fmt.Sprintf(`<a href="%s" style="display:inline-block;padding:10px 24px;background:#22c55e;color:#fff;text-decoration:none;border-radius:6px;font-weight:600;margin:0 8px">✓ Going</a>
  <a href="%s" style="display:inline-block;padding:10px 24px;background:#ef4444;color:#fff;text-decoration:none;border-radius:6px;font-weight:600;margin:0 8px">✗ Can't Make It</a>`, acceptURL, declineURL)
		}
		plain += "Or answer on the calendar: " + openURL + "\n" + footPlain
		htmlBody := fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="utf-8"></head><body style="font-family:system-ui,-apple-system,sans-serif;max-width:480px;margin:0 auto;padding:20px;color:#333">
<div style="text-align:center;margin-bottom:24px">
  <div style="font-size:32px;margin-bottom:8px">🎲</div>
  <h1 style="font-size:20px;margin:0">Are you coming?</h1>
  <p style="margin:8px 0 0;color:#666;font-size:14px">%s asked you to say whether you're coming.</p>
</div>
<div style="background:#f8f9fa;border-radius:8px;padding:20px;margin-bottom:24px">
  <h2 style="font-size:16px;margin:0 0 8px">%s</h2>
  <p style="margin:4px 0;color:#666;font-size:14px"><strong>Campaign:</strong> %s</p>
  <p style="margin:4px 0;color:#666;font-size:14px"><strong>When:</strong> %s</p>
</div>
<div style="text-align:center;margin-bottom:16px">%s</div>
<p style="text-align:center;margin:0 0 24px"><a href="%s" style="color:#6366f1;font-size:13px">Answer on the calendar</a></p>
%s
</body></html>`,
			// Names and the zone are typed by people, so they are escaped; the
			// URLs carry hex tokens and ids only.
			html.EscapeString(asker), html.EscapeString(session.Name), html.EscapeString(campaignName), html.EscapeString(when), buttons, openURL, footHTML)
		if err := h.mailer.SendHTMLMail(ctx, []string{m.Email}, subject, plain, htmlBody); err != nil {
			slog.Warn("night reminder: sending email failed", slog.Any("error", err), slog.String("session_id", session.ID))
		}
	}
}
