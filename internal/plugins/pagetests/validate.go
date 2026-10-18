package pagetests

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limits. They bound the stored document and so a request body; the widget
// keeps to the same numbers as a courtesy.
const (
	MaxNameChars  = 120
	MaxKindChars  = 40
	MaxLabelChars = 60
	MaxResults    = 8
	// MaxTotal is the largest Needed or Limit, and the furthest either side
	// of the tally can count.
	MaxTotal   = 30
	MaxIDChars = 64
	// MaxPerPage keeps one page's block a list a DM can scan.
	MaxPerPage = 20
	// MaxChoices bounds the campaign-wide list the encounter builder reads.
	MaxChoices = 200
	// MaxBodyBytes is well above the largest legal test and low enough that
	// a junk body is refused.
	MaxBodyBytes = 16 << 10
)

// validToken reports whether s is a short safe id: letters, digits, _ and -.
// Page ids (UUIDs) and the result ids the widget makes both fit; anything
// else is refused so an id can never carry markup or path characters.
func validToken(s string) bool {
	if s == "" || len(s) > MaxIDChars {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

// cleanLine checks one line of plain text, trimmed, so stored text is always
// safe to show with textContent and to log.
func cleanLine(field, s string, max int) (string, error) {
	s = strings.TrimSpace(s)
	if !utf8.ValidString(s) {
		return "", errInvalid(field + " is not valid text")
	}
	if utf8.RuneCountInString(s) > max {
		return "", errInvalid(fmt.Sprintf("%s is too long (at most %d characters)", field, max))
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", errInvalid(field + " contains a character that is not allowed")
		}
	}
	return s, nil
}

// validateSetup checks a save body and returns its cleaned name and the
// setup half of the document. A total of 0 means the test has no such
// total; a result can only be offered by a total the test has, and each
// total offers at most one result, so a filled total is never ambiguous.
func validateSetup(in SaveInput) (name string, d doc, err error) {
	if name, err = cleanLine("name", in.Name, MaxNameChars); err != nil {
		return "", doc{}, err
	}
	if name == "" {
		return "", doc{}, errInvalid("give the test a name")
	}
	if d.Kind, err = cleanLine("kind", in.Kind, MaxKindChars); err != nil {
		return "", doc{}, err
	}
	if in.Needed < 0 || in.Needed > MaxTotal || in.Limit < 0 || in.Limit > MaxTotal {
		return "", doc{}, errInvalid(fmt.Sprintf("the successes and failures that end a test are 0 to %d", MaxTotal))
	}
	d.Needed, d.Limit = in.Needed, in.Limit
	if len(in.Results) == 0 {
		return "", doc{}, errInvalid("a test needs at least one result")
	}
	if len(in.Results) > MaxResults {
		return "", doc{}, errInvalid(fmt.Sprintf("a test has at most %d results", MaxResults))
	}
	ids := map[string]bool{}
	ons := map[string]bool{}
	d.Results = make([]Result, 0, len(in.Results))
	for _, r := range in.Results {
		if !validToken(r.ID) || ids[r.ID] {
			return "", doc{}, errInvalid("a result has a bad id")
		}
		ids[r.ID] = true
		if r.Label, err = cleanLine("result", r.Label, MaxLabelChars); err != nil {
			return "", doc{}, err
		}
		if r.Label == "" {
			return "", doc{}, errInvalid("every result needs a name")
		}
		switch r.On {
		case OnNone:
		case OnSuccesses:
			if d.Needed == 0 {
				return "", doc{}, errInvalid(r.Label + ": set how many successes are needed first")
			}
		case OnFailures:
			if d.Limit == 0 {
				return "", doc{}, errInvalid(r.Label + ": set how many failures end the test first")
			}
		default:
			return "", doc{}, errInvalid(r.Label + ": a result is offered when the successes or the failures fill, or never")
		}
		if r.On != OnNone {
			if ons[r.On] {
				return "", doc{}, errInvalid("only one result can be offered when the " + r.On + " fill")
			}
			ons[r.On] = true
		}
		d.Results = append(d.Results, r)
	}
	return name, d, nil
}

// offer is the result whose total has filled, for the DM to confirm. The
// successes are checked first: a party that fills both on the same roll
// has met its goal, and the DM can still pick another result.
func offer(d doc) string {
	if d.Recorded != nil {
		return ""
	}
	full := ""
	switch {
	case d.Needed > 0 && d.Successes >= d.Needed:
		full = OnSuccesses
	case d.Limit > 0 && d.Failures >= d.Limit:
		full = OnFailures
	default:
		return ""
	}
	for _, r := range d.Results {
		if r.On == full {
			return r.ID
		}
	}
	return ""
}

// step moves one side of the tally. A side never goes below zero, and never
// past its total when the test has one (or MaxTotal when it has none), so
// the tally always fits the setup.
func step(d *doc, side string, by int) error {
	if by != 1 && by != -1 {
		return errInvalid("the tally moves one step at a time")
	}
	var n *int
	limit := MaxTotal
	switch side {
	case SideSuccess:
		n = &d.Successes
		if d.Needed > 0 {
			limit = d.Needed
		}
	case SideFailure:
		n = &d.Failures
		if d.Limit > 0 {
			limit = d.Limit
		}
	default:
		return errInvalid("the tally counts successes or failures")
	}
	*n = max(0, min(limit, *n+by))
	return nil
}

// keepTally carries the tally and record across a setup change, trimmed to
// the new totals. A record whose result was removed is cleared, since it
// would name a result the test no longer has.
func keepTally(next *doc, prev doc) {
	next.Successes, next.Failures = prev.Successes, prev.Failures
	if next.Needed > 0 {
		next.Successes = min(next.Successes, next.Needed)
	}
	if next.Limit > 0 {
		next.Failures = min(next.Failures, next.Limit)
	}
	next.Successes, next.Failures = min(next.Successes, MaxTotal), min(next.Failures, MaxTotal)
	if prev.Recorded == nil {
		return
	}
	for _, r := range next.Results {
		if r.ID == prev.Recorded.Result {
			rec := *prev.Recorded
			rec.Label = r.Label
			next.Recorded = &rec
			return
		}
	}
}
