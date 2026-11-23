package cutscenes

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// colorPattern is the only colour form a scene stores: what the engine and a
// colour input both read, and nothing that could carry CSS.
var colorPattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// Normalize checks a scene and returns it clamped: every number brought into
// its range and rounded to a tenth of a second, colours lower-cased and
// filled with the layer's default, words trimmed of control characters. An
// unknown kind or style, a malformed colour, a missing name or words, too many
// layers and words over the limit are refused with a sentence the editor can
// show as it is. The ID is kept as given; the caller decides it.
func Normalize(in Scene) (Scene, error) {
	out := Scene{ID: in.ID}
	name, err := plainText(in.Name)
	if err != nil {
		return out, err
	}
	if name == "" {
		return out, apperror.NewBadRequest("a scene needs a name")
	}
	if utf8.RuneCountInString(name) > MaxNameChars {
		return out, apperror.NewBadRequest(fmt.Sprintf("a scene's name can be at most %d characters", MaxNameChars))
	}
	out.Name = name

	bg := in.Background
	if bg.Kind == "" {
		bg.Kind = BackgroundFallAsleep
	}
	if !known(BackgroundKinds, bg.Kind) {
		return out, apperror.NewBadRequest(fmt.Sprintf("the background %q is not one of %s", bg.Kind, keys(BackgroundKinds)))
	}
	out.Background = Background{Kind: bg.Kind, Seconds: seconds(bg.Seconds, DefaultSeconds), Params: Params{Look: look(bg.Kind, bg.Params.Look)}}

	if len(in.Overlays) > MaxOverlays {
		return out, apperror.NewBadRequest(fmt.Sprintf("a scene can have at most %d layers on top", MaxOverlays))
	}
	out.Overlays = make([]Overlay, 0, len(in.Overlays))
	for i, o := range in.Overlays {
		if !known(OverlayKinds, o.Kind) {
			return out, apperror.NewBadRequest(fmt.Sprintf("layer on top %d: %q is not one of %s", i+1, o.Kind, keys(OverlayKinds)))
		}
		c, err := color(o.Params.Color, defaultColor(o.Kind))
		if err != nil {
			return out, apperror.NewBadRequest(fmt.Sprintf("layer on top %d: %s", i+1, err.Error()))
		}
		out.Overlays = append(out.Overlays, Overlay{Kind: o.Kind, At: at(o.At), Seconds: seconds(o.Seconds, DefaultSeconds), Params: Params{Color: c, Look: look(o.Kind, o.Params.Look)}})
	}

	if len(in.Text) > MaxTextLines {
		return out, apperror.NewBadRequest(fmt.Sprintf("a scene can have at most %d lines of words", MaxTextLines))
	}
	out.Text = make([]TextLine, 0, len(in.Text))
	for i, t := range in.Text {
		say, err := plainText(t.Say)
		if err != nil {
			return out, apperror.NewBadRequest(fmt.Sprintf("words %d: %s", i+1, err.Error()))
		}
		if say == "" {
			return out, apperror.NewBadRequest(fmt.Sprintf("words %d are empty; write something or remove the line", i+1))
		}
		if n := utf8.RuneCountInString(say); n > MaxSayChars {
			return out, apperror.NewBadRequest(fmt.Sprintf("words %d are %d characters; the most is %d", i+1, n, MaxSayChars))
		}
		style := t.Style
		if style == "" {
			style = StylePlain
		}
		if !known(TextStyles, style) {
			return out, apperror.NewBadRequest(fmt.Sprintf("words %d: the style %q is not one of %s", i+1, style, keys(TextStyles)))
		}
		out.Text = append(out.Text, TextLine{Say: say, At: at(t.At), Seconds: lineSeconds(t.Seconds), Style: style})
	}

	end := in.Ending
	if end.Kind == "" {
		end.Kind = EndingTap
	}
	switch end.Kind {
	case EndingTap:
		out.Ending = Ending{Kind: EndingTap}
	case EndingAuto:
		after := DefaultAfter
		if end.After != nil {
			after = clamp(*end.After, 0, MaxAfter, DefaultAfter)
		}
		out.Ending = Ending{Kind: EndingAuto, After: &after}
	default:
		return out, apperror.NewBadRequest(fmt.Sprintf("the ending %q is not one of %s", end.Kind, keys(EndingKinds)))
	}
	return out, nil
}

// NewScene is the scene a "New scene" page starts from: the first built-in
// animation, the screen falling asleep and a green eye opening, with one
// whispered line under it.
func NewScene() Scene {
	return Scene{
		Background: Background{Kind: BackgroundFallAsleep, Seconds: 7},
		Overlays:   []Overlay{{Kind: OverlayEye, At: 7.5, Seconds: 3.5, Params: Params{Color: DefaultEyeColor}}},
		Text:       []TextLine{{Say: "Something has noticed you.", At: 11, Seconds: 5, Style: StyleWhisper}},
		Ending:     Ending{Kind: EndingTap},
	}
}

func known(list []Kind, key string) bool {
	for _, k := range list {
		if k.Key == key {
			return true
		}
	}
	return false
}

func keys(list []Kind) string {
	out := make([]string, len(list))
	for i, k := range list {
		out[i] = k.Key
	}
	return strings.Join(out, ", ")
}

func defaultColor(kind string) string {
	if kind == OverlayEmbers {
		return DefaultEmbersColor
	}
	return DefaultEyeColor
}

// color reads a #rrggbb colour; empty takes the default.
func color(s, def string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return def, nil
	}
	if !colorPattern.MatchString(s) {
		return "", fmt.Errorf("the colour %q must be written #rrggbb", s)
	}
	return s, nil
}

// clamp brings v into [lo, hi] at a tenth of a second; a value that is not a
// number takes def.
func clamp(v, lo, hi, def float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		v = def
	}
	v = math.Round(v*10) / 10
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// seconds is a build-up time; zero (not written) takes def.
func seconds(v, def float64) float64 {
	if v == 0 {
		v = def
	}
	return clamp(v, MinSeconds, MaxSeconds, def)
}

func at(v float64) float64 { return clamp(v, 0, MaxAt, 0) }

// lineSeconds is how long words stay on screen; 0 keeps them to the end.
func lineSeconds(v float64) float64 {
	if v <= 0 || math.IsNaN(v) {
		return 0
	}
	return clamp(v, MinSeconds, MaxSeconds, DefaultSeconds)
}

// plainText trims s and turns control characters (newlines and tabs too)
// into spaces, so a line is one line wherever it is drawn. Invalid UTF-8 is
// refused rather than repaired, so what is stored is what was meant.
func plainText(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", apperror.NewBadRequest("the text is not valid UTF-8")
	}
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " "), nil
}

// look keeps a look only on a kind that has it and only when the engine
// knows it. An unknown one is dropped rather than refused, so a scene written
// for a newer engine still saves and plays its kind's default look here.
func look(kind, key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	if _, ok := lookOf(kind, key); ok {
		return key
	}
	return ""
}
