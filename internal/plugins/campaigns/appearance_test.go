package campaigns

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/colour"
)

// requireBadRequest fails unless err is a 400 apperror.
func requireBadRequest(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a bad request error, got nil")
	}
	ae, ok := err.(*apperror.AppError)
	if !ok || ae.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 AppError, got %T %v", err, err)
	}
}

func TestApplyAppearance_EmptyInputResetsEverything(t *testing.T) {
	// Previously-customised settings must all fall back to Classic.
	s := CampaignSettings{
		Appearance:     &Appearance{Look: "ember"},
		TopbarStyle:    &TopbarStyle{Mode: "solid", Color: "#123456"},
		TopbarContent:  &TopbarContent{Mode: "quote", Quote: "old"},
		AccentColor:    "#6366f1",
		AccentSurface1: "#10b981",
		AccentSurface2: "#f59e0b",
		AccentAction:   "#ef4444",
		AccentApp:      "#3b82f6",
		FontFamily:     "serif",
	}
	backdrop, err := applyAppearance(&s, AppearanceInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backdrop != nil {
		t.Errorf("backdrop = %q, want nil", *backdrop)
	}
	if s.Appearance != nil || s.TopbarStyle != nil {
		t.Errorf("Appearance/TopbarStyle not cleared: %+v %+v", s.Appearance, s.TopbarStyle)
	}
	if want := (&TopbarContent{Mode: "none", Widgets: []string{}, Links: []TopbarLink{}}); !reflect.DeepEqual(s.TopbarContent, want) {
		t.Errorf("TopbarContent = %+v, want %+v", s.TopbarContent, want)
	}
	for name, v := range map[string]string{
		"AccentColor": s.AccentColor, "AccentSurface1": s.AccentSurface1, "AccentSurface2": s.AccentSurface2,
		"AccentAction": s.AccentAction, "AccentApp": s.AccentApp, "FontFamily": s.FontFamily,
	} {
		if v != "" {
			t.Errorf("%s = %q, want cleared", name, v)
		}
	}
}

func TestApplyAppearance_DefaultsStoredAsEmpty(t *testing.T) {
	var in AppearanceInput
	in.Look = "classic"
	in.Nav.Style, in.Nav.Strength, in.Nav.PageName = "ring", "calm", "row"
	in.Colours.Page, in.Colours.Contrast = "cool", "standard"
	in.Type.Body, in.Type.Heading, in.Type.Scale = "inter", "same", "standard"
	in.Buttons.Style = "lift"
	in.Motion.Elevation, in.Motion.Speed = "standard", "standard"

	var s CampaignSettings
	if _, err := applyAppearance(&s, in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Appearance != nil {
		t.Errorf("Appearance = %+v, want nil", s.Appearance)
	}
}

// fieldCase drives one Appearance choice: set puts v into the input, get
// reads the stored value back.
type fieldCase struct {
	name       string
	set        func(in *AppearanceInput, v string)
	get        func(a *Appearance) string
	nonDefault string
}

func appearanceFieldCases() []fieldCase {
	return []fieldCase{
		{"look", func(in *AppearanceInput, v string) { in.Look = v }, func(a *Appearance) string { return a.Look }, "ember"},
		{"nav style", func(in *AppearanceInput, v string) { in.Nav.Style = v }, func(a *Appearance) string { return a.NavStyle }, "comet"},
		{"nav strength", func(in *AppearanceInput, v string) { in.Nav.Strength = v }, func(a *Appearance) string { return a.NavStrength }, "lively"},
		{"nav page name", func(in *AppearanceInput, v string) { in.Nav.PageName = v }, func(a *Appearance) string { return a.NavPageName }, "hidden"},
		{"page tone", func(in *AppearanceInput, v string) { in.Colours.Page = v }, func(a *Appearance) string { return a.PageTone }, "paper"},
		{"contrast", func(in *AppearanceInput, v string) { in.Colours.Contrast = v }, func(a *Appearance) string { return a.Contrast }, "high"},
		{"body font", func(in *AppearanceInput, v string) { in.Type.Body = v }, func(a *Appearance) string { return a.BodyFont }, "lora"},
		{"heading font", func(in *AppearanceInput, v string) { in.Type.Heading = v }, func(a *Appearance) string { return a.HeadingFont }, "cinzel"},
		{"type scale", func(in *AppearanceInput, v string) { in.Type.Scale = v }, func(a *Appearance) string { return a.TypeScale }, "roomy"},
		{"button style", func(in *AppearanceInput, v string) { in.Buttons.Style = v }, func(a *Appearance) string { return a.ButtonStyle }, "ink"},
		{"elevation", func(in *AppearanceInput, v string) { in.Motion.Elevation = v }, func(a *Appearance) string { return a.Elevation }, "dramatic"},
		{"motion speed", func(in *AppearanceInput, v string) { in.Motion.Speed = v }, func(a *Appearance) string { return a.MotionSpeed }, "leisurely"},
		{"header height", func(in *AppearanceInput, v string) { in.Header.Height = v }, func(a *Appearance) string { return a.HeaderHeight }, "tall"},
		{"menu colour", func(in *AppearanceInput, v string) { in.Sidebar.Colour = v }, func(a *Appearance) string { return a.SidebarColour }, "ink"},
		{"menu corner", func(in *AppearanceInput, v string) { in.Sidebar.Corner = v }, func(a *Appearance) string { return a.SidebarCorner }, "subtitle"},
		{"hover card", func(in *AppearanceInput, v string) { in.Hover.Look = v }, func(a *Appearance) string { return a.HoverCard }, "night"},
	}
}

func TestApplyAppearance_Choices(t *testing.T) {
	for _, fc := range appearanceFieldCases() {
		t.Run(fc.name+" valid", func(t *testing.T) {
			var in AppearanceInput
			fc.set(&in, fc.nonDefault)
			var s CampaignSettings
			if _, err := applyAppearance(&s, in); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s.Appearance == nil || fc.get(s.Appearance) != fc.nonDefault {
				t.Errorf("stored %+v, want %q", s.Appearance, fc.nonDefault)
			}
		})
		t.Run(fc.name+" unknown", func(t *testing.T) {
			var in AppearanceInput
			fc.set(&in, "bogus")
			var s CampaignSettings
			_, err := applyAppearance(&s, in)
			requireBadRequest(t, err)
		})
	}

	t.Run("reduce motion stored", func(t *testing.T) {
		var in AppearanceInput
		in.Motion.ReduceAll = true
		var s CampaignSettings
		if _, err := applyAppearance(&s, in); err != nil {
			t.Fatal(err)
		}
		if s.Appearance == nil || !s.Appearance.ReduceMotion {
			t.Errorf("ReduceMotion not stored: %+v", s.Appearance)
		}
	})
}

func TestApplyAppearance_EveryListedValueAccepted(t *testing.T) {
	lists := map[string][]string{
		"look": AppearanceLooks, "nav style": AppearanceNavStyles, "nav strength": AppearanceNavStrengths,
		"nav page name": AppearanceNavPageNames, "page tone": AppearancePageTones, "contrast": AppearanceContrasts,
		"body font": AppearanceBodyFonts, "heading font": AppearanceHeadingFonts, "type scale": AppearanceTypeScales,
		"button style": AppearanceButtonStyles, "elevation": AppearanceElevations, "motion speed": AppearanceSpeeds,
		"hover card": AppearanceHoverCards,
	}
	for _, fc := range appearanceFieldCases() {
		for _, v := range lists[fc.name] {
			t.Run(fc.name+"/"+v, func(t *testing.T) {
				var in AppearanceInput
				fc.set(&in, v)
				var s CampaignSettings
				if _, err := applyAppearance(&s, in); err != nil {
					t.Fatalf("listed value refused: %v", err)
				}
				got := ""
				if s.Appearance != nil {
					got = fc.get(s.Appearance)
				}
				want := v
				if v == lists[fc.name][0] {
					want = ""
				}
				if got != want {
					t.Errorf("stored %q, want %q", got, want)
				}
			})
		}
	}
}

func TestApplyAppearance_Colours(t *testing.T) {
	neon := "#00ff00"
	wantAccent, _ := colour.Tame(neon, false)
	wantDeep, _ := colour.Tame(neon, true)
	if wantAccent == neon {
		t.Fatalf("test premise: Tame(%s,false) should tone the colour", neon)
	}

	t.Run("neon accent toned", func(t *testing.T) {
		var in AppearanceInput
		in.Colours.Accent, in.Colours.S1, in.Colours.S2 = neon, "#00FF00", neon
		var s CampaignSettings
		if _, err := applyAppearance(&s, in); err != nil {
			t.Fatal(err)
		}
		for name, got := range map[string]string{"accent": s.AccentColor, "s1": s.AccentSurface1, "s2": s.AccentSurface2} {
			if got == neon || got != wantAccent {
				t.Errorf("%s = %q, want toned %q", name, got, wantAccent)
			}
		}
	})
	t.Run("header colour uses deep range", func(t *testing.T) {
		var in AppearanceInput
		in.Header.Bg, in.Header.Color = "solid", neon
		var s CampaignSettings
		if _, err := applyAppearance(&s, in); err != nil {
			t.Fatal(err)
		}
		if s.TopbarStyle == nil || s.TopbarStyle.Color != wantDeep {
			t.Errorf("header colour = %+v, want %q", s.TopbarStyle, wantDeep)
		}
	})
	t.Run("gradient colours use deep range", func(t *testing.T) {
		var in AppearanceInput
		in.Header.Bg, in.Header.From, in.Header.To = "gradient", neon, neon
		var s CampaignSettings
		if _, err := applyAppearance(&s, in); err != nil {
			t.Fatal(err)
		}
		if s.TopbarStyle.GradientFrom != wantDeep || s.TopbarStyle.GradientTo != wantDeep {
			t.Errorf("gradient = %+v, want %q", s.TopbarStyle, wantDeep)
		}
	})
	t.Run("already calm colour kept lowercase", func(t *testing.T) {
		var in AppearanceInput
		in.Colours.Accent = "#6366F1"
		var s CampaignSettings
		if _, err := applyAppearance(&s, in); err != nil {
			t.Fatal(err)
		}
		if s.AccentColor != "#6366f1" {
			t.Errorf("accent = %q", s.AccentColor)
		}
	})

	invalid := []string{"red", "#fff", "#12345g", "6366f1", "#6366f1ff"}
	for _, bad := range invalid {
		t.Run("invalid "+bad, func(t *testing.T) {
			for name, set := range map[string]func(*AppearanceInput){
				"accent": func(in *AppearanceInput) { in.Colours.Accent = bad },
				"s1":     func(in *AppearanceInput) { in.Colours.S1 = bad },
				"s2":     func(in *AppearanceInput) { in.Colours.S2 = bad },
				"solid":  func(in *AppearanceInput) { in.Header.Bg, in.Header.Color = "solid", bad },
				"from":   func(in *AppearanceInput) { in.Header.Bg, in.Header.From, in.Header.To = "gradient", bad, "#112233" },
				"to":     func(in *AppearanceInput) { in.Header.Bg, in.Header.From, in.Header.To = "gradient", "#112233", bad },
			} {
				var in AppearanceInput
				set(&in)
				var s CampaignSettings
				_, err := applyAppearance(&s, in)
				if err == nil {
					t.Errorf("%s: invalid colour %q accepted", name, bad)
					continue
				}
				requireBadRequest(t, err)
			}
		})
	}
}

func TestApplyAppearance_Header(t *testing.T) {
	long := strings.Repeat("a", 252) + ".png" // 256 bytes
	cases := []struct {
		name    string
		header  AppearanceHeaderInput
		wantErr bool
		check   func(t *testing.T, s *TopbarStyle)
	}{
		{"default", AppearanceHeaderInput{}, false, func(t *testing.T, s *TopbarStyle) {
			if s != nil {
				t.Errorf("want nil style, got %+v", s)
			}
		}},
		{"solid ok", AppearanceHeaderInput{Bg: "solid", Color: "#1e293b"}, false, func(t *testing.T, s *TopbarStyle) {
			if s.Mode != "solid" || s.Color != "#1e293b" {
				t.Errorf("got %+v", s)
			}
		}},
		{"solid without colour", AppearanceHeaderInput{Bg: "solid"}, true, nil},
		{"gradient ok", AppearanceHeaderInput{Bg: "gradient", From: "#1e293b", To: "#312e81", Dir: "to-br"}, false, func(t *testing.T, s *TopbarStyle) {
			if s.Mode != "gradient" || s.GradientDir != "to-br" {
				t.Errorf("got %+v", s)
			}
		}},
		{"gradient default dir", AppearanceHeaderInput{Bg: "gradient", From: "#1e293b", To: "#312e81"}, false, nil},
		{"gradient missing end", AppearanceHeaderInput{Bg: "gradient", From: "#1e293b"}, true, nil},
		{"gradient missing start", AppearanceHeaderInput{Bg: "gradient", To: "#1e293b"}, true, nil},
		{"gradient bad dir", AppearanceHeaderInput{Bg: "gradient", From: "#1e293b", To: "#312e81", Dir: "sideways"}, true, nil},
		{"image ok", AppearanceHeaderInput{Bg: "image", Image: "abc.png", Scrim: "strong"}, false, func(t *testing.T, s *TopbarStyle) {
			if s.Mode != "image" || s.ImagePath != "abc.png" || s.Scrim != "strong" {
				t.Errorf("got %+v", s)
			}
		}},
		{"image default scrim stored empty", AppearanceHeaderInput{Bg: "image", Image: "abc.png", Scrim: "medium"}, false, func(t *testing.T, s *TopbarStyle) {
			if s.Scrim != "" {
				t.Errorf("scrim = %q", s.Scrim)
			}
		}},
		{"image without picture", AppearanceHeaderInput{Bg: "image"}, true, nil},
		{"image bad scrim", AppearanceHeaderInput{Bg: "image", Image: "abc.png", Scrim: "opaque"}, true, nil},
		{"image climbs out", AppearanceHeaderInput{Bg: "image", Image: "2026/../../b.png"}, true, nil},
		{"image backslash", AppearanceHeaderInput{Bg: "image", Image: `a\b.png`}, true, nil},
		{"image hidden", AppearanceHeaderInput{Bg: "image", Image: "2026/09/.png"}, true, nil},
		{"image leading dot", AppearanceHeaderInput{Bg: "image", Image: ".hidden.png"}, true, nil},
		{"image 255 ok", AppearanceHeaderInput{Bg: "image", Image: long[1:]}, false, nil},
		{"image 256 refused", AppearanceHeaderInput{Bg: "image", Image: long}, true, nil},
		{"sky ok, nothing else kept", AppearanceHeaderInput{Bg: "sky", Color: "#1e293b", From: "#000000", To: "#ffffff", Image: "abc.png"}, false, func(t *testing.T, s *TopbarStyle) {
			if *s != (TopbarStyle{Mode: "sky"}) {
				t.Errorf("got %+v", s)
			}
		}},
		{"unknown bg", AppearanceHeaderInput{Bg: "plaid"}, true, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var in AppearanceInput
			in.Header = tc.header
			var s CampaignSettings
			_, err := applyAppearance(&s, in)
			if tc.wantErr {
				requireBadRequest(t, err)
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, s.TopbarStyle)
			}
		})
	}
}

