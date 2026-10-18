package sessions

import (
	"fmt"
	"math"
	"time"

	"github.com/a-h/templ"
)

// LastSeenBlockType is the page block's type in the block registry.
const LastSeenBlockType = "last_seen"

// LastSeenView is what the block's server-rendered line says for one viewer.
// It is built from SightingsFor's answer for that viewer, so it holds nothing
// the viewer could not already fetch from the sightings route.
type LastSeenView struct {
	// Seen is how many nights count, the "N nights" at the line's end.
	Seen int
	// What is the newest sighting's name; "Not seen yet" when there is none.
	What string
	// When is how long ago that was; empty when there is no dated sighting.
	When string
	// Review is how many nights are suggested to the DM; always 0 for a player.
	Review int
}

// BuildLastSeenView turns a viewer's sightings into the block's line. The
// newest sighting leads (the service sorts undated series last, so a dated
// night always outranks a series-wide link).
func BuildLastSeenView(s *Sightings, now time.Time) LastSeenView {
	v := LastSeenView{What: "Not seen yet"}
	if s == nil {
		return v
	}
	v.Seen = len(s.Seen)
	v.Review = len(s.Suggestions)
	if len(s.Seen) == 0 {
		return v
	}
	top := s.Seen[0]
	if top.Date == "" {
		v.What = "during " + top.SessionName + ", night not set"
		return v
	}
	v.What = top.SessionName
	v.When = AgoText(top.Date, now)
	return v
}

// ShowLastSeen is whether the block draws at all. A player whose page was
// never seen gets nothing, not an empty block; the DM always gets the line,
// because it is where they add a sighting.
func ShowLastSeen(v LastSeenView, dm bool) bool {
	return dm || v.Seen > 0
}

// AgoText says how long before now a night was. The rule is mirrored in
// static/js/last_seen.js (agoText) so the line reads the same after the DM
// changes it without a reload; both are pinned by tests with the same cases.
func AgoText(date string, now time.Time) string {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return ""
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	days := int(math.Round(today.Sub(d).Hours() / 24))
	switch {
	case days < 0:
		return "coming up"
	case days == 0:
		return "today"
	case days == 1:
		return "yesterday"
	case days < 14:
		return fmt.Sprintf("%d days ago", days)
	}
	weeks := int(math.Round(float64(days) / 7))
	if weeks < 9 {
		return fmt.Sprintf("%d weeks ago", weeks)
	}
	months := int(math.Round(float64(days) / 30.44))
	if months < 24 {
		return fmt.Sprintf("%d months ago", months)
	}
	return fmt.Sprintf("%d years ago", int(math.Round(float64(days)/365.25)))
}

// nightsLabel is "1 night" or "N nights".
func nightsLabel(n int) string {
	if n == 1 {
		return "1 night"
	}
	return fmt.Sprintf("%d nights", n)
}

// lastSeenCall is one of the block's handlers in static/js/last_seen.js, as an
// inline IIFE so it works after an HTMX swap of the page (a templ script helper
// or delegated listener would not). fn is always one of that file's fixed
// names, never input.
func lastSeenCall(fn string) templ.ComponentScript {
	return templ.ComponentScript{
		Name:     "sessions_lastseen_" + fn,
		Function: "",
		Call: `(function(el,e){` +
			`if(window.ChronicleLastSeen)window.ChronicleLastSeen.` + fn + `(el,e);` +
			`})(this,event)`,
	}
}

// lastSeenModeCall is lastSeenCall for the two "which nights" buttons, which
// pass the mode they select. mode is one of two fixed words.
func lastSeenModeCall(mode string) templ.ComponentScript {
	return templ.ComponentScript{
		Name:     "sessions_lastseen_mode_" + mode,
		Function: "",
		Call: `(function(el,e){` +
			`if(window.ChronicleLastSeen)window.ChronicleLastSeen.mode(el,'` + mode + `');` +
			`})(this,event)`,
	}
}
