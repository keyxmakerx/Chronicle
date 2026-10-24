package sessions

import "sort"

// SessionGroup is one heading on the Sessions list and the nights under it.
type SessionGroup struct {
	Key      string // "upcoming", "played", "completed", "cancelled"
	Label    string
	Sessions []Session
}

// isPlayed reports whether a planned one-off night's date is behind the
// viewer. Nothing moves a night to Completed on its own, so without this a
// night that already happened stays under Upcoming with an RSVP chip. A
// repeating series keeps its next night ahead, so it stays upcoming.
func isPlayed(s Session, today string) bool {
	return s.Status == StatusPlanned && !s.IsRecurring &&
		s.ScheduledDate != nil && *s.ScheduledDate != "" && *s.ScheduledDate < today
}

// groupSessions splits the list into Upcoming, Played (planned but dated
// before today, newest first, so last night is on top), Completed and
// Cancelled, dropping empty groups. today is the viewer's local
// YYYY-MM-DD, so a night stays upcoming through its own evening.
func groupSessions(list []Session, today string) []SessionGroup {
	groups := []SessionGroup{
		{Key: "upcoming", Label: "Upcoming"},
		{Key: "played", Label: "Played"},
		{Key: "completed", Label: "Completed"},
		{Key: "cancelled", Label: "Cancelled"},
	}
	for _, s := range list {
		i := 3
		switch {
		case isPlayed(s, today):
			i = 1
		case s.Status == StatusPlanned:
			i = 0
		case s.Status == StatusCompleted:
			i = 2
		}
		groups[i].Sessions = append(groups[i].Sessions, s)
	}
	played := groups[1].Sessions
	sort.SliceStable(played, func(a, b int) bool { return *played[a].ScheduledDate > *played[b].ScheduledDate })
	out := groups[:0]
	for _, g := range groups {
		if len(g.Sessions) > 0 {
			out = append(out, g)
		}
	}
	return out
}