func TestApplyAppearance_PictureNames(t *testing.T) {
	bad := []string{"../x.png", `a\b.png`, "..png", "2026/../x.png", ".png", "/x.png"}
	for _, v := range bad {
		for name, set := range map[string]func(*AppearanceInput){
			"logo":     func(in *AppearanceInput) { in.Brand.Logo = v },
			"backdrop": func(in *AppearanceInput) { in.Brand.Backdrop = v },
		} {
			t.Run(name+" "+v, func(t *testing.T) {
				var in AppearanceInput
				set(&in)
				var s CampaignSettings
				_, err := applyAppearance(&s, in)
				requireBadRequest(t, err)
			})
		}
	}
}

func TestApplyAppearance_Widgets(t *testing.T) {
	cases := []struct {
		name     string
		widgets  []string
		wantMode string
		want     []string
		wantErr  bool
	}{
		{"nil", nil, "none", []string{}, false},
		{"links then text", []string{"links", "text"}, "links", []string{"links", "text"}, false},
		{"text then links keeps order", []string{"text", "links"}, "quote", []string{"text", "links"}, false},
		{"text only", []string{"text"}, "quote", []string{"text"}, false},
		{"note first reads as widgets to older code", []string{"note", "links"}, "widgets", []string{"note", "links"}, false},
		{"search alone", []string{"search"}, "widgets", []string{"search"}, false},
		{"four is the cap", []string{"links", "text", "note", "search"}, "links", []string{"links", "text", "note", "search"}, false},
		{"the four live widgets are accepted", []string{"date", "weather", "moon", "session"}, "widgets", []string{"date", "weather", "moon", "session"}, false},
		{"a live widget first reads as widgets to older code", []string{"session", "links"}, "widgets", []string{"session", "links"}, false},
		{"duplicate live widget", []string{"moon", "moon"}, "", nil, true},
		{"sky is not a widget", []string{"sky"}, "", nil, true},
		{"five is over the cap", []string{"links", "text", "note", "search", "links"}, "", nil, true},
		{"five distinct widgets are over the cap", []string{"links", "text", "note", "search", "era"}, "", nil, true},
		{"the era widget is accepted", []string{"era"}, "widgets", []string{"era"}, false},
		{"links only", []string{"links"}, "links", []string{"links"}, false},
		{"duplicate", []string{"links", "links"}, "", nil, true},
		{"unknown", []string{"banner"}, "", nil, true},
		{"empty name", []string{""}, "", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var in AppearanceInput
			in.Header.Widgets = tc.widgets
			var s CampaignSettings
			_, err := applyAppearance(&s, in)
			if tc.wantErr {
				requireBadRequest(t, err)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if s.TopbarContent.Mode != tc.wantMode || !reflect.DeepEqual(s.TopbarContent.Widgets, tc.want) {
				t.Errorf("got mode %q widgets %v, want %q %v", s.TopbarContent.Mode, s.TopbarContent.Widgets, tc.wantMode, tc.want)
			}
		})
	}

	t.Run("text trimmed and limited", func(t *testing.T) {
		var in AppearanceInput
		in.Header.Widgets = []string{"text"}
		in.Header.Text = "  hello  "
		var s CampaignSettings
		if _, err := applyAppearance(&s, in); err != nil {
			t.Fatal(err)
		}
		if s.TopbarContent.Quote != "hello" {
			t.Errorf("quote = %q", s.TopbarContent.Quote)
		}
		in.Header.Text = strings.Repeat("é", 201)
		_, err := applyAppearance(&s, in)
		requireBadRequest(t, err)
		in.Header.Text = strings.Repeat("é", 200)
		if _, err := applyAppearance(&s, in); err != nil {
			t.Errorf("200 runes refused: %v", err)
		}
	})
}

