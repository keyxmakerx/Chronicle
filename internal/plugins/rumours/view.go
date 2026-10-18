package rumours

import (
	"strconv"

	"github.com/a-h/templ"
)

// Presentation helpers for the Rumours block (rumours_block.templ). The
// wording of "how sure" and the number of rumours shown before the rest fold
// away are mirrored in static/js/rumours.js; tests pin both ends.

// inlineShown is how many of the newest rumours the block shows before the
// rest fold behind "N earlier rumours".
const inlineShown = 2

// sureWords is how sure the person who said it was, by step.
var sureWords = [...]string{"Heard it", "Fairly sure", "Swears by it"}

// SureWord is the words for a sureness; anything out of range reads as the
// middle step rather than failing a page render.
func SureWord(n int) string {
	if n < SureMin || n > SureMax {
		return sureWords[SureDefault-1]
	}
	return sureWords[n-1]
}

// belief wording for the party's mark; an unmarked rumour says so.
func beliefWord(b string) string {
	if b == BeliefNone {
		return "unmarked"
	}
	return b
}

func itoa(n int) string { return strconv.Itoa(n) }

func digit(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// firstShown is the rumours the server draws at rest, before the script takes
// over: the newest few, as the block shows them.
func firstShown(v *View) []RumourView {
	if len(v.Rumours) <= inlineShown {
		return v.Rumours
	}
	return v.Rumours[:inlineShown]
}

// earlier is how many rumours sit behind the fold.
func earlier(v *View) int {
	if len(v.Rumours) <= inlineShown {
		return 0
	}
	return len(v.Rumours) - inlineShown
}

func earlierLabel(n int) string {
	if n == 1 {
		return "1 earlier rumour"
	}
	return itoa(n) + " earlier rumours"
}

// cutNotice says plainly that a bound cut the list off.
func cutNotice(v *View) string {
	return "Showing the newest " + itoa(len(v.Rumours)) + " of " + itoa(v.Total) + " rumours on this page."
}

// pairState is how the Director's truth compares with the party's belief: the
// joined mark's colour and its "differs" sign follow it.
func pairState(truth *string, belief string) string {
	t := TruthOpen
	if truth != nil {
		t = *truth
	}
	switch {
	case belief == BeliefNone:
		return "none"
	case t != TruthOpen && belief != t:
		return "differ"
	case belief == t:
		return "agree"
	}
	return "marked"
}

func truthWord(truth *string) string {
	if truth == nil || *truth == TruthOpen {
		return "Open"
	}
	if *truth == TruthTrue {
		return "True"
	}
	return "False"
}

func truthValue(truth *string) string {
	if truth == nil {
		return TruthOpen
	}
	return *truth
}

// pairText is the pending text of the joined mark's party half.
func pairText(truth *string, belief string) string {
	switch pairState(truth, belief) {
	case "none":
		return "party: –"
	case "agree":
		return "agrees"
	}
	return "party: " + belief
}

// pairTip is the hover line of the joined mark, for the Director.
func pairTip(r RumourView) string {
	t := truthValue(r.Truth)
	s := "You: " + t + ". "
	if r.Belief == BeliefNone {
		return s + "Party: not marked yet."
	}
	return s + "Party (marked by " + r.BeliefBy + "): " + r.Belief + "."
}

// call is one of the block's handlers in static/js/rumours.js, as an inline
// IIFE so it works after an HTMX swap of the page (a templ script helper or a
// delegated listener would not). fn is always one of that file's fixed names,
// never input.
func call(fn string) templ.ComponentScript {
	return templ.ComponentScript{
		Name:     "rumours_" + fn,
		Function: "",
		Call: `(function(el,e){` +
			`if(window.ChronicleRumours)window.ChronicleRumours.` + fn + `(el,e);` +
			`})(this,event)`,
	}
}

// valueCall is call for the buttons that pass a fixed word (a sureness). value
// is one of a few literals in the template.
func valueCall(fn, value string) templ.ComponentScript {
	return templ.ComponentScript{
		Name:     "rumours_" + fn + "_" + value,
		Function: "",
		Call: `(function(el,e){` +
			`if(window.ChronicleRumours)window.ChronicleRumours.` + fn + `(el,'` + value + `');` +
			`})(this,event)`,
	}
}

func digitBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// dotClass fills the first n of the three sureness dots.
func dotClass(i, sure int) string {
	if i < sure {
		return "on"
	}
	return ""
}

// addNote is the plain line at the top of the Add form: whose name the rumour
// is written under and what stays the Director's.
func addNote(v *View) string {
	if v.IsDM {
		return "You are the author. Only you see which rumours are true."
	}
	return "Written under your name. The DM keeps what is true; you can mark what the party thinks."
}

// marksHidden is true for a viewer who can neither mark nor see a mark: a
// player's row has no marks line at all until the party marks the rumour, so
// no blank gap stands where the Director's truth would be.
func marksHidden(v *View, r RumourView) bool { return !v.CanMark && r.Belief == BeliefNone }

func beliefTitle(r RumourView) string {
	if r.Belief == BeliefNone {
		return ""
	}
	return "Marked by " + r.BeliefBy
}

func truthLabel(truth *string) string {
	return "What is true: " + truthValue(truth) + ". Press to change. Only you see this."
}

func partyLabel(r RumourView) string {
	if r.Belief == BeliefNone {
		return "Party has not marked this. Press to set for the party."
	}
	return "Party thinks " + r.Belief + ", marked by " + r.BeliefBy + ". Press to set for the party."
}
