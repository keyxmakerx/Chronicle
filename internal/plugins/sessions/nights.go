package sessions

import (
	"context"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// maxNightsRangeDays bounds one game-nights read: the calendar asks for one
// month at a time, so a wider window is a malformed or abusive request, and
// the bound also caps how many per-night roster reads one call can make.
const maxNightsRangeDays = 62

// Game-night answers as the calendar shows them. Silence is NightNoAnswer,
// never a no.
const (
	NightYes      = "yes"
	NightMaybe    = "maybe"
	NightNo       = "no"
	NightNoAnswer = ""
)

// NightMember is one campaign member a game night's roster lists, whether or
// not they have answered.
type NightMember struct {
	UserID string
	Name   string
}

// NightAnswer is one member's answer to one game night.
type NightAnswer struct {
	UserID string `json:"userId"`
	Name   string `json:"name"`
	Answer string `json:"answer"`
	Note   string `json:"note,omitempty"`
	// Excluded is the organizer's own "leave me out of the count" switch.
	Excluded bool `json:"excluded"`
	// Recheck: the night moved after this member answered.
	Recheck bool `json:"recheck"`
	// Carried: a yes to a proposed time, not yet confirmed as Going.
	Carried bool `json:"carried"`
}

// NightTally counts a night's answers. Excluded members count nowhere.
type NightTally struct {
	Going    int `json:"going"`
	Maybe    int `json:"maybe"`
	Cant     int `json:"cant"`
	NoAnswer int `json:"noAnswer"`
}

// GameNight is one night of a session, for the calendar's day card and
// event page. A repeating session yields one GameNight per night, each with
// its own answers.
type GameNight struct {
	SessionID      string `json:"sessionId"`
	Name           string `json:"name"`
	Summary        string `json:"summary,omitempty"`
	Date           string `json:"date"`
	Time           string `json:"time,omitempty"`
	TZ             string `json:"tz,omitempty"`
	Recurring      bool   `json:"recurring"`
	RecurrenceType string `json:"recurrenceType,omitempty"`
	// The series as stored, so the calendar's editor can show and change it:
	// the first night, every-N-weeks for a custom repeat, and the last date.
	SeriesDate         string        `json:"seriesDate,omitempty"`
	RecurrenceInterval int           `json:"recurrenceInterval,omitempty"`
	RecurrenceEndDate  string        `json:"recurrenceEndDate,omitempty"`
	OrganizerID        string        `json:"organizerId"`
	OrganizerName      string        `json:"organizerName,omitempty"`
	Past               bool          `json:"past"`
	Tally              NightTally    `json:"tally"`
	Roster             []NightAnswer `json:"roster"`
	// Viewer-specific: filled by ForViewer, never by the service.
	Mine       *NightAnswer `json:"mine"`
	CanExclude bool         `json:"canExclude"`
}

// ForViewer fills the fields that depend on who is looking: their own
// answer, and whether they may switch themselves out of the count (the
// organizer or the campaign owner, the rule SetExcludedFromCount enforces).
func (n *GameNight) ForViewer(userID string, isCampaignOwner bool) {
	n.Mine = nil
	for i := range n.Roster {
		if n.Roster[i].UserID == userID {
			a := n.Roster[i]
			n.Mine = &a
			break
		}
	}
	n.CanExclude = n.Mine != nil && (n.OrganizerID == userID || isCampaignOwner)
}

// nightAnswerFor maps a stored RSVP status onto the calendar's answer words.
// "invited" and "carried_yes" are not answers the member gave, so both read
// as no answer yet.
func nightAnswerFor(status string) (answer string, carried bool) {
	switch status {
	case RSVPAccepted:
		return NightYes, false
	case RSVPTentative:
		return NightMaybe, false
	case RSVPDeclined:
		return NightNo, false
	case RSVPCarriedYes:
		return NightNoAnswer, true
	default:
		return NightNoAnswer, false
	}
}

// nightRow is one stored answer, from session_attendees or
// session_occurrence_rsvps, in the shape buildNightRoster needs.
type nightRow struct {
	UserID   string
	Status   string
	Note     *string
	Excluded bool
	Recheck  bool
}

// buildNightRoster lists every current member once, with their answer if
// they gave one. Rows for people no longer in the campaign are dropped, so a
// departed member's answer and note never reach the table.
func buildNightRoster(members []NightMember, rows []nightRow) ([]NightAnswer, NightTally) {
	byUser := make(map[string]nightRow, len(rows))
	for _, r := range rows {
		byUser[r.UserID] = r
	}
	roster := make([]NightAnswer, 0, len(members))
	var t NightTally
	for _, m := range members {
		a := NightAnswer{UserID: m.UserID, Name: m.Name}
		if r, ok := byUser[m.UserID]; ok {
			a.Answer, a.Carried = nightAnswerFor(r.Status)
			if r.Note != nil {
				a.Note = *r.Note
			}
			a.Excluded = r.Excluded
			a.Recheck = r.Recheck && a.Answer != NightNoAnswer
		}
		roster = append(roster, a)
		if a.Excluded {
			continue
		}
		switch a.Answer {
		case NightYes:
			t.Going++
		case NightMaybe:
			t.Maybe++
		case NightNo:
			t.Cant++
		default:
			t.NoAnswer++
		}
	}
	return roster, t
}

// occurrenceDates returns the session's nights inside [from, to] (inclusive,
// YYYY-MM-DD). A repeating session steps with computeNextOccurrence from its
// scheduled date, stopping at its end date; the walk is capped like
// nextUpcomingOccurrenceDate's.
func occurrenceDates(s Session, from, to string) []string {
	if s.ScheduledDate == nil || *s.ScheduledDate == "" {
		return nil
	}
	cur := *s.ScheduledDate
	if !s.IsRecurring || s.RecurrenceType == nil {
		if cur >= from && cur <= to {
			return []string{cur}
		}
		return nil
	}
	end := to
	if s.RecurrenceEndDate != nil && *s.RecurrenceEndDate != "" && *s.RecurrenceEndDate < end {
		end = *s.RecurrenceEndDate
	}
	probe := &Session{ScheduledDate: &cur, RecurrenceType: s.RecurrenceType, RecurrenceInterval: s.RecurrenceInterval}
	var out []string
	for i := 0; i < 520 && cur <= end; i++ {
		if cur >= from {
			out = append(out, cur)
		}
		next := computeNextOccurrence(probe)
		if next == "" || next <= cur {
			break
		}
		cur = next
		probe.ScheduledDate = &cur
	}
	return out
}

// validateNightsRange checks from/to are dates, in order, and no wider than
// maxNightsRangeDays.
func validateNightsRange(from, to string) error {
	f, err := time.Parse("2006-01-02", from)
	if err != nil {
		return apperror.NewValidation("from must be a YYYY-MM-DD date")
	}
	t, err := time.Parse("2006-01-02", to)
	if err != nil {
		return apperror.NewValidation("to must be a YYYY-MM-DD date")
	}
	if t.Before(f) {
		return apperror.NewValidation("to must not be before from")
	}
	if t.Sub(f) > maxNightsRangeDays*24*time.Hour {
		return apperror.NewValidation(fmt.Sprintf("the range may be at most %d days", maxNightsRangeDays))
	}
	return nil
}

// ListGameNights returns every planned game night in [from, to], each with
// the full roster of the given members. today (YYYY-MM-DD) marks which
// nights are past. Viewer-specific fields are left for GameNight.ForViewer.
func (s *sessionService) ListGameNights(ctx context.Context, campaignID, from, to, today string, members []NightMember) ([]GameNight, error) {
	if err := validateNightsRange(from, to); err != nil {
		return nil, err
	}
	sessions, err := s.repo.ListByDateRange(ctx, campaignID, from, to)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing game nights: %w", err))
	}
	out := []GameNight{}
	for _, sess := range sessions {
		dates := occurrenceDates(sess, from, to)
		if len(dates) == 0 {
			continue
		}
		// A one-off session's answers live in session_attendees; a series'
		// live per night (see UpdateRSVPDetailed).
		var oneOff []nightRow
		if !sess.IsRecurring {
			atts, err := s.repo.ListAttendees(ctx, sess.ID)
			if err != nil {
				return nil, apperror.NewInternal(fmt.Errorf("listing attendees: %w", err))
			}
			for _, a := range atts {
				oneOff = append(oneOff, nightRow{UserID: a.UserID, Status: a.Status, Note: a.Note, Excluded: a.ExcludedFromCount, Recheck: a.NeedsRecheck})
			}
		}
		for _, d := range dates {
			rows := oneOff
			if sess.IsRecurring {
				occ, err := s.repo.ListOccurrenceRSVPs(ctx, sess.ID, d)
				if err != nil {
					return nil, apperror.NewInternal(fmt.Errorf("listing occurrence rsvps: %w", err))
				}
				rows = make([]nightRow, 0, len(occ))
				for _, o := range occ {
					rows = append(rows, nightRow{UserID: o.UserID, Status: o.Status, Note: o.Note, Excluded: o.ExcludedFromCount, Recheck: o.NeedsRecheck})
				}
			}
			roster, tally := buildNightRoster(members, rows)
			out = append(out, GameNight{
				SessionID:          sess.ID,
				Name:               sess.Name,
				Summary:            strPtrVal(sess.Summary),
				Date:               d,
				Time:               strPtrVal(sess.ScheduledTime),
				TZ:                 strPtrVal(sess.ScheduledTZ),
				Recurring:          sess.IsRecurring,
				RecurrenceType:     strPtrVal(sess.RecurrenceType),
				SeriesDate:         strPtrVal(sess.ScheduledDate),
				RecurrenceInterval: sess.RecurrenceInterval,
				RecurrenceEndDate:  strPtrVal(sess.RecurrenceEndDate),
				OrganizerID:        sess.CreatedBy,
				OrganizerName:      sess.CreatorName,
				Past:               d < today,
				Tally:              tally,
				Roster:             roster,
			})
		}
	}
	return out, nil
}

