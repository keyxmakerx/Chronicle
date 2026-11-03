package sessions

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/email"
	"github.com/keyxmakerx/chronicle/internal/notifyprefs"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/systems"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// mailCampaign is what a game night email takes from its campaign: the name
// above the title, the colour, the layout the owner picked and the word the
// game uses for the person running it.
type mailCampaign struct {
	ID     string
	Name   string
	Accent string
	Layout email.Layout
	GMTerm string
}

// mailCampaignOf reads the campaign's name, site accent and email layout.
func mailCampaignOf(c *campaigns.Campaign) mailCampaign {
	if c == nil {
		return mailCampaign{Layout: email.DefaultLayout}
	}
	st := c.ParseSettings()
	mc := mailCampaign{ID: c.ID, Name: c.Name, Accent: st.AccentColor, Layout: campaigns.EmailLayoutOf(c), GMTerm: layouts.DefaultGMTerm}
	if term := systems.Find(st.SystemID).GMTerm(); term != "" {
		mc.GMTerm = term
	}
	return mc
}

// sender is who the Chat layout shows an email as from: the named person,
// as the campaign's game runner when they are its organizer, or nobody
// (the campaign) when the name is unknown.
func (mc mailCampaign) sender(name string, runsTheGame bool) *email.Sender {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	s := &email.Sender{Name: name}
	if runsTheGame && mc.GMTerm != "" {
		s.Role = "your " + mc.GMTerm
	}
	return s
}

// senderAmong is the Chat sender for userID: their name from the member
// list (or fallback when they aren't on it), as the game runner when they own
// the campaign.
func (mc mailCampaign) senderAmong(members []campaigns.CampaignMember, userID, fallback string) *email.Sender {
	for _, m := range members {
		if m.UserID == userID && userID != "" {
			name := m.DisplayName
			if strings.TrimSpace(name) == "" {
				name = fallback
			}
			return mc.sender(name, m.Role == campaigns.RoleOwner)
		}
	}
	return mc.sender(fallback, false)
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
func (h *Handler) sendMessage(ctx context.Context, mc mailCampaign, to, subject string, m email.Message) error {
	plain, html, err := email.Render(mc.Layout, m)
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

// memberZone is the member's account zone, or "" when it isn't set.
func (h *Handler) memberZone(ctx context.Context, userID string) string {
	if h.userDir != nil {
		if u, err := h.userDir.GetUser(ctx, userID); err == nil && u != nil && u.Timezone != nil && *u.Timezone != "" {
			return *u.Timezone
		}
	}
	return ""
}

// memberWhen is a game night's date and time as one member should read it:
// in their own zone when Chronicle knows it, otherwise in the organizer's
// with its zone label, so an email never shows a bare time.
func memberWhen(s *Session, zone string) string {
	if zone != "" {
		return s.FormatScheduledFor(zone)
	}
	return s.FormatScheduledWhen()
}

// ticketStub is the game night's date for the Ticket layout, on the same
// clock as memberWhen: the member's zone when the night has a time and zone
// and the member has a zone, otherwise the date and time as stored. Nil
// without a date.
func ticketStub(s *Session, zone string) *email.Stub {
	if s.ScheduledDate == nil || len(*s.ScheduledDate) < 10 {
		return nil
	}
	day, err := time.Parse("2006-01-02", (*s.ScheduledDate)[:10])
	if err != nil {
		return nil
	}
	stub := func(t time.Time, clock bool) *email.Stub {
		st := &email.Stub{Weekday: strings.ToUpper(t.Format("Mon")), Day: t.Format("2"), Month: strings.ToUpper(t.Format("Jan"))}
		if clock {
			st.Time = strings.ToLower(strings.Replace(t.Format("3:04PM"), ":00", "", 1))
		}
		return st
	}
	if s.ScheduledTime == nil || *s.ScheduledTime == "" {
		return stub(day, false)
	}
	at, err := time.Parse("2006-01-02 15:04", day.Format("2006-01-02")+" "+*s.ScheduledTime)
	if err != nil {
		return stub(day, false)
	}
	if zone != "" && s.ScheduledTZ != nil {
		from, errFrom := time.LoadLocation(*s.ScheduledTZ)
		to, errTo := time.LoadLocation(zone)
		if errFrom == nil && errTo == nil {
			if t, err := time.ParseInLocation("2006-01-02 15:04", at.Format("2006-01-02 15:04"), from); err == nil {
				return stub(t.In(to), true)
			}
		}
	}
	return stub(at, true)
}

// linksExpire is the small print under one-press answers.
const linksExpire = "These links expire in 7 days."
