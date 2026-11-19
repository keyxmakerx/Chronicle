package colour

import "testing"

// Every colour the app offers or defaults to for tags, categories and the
// armory's types, in the light and dark theme.
var shippedChipColours = []string{
	// The tag picker's palette.
	"#6366f1", "#8b5cf6", "#ec4899", "#f43f5e", "#f97316", "#eab308", "#22c55e", "#14b8a6", "#06b6d4", "#3b82f6",
	// Defaults: grey tag, category presets.
	"#6b7280", "#f59e0b", "#10b981", "#ef4444", "#64748b", "#b45309", "#dc2626",
}

func TestChipInkReadsInBothThemes(t *testing.T) {
	for _, c := range shippedChipColours {
		light, dark := ChipInk(c)
		for _, th := range []struct {
			name     string
			ink      string
			surfaces []string
		}{{"light", light, chipLightSurfaces}, {"dark", dark, chipDarkSurfaces}} {
			for _, s := range th.surfaces {
				bg := Over(c, s, chipTint)
				if got := Contrast(th.ink, bg); got < 4.5 {
					t.Errorf("%s %s: %s on %s is %.2f:1, want >= 4.5", c, th.name, th.ink, bg, got)
				}
			}
		}
	}
}

func TestChipInkKeepsHue(t *testing.T) {
	for _, c := range shippedChipColours {
		light, dark := ChipInk(c)
		for _, got := range []string{light, dark} {
			if gh, ch := oklch(got)[2], oklch(c)[2]; oklch(c)[1] > 0.03 && hueGap(gh, ch) > 8 {
				t.Errorf("%s: shaded %s moved hue %.1f -> %.1f", c, got, ch, gh)
			}
		}
	}
}

func TestChipStyle(t *testing.T) {
	for _, tc := range []struct {
		name, in string
		empty    bool
	}{{"valid", "#6B7280", false}, {"short", "#fff", true}, {"empty", "", true}, {"injection", "#6b7280;x:y", true}} {
		got := ChipStyle(tc.in)
		if (got == "") != tc.empty {
			t.Errorf("%s: ChipStyle(%q) = %q", tc.name, tc.in, got)
		}
	}
	if got := ChipStyle("#6B7280"); got[:len("background-color: #6b7280")] != "background-color: #6b7280" {
		t.Errorf("tint not lower-cased: %q", got)
	}
}

func TestBadgeColoursReadable(t *testing.T) {
	for _, c := range shippedChipColours {
		bg, fg := BadgeColours(c)
		if got := Contrast(fg, bg); got < 4.5 {
			t.Errorf("%s: %s on %s is %.2f:1, want >= 4.5", c, fg, bg, got)
		}
	}
	if bg, fg := BadgeColours("nope"); Contrast(fg, bg) < 4.5 {
		t.Errorf("fallback badge %s on %s unreadable", fg, bg)
	}
}