func TestApplyAppearance_Links(t *testing.T) {
	many := make([]TopbarLink, 9)
	for i := range many {
		many[i] = TopbarLink{Label: fmt.Sprintf("L%d", i), URL: "/x"}
	}
	cases := []struct {
		name    string
		links   []TopbarLink
		wantN   int
		wantErr bool
	}{
		{"empty rows dropped", []TopbarLink{{}, {Label: "  ", URL: " "}, {Label: "Home", URL: "/home"}}, 1, false},
		{"values trimmed", []TopbarLink{{Label: " Home ", URL: " https://example.com "}}, 1, false},
		{"label 30 runes ok", []TopbarLink{{Label: strings.Repeat("é", 30), URL: "/x"}}, 1, false},
		{"label 31 runes refused", []TopbarLink{{Label: strings.Repeat("é", 31), URL: "/x"}}, 0, true},
		{"missing URL", []TopbarLink{{Label: "Home"}}, 0, true},
		{"javascript URL", []TopbarLink{{Label: "Bad", URL: "javascript:alert(1)"}}, 0, true},
		{"protocol-relative URL", []TopbarLink{{Label: "Bad", URL: "//evil.com"}}, 0, true},
		{"eight ok", many[:8], 8, false},
		{"nine refused", many, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var in AppearanceInput
			in.Header.Links = tc.links
			var s CampaignSettings
			_, err := applyAppearance(&s, in)
			if tc.wantErr {
				requireBadRequest(t, err)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(s.TopbarContent.Links) != tc.wantN {
				t.Errorf("links = %+v, want %d", s.TopbarContent.Links, tc.wantN)
			}
			for _, l := range s.TopbarContent.Links {
				if l.Label != strings.TrimSpace(l.Label) || l.URL != strings.TrimSpace(l.URL) {
					t.Errorf("link not trimmed: %+v", l)
				}
			}
		})
	}
}

