// Package calendar - moon_marks.go is a moon the Director sets by hand
// (Umbra): it has no cycle, and holds the state of its latest mark until
// the next one. This file holds the states, the marks and the pure rules:
// which mark a day falls under, the words for each state, and what a player
// may see. Storage is moon_mark_repository.go, the rules moon_marks_service.go.
package calendar

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// How a moon moves.
const (
	MoonCycle  = "cycle"
	MoonManual = "manual"
)

// A manual moon's states, brightest first.
const (
	MoonFull     = "full"
	MoonHalf     = "half"
	MoonCrescent = "crescent"
	MoonDark     = "dark"
	MoonVanished = "vanished"
)

// MoonStates lists the states in the order the editor offers them.
var MoonStates = []string{MoonFull, MoonHalf, MoonCrescent, MoonDark, MoonVanished}

// moonStateWords are the defaults a Director's own words replace.
var moonStateWords = map[string]string{
	MoonFull: "Full", MoonHalf: "Half", MoonCrescent: "Crescent", MoonDark: "Dark", MoonVanished: "Vanished",
}

// moonStatePhase is the phase each state draws as, on the same 0..1 scale as
// a cycling moon (0.5 full). A vanished moon draws dark; StateOn tells it
// apart.
var moonStatePhase = map[string]float64{
	MoonFull: 0.5, MoonHalf: 0.25, MoonCrescent: 0.125, MoonDark: 0, MoonVanished: 0,
}

// Limits keep a manual moon small.
const (
	// MaxMoonStateWord is the longest word a Director may give a state.
	MaxMoonStateWord = 40
	// maxMoonMarks bounds one moon's marks; years of play at a mark a week
	// stay well under it.
	maxMoonMarks = 2000
)

// MoonMark is one day the Director set a manual moon.
type MoonMark struct {
	Year  int    `json:"year"`
	Month int    `json:"month"`
	Day   int    `json:"day"`
	State string `json:"state"`
	// Shown says players see this change. A player's copy of a moon keeps
	// only its shown marks.
	Shown bool `json:"shown"`
	abs   int
}

func (k MoonMark) date() DayDate { return DayDate{Year: k.Year, Month: k.Month, Day: k.Day} }

// ValidMoonState reports whether s is one of the states.
func ValidMoonState(s string) bool {
	_, ok := moonStateWords[s]
	return ok
}

// Manual reports whether the moon is set by hand.
func (m *Moon) Manual() bool { return m.PhaseMode == MoonManual }

// markOn is the mark day abs falls under: the latest at or before it, nil
// before the first or when the marks were never indexed.
func (m *Moon) markOn(abs int) *MoonMark {
	if !m.marksIndexed || len(m.Marks) == 0 {
		return nil
	}
	i := sort.Search(len(m.Marks), func(i int) bool { return m.Marks[i].abs > abs })
	if i == 0 {
		return nil
	}
	return &m.Marks[i-1]
}

// StateOn is a manual moon's state on the day abs, "" before its first mark
// (the viewer knows nothing of it yet).
func (m *Moon) StateOn(abs int) string {
	if mk := m.markOn(abs); mk != nil {
		return mk.State
	}
	return ""
}

// StateWord is the Director's word for state, or its default.
func (m *Moon) StateWord(state string) string {
	if w := strings.TrimSpace(m.StateNames[state]); w != "" {
		return w
	}
	return moonStateWords[state]
}

// StateWordOn is the word for a manual moon's state on the day abs, "Not
// set" before its first mark.
func (m *Moon) StateWordOn(abs int) string {
	if st := m.StateOn(abs); st != "" {
		return m.StateWord(st)
	}
	return "Not set"
}

// startsStateOn reports whether a mark setting the moon to state is on the
// day abs, so a rule "on Umbra's full moon" falls on the day Umbra was set
// full rather than every day it stays full.
func (m *Moon) startsStateOn(abs int, state string) bool {
	mk := m.markOn(abs)
	return mk != nil && mk.abs == abs && mk.State == state
}

// moonRuleState is the manual state a moon rule's phase target asks for.
func moonRuleState(target float64) string {
	switch target {
	case 0.5:
		return MoonFull
	case 0:
		return MoonDark
	default:
		return MoonHalf
	}
}

// indexMoonMarks gives every manual moon's marks their day counters and
// sorts them. cal must carry its Months; a mark on a day the calendar no
// longer has (its structure changed) is dropped rather than misplaced.
func (c *Calendar) indexMoonMarks() {
	for i := range c.Moons {
		m := &c.Moons[i]
		kept := m.Marks[:0]
		for _, k := range m.Marks {
			if k.Month < 1 || k.Month > len(c.Months) || k.Day < 1 || k.Day > c.MonthDays(k.Month-1, k.Year) {
				continue
			}
			k.abs = c.absDayIndex(k.Year, k.Month, k.Day)
			kept = append(kept, k)
		}
		sort.SliceStable(kept, func(a, b int) bool { return kept[a].abs < kept[b].abs })
		m.Marks = kept
		m.marksIndexed = true
	}
}

// HandStateOn is a manual moon's state on a date, "" before its first mark.
// m must be one of c's moons, with its marks indexed.
func (c *Calendar) HandStateOn(m *Moon, year, month, day int) string {
	return m.StateOn(c.absDayIndex(year, month, day))
}

// shownMarksOnly keeps the marks players were shown, so a player's moon
// holds the last state they were told of.
func shownMarksOnly(marks []MoonMark) []MoonMark {
	out := make([]MoonMark, 0, len(marks))
	for _, k := range marks {
		if k.Shown {
			out = append(out, k)
		}
	}
	return out
}

// normalizeMoonStateNames trims the Director's words, drops empty ones and
// ones equal to the default, and refuses an unknown state or an overlong
// word. nil means the defaults.
func normalizeMoonStateNames(in map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for k, v := range in {
		if !ValidMoonState(k) {
			return nil, apperror.NewValidation(fmt.Sprintf("%q is not a moon state; use full, half, crescent, dark or vanished", k))
		}
		v = strings.TrimSpace(v)
		if utf8.RuneCountInString(v) > MaxMoonStateWord {
			return nil, apperror.NewValidation(fmt.Sprintf("a state's word is at most %d characters", MaxMoonStateWord))
		}
		if v == "" || v == moonStateWords[k] {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
