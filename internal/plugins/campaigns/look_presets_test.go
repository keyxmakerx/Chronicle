package campaigns

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// flat is a preset in the fixture's shape: the editor's LOOK_KEYS paths, with
// an unset surface colour as nil like the JS table's null.
func (p lookPreset) flat() map[string]any {
	opt := func(v string) any {
		if v == "" {
			return nil
		}
		return v
	}
	return map[string]any{
		"header.bg": p.HeaderBg, "header.solid": p.HeaderSolid, "header.from": p.HeaderFrom, "header.to": p.HeaderTo, "header.dir": p.HeaderDir,
		"nav.style": p.NavStyle, "nav.strength": p.NavStrength,
		"colours.accent": p.Accent, "colours.s1": opt(p.S1), "colours.s2": opt(p.S2),
		"colours.sidebar": p.Sidebar, "colours.page": p.Page, "colours.contrast": p.Contrast,
		"type.body": p.Body, "type.heading": p.Heading, "type.scale": p.Scale,
		"buttons.style": p.Button, "motion.elevation": p.Elevation, "motion.speed": p.Speed,
	}
}

// TestLookPresetsMatchJS pins the Go table to the fixture that
// test/js/customize_look_pins.test.mjs pins customize_look.js to.
func TestLookPresetsMatchJS(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "test", "js", "fixtures", "look_pins.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pins map[string]map[string]any
	if err := json.Unmarshal(b, &pins); err != nil {
		t.Fatal(err)
	}
	if len(pins) != len(lookPresets) || len(lookPresets) != len(AppearanceLooks) {
		t.Fatalf("fixture has %d looks, Go table %d, AppearanceLooks %d", len(pins), len(lookPresets), len(AppearanceLooks))
	}
	for i, p := range lookPresets {
		if p.ID != AppearanceLooks[i] {
			t.Errorf("preset %d is %q, AppearanceLooks has %q", i, p.ID, AppearanceLooks[i])
		}
		want, ok := pins[p.ID]
		if !ok {
			t.Errorf("look %q missing from the fixture", p.ID)
			continue
		}
		got := p.flat()
		for k, w := range want {
			if got[k] != w {
				t.Errorf("%s %s = %v, fixture %v", p.ID, k, got[k], w)
			}
		}
		if len(got) != len(want) {
			t.Errorf("%s has %d keys, fixture %d", p.ID, len(got), len(want))
		}
	}
}

// TestSeedLook checks every look seeds through the real validation, keeps its
// colours untouched by the colour toning, and that Customize then opens on the
// same values the look defines.
func TestSeedLook(t *testing.T) {
	for _, p := range lookPresets {
		t.Run(p.ID, func(t *testing.T) {
			var s CampaignSettings
			if !seedLook(&s, p.ID) {
				t.Fatal("seeding failed validation")
			}
			if s.AccentColor != p.Accent || s.AccentSurface1 != p.S1 || s.AccentSurface2 != p.S2 {
				t.Errorf("colours = %s/%s/%s, want %s/%s/%s", s.AccentColor, s.AccentSurface1, s.AccentSurface2, p.Accent, p.S1, p.S2)
			}
			switch {
			case p.HeaderBg == "solid" && p.HeaderSolid == "page":
				if s.TopbarStyle != nil {
					t.Errorf("default header should store nothing, got %+v", s.TopbarStyle)
				}
			case p.HeaderBg == "solid":
				if s.TopbarStyle == nil || s.TopbarStyle.Mode != "solid" || s.TopbarStyle.Color != p.HeaderSolid {
					t.Errorf("solid header = %+v", s.TopbarStyle)
				}
			default:
				ts := s.TopbarStyle
				if ts == nil || ts.Mode != "gradient" || ts.GradientFrom != p.HeaderFrom || ts.GradientTo != p.HeaderTo || ts.GradientDir != lookDirs[p.HeaderDir] {
					t.Errorf("gradient header = %+v", ts)
				}
			}
			if p.ID == "classic" {
				if s.Appearance != nil || s.AccentColor != "#6366f1" {
					t.Errorf("classic should be the defaults, got %+v", s.Appearance)
				}
				return
			}
			a := s.Appearance
			if a == nil || a.Look != p.ID || a.NavStyle != p.NavStyle && p.NavStyle != "ring" || a.PageTone != p.Page && p.Page != "cool" {
				t.Fatalf("appearance = %+v", a)
			}
			if p.Heading != "same" && a.HeadingFont != p.Heading {
				t.Errorf("heading font = %q, want %q", a.HeadingFont, p.Heading)
			}
			if p.Body != "inter" && a.BodyFont != p.Body {
				t.Errorf("body font = %q, want %q", a.BodyFont, p.Body)
			}
			if p.Button != "lift" && a.ButtonStyle != p.Button {
				t.Errorf("button style = %q, want %q", a.ButtonStyle, p.Button)
			}
		})
	}
	var s CampaignSettings
	if seedLook(&s, "neon") {
		t.Error("an unknown look must not seed")
	}
}
