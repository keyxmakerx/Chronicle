package layouts

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// legacyChromeCSS reproduces the pre-C-ACCENT-TRIO AccentColorCSS output
// verbatim (the exact fmt.Sprintf shipped before the accentSlotCSS refactor).
// The trio's contract is that chrome-only campaigns render BYTE-IDENTICAL
// CSS to what they rendered before — this oracle pins that.
func legacyChromeCSS(base string) string {
	if base == "" {
		return ""
	}
	r, g, b, ok := parseHex(base)
	if !ok {
		return fmt.Sprintf(":root{--color-accent:%s;}", base)
	}
	hr, hg, hb := clampByte(int(float64(r)*0.88)), clampByte(int(float64(g)*0.88)), clampByte(int(float64(b)*0.88))
	lr, lg, lb := clampByte(int(float64(r)+float64(255-r)*0.6)), clampByte(int(float64(g)+float64(255-g)*0.6)), clampByte(int(float64(b)+float64(255-b)*0.6))
	return fmt.Sprintf(
		":root{--color-accent:%s;--color-accent-hover:#%02x%02x%02x;--color-accent-light:#%02x%02x%02x;"+
			"--color-accent-rgb:%d %d %d;--color-accent-hover-rgb:%d %d %d;--color-accent-light-rgb:%d %d %d;}",
		base, hr, hg, hb, lr, lg, lb,
		r, g, b, hr, hg, hb, lr, lg, lb,
	)
}

// TestAccentColorCSS_ChromeByteIdentical pins the trio's fallback guarantee:
// with only the chrome accent set (or nothing set), output is byte-identical
// to the pre-trio implementation for every input class the picker and legacy
// hand-entered values can produce.
func TestAccentColorCSS_ChromeByteIdentical(t *testing.T) {
	cases := []struct {
		name string
		base string
	}{
		{"unset", ""},
		{"indigo default preset", "#6366f1"},
		{"emerald preset", "#10b981"},
		{"black clamps", "#000000"},
		{"white clamps", "#ffffff"},
		{"invalid passthrough", "rebeccapurple"},
		{"short hex passthrough", "#abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.base != "" {
				ctx = SetAccentColor(ctx, tc.base)
			}
			got := AccentColorCSS(ctx)
			want := legacyChromeCSS(tc.base)
			if got != want {
				t.Errorf("chrome-only output drifted from legacy:\n got: %q\nwant: %q", got, want)
			}
		})
	}
}

// TestAccentColorCSS_SurfacePair covers the slot 2+3 emission: set slots emit
// a full derived block under --color-accent-surface-N, unset slots emit
// nothing at all (inheritance happens at consumers via var() fallback chains,
// costing zero CSS bytes here).
func TestAccentColorCSS_SurfacePair(t *testing.T) {
	t.Run("both surfaces set emit derived blocks after chrome", func(t *testing.T) {
		ctx := SetAccentColor(context.Background(), "#6366f1")
		ctx = SetAccentSurface(ctx, 1, "#10b981")
		ctx = SetAccentSurface(ctx, 2, "#f59e0b")
		got := AccentColorCSS(ctx)

		if !strings.HasPrefix(got, legacyChromeCSS("#6366f1")) {
			t.Fatalf("chrome block must lead and stay byte-identical, got: %q", got)
		}
		for _, want := range []string{
			"--color-accent-surface-1:#10b981;",
			"--color-accent-surface-1-hover:#",
			"--color-accent-surface-1-light:#",
			"--color-accent-surface-1-rgb:16 185 129;",
			"--color-accent-surface-2:#f59e0b;",
			"--color-accent-surface-2-rgb:245 158 11;",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in %q", want, got)
			}
		}
	})

	t.Run("unset surfaces emit nothing", func(t *testing.T) {
		ctx := SetAccentColor(context.Background(), "#6366f1")
		if got := AccentColorCSS(ctx); strings.Contains(got, "surface") {
			t.Errorf("unset surface slots must be absent, got: %q", got)
		}
	})

	t.Run("surface without chrome still emits", func(t *testing.T) {
		ctx := SetAccentSurface(context.Background(), 1, "#10b981")
		got := AccentColorCSS(ctx)
		if !strings.Contains(got, "--color-accent-surface-1:#10b981;") {
			t.Errorf("surface-1 should emit without chrome set, got: %q", got)
		}
		if strings.Contains(got, "--color-accent:") && !strings.Contains(got, "surface") {
			t.Errorf("chrome must not emit when unset, got: %q", got)
		}
	})

	t.Run("surface derivation matches chrome derivation for same base", func(t *testing.T) {
		// One derivation, no forks: the surface hover/light values for a color
		// must equal the chrome hover/light values for that same color.
		chromeOnly := AccentColorCSS(SetAccentColor(context.Background(), "#10b981"))
		surfaceOnly := AccentColorCSS(SetAccentSurface(context.Background(), 1, "#10b981"))
		want := strings.ReplaceAll(chromeOnly, "--color-accent", "--color-accent-surface-1")
		if surfaceOnly != want {
			t.Errorf("surface derivation drifted from chrome derivation:\n got: %q\nwant: %q", surfaceOnly, want)
		}
	})

	t.Run("invalid slot is a no-op", func(t *testing.T) {
		ctx := SetAccentSurface(context.Background(), 3, "#10b981")
		if got := AccentColorCSS(ctx); got != "" {
			t.Errorf("slot 3 does not exist and must not emit, got: %q", got)
		}
		if got := GetAccentSurface(ctx, 3); got != "" {
			t.Errorf("GetAccentSurface(3) must be empty, got: %q", got)
		}
	})
}

