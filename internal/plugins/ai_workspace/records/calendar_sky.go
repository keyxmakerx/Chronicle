package records

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// sunSpec is a calendar block's `sun`: a kind, an exact colour, or both.
// colorSet says the block wrote `color`, so nil then clears it.
type sunSpec struct {
	kind     string
	color    *string
	colorSet bool
}

// sunKindWords maps what an AI may write for a kind to the stored kind,
// including the names the settings show.
var sunKindWords = map[string]string{
	"red": calendar.SunKindRed, "red ember": calendar.SunKindRed, "ember": calendar.SunKindRed,
	"orange": calendar.SunKindOrange, "yellow": calendar.SunKindYellow,
	"white": calendar.SunKindWhite, "blue": calendar.SunKindBlue, "blue-white": calendar.SunKindBlue,
	"blue white": calendar.SunKindBlue,
}

func readSunKind(v string) (string, error) {
	if k, ok := sunKindWords[strings.ToLower(strings.TrimSpace(v))]; ok {
		return k, nil
	}
	return "", badRequestf("sun: %q is not a kind of sun; use red, orange, yellow, white or blue", v)
}

// readSun reads `sun: red` or `sun: {kind: red, color: "#ff6a3a"}`.
func readSun(v any) (*sunSpec, error) {
	switch t := v.(type) {
	case string:
		k, err := readSunKind(t)
		if err != nil {
			return nil, err
		}
		return &sunSpec{kind: k}, nil
	case map[string]any:
		f := make(map[string]any, len(t))
		for key, val := range t {
			f[strings.ToLower(strings.TrimSpace(key))] = val
		}
		r := Record{Fields: f}
		s := &sunSpec{}
		if k := r.Str("kind"); k != "" {
			kind, err := readSunKind(k)
			if err != nil {
				return nil, err
			}
			s.kind = kind
		}
		if r.Has("color") {
			s.colorSet = true
			if c := r.Str("color"); c != "" && !strings.EqualFold(c, "none") {
				s.color = &c
			}
		}
		if s.kind == "" && !s.colorSet {
			return nil, badRequestf("sun needs a kind (red, orange, yellow, white or blue) or a color")
		}
		return s, nil
	}
	return nil, badRequestf("sun is a kind, like `sun: red`, or a map of kind and color")
}

// look is the sun the calendar will have: base with the block's changes.
func (s *sunSpec) look(base calendar.SunLook) calendar.SunLook {
	if base.Kind == "" {
		base = calendar.DefaultSunLook()
	}
	if s.kind != "" {
		base.Kind = s.kind
	}
	if s.colorSet {
		base.Color = s.color
	}
	return base
}

// summary is what the review row says the block does to the sun.
func (s *sunSpec) summary(base calendar.SunLook) string {
	return "gives it " + s.look(base).SunDescription()
}

// moonLook is one moon entry's `strength` and `hidden`; nil leaves each as
// it is. strengthSet with a nil strength returns it to the sky's default.
type moonLook struct {
	strength    *float64
	strengthSet bool
	hidden      *bool
}

func (l moonLook) any() bool { return l.strengthSet || l.hidden != nil }

func readMoonLook(it Record) (moonLook, error) {
	var l moonLook
	if it.Has("strength") {
		l.strengthSet = true
		if !strings.EqualFold(it.Str("strength"), "default") {
			v, _, err := it.Float("strength")
			if err != nil {
				return l, err
			}
			if v < 0 || v > 1 {
				return l, badRequestf("moon %q: strength is 0 to 1, like 0.8", it.Name)
			}
			l.strength = &v
		}
	}
	if b, ok := it.Bool("hidden"); ok {
		l.hidden = &b
	}
	return l, nil
}

// apply sets the look on a moon about to be inserted.
func (l moonLook) apply(m *calendar.MoonInput) {
	if l.strength != nil {
		if t, err := calendar.MoonStrengthTint(*l.strength); err == nil {
			m.Tint = &t
		}
	}
	if l.hidden != nil {
		m.HiddenFromPlayers = *l.hidden
	}
}

// moonLookChanges counts the existing moons whose strength or hidden flag
// the block changes, for the review row.
func (s *calSpec) moonLookChanges(cal *calendar.Calendar) (strength, hide, show int) {
	for i, m := range s.moons {
		l := s.moonLooks[i]
		old := baseMoon(cal, m.Name)
		if old == nil || !l.any() {
			continue
		}
		if l.strengthSet {
			strength++
		}
		if l.hidden != nil && *l.hidden != old.HiddenFromPlayers {
			if *l.hidden {
				hide++
			} else {
				show++
			}
		}
	}
	return
}

// moonPhaseChanges counts the existing moons whose phase offset or colour
// the block changes. A structure save keeps both as stored, so they are
// written after it (see setMoonLooks).
func (s *calSpec) moonPhaseChanges(cal *calendar.Calendar) int {
	n := 0
	for _, m := range s.moons {
		if old := baseMoon(cal, m.Name); old != nil && moonPhaseChanged(*old, m) {
			n++
		}
	}
	return n
}

func moonPhaseChanged(old calendar.Moon, m calendar.MoonInput) bool {
	return old.PhaseOffset != m.PhaseOffset ||
		!strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(old.Color), "#"), strings.TrimPrefix(strings.TrimSpace(m.Color), "#"))
}

// setMoonLooks writes the phase offset, colour, strength and hidden flag of
// the moons the calendar already had; a new moon took them when it was
// inserted.
func (k CalendarKind) setMoonLooks(ctx context.Context, campaignID string, s *calSpec, cal *calendar.Calendar) error {
	for i, m := range s.moons {
		l := s.moonLooks[i]
		old := baseMoon(cal, m.Name)
		if old == nil {
			continue
		}
		if moonPhaseChanged(*old, m) {
			if err := k.Svc.SetMoonPhase(ctx, old.ID, cal.ID, campaignID, m.PhaseOffset, m.Color); err != nil {
				return err
			}
		}
		if !l.any() {
			continue
		}
		if l.strengthSet {
			if err := k.Svc.SetMoonStrength(ctx, old.ID, cal.ID, campaignID, l.strength); err != nil {
				return err
			}
		}
		if l.hidden != nil && *l.hidden != old.HiddenFromPlayers {
			if err := k.Svc.SetMoonHidden(ctx, old.ID, cal.ID, campaignID, *l.hidden); err != nil {
				return err
			}
		}
	}
	return nil
}

// moonLookKeys is an exported moon's strength and hidden keys.
func moonLookKeys(m calendar.Moon) string {
	var b strings.Builder
	if v, ok := calendar.MoonStrength(m.Tint); ok {
		b.WriteString(", strength: " + strconv.FormatFloat(v, 'f', -1, 64))
	}
	if m.HiddenFromPlayers {
		b.WriteString(", hidden: true")
	}
	return b.String()
}

// sunLine is the exported calendar's `sun` key.
func sunLine(l calendar.SunLook) string {
	kind := l.Kind
	if kind == "" {
		kind = calendar.SunKindYellow
	}
	if l.Color == nil {
		return "sun: " + kind + "\n"
	}
	return fmt.Sprintf("sun: {kind: %s, color: %s}\n", kind, quote(*l.Color))
}
