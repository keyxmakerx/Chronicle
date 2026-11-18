package sessions

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// NotifSessionScheduled tells members a new game night is on the calendar.
// It asks for an answer, so it also raises the call-to-action banner
// (calloutNotifTypes) until the member answers that night, when it is marked
// read. Without it, a campaign with no email set up told players nothing.
const NotifSessionScheduled = "session_scheduled"

// sessionLink is the in-app URL of one session's page.
func sessionLink(campaignID, sessionID string) string {
	return fmt.Sprintf("/campaigns/%s/sessions/%s", campaignID, sessionID)
}

// sessionIDFromLink reads the session id back out of a sessionLink, or ""
// when the link is not one.
func sessionIDFromLink(link *string) string {
	if link == nil {
		return ""
	}
	parts := strings.Split(strings.Trim(strings.TrimSpace(*link), "/"), "/")
	if len(parts) != 4 || parts[0] != "campaigns" || parts[2] != "sessions" {
		return ""
	}
	return parts[3]
}

// NotifySessionScheduled tells every member but the scheduler that a night
// was scheduled, with its date and time on the second line.
func (s *sessionService) NotifySessionScheduled(ctx context.Context, session *Session, schedulerID, schedulerName string, memberIDs []string) error {
	recipients := make([]string, 0, len(memberIDs))
	for _, id := range memberIDs {
		if id != schedulerID {
			recipients = append(recipients, id)
		}
	}
	msg := fmt.Sprintf("%s scheduled %s. Are you coming?", askerLabel(schedulerName), session.Name)
	return s.NotifyUsersWithDetail(ctx, recipients, session.CampaignID, NotifSessionScheduled,
		msg, session.FormatScheduledWhen(), sessionLink(session.CampaignID, session.ID))
}

// clearScheduledAsk marks the member's "are you coming?" for this session
// read once they have answered it, so the banner stops asking. Best effort:
// the answer is already saved, and a stale banner is not worth failing it.
func (s *sessionService) clearScheduledAsk(ctx context.Context, userID, campaignID, sessionID string) {
	if err := s.repo.MarkNotificationsReadForLink(ctx, userID, NotifSessionScheduled, sessionLink(campaignID, sessionID)); err != nil {
		slog.Warn("clearing a scheduled-night notification", slog.String("session_id", sessionID), slog.Any("error", err))
	}
}

// OpenAsks keeps, of a member's notifications, the "are you coming?" asks
// that can still be answered, and passes everything else through. An ask is
// cleared only by the member's own answer, so the one for a night that has
// been played, marked complete, cancelled or removed stays unread; the banner
// must not send anyone to answer a night that is over. The bell keeps the row
// either way, since it is a record of what was said.
func (s *sessionService) OpenAsks(ctx context.Context, notes []Notification) []Notification {
	today := s.today()
	out := make([]Notification, 0, len(notes))
	open := map[string]bool{}
	for _, n := range notes {
		id := sessionIDFromLink(n.Link)
		if n.Type != NotifSessionScheduled || n.ReadAt != nil || id == "" {
			out = append(out, n)
			continue
		}
		takesAnswers, seen := open[id]
		if !seen {
			// A night that cannot be read is not asked about: saying nothing
			// is the banner's answer to anything it cannot confirm.
			sess, err := s.repo.FindByID(ctx, id)
			takesAnswers = err == nil && sess != nil && sess.AcceptsRSVP() && !isPlayed(*sess, today)
			open[id] = takesAnswers
		}
		if takesAnswers {
			out = append(out, n)
		}
	}
	return out
}
