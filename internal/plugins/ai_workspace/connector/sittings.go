package connector

import "time"

// SittingGap is how long a connection is quiet before its next call starts
// a new sitting.
const SittingGap = 30 * time.Minute

// Sitting is one stretch of an AI's work: calls with no gap longer than
// SittingGap between them, so a long history reads as a few lines.
type Sitting struct {
	Start, End time.Time
	// Calls are the sitting's calls, newest first.
	Calls []Call
	// Reads counts pages read, Lookups the other look-ups answered, Asks the
	// requests for changes sent, and Refused the calls not answered.
	Reads, Lookups, Asks, Refused int
}

// Sittings groups calls, given newest first, into sittings, newest first.
func Sittings(calls []Call) []Sitting {
	var out []Sitting
	for _, c := range calls {
		if n := len(out); n == 0 || out[n-1].Start.Sub(c.At) > SittingGap {
			out = append(out, Sitting{End: c.At})
		}
		s := &out[len(out)-1]
		s.Start = c.At
		s.Calls = append(s.Calls, c)
		switch {
		case c.Tool == toolConnect:
			// Connecting is a line of its own, not something the AI looked up.
		case !c.OK:
			s.Refused++
		case c.Tool == toolAsk:
			s.Asks++
		case c.Tool == "read_page":
			s.Reads++
		default:
			s.Lookups++
		}
	}
	return out
}
