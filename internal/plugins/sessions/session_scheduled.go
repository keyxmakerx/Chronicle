package sessions

import (
	"context"
	"fmt"
	"log/slog"
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
