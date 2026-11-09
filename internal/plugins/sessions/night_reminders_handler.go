package sessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/email"
	"github.com/keyxmakerx/chronicle/internal/notifyprefs"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RemindNightAPI asks players to answer one game night, on the site and by
// email. Only the night's organiser, the campaign owner or a co-Director may.
// POST /campaigns/:id/sessions/:sid/remind {"date": "YYYY-MM-DD", "userId": optional}
func (h *Handler) RemindNightAPI(c *echo.Context) error {
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
		go h.sendNightReminderEmails(context.Background(), session, rem.Night, mailCampaignOf(cc.Campaign), askerLabel(asker), to)
	}
	return c.JSON(http.StatusOK, rem.Result)
}

// sendNightReminderEmails emails each reminded player. A one-off night's
// email carries the same one-press Going, Maybe and Can't make it answers as
// the invite; a repeating night's answers are kept per night, which those
// links can't name, so its email opens the night on the calendar instead.
// Best-effort and off the request path: the bell already carries it.
func (h *Handler) sendNightReminderEmails(ctx context.Context, session *Session, night GameNight, mc mailCampaign, asker string, members []campaigns.CampaignMember) {
	if h.mailer == nil || !h.mailer.IsConfigured(ctx) {
		return
	}
	when := nightWhenWords(night.Date, night.Time)
	if night.TZ != "" {
		when += " (" + night.TZ + ")"
	}
	openURL := h.baseURL + nightsLink(session.CampaignID, session.ID, night.Date)
	subject := fmt.Sprintf("Are you coming? %s — %s", session.Name, mc.Name)
	// The stub reads the night as the words above do, on its own clock.
	stub := ticketStub(&Session{ScheduledDate: &night.Date, ScheduledTime: &night.Time}, "")
	var from *email.Sender
	if asker != askerLabel("") {
		from = mc.sender(asker, false)
	}
	for _, m := range members {
		if m.Email == "" || !h.emailWanted(ctx, m.UserID, notifyprefs.GameNightInvites) {
			continue
		}
		msg := h.message(mc, notifyprefs.GameNightInvites, session.Name)
		msg.Lines = []string{fmt.Sprintf("%s asked whether you're coming on %s.", asker, when)}
		msg.Stub = stub
		msg.Sender = from
		if !night.Recurring {
			tokens, err := h.svc.CreateRSVPTokens(ctx, session.ID, m.UserID)
			if err != nil {
				slog.Warn("night reminder: creating rsvp tokens failed", slog.Any("error", err), slog.String("user_id", m.UserID))
				continue
			}
			msg.Answers = h.rsvpAnswers(tokens)
			msg.Small = linksExpire
		}
		msg.Links = []email.Answer{{Label: "Answer on the calendar", URL: openURL}}
		if err := h.sendMessage(ctx, mc, m.Email, subject, msg); err != nil {
			slog.Warn("night reminder: sending email failed", slog.Any("error", err), slog.String("session_id", session.ID))
		}
	}
}
