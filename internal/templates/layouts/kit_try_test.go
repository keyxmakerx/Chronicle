package layouts

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// The try-out offers every kit the server can draw, Material first.
func TestKitTryOrderCoversKits(t *testing.T) {
	if czKitOrder[0] != "material" {
		t.Fatalf("Material is not first: %v", czKitOrder)
	}
	got := slices.Clone(czKitOrder[1:])
	var want []string
	for id := range czKits {
		want = append(want, id)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("czKitOrder %v does not match czKits %v", got, want)
	}
}

// Each kit's tokens are keyed on html[data-kit-try], start from a reset of
// everything a saved kit sets, and are the ones AppearanceCSS writes for
// that kit.
func TestKitTryCSS(t *testing.T) {
	if css := KitTryCSS(context.Background()); css != "" {
		t.Errorf("no look saved: want no CSS, got %q", css)
	}
	if css := KitTryCSS(SetAppearance(context.Background(), &AppearanceData{PageLook: "standard"})); css != "" {
		t.Errorf("standard page look: want no CSS, got %q", css)
	}

	tests := []struct {
		name   string
		a      AppearanceData
		accent string
		want   []string
		absent []string
	}{
		{
			name: "kit faces and Material's tall capitals",
			a:    AppearanceData{PageLook: "annals", StyleKit: "sketchbook"},
			want: []string{
				`:root[data-kit-try="ornate"]{` + kitTryResets + `--kit-hf:'Cormorant Garamond'`,
				`:root[data-kit-try="sharp"]{` + kitTryResets + `--kit-hf:'Source Serif 4', Georgia, serif;--kit-lf:'Oswald'`,
				`:root[data-kit-try="material"]{` + kitTryResets + `--annals-hf:'Oswald'`,
				fontFaceCSS("cormorant"), fontFaceCSS("kalam"), fontFaceCSS("oswald"),
			},
			absent: []string{":root.dark[data-kit-try="},
		},
		{
			name:   "owner's faces win and the campaign colour is checked per kit",
			a:      AppearanceData{PageLook: "annals", HeadingFont: "cinzel", BodyFont: "lora"},
			accent: "#9a4a26",
			want: []string{
				`:root[data-kit-try="ornate"]{` + kitTryResets + `--kit-user-acc:`,
				`:root.dark[data-kit-try="ornate"]{--kit-user-acc:`,
				`:root[data-kit-try="material"]{` + kitTryResets + `}`,
			},
			absent: []string{"--kit-hf:'", "--annals-hf:'"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := SetAppearance(context.Background(), &tc.a)
			if tc.accent != "" {
				ctx = SetAccentColor(ctx, tc.accent)
			}
			css := KitTryCSS(ctx)
			for _, id := range czKitOrder {
				if !strings.Contains(css, `:root[data-kit-try="`+id+`"]{`) {
					t.Errorf("no block for %s", id)
				}
			}
			for _, w := range tc.want {
				if !strings.Contains(css, w) {
					t.Errorf("CSS misses %q", w)
				}
			}
			for _, w := range tc.absent {
				if strings.Contains(css, w) {
					t.Errorf("CSS has %q", w)
				}
			}
		})
	}
}

// A docked screen's kits are keyed on the dock and the drawer's tiles, are
// drawn whatever the page look, and keep the owner's faces and colour rules.
func TestDockKitsCSS(t *testing.T) {
	tests := []struct {
		name   string
		a      *AppearanceData
		accent string
		want   []string
		absent []string
	}{
		{
			name: "no look saved still draws every kit",
			want: []string{
				`.dms-dock[data-dms-kit="ornate"],.dms-kt[data-dms-kit="ornate"]{` + kitTryResets + `--kit-hf:'Cormorant Garamond'`,
				`.dms-dock[data-dms-kit="material"],.dms-kt[data-dms-kit="material"]{` + kitTryResets + `}`,
				fontFaceCSS("cormorant"), fontFaceCSS("kalam"),
			},
			absent: []string{":root", "--annals-hf:'", ".dark .dms-dock"},
		},
		{
			name:   "standard page look, owner's faces and colour",
			a:      &AppearanceData{PageLook: "standard", HeadingFont: "cinzel", BodyFont: "lora"},
			accent: "#9a4a26",
			want: []string{
				`.dms-dock[data-dms-kit="sharp"],.dms-kt[data-dms-kit="sharp"]{` + kitTryResets + `--kit-user-acc:`,
				`.dark .dms-dock[data-dms-kit="sharp"],.dark .dms-kt[data-dms-kit="sharp"]{--kit-user-acc:`,
			},
			absent: []string{"--kit-hf:'"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.a != nil {
				ctx = SetAppearance(ctx, tc.a)
			}
			if tc.accent != "" {
				ctx = SetAccentColor(ctx, tc.accent)
			}
			css := DockKitsCSS(ctx)
			for _, id := range czKitOrder {
				if !strings.Contains(css, `.dms-dock[data-dms-kit="`+id+`"]`) {
					t.Errorf("no block for %s", id)
				}
			}
			for _, w := range tc.want {
				if !strings.Contains(css, w) {
					t.Errorf("CSS misses %q", w)
				}
			}
			for _, w := range tc.absent {
				if strings.Contains(css, w) {
					t.Errorf("CSS has %q", w)
				}
			}
		})
	}
}