func TestApplyAppearance_Brand(t *testing.T) {
	t.Run("name trimmed", func(t *testing.T) {
		var in AppearanceInput
		in.Brand.Name = "  Therin  "
		var s CampaignSettings
		if _, err := applyAppearance(&s, in); err != nil {
			t.Fatal(err)
		}
		if s.BrandName != "Therin" {
			t.Errorf("name = %q", s.BrandName)
		}
	})
	t.Run("name counted in runes", func(t *testing.T) {
		var in AppearanceInput
		var s CampaignSettings
		in.Brand.Name = strings.Repeat("é", 40) // 80 bytes
		if _, err := applyAppearance(&s, in); err != nil {
			t.Errorf("40 runes refused: %v", err)
		}
		in.Brand.Name = strings.Repeat("é", 41)
		_, err := applyAppearance(&s, in)
		requireBadRequest(t, err)
	})
	t.Run("welcome counted in runes", func(t *testing.T) {
		var in AppearanceInput
		var s CampaignSettings
		in.Brand.Welcome = strings.Repeat("日", 500) // 1500 bytes
		if _, err := applyAppearance(&s, in); err != nil {
			t.Errorf("500 runes refused: %v", err)
		}
		if s.WelcomeMessage != in.Brand.Welcome {
			t.Error("welcome not stored")
		}
		in.Brand.Welcome = strings.Repeat("日", 501)
		_, err := applyAppearance(&s, in)
		requireBadRequest(t, err)
	})
	t.Run("logo and backdrop", func(t *testing.T) {
		var in AppearanceInput
		in.Brand.Logo = "logo.png"
		in.Brand.Backdrop = "abc.png"
		var s CampaignSettings
		backdrop, err := applyAppearance(&s, in)
		if err != nil {
			t.Fatal(err)
		}
		if backdrop == nil || *backdrop != "abc.png" {
			t.Errorf("backdrop = %v", backdrop)
		}
		if s.BrandLogo != "logo.png" {
			t.Errorf("logo = %q", s.BrandLogo)
		}
	})
}