// nextNightWindowDays bounds how far ahead NextGameNight looks. A night
// further out than a season is not a "next game night" worth counting down
// to, and the bound caps the recurrence walk for a pathological series.
const nextNightWindowDays = 120

// NextNight is the soonest game night still ahead, with no roster: the header
// widget needs only a name and a moment, and building a roster costs a read
// per member per night on every page load.
type NextNight struct {
	SessionID string
	Name      string
	// Date is YYYY-MM-DD and Time is "HH:MM" ("" when the night has no start
	// time), both on the wall clock of TZ.
	Date string
	Time string
	// TZ is the IANA zone the night was scheduled in; "" means UTC.
	TZ string
	// At is the night's start instant (end of day when no time is set), so a
	// caller can say "in 3 days" without redoing the zone arithmetic.
	At time.Time
}

// nightInstant turns a night's wall-clock date and optional time into an
// instant in its own zone. A night with no time counts as lasting all day, so
// it stays "next" until the day is over; an unknown zone falls back to UTC
// rather than dropping a night the table still plans to play.
func nightInstant(date, hhmm, tz string) (time.Time, bool) {
	loc := time.UTC
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	day, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return time.Time{}, false
	}
	if hhmm == "" {
		return day.Add(24*time.Hour - time.Second), true
	}
	t, err := time.ParseInLocation("2006-01-02 15:04", date+" "+hhmm, loc)
	if err != nil {
		return day.Add(24*time.Hour - time.Second), true
	}
	return t, true
}

