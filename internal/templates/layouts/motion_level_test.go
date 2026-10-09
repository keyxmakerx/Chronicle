package layouts

import (
	"context"
	"strings"
	"testing"
)

// A person can lower the owner's motion level, never raise it; the plain
// look writes no data-motion at all.
func TestMotionLevel(t *testing.T) {
	cases := []struct {
		name   string
		owner  *AppearanceData
		person *ViewPrefsData
		want   string
	}{
		{"nothing set", nil, nil, "full"},
		{"owner calm", &AppearanceData{ReduceMotion: true}, nil, "calm"},
		{"person calm", nil, &ViewPrefsData{Motion: "calm"}, "calm"},
		{"person off", nil, &ViewPrefsData{Motion: "off"}, "off"},
		{"person off beats owner calm", &AppearanceData{ReduceMotion: true}, &ViewPrefsData{Motion: "off"}, "off"},
		{"person as owner set it keeps owner calm", &AppearanceData{ReduceMotion: true}, &ViewPrefsData{Motion: "owner"}, "calm"},
		{"unknown person value follows owner", nil, &ViewPrefsData{Motion: "wild"}, "full"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.owner != nil {
				ctx = SetAppearance(ctx, tc.owner)
			}
			if tc.person != nil {
				ctx = SetViewPrefs(ctx, tc.person)
			}
			if got := MotionLevel(ctx); got != tc.want {
				t.Fatalf("MotionLevel = %q, want %q", got, tc.want)
			}
			attr, set := AppearanceAttrs(ctx)["data-motion"]
			if tc.want == "full" && set {
				t.Fatalf("data-motion = %v, want absent at full", attr)
			}
			if tc.want != "full" && attr != tc.want {
				t.Fatalf("data-motion = %v, want %q", attr, tc.want)
			}
		})
	}
}

// Calm and Off both raise the "wants less motion" flag scripts read.
func TestViewPrefAttrs_OffWritesCalmFlag(t *testing.T) {
	ctx := SetViewPrefs(context.Background(), &ViewPrefsData{Theme: "device", Motion: "off"})
	if got := ViewPrefAttrs(ctx)["data-view-motion"]; got != "calm" {
		t.Fatalf("data-view-motion = %v, want calm", got)
	}
}

// Motion speed retimes the paper moves along with the chrome durations.
func TestAppearanceCSS_SpeedRetimesPaperMoves(t *testing.T) {
	cases := []struct{ speed, want string }{
		{"snappy", "--dur-slide:240ms;--dur-turn:380ms;"},
		{"leisurely", "--dur-slide:520ms;--dur-turn:760ms;"},
	}
	for _, tc := range cases {
		t.Run(tc.speed, func(t *testing.T) {
			css := AppearanceCSS(SetAppearance(context.Background(), &AppearanceData{MotionSpeed: tc.speed}))
			if !strings.Contains(css, tc.want) {
				t.Fatalf("AppearanceCSS = %q, want it to contain %q", css, tc.want)
			}
		})
	}
}