// TestApplyAppearance_FailureLeavesSettingsUntouched pins that a refused
// save writes nothing, since the service marshals s only on success.
func TestApplyAppearance_FailureLeavesSettingsUntouched(t *testing.T) {
	orig := CampaignSettings{BrandName: "Keep", AccentColor: "#6366f1", FontFamily: "serif"}
	s := orig
	var in AppearanceInput
	in.Brand.Name = "Changed"
	in.Nav.Style = "bogus"
	if _, err := applyAppearance(&s, in); err == nil {
		t.Fatal("expected error")
	}
	if !reflect.DeepEqual(s, orig) {
		t.Errorf("settings mutated on failure: %+v", s)
	}
}

// TestCustomizeFontsCoverEveryChoice checks every selectable face has
// self-hosted files. It lives here because layouts cannot import campaigns.
func TestCustomizeFontsCoverEveryChoice(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "static", "fonts", "customize")
	raw, err := os.ReadFile(filepath.Join(dir, "fonts.json"))
	if err != nil {
		t.Fatalf("read fonts.json: %v", err)
	}
	var fonts map[string][]struct {
		File string `json:"file"`
	}
	if err := json.Unmarshal(raw, &fonts); err != nil {
		t.Fatalf("parse fonts.json: %v", err)
	}
	var ids []string
	for _, id := range AppearanceBodyFonts {
		if id != "inter" {
			ids = append(ids, id)
		}
	}
	for _, id := range AppearanceHeadingFonts {
		if id != "same" {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			faces := fonts[id]
			if len(faces) == 0 {
				t.Fatalf("no faces for %q in fonts.json", id)
			}
			for _, f := range faces {
				if _, err := os.Stat(filepath.Join(dir, f.File)); err != nil {
					t.Errorf("face file %q: %v", f.File, err)
				}
			}
		})
	}
}

