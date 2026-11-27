package sessions

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
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

// takesAnswers reports whether a night can still be answered on the day
// given: it has not been completed or cancelled, and its date is not behind
// that day. A night that is over is a record of who came, not a question.
func takesAnswers(s *Session, today string) bool {
	return s != nil && s.AcceptsRSVP() && !isPlayed(*s, today)
}

// OpenAsks keeps, of a member's notifications, the "are you coming?" asks
// that can still be answered, and passes everything else through. An ask
// stays unread until the member answers or opens it, so the one for a night
// that has been played, marked complete, cancelled or removed would ask for
// ever; the banner and the bell's count must not send anyone to answer a
// night that is over. The bell keeps the row either way, since it is a
// record of what was said.
//
// today gives the reader's own date for a campaign's nights, which is the
// day the night's page marks it Played by; nil means the day where the date
// ends last, so no night is closed early.
//
// A date poll's ask ("new scheduling proposal") is judged the same way: it
// stays only while the poll is open and the member has answered none of its
// dates. Answering from the calendar or an email, or the Director picking a
// date, never touches that notification row, so the row alone would keep the
// banner asking after the question was settled.
func (s *sessionService) OpenAsks(ctx context.Context, notes []Notification, today func(campaignID string) string) []Notification {
	out := make([]Notification, 0, len(notes))
	open := map[string]bool{}
	polls := map[string]bool{}
	for _, n := range notes {
		if pid := proposalIDFromLink(n.Link); n.Type == NotifProposalCreated && n.ReadAt == nil && pid != "" {
			key := pid + "|" + n.UserID
			waiting, seen := polls[key]
			if !seen {
				waiting = s.pollAwaits(ctx, pid, n.UserID)
				polls[key] = waiting
			}
			if waiting {
				out = append(out, n)
			}
			continue
		}
		id := sessionIDFromLink(n.Link)
		if n.Type != NotifSessionScheduled || n.ReadAt != nil || id == "" {
			out = append(out, n)
			continue
		}
		takes, seen := open[id]
		if !seen {
			// A night that cannot be read is not asked about: saying nothing
			// is the banner's answer to anything it cannot confirm.
			sess, err := s.repo.FindByID(ctx, id)
			if err == nil && sess != nil {
				day := s.today()
				if today != nil {
					day = today(sess.CampaignID)
				}
				takes = takesAnswers(sess, day)
			}
			open[id] = takes
		}
		if takes {
			out = append(out, n)
		}
	}
	return out
}

// pollAwaits reports whether a date poll is still waiting on this member: it
// is open and they have answered none of its dates. A poll that cannot be
// read is not asked about, as with a night.
func (s *sessionService) pollAwaits(ctx context.Context, proposalID, userID string) bool {
	p, err := s.repo.FindProposalByID(ctx, proposalID)
	if err != nil || p == nil || p.Status != ProposalOpen {
		return false
	}
	responses, err := s.repo.ListProposalResponses(ctx, proposalID)
	if err != nil {
		return false
	}
	for _, r := range responses {
		if r.UserID == userID {
			return false
		}
	}
	return true
}

// proposalIDFromLink reads the proposal id back out of a proposalLink, or ""
// when the link is not one.
func proposalIDFromLink(link *string) string {
	if link == nil {
		return ""
	}
	parts := strings.Split(strings.Trim(strings.TrimSpace(*link), "/"), "/")
	if len(parts) != 4 || parts[0] != "campaigns" || parts[2] != "proposals" {
		return ""
	}
	return parts[3]
}

// unreadAskScan bounds how many of a member's unread asks the bell reads to
// leave the finished ones out of its count.
const unreadAskScan = 100

// ListMyUnreadAsks returns the member's unread "are you coming?" asks,
// newest first.
func (s *sessionService) ListMyUnreadAsks(ctx context.Context, userID string) ([]Notification, error) {
	asks, err := s.repo.ListUnreadNotificationsOfType(ctx, userID, NotifSessionScheduled, unreadAskScan)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing unread asks: %w", err))
	}
	return asks, nil
}