// TestAccentColorCSS_SemanticSlots covers the C-ACCENT-SLOTS emission: the
// two NEW semantic slots (Action highlight, App accent) emit a full derived
// block under their own custom property when set, nothing when unset, and
// never disturb the site/legacy-surface emission that precedes them
// (pinned byte-identical by TestAccentColorCSS_ChromeByteIdentical).
func TestAccentColorCSS_SemanticSlots(t *testing.T) {
	t.Run("both new slots set emit derived blocks after site + legacy surface", func(t *testing.T) {
		ctx := SetAccentColor(context.Background(), "#6366f1")
		ctx = SetAccentAction(ctx, "#f59e0b")
		ctx = SetAccentApp(ctx, "#0ea5e9")
		got := AccentColorCSS(ctx)

		if !strings.HasPrefix(got, legacyChromeCSS("#6366f1")) {
			t.Fatalf("site block must lead and stay byte-identical, got: %q", got)
		}
		for _, want := range []string{
			"--color-accent-action:#f59e0b;",
			"--color-accent-action-hover:#",
			"--color-accent-action-light:#",
			"--color-accent-action-rgb:245 158 11;",
			"--color-accent-app:#0ea5e9;",
			"--color-accent-app-rgb:14 165 233;",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in %q", want, got)
			}
		}
	})

	t.Run("unset new slots emit nothing (zero-change guarantee)", func(t *testing.T) {
		ctx := SetAccentColor(context.Background(), "#6366f1")
		ctx = SetAccentSurface(ctx, 1, "#10b981")
		got := AccentColorCSS(ctx)
		if strings.Contains(got, "accent-action") || strings.Contains(got, "accent-app") {
			t.Errorf("unset action/app slots must be absent, got: %q", got)
		}
		// And with genuinely everything unset, output is exactly the legacy
		// chrome-only oracle — pins the zero-change guarantee end to end.
		bare := AccentColorCSS(context.Background())
		if bare != legacyChromeCSS("") {
			t.Errorf("fully-unset output drifted from legacy: got %q want %q", bare, legacyChromeCSS(""))
		}
	})

	t.Run("action/app emit without any other slot set", func(t *testing.T) {
		got := AccentColorCSS(SetAccentAction(context.Background(), "#f59e0b"))
		if !strings.Contains(got, "--color-accent-action:#f59e0b;") {
			t.Errorf("action should emit without site/surface set, got: %q", got)
		}
		if strings.Contains(got, "--color-accent:") {
			t.Errorf("site must not emit when unset, got: %q", got)
		}
	})

	t.Run("new-slot derivation matches site derivation for same base", func(t *testing.T) {
		// One derivation, no forks — same guarantee as the legacy surface pair.
		siteOnly := AccentColorCSS(SetAccentColor(context.Background(), "#10b981"))
		actionOnly := AccentColorCSS(SetAccentAction(context.Background(), "#10b981"))
		wantAction := strings.ReplaceAll(siteOnly, "--color-accent", "--color-accent-action")
		if actionOnly != wantAction {
			t.Errorf("action derivation drifted from site derivation:\n got: %q\nwant: %q", actionOnly, wantAction)
		}
		appOnly := AccentColorCSS(SetAccentApp(context.Background(), "#10b981"))
		wantApp := strings.ReplaceAll(siteOnly, "--color-accent", "--color-accent-app")
		if appOnly != wantApp {
			t.Errorf("app derivation drifted from site derivation:\n got: %q\nwant: %q", appOnly, wantApp)
		}
	})

	t.Run("getters default empty", func(t *testing.T) {
		ctx := context.Background()
		if got := GetAccentAction(ctx); got != "" {
			t.Errorf("GetAccentAction on empty context = %q, want empty", got)
		}
		if got := GetAccentApp(ctx); got != "" {
			t.Errorf("GetAccentApp on empty context = %q, want empty", got)
		}
	})
}
