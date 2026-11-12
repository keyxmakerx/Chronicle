package sessions

import "time"

// nightsToday is the earliest calendar day still under way anywhere on
// Earth. A night's date carries no zone of its own to the people it is
// announced to, so a night counts as over only once its day has ended in
// every zone (UTC-12 is last); otherwise a change to tonight's game could be
// swallowed for players in the Americas just after midnight UTC.
func nightsToday(now time.Time) string {
	return now.UTC().Add(-12 * time.Hour).Format("2006-01-02")
}

// upcomingNight reports the night a cancel, move or back-on notice is about:
// the date itself for a one-off night, the next night on or after today for
// a repeating one. ok is false when every night is already over, which is
// when telling people would only confuse them. A night with no date is
// always announced, since there is nothing to be late for.
func upcomingNight(s *Session, today string) (date string, ok bool) {
	if s.ScheduledDate == nil || *s.ScheduledDate == "" {
		return "", true
	}
	if !s.IsRecurring || s.RecurrenceType == nil {
		return *s.ScheduledDate, *s.ScheduledDate >= today
	}
	next := nextUpcomingOccurrenceDate(s, today)
	if next < today {
		return "", false
	}
	// nextUpcomingOccurrenceDate keeps counting past the series' end date.
	if s.RecurrenceEndDate != nil && *s.RecurrenceEndDate != "" && next > *s.RecurrenceEndDate {
		return "", false
	}
	return next, true
}

// NightAudience is who hears about a change to a game night and which night
// the notice names.
type NightAudience struct {
	// UserIDs said Going or Maybe to a night that has not happened yet.
	UserIDs []string
	// Night is the session as the notice should describe it: a repeating
	// series carries its next upcoming date instead of the series' first.
	Night *Session
}
