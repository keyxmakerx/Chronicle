package trackers

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limits. They bound what a Director can store and keep the block's layout
// sane; the widget enforces the same numbers as a courtesy.
const (
	MaxNameChars    = 80
	MaxTextChars    = 300
	MaxNoteChars    = 200
	MaxSteps        = 20
	MinSteps        = 2
	MaxBands        = 12
	MinSegments     = 2
	MaxSegments     = 24
	MaxKindsPerCamp = 200
	MaxTrackersPage = 40
	MaxEntries      = 10000
	MaxAbsValue     = 1000000
	MaxIDChars      = 64
	LogShown        = 100
	MaxBodyBytes    = 64 << 10
)

// validToken reports whether s is a short safe id: letters, digits, _ and -.
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

// cleanText trims s and checks it is single-line plain text within max
// characters, so stored text is always safe to show with textContent.
func cleanText(field, s string, max int) (string, error) {
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

// cleanName is cleanText for a required name.
func cleanName(field, s string) (string, error) {
	s, err := cleanText(field, s, MaxNameChars)
	if err != nil {
		return "", err
	}
	if s == "" {
		return "", errInvalid(field + " is required")
	}
	return s, nil
}

func validVisibility(v string) bool { return v == SeeDirectors || v == SeeEveryone }

func inRange(n int) bool { return n >= -MaxAbsValue && n <= MaxAbsValue }

// validateKind checks and normalises a kind body. Fields a shape doesn't use
// are cleared, so a stored kind never carries stale steps or bounds.
func validateKind(in KindInput) (Kind, error) {
	var k Kind
	name, err := cleanName("name", in.Name)
	if err != nil {
		return k, err
	}
	k.Name = name
	if in.Visibility == "" {
		in.Visibility = SeeDirectors
	}
	if !validVisibility(in.Visibility) {
		return k, errInvalid("visibility must be directors or everyone")
	}
	k.Visibility = in.Visibility
	if in.EveryTypeID < 0 {
		return k, errInvalid("unknown category")
	}
	k.EveryTypeID = in.EveryTypeID
	k.Shape = in.Shape
	k.Steps = []Step{}
	k.Bands = []Band{}

	switch in.Shape {
	case ShapeSteps:
		if len(in.Steps) < MinSteps || len(in.Steps) > MaxSteps {
			return k, errInvalid(fmt.Sprintf("a steps tracker needs %d to %d steps", MinSteps, MaxSteps))
		}
		for i, s := range in.Steps {
			n, err := cleanName(fmt.Sprintf("step %d's name", i+1), s.Name)
			if err != nil {
				return k, err
			}
			t, err := cleanText(fmt.Sprintf("step %d's text", i+1), s.Text, MaxTextChars)
			if err != nil {
				return k, err
			}
			k.Steps = append(k.Steps, Step{Name: n, Text: t})
		}
		return k, nil
	case ShapeClock:
		if in.Max == nil || *in.Max < MinSegments || *in.Max > MaxSegments {
			return k, errInvalid(fmt.Sprintf("a clock needs %d to %d segments", MinSegments, MaxSegments))
		}
		seg := *in.Max
		k.Max = &seg
	case ShapeCount:
		if in.Min != nil {
			if !inRange(*in.Min) {
				return k, errInvalid("the lowest value is out of range")
			}
			lo := *in.Min
			k.Min = &lo
		}
		if in.Max != nil {
			if !inRange(*in.Max) {
				return k, errInvalid("the highest value is out of range")
			}
			hi := *in.Max
			k.Max = &hi
		}
		if k.Min != nil && k.Max != nil && *k.Min >= *k.Max {
			return k, errInvalid("the highest value must be above the lowest")
		}
	default:
		return k, errInvalid("shape must be count, steps or clock")
	}

	if len(in.Bands) > MaxBands {
		return k, errInvalid(fmt.Sprintf("at most %d bands", MaxBands))
	}
	seen := map[int]bool{}
	for i, b := range in.Bands {
		if !inRange(b.At) {
			return k, errInvalid(fmt.Sprintf("band %d starts out of range", i+1))
		}
		if seen[b.At] {
			return k, errInvalid(fmt.Sprintf("two bands start at %d", b.At))
		}
		seen[b.At] = true
		t, err := cleanText(fmt.Sprintf("band %d's text", i+1), b.Text, MaxTextChars)
		if err != nil {
			return k, err
		}
		if t == "" {
			return k, errInvalid(fmt.Sprintf("band %d needs some text", i+1))
		}
		k.Bands = append(k.Bands, Band{At: b.At, Text: t})
	}
	return k, nil
}

// validateLog checks one change. A step is an add of -1 or +1; larger adds
// and sets come from the DM typing a number or from the AI.
func validateLog(in LogInput) (LogInput, error) {
	if in.Op != OpAdd && in.Op != OpSet {
		return in, errInvalid("op must be add or set")
	}
	if !inRange(in.Amount) {
		return in, errInvalid("amount is out of range")
	}
	if in.Op == OpAdd && in.Amount == 0 {
		return in, errInvalid("adding 0 changes nothing")
	}
	note, err := cleanText("note", in.Note, MaxNoteChars)
	if err != nil {
		return in, err
	}
	in.Note = note
	return in, nil
}
