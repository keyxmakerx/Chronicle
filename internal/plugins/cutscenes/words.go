package cutscenes

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// The plain words a scene is said in: the list page's rows, the AI review's
// lines and the AI lookup all use these, so a scene reads the same wherever
// it is described.

// Secs says a number of seconds: "7 s", "7.5 s".
func Secs(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64) + " s"
}

// num is a number as a form field shows it.
func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func kindName(list []Kind, key string) string {
	for _, k := range list {
		if k.Key == key {
			return k.Name
		}
	}
	return key
}

// BackgroundWords says the background: "falling asleep over 7 s".
func BackgroundWords(b Background) string {
	switch b.Kind {
	case BackgroundFallAsleep:
		return "falling asleep over " + Secs(b.Seconds) + LookWords(b.Kind, b.Params.Look)
	case BackgroundFadeToBlack:
		return "fade to black over " + Secs(b.Seconds)
	case BackgroundBlack:
		return "black at once"
	case BackgroundNone:
		return "nothing"
	}
	return b.Kind
}

// OverlayWords says one layer on top: "green eye at 7.5 s".
func OverlayWords(o Overlay) string {
	noun := "eye"
	switch o.Kind {
	case OverlayEye:
	case OverlayEmbers:
		noun = "embers"
	case OverlayLetterbox:
		noun = "film bars"
	default:
		noun = o.Kind
	}
	c := ColorName(o.Params.Color)
	if c != "" {
		noun = c + " " + noun
	}
	return fmt.Sprintf("%s at %s, building over %s", noun, Secs(o.At), Secs(o.Seconds)) + LookWords(o.Kind, o.Params.Look) + PupilWords(o)
}

// PupilWords names an eye's pupil, saying when it is the look's own because
// none was picked: `, pupil "Dragon slit"`. It is empty for anything but an eye.
func PupilWords(o Overlay) string {
	if o.Kind != OverlayEye {
		return ""
	}
	if p, ok := pupilOf(o.Params.Pupil); ok {
		return fmt.Sprintf(", pupil %q", p.Name)
	}
	p, _ := pupilOf(lookPupil(o.Params.Look))
	return fmt.Sprintf(", pupil %q (the look's own)", p.Name)
}

// lookPupil is the pupil an eye look draws when the layer names none.
func lookPupil(look string) string {
	l, _ := lookOf(OverlayEye, LookKey(OverlayEye, look))
	return l.Pupil
}

// LookWords names a layer's look for a kind that has several, saying when it
// is the default because none was picked: `, look "Heavy eyelids"`. It is
// empty for a kind with one way of being drawn.
func LookWords(kind, key string) string {
	list := Looks[kind]
	if len(list) == 0 {
		return ""
	}
	if l, ok := lookOf(kind, key); ok {
		return fmt.Sprintf(", look %q", l.Name)
	}
	return fmt.Sprintf(", look %q (the default)", list[0].Name)
}

// LookKey is the look the editor's picker shows selected: the layer's own,
// or the default when it has none.
func LookKey(kind, key string) string {
	if _, ok := lookOf(kind, key); ok {
		return key
	}
	if list := Looks[kind]; len(list) > 0 {
		return list[0].Key
	}
	return ""
}

// PupilKey is the pupil the editor's picker shows selected: the layer's
// own, or "" (the look's own) when it names none or an unknown one.
func PupilKey(key string) string {
	if _, ok := pupilOf(key); ok {
		return key
	}
	return ""
}

// TextWords says one line of words: "'Something has noticed you.' at 11 s".
func TextWords(t TextLine) string {
	s := fmt.Sprintf("%q at %s", t.Say, Secs(t.At))
	if t.Seconds > 0 {
		s += " for " + Secs(t.Seconds)
	} else {
		s += " to the end"
	}
	if t.Style != "" && t.Style != StylePlain {
		s += ", " + strings.ToLower(kindName(TextStyles, t.Style))
	}
	if t.Layout != "" {
		s += ", " + strings.ToLower(kindName(TextLayouts, t.Layout))
	}
	if t.In != "" {
		s += ", " + strings.ToLower(kindName(TextArrivals, t.In))
	}
	if t.Out != "" {
		s += ", " + strings.ToLower(kindName(TextLeavings, t.Out))
	}
	return s
}

// EndingWords says how the scene closes.
func EndingWords(e Ending) string {
	if e.Kind == EndingAuto {
		after := DefaultAfter
		if e.After != nil {
			after = *e.After
		}
		return "closes by itself " + Secs(after) + " after the last layer"
	}
	return "closes when they tap"
}

// SceneWords is the one-line summary under a scene's name on the list page.
func SceneWords(s Scene) string {
	parts := []string{"Background: " + BackgroundWords(s.Background)}
	if n := len(s.Overlays); n > 0 {
		parts = append(parts, "On top: "+OverlayWords(s.Overlays[0]))
		if n > 1 {
			parts[len(parts)-1] += fmt.Sprintf(" and %d more", n-1)
		}
	}
	switch n := len(s.Text); n {
	case 0:
	case 1:
		parts = append(parts, "1 line of words")
	default:
		parts = append(parts, fmt.Sprintf("%d lines of words", n))
	}
	return strings.Join(parts, " · ")
}

// ColorName names a #rrggbb colour in a plain word ("green", "deep red"), for
// review lines; "" for an empty or malformed colour.
func ColorName(hex string) string {
	if !colorPattern.MatchString(hex) {
		return ""
	}
	v, _ := strconv.ParseUint(hex[1:], 16, 32)
	r, g, b := float64(v>>16&0xff)/255, float64(v>>8&0xff)/255, float64(v&0xff)/255
	mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l := (mx + mn) / 2
	d := mx - mn
	if d < 0.08 {
		switch {
		case l > 0.85:
			return "white"
		case l < 0.15:
			return "black"
		}
		return "grey"
	}
	var h float64
	switch mx {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	var name string
	switch {
	case h < 15 || h >= 345:
		name = "red"
	case h < 40:
		name = "orange"
	case h < 65:
		name = "yellow"
	case h < 165:
		name = "green"
	case h < 195:
		name = "teal"
	case h < 255:
		name = "blue"
	case h < 290:
		name = "purple"
	default:
		name = "pink"
	}
	if l < 0.3 {
		return "deep " + name
	}
	return name
}

// Until says how long is left of a window, roughly: "12 hours", "3 days",
// "40 minutes"; "" once it has passed.
func Until(end, now time.Time) string {
	d := end.Sub(now)
	switch {
	case d <= 0:
		return ""
	case d < time.Hour:
		return plural(int(math.Ceil(d.Minutes())), "minute")
	case d < 48*time.Hour:
		return plural(int(math.Round(d.Hours())), "hour")
	}
	return plural(int(math.Round(d.Hours()/24)), "day")
}

// Ago says how long ago a time was, roughly.
func Ago(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute") + " ago"
	case d < 48*time.Hour:
		return plural(int(d.Hours()), "hour") + " ago"
	}
	return plural(int(d.Hours()/24), "day") + " ago"
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(n) + " " + unit + "s"
}

// names joins people's names: "Ana", "Ana and Bo", "Ana, Bo and Cy".
func names(vs []ShowingViewer) string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Name)
	}
	switch len(out) {
	case 0:
		return "nobody"
	case 1:
		return out[0]
	}
	return strings.Join(out[:len(out)-1], ", ") + " and " + out[len(out)-1]
}

func nowUTC() time.Time { return time.Now().UTC() }
