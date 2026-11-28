package cutscenes

import (
	"math"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func f64(v float64) *float64 { return &v }

func TestNormalize(t *testing.T) {
	base := func() Scene { s := NewScene(); s.Name = "The Eye Opens"; return s }
	tests := []struct {
		name    string
		edit    func(*Scene)
		wantErr string
		check   func(t *testing.T, s Scene)
	}{
		{name: "the starting scene passes unchanged", edit: func(*Scene) {}, check: func(t *testing.T, s Scene) {
			if s.Background != (Background{Kind: BackgroundFallAsleep, Seconds: 7}) || s.Overlays[0].Params.Color != DefaultEyeColor || s.Text[0].Style != StyleWhisper || s.Ending.Kind != EndingTap || s.Ending.After != nil {
				t.Errorf("got %+v", s)
			}
		}},
		{name: "name is required", edit: func(s *Scene) { s.Name = "  \t " }, wantErr: "needs a name"},
		{name: "name too long", edit: func(s *Scene) { s.Name = strings.Repeat("a", MaxNameChars+1) }, wantErr: "at most 120"},
		{name: "name control characters become spaces", edit: func(s *Scene) { s.Name = "A\nB\t C" }, check: func(t *testing.T, s Scene) {
			if s.Name != "A B C" {
				t.Errorf("name %q", s.Name)
			}
		}},
		{name: "invalid utf-8 refused", edit: func(s *Scene) { s.Name = "bad\xff" }, wantErr: "UTF-8"},
		{name: "no moments stay nil", edit: func(s *Scene) { s.Moments = []Moment{} }, check: func(t *testing.T, s Scene) {
			if s.Moments != nil {
				t.Errorf("moments %+v", s.Moments)
			}
		}},
		{name: "a moment is clamped, trimmed and its ways put in order; none is writing it", edit: func(s *Scene) {
			s.Moments = []Moment{{At: 500, Ask: " What\ndo they whisper? ", Ways: []Way{{Kind: "Director"}, {Kind: "write"}, {Kind: "write"}}, To: "one", Note: " the\tbell "}, {At: -2, Ask: "Q", To: "each"}}
		}, check: func(t *testing.T, s Scene) {
			want := []Moment{{At: MaxAt, Ask: "What do they whisper?", Ways: []Way{{Kind: WayWrite}, {Kind: WayDirector}}, To: ToOne, Note: "the bell"}, {At: 0, Ask: "Q", Ways: []Way{{Kind: WayWrite}}}}
			if !reflect.DeepEqual(s.Moments, want) {
				t.Errorf("moments %+v", s.Moments)
			}
		}},
		{name: "a moment needs a question", edit: func(s *Scene) { s.Moments = []Moment{{At: 1, Ask: " "}} }, wantErr: "moment 1 has no question"},
		{name: "a question too long", edit: func(s *Scene) { s.Moments = []Moment{{Ask: strings.Repeat("a", MaxAskChars+1)}} }, wantErr: "the most is 200"},
		{name: "a note too long", edit: func(s *Scene) { s.Moments = []Moment{{Ask: "Q", Note: strings.Repeat("a", MaxNoteChars+1)}} }, wantErr: "note is 501 characters"},
		{name: "too many moments", edit: func(s *Scene) { s.Moments = make([]Moment, MaxMoments+1) }, wantErr: "at most 6 moments"},
		{name: "an unknown way refused by name", edit: func(s *Scene) { s.Moments = []Moment{{Ask: "Q", Ways: []Way{{Kind: "roll"}}}} }, wantErr: `the way to answer "roll" is not one of write, director`},
		{name: "an unknown answerer refused by name", edit: func(s *Scene) { s.Moments = []Moment{{Ask: "Q", To: "ana"}} }, wantErr: `who answers "ana" is not one of each, one`},
		{name: "unknown background refused", edit: func(s *Scene) { s.Background.Kind = "explode" }, wantErr: `the background "explode" is not one of fall-asleep`},
		{name: "empty background defaults to falling asleep", edit: func(s *Scene) { s.Background = Background{} }, check: func(t *testing.T, s Scene) {
			if s.Background.Kind != BackgroundFallAsleep || s.Background.Seconds != DefaultSeconds {
				t.Errorf("bg %+v", s.Background)
			}
		}},
		{name: "a black background's separate arrival is stored as its old kind name", edit: func(s *Scene) {
			s.Background = Background{Kind: BackgroundBlack, In: ArriveFade, Seconds: 3}
		}, check: func(t *testing.T, s Scene) {
			if s.Background != (Background{Kind: BackgroundFadeToBlack, Seconds: 3}) {
				t.Errorf("bg %+v", s.Background)
			}
		}},
		{name: "an arrival wins over an old kind name, and nothing stays nothing", edit: func(s *Scene) {
			s.Background = Background{Kind: BackgroundFallAsleep, In: ArriveCut}
		}, check: func(t *testing.T, s Scene) {
			if s.Background.Kind != BackgroundBlack || s.Background.In != "" {
				t.Errorf("bg %+v", s.Background)
			}
			n, err := Normalize(Scene{Name: "N", Background: Background{Kind: BackgroundNone, In: ArriveFallAsleep}})
			if err != nil || n.Background.Kind != BackgroundNone {
				t.Errorf("none: %+v %v", n.Background, err)
			}
		}},
		{name: "an unknown arrival is refused", edit: func(s *Scene) { s.Background.In = "swirl" }, wantErr: `the background's arrival "swirl" is not one of cut, fade, fall-asleep`},
		{name: "a line's place, arrival and leaving are kept, the defaults stored blank", edit: func(s *Scene) {
			s.Text = []TextLine{
				{Say: "Saltreach", Style: StyleTitle, Layout: " Corner ", In: TextSlide, Out: TextSlide},
				{Say: "Middle", Layout: LayoutMiddle},
				{Say: "Chapter two", Style: StyleCaption, In: TextSettle, Out: TextSink},
			}
		}, check: func(t *testing.T, s Scene) {
			want := []TextLine{
				{Say: "Saltreach", Seconds: 0, Style: StyleTitle, Layout: LayoutCorner, In: TextSlide, Out: TextSlide},
				{Say: "Middle", Style: StylePlain},
				{Say: "Chapter two", Style: StyleCaption, In: TextSettle, Out: TextSink},
			}
			for i := range want {
				if s.Text[i] != want[i] {
					t.Errorf("line %d: %+v, want %+v", i+1, s.Text[i], want[i])
				}
			}
		}},
		{name: "an unknown place is refused", edit: func(s *Scene) { s.Text[0].Layout = "top" }, wantErr: `words 1: the place "top" is not one of middle, corner`},
		{name: "an unknown arrival for words is refused", edit: func(s *Scene) { s.Text[0].In = "spin" }, wantErr: `words 1: the arrival "spin" is not one of fade, rise, settle, slide`},
		{name: "an unknown leaving for words is refused", edit: func(s *Scene) { s.Text[0].Out = "explode" }, wantErr: `words 1: the leaving "explode" is not one of fade, sink, slide`},
		{name: "film bars take no colour", edit: func(s *Scene) {
			s.Overlays = append(s.Overlays, Overlay{Kind: OverlayLetterbox, Seconds: 1, Params: Params{Color: "#ff0000"}})
		}, check: func(t *testing.T, s Scene) {
			if s.Overlays[1] != (Overlay{Kind: OverlayLetterbox, Seconds: 1}) {
				t.Errorf("letterbox %+v", s.Overlays[1])
			}
		}},
		{name: "background params are dropped", edit: func(s *Scene) { s.Background.Params.Color = "#ffffff" }, check: func(t *testing.T, s Scene) {
			if s.Background.Params.Color != "" {
				t.Errorf("bg params kept: %+v", s.Background.Params)
			}
		}},
		{name: "a falling-asleep look is kept, case and spaces aside", edit: func(s *Scene) { s.Background.Params.Look = " Eyelids " }, check: func(t *testing.T, s Scene) {
			if s.Background.Params.Look != "eyelids" {
				t.Errorf("bg look %q", s.Background.Params.Look)
			}
		}},
		{name: "an unknown background look is dropped", edit: func(s *Scene) { s.Background.Params.Look = "swirl" }, check: func(t *testing.T, s Scene) {
			if s.Background.Params.Look != "" {
				t.Errorf("bg look %q", s.Background.Params.Look)
			}
		}},
		{name: "a look on a background without looks is dropped", edit: func(s *Scene) {
			s.Background.Kind = BackgroundFadeToBlack
			s.Background.Params.Look = "ink"
		}, check: func(t *testing.T, s Scene) {
			if s.Background.Params.Look != "" {
				t.Errorf("bg look %q", s.Background.Params.Look)
			}
		}},
		{name: "an eye look is kept", edit: func(s *Scene) { s.Overlays[0].Params.Look = "rift" }, check: func(t *testing.T, s Scene) {
			if s.Overlays[0].Params.Look != "rift" {
				t.Errorf("eye look %q", s.Overlays[0].Params.Look)
			}
		}},
		{name: "an eye look from the background's list is dropped", edit: func(s *Scene) { s.Overlays[0].Params.Look = "drift" }, check: func(t *testing.T, s Scene) {
			if s.Overlays[0].Params.Look != "" {
				t.Errorf("eye look %q", s.Overlays[0].Params.Look)
			}
		}},
		{name: "an eye pupil is kept, case and spaces aside", edit: func(s *Scene) { s.Overlays[0].Params.Pupil = " Slit " }, check: func(t *testing.T, s Scene) {
			if s.Overlays[0].Params.Pupil != PupilSlit {
				t.Errorf("eye pupil %q", s.Overlays[0].Params.Pupil)
			}
		}},
		{name: "an unknown pupil is dropped, so the look's own is drawn", edit: func(s *Scene) { s.Overlays[0].Params.Pupil = "cat" }, check: func(t *testing.T, s Scene) {
			if s.Overlays[0].Params.Pupil != "" {
				t.Errorf("eye pupil %q", s.Overlays[0].Params.Pupil)
			}
		}},
		{name: "a pupil on embers is dropped", edit: func(s *Scene) {
			s.Overlays = append(s.Overlays, Overlay{Kind: OverlayEmbers, Params: Params{Pupil: PupilSlit}})
		}, check: func(t *testing.T, s Scene) {
			if s.Overlays[1].Params.Pupil != "" {
				t.Errorf("embers params %+v", s.Overlays[1].Params)
			}
		}},
		{name: "a look on embers is dropped", edit: func(s *Scene) {
			s.Overlays = append(s.Overlays, Overlay{Kind: OverlayEmbers, Params: Params{Look: "gather"}})
		}, check: func(t *testing.T, s Scene) {
			if s.Overlays[1].Params.Look != "" || s.Overlays[1].Params.Color != DefaultEmbersColor {
				t.Errorf("embers params %+v", s.Overlays[1].Params)
			}
		}},
		{name: "seconds clamp low and high", edit: func(s *Scene) {
			s.Background.Seconds = 0.1
			s.Overlays[0].Seconds = 500
		}, check: func(t *testing.T, s Scene) {
			if s.Background.Seconds != MinSeconds || s.Overlays[0].Seconds != MaxSeconds {
				t.Errorf("got %v %v", s.Background.Seconds, s.Overlays[0].Seconds)
			}
		}},
		{name: "negative start clamps to 0, far start to 120", edit: func(s *Scene) {
			s.Overlays[0].At = -3
			s.Text[0].At = 1e9
		}, check: func(t *testing.T, s Scene) {
			if s.Overlays[0].At != 0 || s.Text[0].At != MaxAt {
				t.Errorf("got %v %v", s.Overlays[0].At, s.Text[0].At)
			}
		}},
		{name: "not-a-number takes the default", edit: func(s *Scene) { s.Overlays[0].At = math.NaN(); s.Background.Seconds = math.Inf(1) }, check: func(t *testing.T, s Scene) {
			if s.Overlays[0].At != 0 || s.Background.Seconds != DefaultSeconds {
				t.Errorf("got %v %v", s.Overlays[0].At, s.Background.Seconds)
			}
		}},
		{name: "rounded to a tenth", edit: func(s *Scene) { s.Overlays[0].At = 7.46 }, check: func(t *testing.T, s Scene) {
			if s.Overlays[0].At != 7.5 {
				t.Errorf("at %v", s.Overlays[0].At)
			}
		}},
		{name: "words seconds 0 stays to the end", edit: func(s *Scene) { s.Text[0].Seconds = 0 }, check: func(t *testing.T, s Scene) {
			if s.Text[0].Seconds != 0 {
				t.Errorf("seconds %v", s.Text[0].Seconds)
			}
		}},
		{name: "words seconds clamp", edit: func(s *Scene) { s.Text[0].Seconds = 0.2 }, check: func(t *testing.T, s Scene) {
			if s.Text[0].Seconds != MinSeconds {
				t.Errorf("seconds %v", s.Text[0].Seconds)
			}
		}},
		{name: "unknown overlay refused", edit: func(s *Scene) { s.Overlays[0].Kind = "dragon" }, wantErr: `layer on top 1: "dragon" is not one of eye, embers`},
		{name: "too many overlays refused", edit: func(s *Scene) {
			for len(s.Overlays) <= MaxOverlays {
				s.Overlays = append(s.Overlays, Overlay{Kind: OverlayEmbers})
			}
		}, wantErr: "at most 8 layers"},
		{name: "colour must be #rrggbb", edit: func(s *Scene) { s.Overlays[0].Params.Color = "red; background:url(x)" }, wantErr: "#rrggbb"},
		{name: "three-digit colour refused", edit: func(s *Scene) { s.Overlays[0].Params.Color = "#fff" }, wantErr: "#rrggbb"},
		{name: "colour lower-cased", edit: func(s *Scene) { s.Overlays[0].Params.Color = "#3DFF7A" }, check: func(t *testing.T, s Scene) {
			if s.Overlays[0].Params.Color != "#3dff7a" {
				t.Errorf("colour %q", s.Overlays[0].Params.Color)
			}
		}},
		{name: "empty colour takes the kind's default", edit: func(s *Scene) { s.Overlays = []Overlay{{Kind: OverlayEmbers}, {Kind: OverlayEye}} }, check: func(t *testing.T, s Scene) {
			if s.Overlays[0].Params.Color != DefaultEmbersColor || s.Overlays[1].Params.Color != DefaultEyeColor {
				t.Errorf("colours %+v", s.Overlays)
			}
		}},
		{name: "empty words refused", edit: func(s *Scene) { s.Text[0].Say = "   " }, wantErr: "words 1 are empty"},
		{name: "words too long", edit: func(s *Scene) { s.Text[0].Say = strings.Repeat("é", MaxSayChars+1) }, wantErr: "the most is 280"},
		{name: "words at the limit pass", edit: func(s *Scene) { s.Text[0].Say = strings.Repeat("é", MaxSayChars) }},
		{name: "html is kept as plain text, never parsed", edit: func(s *Scene) { s.Text[0].Say = "<b>hi</b>" }, check: func(t *testing.T, s Scene) {
			if s.Text[0].Say != "<b>hi</b>" {
				t.Errorf("say %q", s.Text[0].Say)
			}
		}},
		{name: "too many lines refused", edit: func(s *Scene) {
			for len(s.Text) <= MaxTextLines {
				s.Text = append(s.Text, TextLine{Say: "x"})
			}
		}, wantErr: "at most 12 lines"},
		{name: "unknown style refused", edit: func(s *Scene) { s.Text[0].Style = "shout" }, wantErr: `the style "shout"`},
		{name: "empty style is plain", edit: func(s *Scene) { s.Text[0].Style = "" }, check: func(t *testing.T, s Scene) {
			if s.Text[0].Style != StylePlain {
				t.Errorf("style %q", s.Text[0].Style)
			}
		}},
		{name: "unknown ending refused", edit: func(s *Scene) { s.Ending.Kind = "fade" }, wantErr: `the ending "fade"`},
		{name: "tap drops after", edit: func(s *Scene) { s.Ending = Ending{Kind: EndingTap, After: f64(9)} }, check: func(t *testing.T, s Scene) {
			if s.Ending.After != nil {
				t.Errorf("after kept on tap")
			}
		}},
		{name: "auto without after takes the default", edit: func(s *Scene) { s.Ending = Ending{Kind: EndingAuto} }, check: func(t *testing.T, s Scene) {
			if s.Ending.After == nil || *s.Ending.After != DefaultAfter {
				t.Errorf("after %v", s.Ending.After)
			}
		}},
		{name: "auto after clamps", edit: func(s *Scene) { s.Ending = Ending{Kind: EndingAuto, After: f64(999)} }, check: func(t *testing.T, s Scene) {
			if *s.Ending.After != MaxAfter {
				t.Errorf("after %v", *s.Ending.After)
			}
		}},
		{name: "auto after 0 is kept", edit: func(s *Scene) { s.Ending = Ending{Kind: EndingAuto, After: f64(0)} }, check: func(t *testing.T, s Scene) {
			if *s.Ending.After != 0 {
				t.Errorf("after %v", *s.Ending.After)
			}
		}},
		{name: "nil lists become empty lists", edit: func(s *Scene) { s.Overlays, s.Text = nil, nil }, check: func(t *testing.T, s Scene) {
			if s.Overlays == nil || s.Text == nil {
				t.Errorf("nil list survived")
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := base()
			tt.edit(&in)
			got, err := Normalize(in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				if apperror.SafeCode(err) != 400 {
					t.Errorf("code %d, want 400", apperror.SafeCode(err))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
			// Normalizing twice changes nothing.
			again, err := Normalize(got)
			if err != nil || SceneBlock(again) != SceneBlock(got) {
				t.Errorf("not idempotent: %v\n%s\n%s", err, SceneBlock(got), SceneBlock(again))
			}
		})
	}
}

func TestSceneFromForm(t *testing.T) {
	tests := []struct {
		name    string
		form    url.Values
		wantErr string
		check   func(t *testing.T, s Scene)
	}{
		{name: "rows line up by position and spare rows are skipped", form: url.Values{
			"name": {"N"}, "bg_kind": {"black"}, "bg_seconds": {"2,5"},
			"ov_kind": {"eye", "", "embers"}, "ov_at": {"1", "9", "3"}, "ov_seconds": {"2", "9", "4"}, "ov_color": {"#111111", "#999999", "#222222"},
			"tx_say": {"one", "  ", "three"}, "tx_at": {"1", "2", "3"}, "tx_seconds": {"0", "", "4"}, "tx_style": {"title", "plain", "whisper"},
			"end_kind": {"auto"}, "end_after": {"5"},
		}, check: func(t *testing.T, s Scene) {
			if s.Background.Seconds != 2.5 || len(s.Overlays) != 2 || s.Overlays[1].Kind != OverlayEmbers || s.Overlays[1].Params.Color != "#222222" || s.Overlays[1].At != 3 {
				t.Errorf("overlays %+v bg %+v", s.Overlays, s.Background)
			}
			if len(s.Text) != 2 || s.Text[1].Say != "three" || s.Text[1].Style != StyleWhisper || s.Text[1].Seconds != 4 {
				t.Errorf("text %+v", s.Text)
			}
			if s.Ending.Kind != EndingAuto || *s.Ending.After != 5 {
				t.Errorf("ending %+v", s.Ending)
			}
		}},
		{name: "looks are read, the eye's by row", form: url.Values{"name": {"N"}, "bg_kind": {"fall-asleep"}, "bg_look": {"eyelids"},
			"ov_kind": {"embers", "", "eye"}, "ov_look": {"gather", "rift", "rift"}, "ov_pupil": {"", "", "slit"}}, check: func(t *testing.T, s Scene) {
			if s.Background.Params.Look != "eyelids" || len(s.Overlays) != 2 || s.Overlays[1].Params.Look != "rift" || s.Overlays[1].Params.Pupil != "slit" {
				t.Errorf("looks: bg %+v overlays %+v", s.Background, s.Overlays)
			}
		}},
		{name: "the background's arrival and each line's place, arrival and leaving are read by row", form: url.Values{
			"name": {"N"}, "bg_kind": {"black"}, "bg_in": {"fall-asleep"},
			"tx_say": {"one", "", "three"}, "tx_layout": {"corner", "corner", "middle"}, "tx_in": {"slide", "fade", ""}, "tx_out": {"", "sink", "fade"},
		}, check: func(t *testing.T, s Scene) {
			if s.Background.Kind != BackgroundBlack || s.Background.In != ArriveFallAsleep {
				t.Errorf("bg %+v", s.Background)
			}
			if len(s.Text) != 2 || s.Text[0].Layout != LayoutCorner || s.Text[0].In != TextSlide || s.Text[0].Out != "" ||
				s.Text[1].Layout != LayoutMiddle || s.Text[1].In != "" || s.Text[1].Out != TextFade {
				t.Errorf("text %+v", s.Text)
			}
		}},
		{name: "empty numbers read as 0", form: url.Values{"name": {"N"}, "tx_say": {"x"}}, check: func(t *testing.T, s Scene) {
			if s.Text[0].At != 0 || s.Text[0].Seconds != 0 {
				t.Errorf("text %+v", s.Text)
			}
		}},
		{name: "tap ignores after", form: url.Values{"name": {"N"}, "end_kind": {"tap"}, "end_after": {"abc"}}, check: func(t *testing.T, s Scene) {
			if s.Ending.After != nil {
				t.Errorf("after read on tap")
			}
		}},
		{name: "moments line up by row, their ways split, and spare rows are skipped", form: url.Values{
			"name": {"N"}, "mo_ask": {"Why?", " ", "Who?"}, "mo_at": {"4", "9", "7,5"}, "mo_ways": {"write director", "write", "director"},
			"mo_to": {"", "each", "one"}, "mo_note": {"mine", "", ""},
		}, check: func(t *testing.T, s Scene) {
			want := []Moment{{At: 4, Ask: "Why?", Ways: []Way{{Kind: "write"}, {Kind: "director"}}, Note: "mine"}, {At: 7.5, Ask: "Who?", Ways: []Way{{Kind: "director"}}, To: "one"}}
			if !reflect.DeepEqual(s.Moments, want) {
				t.Errorf("moments %+v", s.Moments)
			}
		}},
		{name: "a moment's time is a number", form: url.Values{"name": {"N"}, "mo_ask": {"Q"}, "mo_at": {"soon"}}, wantErr: "moment 1's time"},
		{name: "a word is not a number", form: url.Values{"name": {"N"}, "bg_seconds": {"seven"}}, wantErr: "the background's time must be a number"},
		{name: "NaN is not a number", form: url.Values{"name": {"N"}, "ov_kind": {"eye"}, "ov_at": {"NaN"}}, wantErr: "layer on top 1's start"},
		{name: "Inf is not a number", form: url.Values{"name": {"N"}, "tx_say": {"x"}, "tx_seconds": {"Inf"}}, wantErr: "words 1's time on screen"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := SceneFromForm(tt.form)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, s)
		})
	}
}

func TestColorName(t *testing.T) {
	tests := map[string]string{
		"#3dff7a": "green", "#ff0000": "red", "#ff9a3c": "orange", "#ffee00": "yellow", "#2060ff": "blue",
		"#9933ff": "purple", "#ff66cc": "pink", "#00cccc": "teal", "#ffffff": "white", "#000000": "black",
		"#808080": "grey", "#400000": "deep red", "": "", "#fff": "", "red": "",
	}
	for in, want := range tests {
		if got := ColorName(in); got != want {
			t.Errorf("ColorName(%q) = %q, want %q", in, got, want)
		}
	}
}
