package designs

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Reason codes are stored with a gallery entry and sent to the client; they
// name the kind of problem, never the matched word, so a flagged publisher is
// not handed a way to probe the list.
const (
	ReasonMaturity    = "maturity"
	ReasonLink        = "link"
	ReasonEmail       = "email"
	ReasonHandle      = "handle"
	ReasonCaps        = "caps"
	ReasonRepeat      = "repeat"
	ReasonNameRepeat  = "name_repeat"
	ReasonRate        = "rate"
	ReasonDuplicate   = "duplicate"
	ReasonReported    = "reported"
	ReasonResubmitted = "resubmitted"
	ReasonRemoved     = "removed"
)

// reasonWords is what the publisher reads for each code.
var reasonWords = map[string]string{
	ReasonMaturity:    "The name or note may include language that is not suitable for everyone.",
	ReasonLink:        "The text contains a web address or invite link.",
	ReasonEmail:       "The text contains an email address.",
	ReasonHandle:      "The text contains an @name.",
	ReasonCaps:        "The text is mostly capital letters.",
	ReasonRepeat:      "The text repeats the same character many times.",
	ReasonNameRepeat:  "The note repeats the name.",
	ReasonRate:        "Several designs were shared in a short time.",
	ReasonDuplicate:   "You already shared a design with exactly these choices.",
	ReasonReported:    "Other owners reported this design.",
	ReasonResubmitted: "A design that was not approved was sent in again.",
	ReasonRemoved:     "An admin took this design down.",
}

// ReasonText turns a stored code into plain words; an unknown code reads as a
// generic review note rather than leaking the code.
func ReasonText(code string) string {
	if w, ok := reasonWords[code]; ok {
		return w
	}
	return "This design needs a look from an admin."
}

// Spam limits.
const (
	capsMinLetters   = 8
	capsShare        = 0.70
	repeatCharRun    = 6
	nameRepeatCount  = 3
	nameRepeatMinLen = 3
	// MaxGalleryPerDay is how many publishes or updates one user may make in
	// 24 hours before the next one waits for review.
	MaxGalleryPerDay = 5
)

var (
	reLink = regexp.MustCompile(`(?i)\b(?:https?|ftp)://|\bwww\.|discord\.gg|discord(?:app)?\.com/invite`)
	// Domain-like text: a label, a dot and a common top-level domain. The list
	// is short on purpose; "e.g." and "St. John" are not domains.
	reDomain = regexp.MustCompile(`(?i)\b[a-z0-9][a-z0-9-]*\.(?:com|net|org|gg|io|co|me|xyz|ly|tv|app|dev|info|biz|ru|cn|tk|link|site|online|shop|store|club|top|live|fun|to|cc|us|uk|de|gl|ws)\b`)
	reEmail  = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`)
	reHandle = regexp.MustCompile(`(?:^|[\s(])@[A-Za-z0-9_.]{2,}`)
)

// capsHeavy is true for 8+ letters of which more than 70% are capitals.
func capsHeavy(s string) bool {
	letters, upper := 0, 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	return letters >= capsMinLetters && float64(upper)/float64(letters) > capsShare
}

// longRun is true for a run of six or more of one visible character.
func longRun(s string) bool {
	var prev rune
	n := 0
	for _, r := range s {
		if unicode.IsSpace(r) {
			prev, n = 0, 0
			continue
		}
		if r == prev {
			n++
		} else {
			prev, n = r, 1
		}
		if n >= repeatCharRun {
			return true
		}
	}
	return false
}

// nameRepeated is true when the note says the name three or more times, which
// covers a note that is only the name pasted over and over.
func nameRepeated(name, note string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if utf8.RuneCountInString(name) < nameRepeatMinLen {
		return false
	}
	return strings.Count(strings.ToLower(note), name) >= nameRepeatCount
}

// textReasons runs the free-text checks over the name, the note and the
// shown-as name, in a fixed order so the stored reasons are stable. The design
// data is not read here: the kind has already validated it as known parts.
func textReasons(l *maturityList, name, note, maker string) []string {
	fields := []string{name, note, maker}
	var out []string
	add := func(code string, test func(string) bool) {
		for _, f := range fields {
			if test(f) {
				out = append(out, code)
				return
			}
		}
	}
	add(ReasonMaturity, l.flags)
	add(ReasonLink, func(s string) bool { return reLink.MatchString(s) || reDomain.MatchString(s) })
	add(ReasonEmail, reEmail.MatchString)
	add(ReasonHandle, reHandle.MatchString)
	add(ReasonCaps, capsHeavy)
	add(ReasonRepeat, longRun)
	if nameRepeated(name, note) {
		out = append(out, ReasonNameRepeat)
	}
	return out
}
