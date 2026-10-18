package entities

import (
	"fmt"
	"strings"

	"github.com/a-h/templ"
)

// Presentation helpers for the Standing block (standing_block.templ). The
// "Why:" line rule is mirrored in static/js/standing.js (whyText) so the line
// reads the same after the Director adds or removes a reason without a reload;
// both are pinned by tests with the same cases.

// whyLeadWords is how many words of the newest reason the "Why:" line quotes.
const whyLeadWords = 4

// whyListMax is the most reasons the line spells out; past it the line counts.
const whyListMax = 8

// StandingWhyText is the quiet line under the word: the newest reason, then
// how many more there are. total is how many reasons this viewer can see;
// newest is the newest one's text.
func StandingWhyText(total int, newest string) string {
	switch {
	case total <= 0:
		return "Why: no reasons shared yet"
	case total > whyListMax:
		return fmt.Sprintf("Why: %d reasons", total)
	}
	words := strings.Fields(newest)
	lead := newest
	if len(words) > whyLeadWords {
		lead = strings.Join(words[:whyLeadWords], " ") + "…"
	}
	if total == 1 {
		return "Why: " + lead
	}
	return fmt.Sprintf("Why: %s and %d more", lead, total-1)
}

// standingFirst is the subject the block draws first.
func standingFirst(v *StandingView) StandingSubjectView {
	if v == nil || len(v.Subjects) == 0 {
		return StandingSubjectView{Key: StandingPartyWire, Name: "The party", Level: StandingLevelDefault, Word: StandingWord(StandingLevelDefault), Reasons: []StandingReasonView{}}
	}
	return v.Subjects[0]
}

func standingWhyFor(s StandingSubjectView) string {
	newest := ""
	if len(s.Reasons) > 0 {
		newest = s.Reasons[0].Text
	}
	return StandingWhyText(s.Total, newest)
}

// standingWordText is what the word slot says: the word, or that the Director
// has not set one yet.
func standingWordText(s StandingSubjectView) string {
	if !s.Set {
		return "Not set yet"
	}
	return s.Word
}

func standingStepsLabel(s StandingSubjectView) string {
	if !s.Set {
		return "Standing not set yet"
	}
	return fmt.Sprintf("Standing: %s, step %d of 5", s.Word, s.Level+1)
}

// standingThumbStyle places the step thumb; an unset standing has none.
func standingThumbStyle(s StandingSubjectView) templ.SafeCSS {
	return templ.SafeCSS(fmt.Sprintf("--i:%d;", s.Level))
}

func standingDigit(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// standingDMOnlyShown is whether the Director is looking at a standing that
// players cannot see, which gets the amber edge and the DM tag.
func standingDMOnlyShown(v *StandingView) bool {
	return v != nil && v.CanEdit && v.PlayersSee != nil && !*v.PlayersSee
}

// standingCall is one of the block's handlers in static/js/standing.js, as an
// inline IIFE so it works after an HTMX swap of the page (a templ script helper
// or delegated listener would not). fn is always one of that file's fixed
// names, never input.
func standingCall(fn string) templ.ComponentScript {
	return templ.ComponentScript{
		Name:     "entities_standing_" + fn,
		Function: "",
		Call: `(function(el,e){` +
			`if(window.ChronicleStanding)window.ChronicleStanding.` + fn + `(el,e);` +
			`})(this,event)`,
	}
}

// standingValueCall is standingCall for the buttons that pass a fixed word
// (an effect, a filter). value is one of a few literals in the template.
func standingValueCall(fn, value string) templ.ComponentScript {
	return templ.ComponentScript{
		Name:     "entities_standing_" + fn + "_" + value,
		Function: "",
		Call: `(function(el,e){` +
			`if(window.ChronicleStanding)window.ChronicleStanding.` + fn + `(el,'` + value + `');` +
			`})(this,event)`,
	}
}

// standingDigitBool is "true"/"false" for aria-checked and aria-pressed.
func standingDigitBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func standingItoa(n int) string { return fmt.Sprintf("%d", n) }