// TestPictureName pins which picture names a Save accepts: the media
// service's own "YYYY/MM/<id>.<ext>" form must pass, since that is what the
// picture upload returns, and nothing that could leave the media directory.
func TestPictureName(t *testing.T) {
	cases := []struct {
		in    string
		valid bool
	}{
		{"", true},
		{"2026/09/b7c17bb1-6563-462c-8b49-5b2e8bd57108.png", true},
		{"b7c17bb1.png", true},
		{"../etc/passwd", false},
		{"2026/../../x.png", false},
		{"/abs/x.png", false},
		{"2026//x.png", false},
		{".hidden.png", false},
		{`2026\09\x.png`, false},
		{strings.Repeat("a", 256), false},
	}
	for _, c := range cases {
		_, err := pictureName("logo", c.in)
		if (err == nil) != c.valid {
			t.Errorf("pictureName(%q): err=%v, want valid=%v", c.in, err, c.valid)
		}
	}
}

func TestApplyAppearance_Sidebar(t *testing.T) {
	const pic = "2026/09/menu-banner.png"
	cases := []struct {
		name    string
		in      AppearanceSidebarInput
		wantErr bool
		check   func(t *testing.T, a *Appearance)
	}{
		{"defaults store nothing", AppearanceSidebarInput{Colour: "charcoal", Corner: "plain", Glow: "accent",
			Own: "#2b4a3a", Banner: pic, Subtitle: "left over", GlowColour: "#22d3ee"}, false, func(t *testing.T, a *Appearance) {
			if a != nil {
				t.Errorf("a default menu must store nothing, got %+v", a)
			}
		}},
		{"ink", AppearanceSidebarInput{Colour: "ink"}, false, func(t *testing.T, a *Appearance) {
			if a.SidebarColour != "ink" || a.SidebarOwn != "" {
				t.Errorf("got %+v", a)
			}
		}},
		{"tinted drops a stale own colour", AppearanceSidebarInput{Colour: "tinted", Own: "#2b4a3a"}, false, func(t *testing.T, a *Appearance) {
			if a.SidebarColour != "tinted" || a.SidebarOwn != "" {
				t.Errorf("got %+v", a)
			}
		}},
		{"own colour is stored tamed", AppearanceSidebarInput{Colour: "own", Own: "#2B4A3A"}, false, func(t *testing.T, a *Appearance) {
			if a.SidebarColour != "own" || a.SidebarOwn != "#2b4a3a" {
				t.Errorf("got %+v", a)
			}
		}},
		{"own colour needs a colour", AppearanceSidebarInput{Colour: "own"}, true, nil},
		{"own colour must be a hex", AppearanceSidebarInput{Colour: "own", Own: "green"}, true, nil},
		{"unknown colour", AppearanceSidebarInput{Colour: "white"}, true, nil},
		{"light menu is not offered", AppearanceSidebarInput{Colour: "light"}, true, nil},
		{"subtitle trimmed", AppearanceSidebarInput{Corner: "subtitle", Subtitle: "  Session 23  "}, false, func(t *testing.T, a *Appearance) {
			if a.SidebarCorner != "subtitle" || a.SidebarSubtitle != "Session 23" {
				t.Errorf("got %+v", a)
			}
		}},
		{"subtitle of 40 runes", AppearanceSidebarInput{Corner: "subtitle", Subtitle: strings.Repeat("é", 40)}, false, nil},
		{"subtitle of 41 runes", AppearanceSidebarInput{Corner: "subtitle", Subtitle: strings.Repeat("é", 41)}, true, nil},
		{"subtitle with a line break", AppearanceSidebarInput{Corner: "subtitle", Subtitle: "one\ntwo"}, true, nil},
		{"subtitle dropped for a plain corner", AppearanceSidebarInput{Corner: "plain", Subtitle: "stale"}, false, func(t *testing.T, a *Appearance) {
			if a != nil {
				t.Errorf("got %+v", a)
			}
		}},
		{"banner", AppearanceSidebarInput{Corner: "banner", Banner: pic}, false, func(t *testing.T, a *Appearance) {
			if a.SidebarCorner != "banner" || a.SidebarBanner != pic {
				t.Errorf("got %+v", a)
			}
		}},
		{"banner needs a picture", AppearanceSidebarInput{Corner: "banner"}, true, nil},
		{"banner path climbing out", AppearanceSidebarInput{Corner: "banner", Banner: "../secret.png"}, true, nil},
		{"banner dropped for a subtitle corner", AppearanceSidebarInput{Corner: "subtitle", Banner: pic}, false, func(t *testing.T, a *Appearance) {
			if a.SidebarBanner != "" {
				t.Errorf("stale banner kept: %+v", a)
			}
		}},
		{"unknown corner", AppearanceSidebarInput{Corner: "logo-only"}, true, nil},
		{"own glow", AppearanceSidebarInput{Glow: "own", GlowColour: "#3B9FB5"}, false, func(t *testing.T, a *Appearance) {
			if a.PeekGlow != "own" || a.PeekGlowColour != "#3b9fb5" {
				t.Errorf("got %+v", a)
			}
		}},
		{"own glow needs a colour", AppearanceSidebarInput{Glow: "own"}, true, nil},
		{"glow colour must be a hex", AppearanceSidebarInput{Glow: "own", GlowColour: "cyan"}, true, nil},
		{"glow colour dropped while following the accent", AppearanceSidebarInput{Glow: "accent", GlowColour: "#3b9fb5"}, false, func(t *testing.T, a *Appearance) {
			if a != nil {
				t.Errorf("got %+v", a)
			}
		}},
		{"unknown glow", AppearanceSidebarInput{Glow: "rainbow"}, true, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := AppearanceInput{Sidebar: tc.in}
			var s CampaignSettings
			_, err := applyAppearance(&s, in)
			if tc.wantErr {
				requireBadRequest(t, err)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.check != nil {
				tc.check(t, s.Appearance)
			}
		})
	}
}

