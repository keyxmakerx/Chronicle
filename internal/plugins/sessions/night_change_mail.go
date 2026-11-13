package sessions

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/email"
	"github.com/keyxmakerx/chronicle/internal/notifyprefs"
)

// nightChange is what happened to a game night that people had said Going
// or Maybe to.
type nightChange int

const (
	nightMoved nightChange = iota
	nightCancelled
	nightBackOn
)

// emailNightChange tells everyone who said Going or Maybe that the night
// moved, was cancelled or is back on, in the campaign's layout. People who
// said Can't make it get the bell only: a change rarely matters to them, and
// the bell still reaches them. before is the night as it was (moves only);
// actorID is who made the change, shown as the sender in the Chat layout.
func (h *Handler) emailNightChange(ctx context.Context, mc mailCampaign, change nightChange, before, s *Session, actorID string) {
	if h.mailer == nil || h.memberLister == nil || s == nil || !h.mailer.IsConfigured(ctx) {
		return
	}
	aud, err := h.svc.NightChangeAudience(ctx, s)
	if err != nil {
		slog.Warn("listing who to email about a changed game night", slog.Any("error", err), slog.String("session_id", s.ID))
		return
	}
	if aud == nil || len(aud.UserIDs) == 0 {
		return
	}
	// A repeating series is described by its next night, not its first.
	s = aud.Night
	want := make(map[string]bool, len(aud.UserIDs))
	for _, id := range aud.UserIDs {
		want[id] = true
	}
	members, err := h.memberLister.ListMembers(ctx, mc.ID)
	if err != nil {
		slog.Warn("listing members to email about a changed game night", slog.Any("error", err), slog.String("session_id", s.ID))
		return
	}
	var from *email.Sender
	if actorID != "" {
		from = mc.senderAmong(members, actorID, h.resolveDisplayName(ctx, actorID))
	}
	for _, m := range members {
		if !want[m.UserID] || m.Email == "" || !h.emailWanted(ctx, m.UserID, notifyprefs.GameNightChanges) {
			continue
		}
		subject, msg, ok := h.nightChangeMessage(ctx, mc, change, before, s, m.UserID)
		if !ok {
			continue
		}
		msg.Sender = from
		if err := h.sendMessage(ctx, mc, m.Email, subject, msg); err != nil {
			slog.Warn("failed to send a changed game night email", slog.Any("error", err), slog.String("to", m.Email), slog.String("session_id", s.ID))
		}
	}
}

// nightChangeMessage writes one member's email about the change, on their
// own clock. A move to a one-off night asks again with one-press answers,
// since the member's answer now needs checking; a repeating night links to
// its page instead, because those answers are kept per night.
func (h *Handler) nightChangeMessage(ctx context.Context, mc mailCampaign, change nightChange, before, s *Session, userID string) (string, email.Message, bool) {
	zone := h.memberZone(ctx, userID)
	when := memberWhen(s, zone)
	page := h.baseURL + "/campaigns/" + mc.ID + "/sessions/" + s.ID
	msg := h.message(mc, notifyprefs.GameNightChanges, s.Name)
	switch change {
	case nightMoved:
		moved := "Its date is no longer set."
		if when != "" {
			moved = "It has moved to " + when + "."
		}
		if before != nil {
			if was := memberWhen(before, zone); was != "" {
				moved += " It was " + was + "."
			}
		}
		msg.Lines = []string{moved, "Can you still make it?"}
		msg.Stub = ticketStub(s, zone)
		if s.IsRecurring {
			msg.Links = []email.Answer{{Label: "Check my answer", URL: page}}
			return fmt.Sprintf("Moved: %s — %s", s.Name, mc.Name), msg, true
		}
		tokens, err := h.svc.CreateRSVPTokens(ctx, s.ID, userID)
		if err != nil {
			slog.Warn("failed to create rsvp tokens for a moved game night", slog.Any("error", err), slog.String("user_id", userID))
			return "", msg, false
		}
		msg.Answers = h.rsvpAnswers(tokens)
		msg.Links = []email.Answer{{Label: "Suggest another time", URL: h.baseURL + "/rsvp/" + tokens.Suggest + "/suggest"}}
		msg.Small = linksExpire
		return fmt.Sprintf("Moved: %s — %s", s.Name, mc.Name), msg, true
	case nightCancelled:
		line := "This game night is cancelled."
		if when != "" {
			line = "The game night on " + when + " is cancelled."
		}
		msg.Lines = []string{line, "Your answer is kept, so if it's put back you won't need to answer again."}
		msg.Links = []email.Answer{{Label: "See game nights", URL: h.baseURL + "/campaigns/" + mc.ID + "/game-nights"}}
		return fmt.Sprintf("Cancelled: %s — %s", s.Name, mc.Name), msg, true
	case nightBackOn:
		line := "It's back on, with your answer as you left it."
		if when != "" {
			line = "It's back on for " + when + ", with your answer as you left it."
		}
		msg.Lines = []string{line}
		msg.Stub = ticketStub(s, zone)
		msg.Links = []email.Answer{{Label: "See the game night", URL: page}}
		return fmt.Sprintf("Back on: %s — %s", s.Name, mc.Name), msg, true
	}
	return "", msg, false
}

// EmailBackOn sends the back-on email for a game night restored from the
// campaign Trash, where no request carries the campaign or who restored it.
func (h *Handler) EmailBackOn(ctx context.Context, campaignID string, s *Session) {
	if h.campaignReader == nil {
		return
	}
	c, err := h.campaignReader.GetByID(ctx, campaignID)
	if err != nil {
		slog.Warn("reading the campaign for a back-on email", slog.Any("error", err), slog.String("campaign_id", campaignID))
		return
	}
	h.emailNightChange(ctx, mailCampaignOf(c), nightBackOn, nil, s, "")
}
