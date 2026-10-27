package designlab

import "regexp"

// idPattern is the shape of a campaign or calendar id the page will pass on.
var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// BenchSky names the calendar whose sky the crafting bench's window shows.
// Empty means the bench's own demo calendar.
type BenchSky struct {
	CampaignID string
	CalendarID string
}

// NewBenchSky keeps the pair only when both are plain ids, since they go into
// the page's attributes and from there into API paths. The calendar API still
// decides whether the viewer may read that calendar.
func NewBenchSky(campaignID, calendarID string) BenchSky {
	if !idPattern.MatchString(campaignID) || !idPattern.MatchString(calendarID) {
		return BenchSky{}
	}
	return BenchSky{CampaignID: campaignID, CalendarID: calendarID}
}
