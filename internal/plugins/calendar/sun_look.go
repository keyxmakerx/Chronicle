package calendar

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// The kinds of star a calendar's sun can be. The sky draws each in its own
// colour and warms or cools the daylight and the sun's glow to match;
// yellow draws the sun exactly as it was before suns had a kind.
const (
	SunKindRed    = "red"
	SunKindOrange = "orange"
	SunKindYellow = "yellow"
	SunKindWhite  = "white"
	SunKindBlue   = "blue"
)

// SunKinds lists the kinds in order, coolest-burning first.
var SunKinds = []string{SunKindRed, SunKindOrange, SunKindYellow, SunKindWhite, SunKindBlue}

// SunLook is the calendar's sun: its kind of star, and an optional exact
// colour that overrides the kind's own.
type SunLook struct {
	Kind  string  `json:"kind"`
	Color *string `json:"color,omitempty"`
}

// DefaultSunLook is a calendar's sun until its owner changes it, the same
// values migration 030's column defaults hold.
func DefaultSunLook() SunLook {
	return SunLook{Kind: SunKindYellow}
}

// IsSunKind reports whether k is one of SunKinds.
func IsSunKind(k string) bool {
	for _, s := range SunKinds {
		if s == k {
			return true
		}
	}
	return false
}

// normalizeSunLook lower-cases the kind, defaults an empty one to yellow,
// and expands a short colour, refusing anything the sky cannot draw.
func normalizeSunLook(l SunLook) (SunLook, error) {
	l.Kind = strings.ToLower(strings.TrimSpace(l.Kind))
	if l.Kind == "" {
		l.Kind = SunKindYellow
	}
	if !IsSunKind(l.Kind) {
		return l, apperror.NewBadRequest("the sun's kind must be red, orange, yellow, white or blue")
	}
	if l.Color != nil {
		c := strings.TrimSpace(*l.Color)
		if c == "" {
			l.Color = nil
			return l, nil
		}
		if c[0] != '#' {
			c = "#" + c
		}
		if !hexColorPattern.MatchString(c) {
			return l, apperror.NewBadRequest("the sun's color must be a hex color (#rgb or #rrggbb)")
		}
		c = strings.ToLower(normalizeColor(c))
		l.Color = &c
	}
	return l, nil
}

// MoonStrength reads a moon's stored tint as its colour strength, 0 to 1.
// ok is false when none is set (or what is stored is not a number, which
// the sky also ignores), and the sky then draws DefaultMoonStrength.
func MoonStrength(tint *string) (v float64, ok bool) {
	if tint == nil {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(*tint), 64)
	if err != nil || math.IsNaN(f) || f < 0 || f > 1 {
		return 0, false
	}
	return f, true
}

// DefaultMoonStrength is how strongly the sky shows a moon's colour when the
// moon has no strength of its own (sky_moon.js specFor).
const DefaultMoonStrength = 0.4

// MoonStrengthTint is the stored form of a colour strength, refusing one
// outside 0 to 1.
func MoonStrengthTint(v float64) (string, error) {
	if math.IsNaN(v) || v < 0 || v > 1 {
		return "", apperror.NewBadRequest("a moon's strength must be between 0 and 1")
	}
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64), nil
}

// moonStrengthPtr is a moon's stored strength for an export, nil when the
// moon uses the sky's default.
func moonStrengthPtr(tint *string) *float64 {
	if v, ok := MoonStrength(tint); ok {
		return &v
	}
	return nil
}

// importedMoonTint is the stored form of a file's moon strength; one outside
// 0 to 1 is dropped, leaving the sky's default.
func importedMoonTint(v *float64) *string {
	if v == nil {
		return nil
	}
	t, err := MoonStrengthTint(*v)
	if err != nil {
		return nil
	}
	return &t
}

// sunKindLabel is a kind's name as the owner reads it.
func sunKindLabel(k string) string {
	switch k {
	case SunKindRed:
		return "red ember"
	case SunKindBlue:
		return "blue-white"
	}
	return k
}

// SunDescription is the sun in words, for the AI's calendar lookup.
func (l SunLook) SunDescription() string {
	kind := l.Kind
	if kind == "" {
		kind = SunKindYellow
	}
	s := "a " + sunKindLabel(kind) + " sun"
	if l.Color != nil {
		s += fmt.Sprintf(", drawn %s", *l.Color)
	}
	return s
}