// NextGameNight returns the soonest planned night that has not started, or
// nil when nothing is scheduled. now is passed in so the choice is testable
// and one request sees one clock.
func (s *sessionService) NextGameNight(ctx context.Context, campaignID string, now time.Time) (*NextNight, error) {
	// A night in a zone far west of UTC can still be ahead when UTC's date has
	// rolled over, so the window opens a day early; nightInstant does the
	// exact cut.
	from := now.UTC().AddDate(0, 0, -1).Format("2006-01-02")
	to := now.UTC().AddDate(0, 0, nextNightWindowDays).Format("2006-01-02")
	sessions, err := s.repo.ListByDateRange(ctx, campaignID, from, to)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("finding the next game night: %w", err))
	}
	var best *NextNight
	for _, sess := range sessions {
		for _, d := range occurrenceDates(sess, from, to) {
			at, ok := nightInstant(d, strPtrVal(sess.ScheduledTime), strPtrVal(sess.ScheduledTZ))
			if !ok || at.Before(now) {
				continue
			}
			if best == nil || at.Before(best.At) {
				best = &NextNight{
					SessionID: sess.ID, Name: sess.Name, Date: d,
					Time: strPtrVal(sess.ScheduledTime), TZ: strPtrVal(sess.ScheduledTZ), At: at,
				}
			}
		}
	}
	return best, nil
}
