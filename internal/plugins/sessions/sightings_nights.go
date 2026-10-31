package sessions

import (
	"context"
	"sort"
)

// maxSightingNights bounds the nights list a page's Last seen draws. A weekly
// series that has run for years is the case it guards; the newest are kept
// because those are the nights someone is still deciding about.
const maxSightingNights = 400

// SightingNight is one played game night: the session and the night's date.
type SightingNight struct {
	SessionID   string `json:"sessionId"`
	SessionName string `json:"sessionName"`
	Date        string `json:"date"`
	Recurring   bool   `json:"recurring"`
}

// SightingNights is the played nights, newest first. Truncated says older
// nights were left off at the bound.
type SightingNights struct {
	Nights    []SightingNight `json:"nights"`
	Truncated bool            `json:"truncated"`
}

// NightsFor reads the campaign's sessions and expands each into its played
// nights (dated on or before today). Cancelled sessions and sessions with no
// date have no night to answer for, so they are left out. It returns only
// names and dates, which every member already sees on the calendar;
// nothing here is DM-only, so it takes no viewer.
func (s *sightingService) NightsFor(ctx context.Context, campaignID, today string) (*SightingNights, error) {
	list, err := s.sessions.ListByCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := &SightingNights{Nights: []SightingNight{}}
	for _, sess := range list {
		if sess.Status == StatusCancelled || sess.ScheduledDate == nil || *sess.ScheduledDate == "" {
			continue
		}
		for _, d := range occurrenceDates(sess, "0000-01-01", today) {
			out.Nights = append(out.Nights, SightingNight{
				SessionID: sess.ID, SessionName: sess.Name, Date: d, Recurring: sess.IsRecurring,
			})
		}
	}
	sort.SliceStable(out.Nights, func(i, j int) bool {
		a, b := out.Nights[i], out.Nights[j]
		if a.Date != b.Date {
			return a.Date > b.Date
		}
		return a.SessionName < b.SessionName
	})
	if len(out.Nights) > maxSightingNights {
		out.Nights = out.Nights[:maxSightingNights]
		out.Truncated = true
	}
	return out, nil
}
