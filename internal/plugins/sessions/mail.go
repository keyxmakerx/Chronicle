package sessions

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/email"
	"github.com/keyxmakerx/chronicle/internal/notifyprefs"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// mailCampaign is what a game night email shows of its campaign: the name
// above the title and the colour of the links.
type mailCampaign struct {
	ID     string
	Name   string
	Accent string
}

// mailCampaignOf reads the campaign's name and site accent.
func mailCampaignOf(c *campaigns.Campaign) mailCampaign {
	if c == nil {
		return mailCampaign{}
	}
	return mailCampaign{ID: c.ID, Name: c.Name, Accent: c.ParseSettings().AccentColor}
}

// message starts an email for this campaign, ending with the line that
// names the switch that sent it and links to the reader's choices.
func (h *Handler) message(mc mailCampaign, category, title string) email.Message {
	m := email.Message{Campaign: mc.Name, Accent: mc.Accent, Title: title}
	if label := notifyprefs.LabelOf(category); label != "" {
		m.Why = fmt.Sprintf("You get this because %q is on for email.", label)
		m.PrefsURL = h.baseURL + "/account#notifications"
	}
	return m
}

// sendMessage renders m in the campaign's layout and sends it to one person.
func (h *Handler) sendMessage(ctx context.Context, to, subject string, m email.Message) error {
	plain, html, err := email.Render(email.DefaultLayout, m)
	if err != nil {
		return err
	}
	return h.mailer.SendHTMLMail(ctx, []string{to}, subject, plain, html)
}

// rsvpAnswers are the one-press answers to a game night invite.
func (h *Handler) rsvpAnswers(t RSVPTokens) []email.Answer {
	return []email.Answer{
		{Label: "Going", URL: h.baseURL + "/rsvp/" + t.Accept},
		{Label: "Maybe", URL: h.baseURL + "/rsvp/" + t.Maybe},
		{Label: "Can't make it", URL: h.baseURL + "/rsvp/" + t.Decline},
	}
}

// memberWhen is a game night's date and time as one member should read it:
// in their own zone when Chronicle knows it, otherwise in the organizer's
// with its zone label, so an email never shows a bare time.
func (h *Handler) memberWhen(ctx context.Context, s *Session, userID string) string {
	if h.userDir != nil {
		if u, err := h.userDir.GetUser(ctx, userID); err == nil && u != nil && u.Timezone != nil && *u.Timezone != "" {
			return s.FormatScheduledFor(*u.Timezone)
		}
	}
	return s.FormatScheduledWhen()
}

// linksExpire is the small print under one-press answers.
const linksExpire = "These links expire in 7 days."