func TestApplyAppearance_MovingHeader(t *testing.T) {
	cases := []struct {
		name    string
		h       AppearanceHeaderInput
		wantErr bool
	}{
		{"two colours", AppearanceHeaderInput{Bg: "moving", From: "#0f172a", To: "#3b1d5e", Dir: "to-r"}, false},
		{"direction optional", AppearanceHeaderInput{Bg: "moving", From: "#0f172a", To: "#3b1d5e"}, false},
		{"needs both colours", AppearanceHeaderInput{Bg: "moving", From: "#0f172a"}, true},
		{"colours must be hex", AppearanceHeaderInput{Bg: "moving", From: "red", To: "blue"}, true},
		{"bad direction", AppearanceHeaderInput{Bg: "moving", From: "#0f172a", To: "#3b1d5e", Dir: "diagonal"}, true},
		{"animated is the editor's old name, not a stored mode", AppearanceHeaderInput{Bg: "animated", From: "#0f172a", To: "#3b1d5e"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var s CampaignSettings
			_, err := applyAppearance(&s, AppearanceInput{Header: tc.h})
			if tc.wantErr {
				requireBadRequest(t, err)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if s.TopbarStyle == nil || s.TopbarStyle.Mode != "moving" || s.TopbarStyle.GradientFrom != "#0f172a" || s.TopbarStyle.GradientTo != "#3b1d5e" {
				t.Errorf("TopbarStyle = %+v", s.TopbarStyle)
			}
		})
	}
}

func TestAppearancePictures_IncludesMenuBanner(t *testing.T) {
	var in AppearanceInput
	in.Brand.Logo, in.Sidebar.Banner = "a.png", "b.png"
	got := in.AppearancePictures()
	if len(got) != 2 || got[0] != "a.png" || got[1] != "b.png" {
		t.Errorf("AppearancePictures = %v, want the logo and the menu banner", got)
	}
}

// TestApplyAppearance_EditorPayload feeds the exact JSON the editor's Save
// sent (captured from customize_look.js) through the real decoder, so a
// renamed key on either side fails here rather than silently dropping a
// choice.
func TestApplyAppearance_EditorPayload(t *testing.T) {
	const payload = `{"look":"classic","brand":{"name":"","logo":"","welcome":"","backdrop":""},"header":{"bg":"moving","height":"tall","color":"","from":"#0f172a","to":"#1e2a5a","dir":"to-r","image":"","scrim":"medium","widgets":["search","note"],"links":[],"text":""},"colours":{"accent":"#6366f1","s1":"","s2":"","page":"cool","contrast":"standard"},"nav":{"style":"ring","strength":"calm","pageName":"row"},"type":{"body":"inter","heading":"same","scale":"standard"},"buttons":{"style":"lift"},"motion":{"elevation":"standard","speed":"standard","reduceAll":false},"sidebar":{"colour":"tinted","own":"","corner":"subtitle","subtitle":"The Drowned Crown, session 23","banner":"","glow":"own","glowColour":"#3b9fb5"}}`
	var in AppearanceInput
	if err := json.Unmarshal([]byte(payload), &in); err != nil {
		t.Fatal(err)
	}
	var s CampaignSettings
	if _, err := applyAppearance(&s, in); err != nil {
		t.Fatal(err)
	}
	a := s.Appearance
	if a == nil || a.HeaderHeight != "tall" || a.SidebarColour != "tinted" || a.SidebarCorner != "subtitle" ||
		a.SidebarSubtitle != "The Drowned Crown, session 23" || a.PeekGlow != "own" || a.PeekGlowColour != "#3b9fb5" {
		t.Errorf("Appearance = %+v", a)
	}
	if s.TopbarStyle == nil || s.TopbarStyle.Mode != "moving" || s.TopbarStyle.GradientDir != "to-r" {
		t.Errorf("TopbarStyle = %+v", s.TopbarStyle)
	}
	if got := strings.Join(s.TopbarContent.Widgets, ","); got != "search,note" || s.TopbarContent.Mode != "widgets" {
		t.Errorf("TopbarContent = %+v", s.TopbarContent)
	}
}
